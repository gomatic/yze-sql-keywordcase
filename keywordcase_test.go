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
	assert.Equal(t, "yze/keywordcase", diags[0].Rule)
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

func TestDiagnosticsIgnoresKeywordSpelledIdentifiers(t *testing.T) {
	// libpg_query classifies keywords context-free, so unquoted identifiers spelling
	// unreserved/column-name keywords (Version, Level, Name…) scan as keyword tokens.
	// The parse tree proves them identifiers; none may be reported as a keyword.
	for _, source := range []sql.SQL{
		"select Version, Level, t.Name from t;",           // ColumnRef, plain and qualified
		"select 1 from Level;",                            // RangeVar relation
		"select 1 from MySchema.Level;",                   // RangeVar, schema-qualified
		"create table t (Version int);",                   // ColumnDef column name
		"insert into t (Version) values (1);",             // ResTarget INSERT column
		"update t set Level = 1;",                         // ResTarget UPDATE SET column
		"select 1 as Version;",                            // ResTarget SELECT alias (no own location)
		"select 1 from t Level;",                          // Alias, bare FROM alias
		"select Version from (values (1)) as v(Version);", // Alias column renames
		"select t./* legal */Name from t;",                // comment inside a qualified name
		`select "Q".Name from t;`,                         // quoted first field, bare second
		"create table t (v Version);",                     // TypeName, user-defined type
		"select Version();",                               // FuncCall function name
		"select Level.* from Level;",                      // ColumnRef ending in a star
		"table Level;",                                    // TABLE shorthand: ColumnRef with no source location
	} {
		diags, err := keywordcase.Diagnostics("schema.sql", source)
		require.NoError(t, err, "source %q", source)
		assert.Empty(t, diags, "source %q", source)
	}
}

func TestDiagnosticsStillFlagsIdentifierCapableKeywordsUsedAsKeywords(t *testing.T) {
	// UPDATE and SET are unreserved (identifier-capable) keywords; in keyword
	// position they must still be flagged, while the Level column is not.
	diags, err := keywordcase.Diagnostics("schema.sql", "Update t Set Level = 1;")

	require.NoError(t, err)
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, `"Update"`)
	assert.Contains(t, diags[1].Message, `"Set"`)
}

func TestDiagnosticsFlagsColNameKeywordInKeywordPosition(t *testing.T) {
	// VALUES is a column-name-class keyword; as the VALUES clause it is a keyword.
	diags, err := keywordcase.Diagnostics("schema.sql", "insert INTO t (x) VALUES (1);")

	require.NoError(t, err)
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, `"INTO"`)
	assert.Contains(t, diags[1].Message, `"VALUES"`)
}

func TestDiagnosticsUnparseableSourceFlagsOnlyCertainKeywords(t *testing.T) {
	// PL/pgSQL-ish text scans but does not parse, so identifier-capable keywords
	// (BEGIN is unreserved) cannot be told apart from identifiers and are skipped;
	// END is reserved — never an identifier — and stays flagged.
	diags, err := keywordcase.Diagnostics("schema.sql", "BEGIN PERFORM 1; END;")

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"END"`)
}

func TestDiagnosticsScansDollarQuotedBody(t *testing.T) {
	// The body is SQL: its keywords are held to the standard, at outer positions,
	// while its Level column stays an identifier.
	diags, err := keywordcase.Diagnostics("schema.sql", "do $$ UPDATE t SET Level = 1; $$;")

	require.NoError(t, err)
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, `"UPDATE"`)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 7, diags[0].Col, "body starts after the $$ delimiter at column 6")
	assert.Contains(t, diags[1].Message, `"SET"`)
}

func TestDiagnosticsPositionsInsideMultiLineDollarBody(t *testing.T) {
	// The tagged body spans lines; the finding must carry the absolute line/col of
	// the uppercase keyword inside it.
	source := sql.SQL("do $fn$\nbegin\n  SELECT 1;\nend\n$fn$;")
	diags, err := keywordcase.Diagnostics("fn.sql", source)

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"SELECT"`)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 3, diags[0].Col)
}

func TestDiagnosticsSkipsNonSQLDollarBody(t *testing.T) {
	// The body fails to scan (unterminated quote from the apostrophe), so it is not
	// SQL and is skipped silently — its uppercase SELECT is data, not a keyword.
	diags, err := keywordcase.Diagnostics("schema.sql", "select $$don't SELECT$$;")

	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDiagnosticsBoundsDollarQuoteRecursionDepth(t *testing.T) {
	// Four levels of nested dollar quotes are scanned; the fifth is not. The only
	// finding is the uppercase SELECT at depth four — FROM at depth five stays dark.
	source := sql.SQL("select $a$select $b$select $c$select $d$SELECT $e$FROM$e$;$d$;$c$;$b$;$a$;")
	diags, err := keywordcase.Diagnostics("deep.sql", source)

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"SELECT"`)
	assert.Equal(t, 41, diags[0].Col)
}

func TestDiagnosticsEmptyAndTrivialSourcesAreClean(t *testing.T) {
	for _, source := range []sql.SQL{"", "   ", "\n\n", ";"} {
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
