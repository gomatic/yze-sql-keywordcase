package keywordcase

import (
	"testing"

	sql "github.com/gomatic/go-sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSequenceStopsWhenQualifiedNameIsNotDotted pins the conservative stop inside the
// token matcher. A real parse only ever yields a multi-part name whose first part
// matches the source when the source is literally dot-separated, so the missing-dot
// guard cannot be reached through Diagnostics — it is exercised directly here to pin
// that a malformed sequence marks nothing past the first name instead of misreading
// the stream.
func TestSequenceStopsWhenQualifiedNameIsNotDotted(t *testing.T) {
	source := sql.SQL("a b")
	result, err := sql.Scan(source)
	require.NoError(t, err)
	c := collector{
		index:   tokenIndex(result.Tokens),
		offsets: map[int]bool{},
		names:   map[string]bool{},
		source:  source,
		tokens:  result.Tokens,
	}

	c.sequence(0, []string{"a", "b"})

	assert.Equal(t, map[int]bool{0: true}, c.offsets,
		"the first name is marked; the missing dot stops the walk before the second")
	assert.Empty(t, c.names)
}

// TestMatchesComparesBareIdentifiersFoldedAndQuotedOnesExactly names matches'
// claim, which is PostgreSQL's identifier rule and not a convenience. A bare
// identifier is folded to lowercase by the parser, so `T.Name` and `t.name` are
// the same column; a QUOTED identifier is case-sensitive, so `"Name"` and
// `"name"` are two different columns. Comparing quoted identifiers
// case-insensitively would let the collector vouch for a position that is
// actually a different column, and the analyzer would then stay silent about a
// genuine keyword.
func TestMatchesComparesBareIdentifiersFoldedAndQuotedOnesExactly(t *testing.T) {
	t.Parallel()

	// Bare: whatever the case, the qualified name resolves and the keyword-class
	// token is treated as an identifier rather than flagged.
	for _, src := range []string{
		`SELECT t.Name FROM t`,
		`SELECT T.NAME FROM t`,
		`SELECT t.name FROM t`,
	} {
		diags, err := Diagnostics("q.sql", sql.SQL(src))
		require.NoError(t, err, src)
		for _, d := range diags {
			assert.NotContains(t, d.Message, "Name", "a bare identifier folds and must not be flagged: %s", src)
		}
	}

	// Quoted: the exact spelling is the identity, and quotes are not part of it.
	diags, err := Diagnostics("q.sql", sql.SQL(`SELECT t."Name" FROM t`))
	require.NoError(t, err)
	for _, d := range diags {
		assert.NotContains(t, d.Message, `"Name"`, "a quoted identifier is an identifier, not a keyword")
	}
}
