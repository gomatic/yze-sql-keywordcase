package keywordcase

import (
	"testing"

	sql "github.com/gomatic/go-sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticsColumnIsMidLineForSecondKeyword(t *testing.T) {
	// FROM begins at the 8th column; the running column accumulation must report it.
	diags, err := Diagnostics("schema.sql", "SELECT FROM t;")

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
	diags, err := Diagnostics("schema.sql", "select 'é' FROM t;")

	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"FROM"`)
	assert.Equal(t, 13, diags[0].Col)
}

// TestAdvanceCountsBytesNotRunesAndStopsAtEOF names advance's two claims. The
// column is a 1-based BYTE count on purpose — go/token.Position.Column, the
// convention every yze analyzer and the stickler consumer share — so a
// multi-byte character must move the column by its byte length, not by one.
// Reporting a rune column here would put this analyzer's findings at different
// positions from every other rule's for the same byte.
//
// The EOF bound is the safety half: a token offset past end-of-source would
// index out of range and panic on whatever SQL produced it.
func TestAdvanceCountsBytesNotRunesAndStopsAtEOF(t *testing.T) {
	t.Parallel()

	// "-- é\nSELECT 1" — the accented rune is two bytes, so SELECT begins on
	// line 2 and the column arithmetic must have counted both of them.
	diags, err := Diagnostics("q.sql", sql.SQL("-- é\nSELECT 1"))
	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Equal(t, 2, diags[0].Line, "the newline moved the cursor to line 2")
	assert.Equal(t, 1, diags[0].Col, "and reset the column")

	// A multi-byte character before the keyword ON THE SAME LINE must push the
	// column by its byte width.
	diags, err = Diagnostics("q.sql", sql.SQL("SELECT 'é' FROM t WHERE X"))
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	for _, d := range diags {
		assert.Positive(t, d.Col)
	}

	// Unterminated input must not walk the cursor past the end of the source.
	assert.NotPanics(t, func() {
		_, _ = Diagnostics("q.sql", sql.SQL("SELECT 'unterminated"))
	}, "a token offset at or past EOF must stop the walk, not index out of range")
}

// TestAdvanceStopsAtEndOfSourceRatherThanIndexingPastIt covers the bound the
// doc comment justifies. A token offset past end-of-source cannot be produced
// by the scanner today, so this is the only way to reach the guard — and it is
// worth reaching, because the alternative to stopping is `source[c.offset]` on
// an out-of-range index, which panics inside a linter on whatever SQL produced
// it. A defensive bound that nothing exercises is a bound nobody can trust.
func TestAdvanceStopsAtEndOfSourceRatherThanIndexingPastIt(t *testing.T) {
	t.Parallel()
	source := sql.SQL("ab\ncd")

	assert.NotPanics(t, func() {
		got := start().advance(source, len(source)+1000)
		assert.Equal(t, len(source), got.offset, "the walk stops at end-of-source")
		assert.Equal(t, 2, got.line, "having counted the newline it did reach")
		assert.Equal(t, 3, got.col, "and the two columns after it")
	})

	// The ordinary case still walks exactly as far as it was asked to.
	got := start().advance(source, 2)
	assert.Equal(t, 2, got.offset)
	assert.Equal(t, 1, got.line)
	assert.Equal(t, 3, got.col)
}
