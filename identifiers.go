package keywordcase

import (
	"strings"

	sql "github.com/gomatic/go-sql"
	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// identifiers records where the parse tree proved a keyword-spelled token is really an
// identifier. libpg_query's scanner assigns KeywordKind by table lookup, context-free,
// so an unquoted column named "Version" scans as a keyword token — only the parser can
// tell the two uses apart. Tokens are cleared either by exact byte offset (nodes that
// carry a source location) or by spelling (alias names, which carry none). When the
// source does not parse at all (a PL/pgSQL body, a fragment), isConservative is set and
// every identifier-capable keyword token is skipped: a missed keyword is recoverable,
// calling an identifier a keyword is a false positive that blocks an innocent build.
type identifiers struct {
	offsets        map[int]bool
	names          map[string]bool
	isConservative bool
}

// covers reports whether the keyword-spelled token at byte offset, spelled lower in its
// canonical lowercase form, is proven (or presumed, when conservative) to be an identifier.
func (ids identifiers) covers(offset int, lower string) bool {
	return ids.isConservative || ids.offsets[offset] || ids.names[lower]
}

// identifierCapable reports whether kind can also be a bare identifier: PostgreSQL's
// unreserved and column-name keyword classes double as identifiers (column, table, type,
// and function names), while reserved and type/function-name keywords cannot appear bare.
func identifierCapable(kind pg_query.KeywordKind) bool {
	return kind == pg_query.KeywordKind_UNRESERVED_KEYWORD || kind == pg_query.KeywordKind_COL_NAME_KEYWORD
}

// identify parses source and collects every token position the parse tree shows to be an
// identifier. tokens is the scan of the same source, so every location in the tree names
// a real token start. A source that does not parse yields the conservative mode.
func identify(source sql.SQL, tokens []*pg_query.ScanToken) identifiers {
	tree, err := sql.Parse(source)
	if err != nil {
		return identifiers{isConservative: true}
	}
	c := collector{
		index:   tokenIndex(tokens),
		offsets: map[int]bool{},
		names:   map[string]bool{},
		source:  source,
		tokens:  tokens,
	}
	walkMessage(tree.ProtoReflect(), c.visit)
	return identifiers{offsets: c.offsets, names: c.names}
}

// tokenIndex maps each token's byte start to its position in the token stream.
func tokenIndex(tokens []*pg_query.ScanToken) map[int]int {
	index := make(map[int]int, len(tokens))
	for i, token := range tokens {
		index[int(token.Start)] = i
	}
	return index
}

// collector accumulates identifier evidence while walking the parse tree. Its maps are
// shared references, so the value receiver still records into the one shared result.
type collector struct {
	index   map[int]int
	offsets map[int]bool
	names   map[string]bool
	source  sql.SQL
	tokens  []*pg_query.ScanToken
}

// visit records the identifier tokens of the node kinds that name things: column
// references, relations, column definitions, type and function names (all located), and
// targets and aliases (located when possible, by spelling otherwise).
func (c collector) visit(m protoreflect.Message) {
	switch node := m.Interface().(type) {
	case *pg_query.ColumnRef:
		c.sequence(int(node.Location), fieldNames(node.Fields))
	case *pg_query.RangeVar:
		c.sequence(int(node.Location), rangeVarNames(node))
	case *pg_query.ColumnDef:
		c.sequence(int(node.Location), []string{node.Colname})
	case *pg_query.TypeName:
		c.sequence(int(node.Location), fieldNames(node.Names))
	case *pg_query.FuncCall:
		c.sequence(int(node.Location), fieldNames(node.Funcname))
	case *pg_query.ResTarget:
		c.resTarget(node)
	case *pg_query.Alias:
		c.alias(node)
	}
}

// sequence marks the dot-separated identifier tokens starting at byte offset location,
// pairing each token against its parse-tree name in order. Any mismatch stops without
// marking further: the tokens are then not the names spelled out in the source (a
// resolved builtin type like pg_catalog.int4, say), so they keep their keyword treatment.
func (c collector) sequence(location int, names []string) {
	idx, ok := c.index[location]
	if !ok {
		return
	}
	for i, name := range names {
		idx, ok = c.expect(idx, i, name)
		if !ok {
			return
		}
	}
}

// expect consumes a leading dot separator when i > 0, then the token spelling name,
// marking that token as an identifier. It reports the stream index after the consumed
// name. The tokens it steps across exist because location and names come from a
// successful parse of the very source tokens was scanned from.
func (c collector) expect(idx, i int, name string) (int, bool) {
	if i > 0 {
		if c.text(idx) != "." {
			return idx, false
		}
		idx = c.skipComments(idx + 1)
	}
	if !c.matches(idx, name) {
		return idx, false
	}
	c.offsets[int(c.tokens[idx].Start)] = true
	return c.skipComments(idx + 1), true
}

// matches reports whether the token at idx spells name: bare identifiers compare
// case-insensitively (the parser stores them folded to lowercase), quoted identifiers
// exactly, without their quotes.
func (c collector) matches(idx int, name string) bool {
	text := c.text(idx)
	if strings.HasPrefix(text, `"`) {
		return strings.Trim(text, `"`) == name
	}
	return strings.ToLower(text) == name
}

// text is the source text of the token at idx.
func (c collector) text(idx int) string {
	token := c.tokens[idx]
	return string(c.source[token.Start:token.End])
}

// skipComments advances idx past comment tokens, which the scanner emits between the
// identifier tokens of a qualified name (`t./* legal */Name`).
func (c collector) skipComments(idx int) int {
	for idx < len(c.tokens) && isComment(c.tokens[idx].Token) {
		idx++
	}
	return idx
}

// isComment reports whether kind is one of the scanner's two comment token kinds.
func isComment(kind pg_query.Token) bool {
	return kind == pg_query.Token_SQL_COMMENT || kind == pg_query.Token_C_COMMENT
}

// resTarget marks an INSERT column or UPDATE SET target, whose location is the name
// token itself. A SELECT alias's location points at its expression instead, so an
// unmatched name is remembered by spelling — the alias token has no location of its own.
func (c collector) resTarget(node *pg_query.ResTarget) {
	if node.Name == "" {
		return
	}
	idx, ok := c.index[int(node.Location)]
	if ok && c.matches(idx, node.Name) {
		c.offsets[int(c.tokens[idx].Start)] = true
		return
	}
	c.names[node.Name] = true
}

// alias remembers alias spellings (FROM-clause aliases and their column renames). Alias
// nodes carry no source location, so they clear tokens by name: a keyword token spelling
// an alias is skipped rather than flagged, preferring a missed keyword over calling an
// identifier a keyword.
func (c collector) alias(node *pg_query.Alias) {
	c.names[node.Aliasname] = true
	for _, col := range node.Colnames {
		c.names[col.GetString_().GetSval()] = true
	}
}

// fieldNames extracts the leading String names of a node list (ColumnRef fields, TypeName
// names, FuncCall funcname), stopping at the first non-name node (A_Star, A_Indices).
func fieldNames(fields []*pg_query.Node) []string {
	var names []string
	for _, field := range fields {
		s := field.GetString_()
		if s == nil {
			return names
		}
		names = append(names, s.Sval)
	}
	return names
}

// rangeVarNames is the qualified relation name as spelled: catalog, schema, and relation,
// keeping only the parts present.
func rangeVarNames(node *pg_query.RangeVar) []string {
	var names []string
	for _, name := range []string{node.Catalogname, node.Schemaname, node.Relname} {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// walkMessage visits m and every message reachable from it, depth-first. pg_query's
// schema has no map fields, so lists and singular messages are the only compound shapes.
func walkMessage(m protoreflect.Message, visit func(protoreflect.Message)) {
	visit(m)
	m.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		walkField(fd, value, visit)
		return true
	})
}

// walkField recurses into a populated field's message values, skipping scalars.
func walkField(fd protoreflect.FieldDescriptor, value protoreflect.Value, visit func(protoreflect.Message)) {
	switch {
	case fd.IsList() && fd.Kind() == protoreflect.MessageKind:
		walkList(value.List(), visit)
	case fd.Kind() == protoreflect.MessageKind:
		walkMessage(value.Message(), visit)
	}
}

// walkList visits every message element of list.
func walkList(list protoreflect.List, visit func(protoreflect.Message)) {
	for i := range list.Len() {
		walkMessage(list.Get(i).Message(), visit)
	}
}
