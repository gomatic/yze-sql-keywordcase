package keywordcase_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	sql "github.com/gomatic/go-sql"
	goyze "github.com/gomatic/go-yze"

	keywordcase "github.com/sqlrest/yze-sql-keywordcase"
)

// FuzzDiagnostics drives arbitrary text into the libpg_query (cgo) scanner. The
// contract under fuzz: Diagnostics never panics, returns only nil or an error
// wrapping sql.ErrScan, and every diagnostic it does return carries a valid
// 1-based position. The seed corpus covers the edge matrix — case variants,
// quoted identifiers, keywords inside string literals, multi-byte runes, comments,
// and lexical errors.
func FuzzDiagnostics(f *testing.F) {
	for _, seed := range []string{
		"",
		"   ",
		";",
		"select 1;",
		"SELECT 1;",
		"Select 1;",
		`select "SELECT" from t;`,
		"select 'SELECT' from t;",
		"select 'é' FROM t;",
		"create table foo (id INT);",
		"-- comment\nSELECT;",
		"select 'unterminated",
		"DO $$ BEGIN PERFORM 1; END $$;",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		diags, err := keywordcase.Diagnostics("fuzz.sql", source)
		if err != nil {
			if !errors.Is(err, sql.ErrScan) {
				t.Fatalf("error not wrapping sql.ErrScan: %v", err)
			}
			if diags != nil {
				t.Fatalf("diagnostics returned alongside an error: %v", diags)
			}
			return
		}

		// Determinism: the same source must always yield the same diagnostics.
		again, _ := keywordcase.Diagnostics("fuzz.sql", source)
		if !reflect.DeepEqual(diags, again) {
			t.Fatalf("non-deterministic diagnostics for %q", source)
		}

		for _, diag := range diags {
			assertPositionInSource(t, diag, source)
		}
	})
}

// assertPositionInSource fails when a diagnostic's position lands outside source:
// line beyond the line count, or a byte column wider than the source itself. A
// wrong-but-positive column (the byte/rune confusion) overruns this bound.
func assertPositionInSource(t *testing.T, diag goyze.Diagnostic, source string) {
	t.Helper()
	maxLine := strings.Count(source, "\n") + 1
	if diag.Line < 1 || diag.Line > maxLine {
		t.Fatalf("line %d out of range [1,%d] for %q", diag.Line, maxLine, source)
	}
	if diag.Col < 1 || diag.Col > len(source)+1 {
		t.Fatalf("col %d out of range [1,%d] for %q", diag.Col, len(source)+1, source)
	}
}
