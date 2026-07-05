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
