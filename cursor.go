package keywordcase

import (
	sql "github.com/gomatic/go-sql"
)

// Position arithmetic. The column is a 1-based BYTE count, matching
// go/token.Position.Column — the convention go-yze and the stickler consumer
// share — so every yze analyzer reports the same column for the same byte. A
// rune-based column here would silently disagree with every other rule.

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
func (c cursor) advance(source sql.SQL, target int) cursor {
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
