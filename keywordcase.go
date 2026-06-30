// Package keywordcase reports PostgreSQL keywords that are not written in
// lowercase, per the gomatic SQL standard (keywords are lowercase; uppercase is the
// error). It tokenizes SQL with the shared gomatic/go-sql library — a thin wrapper
// over libpg_query, PostgreSQL's own lexer — and flags any keyword token whose text
// is not already its lowercase form.
package keywordcase

import (
	"fmt"
	"strings"

	sql "github.com/gomatic/go-sql"
	goyze "github.com/gomatic/go-yze"
	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// Tool is the runner name stamped on every diagnostic, matching the yze-sql family.
const Tool = "yze-sql"

// Rule is the stable rule id every diagnostic carries.
const Rule = "yze-sql/keywordcase"

// message formats an uppercase-keyword finding (actual then canonical lowercase).
const message = "SQL keyword %q should be lowercase %q"

// Diagnostics reports every keyword token in source whose text is not its canonical
// lowercase spelling. A lexical error scanning source is returned (wrapped in
// sql.ErrScan) so the caller can surface it as a tool failure rather than a clean
// pass. path is stamped on each diagnostic's location.
func Diagnostics(path, source string) ([]goyze.Diagnostic, error) {
	result, err := sql.Scan(sql.SQL(source))
	if err != nil {
		return nil, err
	}
	var diags []goyze.Diagnostic
	for _, token := range result.Tokens {
		if diag, ok := keywordDiagnostic(path, source, token); ok {
			diags = append(diags, diag)
		}
	}
	return diags, nil
}

// keywordDiagnostic returns a diagnostic when token is a keyword whose text is not
// lowercase, and ok=false otherwise.
func keywordDiagnostic(path, source string, token *pg_query.ScanToken) (goyze.Diagnostic, bool) {
	if token.KeywordKind == pg_query.KeywordKind_NO_KEYWORD {
		return goyze.Diagnostic{}, false
	}
	word := source[token.Start:token.End]
	lower := strings.ToLower(word)
	if word == lower {
		return goyze.Diagnostic{}, false
	}
	line, col := position(source, int(token.Start))
	return goyze.Diagnostic{
		Tool:     Tool,
		Rule:     Rule,
		Path:     path,
		Line:     line,
		Col:      col,
		Severity: goyze.SeverityError,
		Message:  fmt.Sprintf(message, word, lower),
	}, true
}

// position converts a byte offset into source to a 1-based line and column.
func position(source string, offset int) (int, int) {
	line, col := 1, 1
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			line, col = line+1, 1
			continue
		}
		col++
	}
	return line, col
}
