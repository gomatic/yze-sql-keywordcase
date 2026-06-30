// Package keywordcase reports PostgreSQL keywords that are not written in
// lowercase, per the gomatic SQL standard (keywords are lowercase; uppercase and
// title-case are the error). It tokenizes SQL with the shared gomatic/go-sql
// library — a thin wrapper over libpg_query, PostgreSQL's own lexer — and flags
// any keyword token whose text is not already its lowercase form. Quoted
// identifiers and keyword text inside string literals are not keyword tokens, so
// they are never flagged.
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

// Diagnostics reports every keyword token in source whose text is not its canonical
// lowercase spelling. A lexical error scanning source is returned (wrapped in
// sql.ErrScan) so the caller can surface it as a tool failure rather than a clean
// pass. path is stamped on each diagnostic's location. Positions are computed in a
// single forward pass over source, so a file of any size costs O(n).
func Diagnostics(path, source string) ([]goyze.Diagnostic, error) {
	result, err := sql.Scan(sql.SQL(source))
	if err != nil {
		return nil, err
	}
	var diags []goyze.Diagnostic
	// libpg_query emits tokens strictly in ascending Start order, so the cursor
	// only ever moves forward and the whole walk costs one pass over source.
	at := start()
	for _, token := range result.Tokens {
		at = at.advance(source, int(token.Start))
		if diag, ok := keywordDiagnostic(path, source, token, at); ok {
			diags = append(diags, diag)
		}
	}
	return diags, nil
}

// cursor is an immutable 1-based line/column position together with the byte offset
// it has reached in the source. The column is a 1-based byte count, matching
// go/token.Position.Column — the convention go-yze and the stickler consumer use,
// so every yze analyzer reports the same column for the same byte. The cursor only
// moves forward, so walking it across the ordered token stream is a single pass.
type cursor struct {
	offset int
	line   int
	col    int
}

// start is the cursor at the beginning of a source: line 1, column 1, offset 0.
func start() cursor {
	return cursor{offset: 0, line: 1, col: 1}
}

// advance walks the cursor forward to byte offset target, counting a newline as a
// line break and every other byte as one column. The `offset < len(source)` bound
// keeps the byte index in range: if a token offset ever landed past
// end-of-source, indexing source would panic, so the bound stops at EOF instead.
// target is never behind the cursor because tokens arrive in ascending order.
func (c cursor) advance(source string, target int) cursor {
	for c.offset < target && c.offset < len(source) {
		if source[c.offset] == '\n' {
			c.line, c.col = c.line+1, 1
		} else {
			c.col++
		}
		c.offset++
	}
	return c
}

// keywordDiagnostic returns a diagnostic when token is a keyword whose text is not
// lowercase (uppercase or title-case), located at at, and ok=false otherwise.
func keywordDiagnostic(path, source string, token *pg_query.ScanToken, at cursor) (goyze.Diagnostic, bool) {
	if token.KeywordKind == pg_query.KeywordKind_NO_KEYWORD {
		return goyze.Diagnostic{}, false
	}
	word := source[token.Start:token.End]
	lower := strings.ToLower(word)
	if word == lower {
		return goyze.Diagnostic{}, false
	}
	return goyze.Diagnostic{
		Tool:     Tool,
		Rule:     Rule,
		Path:     path,
		Line:     at.line,
		Col:      at.col,
		Severity: goyze.SeverityError,
		Message:  fmt.Sprintf(message, word, lower),
	}, true
}
