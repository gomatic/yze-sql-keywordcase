package keywordcase_test

import (
	"errors"
	"testing"

	sql "github.com/gomatic/go-sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keywordcase "github.com/gomatic/yze-sql-keywordcase"
)

func TestDiagnosticsFlagsUppercaseKeywords(t *testing.T) {
	diags, err := keywordcase.Diagnostics("schema.sql", "CREATE TABLE foo (id INT);")

	require.NoError(t, err)
	require.Len(t, diags, 3)
	assert.Equal(t, "yze-sql/keywordcase", diags[0].Rule)
	assert.Equal(t, "schema.sql", diags[0].Path)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 1, diags[0].Col)
	assert.Contains(t, diags[0].Message, `"CREATE"`)
	assert.Contains(t, diags[0].Message, `"create"`)
	assert.Contains(t, diags[1].Message, `"table"`)
	assert.Contains(t, diags[2].Message, `"int"`)
}

func TestDiagnosticsCleanForLowercaseKeywords(t *testing.T) {
	diags, err := keywordcase.Diagnostics("schema.sql", "create table foo (id int);")

	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDiagnosticsFlagsMixedCaseAcrossLines(t *testing.T) {
	// The keyword sits on line 2, exercising the newline branch of the position walk.
	diags, err := keywordcase.Diagnostics("schema.sql", "select a\nFROM t;")

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Equal(t, 2, diags[0].Line)
	assert.Equal(t, 1, diags[0].Col)
	assert.Contains(t, diags[0].Message, `"FROM"`)
}

func TestDiagnosticsColumnIsMidLineForSecondKeyword(t *testing.T) {
	// FROM begins at the 8th column; the running column accumulation must report it.
	diags, err := keywordcase.Diagnostics("schema.sql", "SELECT FROM t;")

	require.NoError(t, err)
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[1].Line)
	assert.Equal(t, 8, diags[1].Col)
	assert.Contains(t, diags[1].Message, `"FROM"`)
}

func TestDiagnosticsColumnIsByteColumnPerYzeContract(t *testing.T) {
	// The column is a 1-based byte count (go/token.Position.Column), matching every
	// other yze analyzer and the stickler consumer. A multi-byte rune ('é', 2 bytes)
	// before FROM advances the column by two, so FROM is at byte column 13, not 12.
	diags, err := keywordcase.Diagnostics("schema.sql", "select 'é' FROM t;")

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"FROM"`)
	assert.Equal(t, 13, diags[0].Col)
}

func TestDiagnosticsFlagsTitleCaseKeyword(t *testing.T) {
	// The realistic human form: not all-caps, still not lowercase.
	diags, err := keywordcase.Diagnostics("schema.sql", "Select 1;")

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"Select"`)
	assert.Contains(t, diags[0].Message, `"select"`)
}

func TestDiagnosticsIgnoresQuotedIdentifier(t *testing.T) {
	// A double-quoted identifier that spells a keyword is an identifier, not a
	// keyword, and must never be flagged.
	diags, err := keywordcase.Diagnostics("schema.sql", `select "SELECT" from t;`)

	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDiagnosticsIgnoresKeywordInsideStringLiteral(t *testing.T) {
	// Keyword text inside a string literal is data, not a keyword.
	diags, err := keywordcase.Diagnostics("schema.sql", "select 'SELECT' from t;")

	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDiagnosticsEmptyAndTrivialSourcesAreClean(t *testing.T) {
	for _, source := range []string{"", "   ", "\n\n", ";"} {
		diags, err := keywordcase.Diagnostics("schema.sql", source)
		require.NoError(t, err, "source %q", source)
		assert.Empty(t, diags, "source %q", source)
	}
}

func TestDiagnosticsReportsScanError(t *testing.T) {
	// An unterminated string literal is a lexical error from the scanner.
	_, err := keywordcase.Diagnostics("schema.sql", "select 'unterminated")

	assert.True(t, errors.Is(err, sql.ErrScan))
}

func TestReportAggregatesFiles(t *testing.T) {
	read := func(path string) ([]byte, error) {
		return map[string][]byte{
			"a.sql": []byte("SELECT 1;"),
			"b.sql": []byte("select 1;"),
		}[path], nil
	}
	report, err := keywordcase.Report(read, []string{"a.sql", "b.sql"})

	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 1, "a.sql has one uppercase keyword; b.sql is clean")
	assert.Equal(t, "a.sql", report.Diagnostics[0].Path)
}

func TestReportSurfacesReadError(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, errors.New("disk boom") }
	_, err := keywordcase.Report(read, []string{"a.sql"})

	assert.True(t, errors.Is(err, keywordcase.ErrReadFile))
}

func TestReportSurfacesScanError(t *testing.T) {
	read := func(string) ([]byte, error) { return []byte("select 'unterminated"), nil }
	_, err := keywordcase.Report(read, []string{"a.sql"})

	assert.True(t, errors.Is(err, sql.ErrScan))
}
