package keywordcase_test

import (
	"errors"
	"testing"

	sql "github.com/gomatic/go-sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	keywordcase "github.com/sqlrest/yze-sql-keywordcase"
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
