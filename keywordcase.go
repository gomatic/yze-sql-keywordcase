// Package keywordcase reports PostgreSQL keywords that are not written in
// lowercase, per the gomatic SQL standard (keywords are lowercase; uppercase and
// title-case are the error). It tokenizes SQL with the shared gomatic/go-sql
// library — a thin wrapper over libpg_query, PostgreSQL's own lexer — and flags
// any keyword token whose text is not already its lowercase form. Quoted
// identifiers and keyword text inside string literals are not keyword tokens, so
// they are never flagged.
//
// The scanner classifies keywords by table lookup, context-free, so an unquoted
// identifier that spells an unreserved or column-name keyword (a column named
// Version, say) scans as a keyword token. The parse tree disambiguates: tokens it
// proves to be identifiers are never flagged, and when a source does not parse as
// SQL at all only the reserved and type/function-name keyword classes — which can
// never be bare identifiers — are flagged.
//
// Dollar-quoted string bodies ($$…$$), the fleet-standard form for function and DO
// bodies, are scanned recursively so the SQL inside them is held to the same
// standard; a body that is not lexable SQL (a PL/Python body, say) is skipped
// silently, and recursion stops after [nestingLimit] levels of dollar quoting.
package keywordcase

import (
	"fmt"
	"strings"

	sql "github.com/gomatic/go-sql"
	goyze "github.com/gomatic/go-yze"
	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// Name is the analyzer's stable identifier — the suffix of its flat rule id and
// the key the yze suite catalogs it under.
const Name = "keywordcase"

// Tool is the suite name stamped on every diagnostic. The analyzer is bundled into
// the yze suite (the language group lives in the repo path and [Category], not the
// rule id), so it reports as "yze", not a separate "yze-sql" tool.
const Tool = "yze"

// Rule is the stable, flat rule id every diagnostic carries: "yze/" + [Name].
const Rule = Tool + "/" + Name

// Category is the language group this analyzer belongs to, used by the yze suite to
// run it only when processing SQL.
const Category = "sql"

// message formats a non-lowercase-keyword finding (actual then canonical lowercase).
const message = "SQL keyword %q should be lowercase %q"

// nesting counts remaining levels of dollar-quoted bodies to recurse into.
type nesting int

// nestingLimit bounds the dollar-quote recursion. Nesting dollar quotes requires a
// distinct tag per level, so real SQL rarely exceeds two; four is comfortably past
// any legitimate depth while still bounding adversarial input.
const nestingLimit nesting = 4

// Path is the file path stamped on each diagnostic's location.
type Path string

// Diagnostics reports every keyword token in source whose text is not its canonical
// lowercase spelling. A lexical error scanning source is returned (wrapped in
// sql.ErrScan) so the caller can surface it as a tool failure rather than a clean
// pass. path is stamped on each diagnostic's location. Positions are computed in a
// single forward pass over source, so a file of any size costs O(n).
func Diagnostics(path Path, source sql.SQL) ([]goyze.Diagnostic, error) {
	return diagnose(path, source, start(), nestingLimit)
}

// diagnose scans source and reports its non-lowercase keyword tokens, positioning
// them from base — the cursor for source's first byte, which is mid-file when source
// is a dollar-quoted body. depth limits how many more dollar-quote levels to enter.
func diagnose(path Path, source sql.SQL, base cursor, depth nesting) ([]goyze.Diagnostic, error) {
	result, err := sql.Scan(source)
	if err != nil {
		return nil, err
	}
	ids := identify(source, result.Tokens)
	var diags []goyze.Diagnostic
	// libpg_query emits tokens strictly in ascending Start order, so the cursor
	// only ever moves forward and the whole walk costs one pass over source.
	at := base
	for _, token := range result.Tokens {
		at = at.advance(source, int(token.Start))
		diags = append(diags, tokenDiagnostics(path, source, token, at, ids, depth)...)
	}
	return diags, nil
}

// tokenDiagnostics reports token's findings: a dollar-quoted string contributes the
// findings of its recursively scanned body, any other token at most its own
// non-lowercase-keyword diagnostic.
func tokenDiagnostics(
	path Path, source sql.SQL, token *pg_query.ScanToken, at cursor, ids identifiers, depth nesting,
) []goyze.Diagnostic {
	if quoted, ok := dollarBody(source, token); ok {
		return bodyDiagnostics(path, quoted, at, depth)
	}
	if diag, ok := keywordDiagnostic(path, source, token, at, ids); ok {
		return []goyze.Diagnostic{diag}
	}
	return nil
}

// dollar is the inside of a dollar-quoted string token: the body between the $tag$
// delimiters and the byte width of the opening delimiter before it.
type dollar struct {
	body  sql.SQL
	width int
}

// dollarBody extracts the body of a dollar-quoted string token. Only string-constant
// tokens opening with '$' are dollar-quoted; the scanner guarantees they carry a
// matching $tag$ delimiter at each end, so the delimiter slicing cannot miss.
func dollarBody(source sql.SQL, token *pg_query.ScanToken) (dollar, bool) {
	if token.Token != pg_query.Token_SCONST {
		return dollar{}, false
	}
	text := string(source[token.Start:token.End])
	if !strings.HasPrefix(text, "$") {
		return dollar{}, false
	}
	width := strings.Index(text[1:], "$") + 2
	return dollar{body: sql.SQL(text[width : len(text)-width]), width: width}, true
}

// bodyDiagnostics recursively scans a dollar-quoted body as SQL, offsetting positions
// by the body's place in the outer source: at is the cursor at the opening delimiter,
// and the delimiter never contains a newline, so the body starts on at's line, width
// bytes to the right. A body that fails to scan is not SQL (a PL/Python body, say)
// and is skipped silently; depth exhaustion likewise ends the recursion quietly.
func bodyDiagnostics(path Path, quoted dollar, at cursor, depth nesting) []goyze.Diagnostic {
	if depth == 0 {
		return nil
	}
	inner, err := diagnose(path, quoted.body, cursor{offset: 0, line: at.line, col: at.col + quoted.width}, depth-1)
	if err != nil {
		return nil
	}
	return inner
}

// keywordDiagnostic returns a diagnostic when token is a keyword whose text is not
// lowercase (uppercase or title-case), located at at, and ok=false otherwise. A
// token whose keyword class can double as a bare identifier is only flagged when
// ids cannot vouch for it being an identifier.
func keywordDiagnostic(
	path Path, source sql.SQL, token *pg_query.ScanToken, at cursor, ids identifiers,
) (goyze.Diagnostic, bool) {
	if token.KeywordKind == pg_query.KeywordKind_NO_KEYWORD {
		return goyze.Diagnostic{}, false
	}
	word := string(source[token.Start:token.End])
	lower := strings.ToLower(word)
	if word == lower {
		return goyze.Diagnostic{}, false
	}
	if identifierCapable(token.KeywordKind) && ids.covers(int(token.Start), lower) {
		return goyze.Diagnostic{}, false
	}
	return goyze.Diagnostic{
		Tool:     Tool,
		Rule:     Rule,
		Path:     string(path),
		Line:     at.line,
		Col:      at.col,
		Severity: goyze.SeverityError,
		Message:  fmt.Sprintf(message, word, lower),
	}, true
}
