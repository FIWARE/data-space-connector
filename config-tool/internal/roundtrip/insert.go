package roundtrip

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	mapTag = "!!map"
	seqTag = "!!seq"
	// flowSeparator joins the entries of a flow collection.
	flowSeparator = ", "
)

// group is a pending insert: the new keys of one source mapping, rendered at one offset in
// lexicographic order. Their values live in the current tree under node.
type group struct {
	parent Path
	node   *yaml.Node
	keys   map[string]bool
	offset int
	// col is the column of the parent's keys; flow is set for a one-line flow mapping, whose new
	// entries go inside its brackets.
	col  int
	flow bool
}

// insert sets the missing key p[i] below the existing node n at p[:i] (nil in an empty document),
// creating the mappings p[i+1:] on the way.
func (d *Document) insert(p Path, i int, n *yaml.Node, value any) error {
	for _, seg := range p[i:] {
		if _, ok := seg.Index(); ok {
			return fmt.Errorf("%w: %s: sequence items are never created", ErrNotFound, p[:i+1])
		}
	}
	if n != nil && n.Kind != yaml.MappingNode && !isNull(n) {
		return fmt.Errorf("%w: %s", ErrNotMapping, p[:i])
	}
	key, _ := p[i].Key()
	q := p[:i]
	empty := n == nil
	if empty {
		n = &yaml.Node{Kind: yaml.MappingNode, Tag: mapTag}
	}
	switch r, exact, inGroup := d.owner(q); {
	case inGroup || (r != nil && !exact):
	case r != nil:
		if err := d.prepareCollection(r); err != nil {
			return err
		}
	default:
		if err := d.insertAt(q, n, key); err != nil {
			return err
		}
	}
	if empty {
		d.root.Content = []*yaml.Node{n}
	}
	if isNull(n) {
		*n = yaml.Node{Kind: yaml.MappingNode, Tag: mapTag, Anchor: n.Anchor, Line: n.Line, Column: n.Column}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: strTag, Value: key}, nodeFor(nest(p[i+1:], value)))
	return nil
}

// insertAt records a new key below the source node at q, whose current node is n: in the insert
// group of a block or one-line flow mapping, else as a replacement of the whole value.
func (d *Document) insertAt(q Path, n *yaml.Node, key string) error {
	var src *yaml.Node
	var loc located
	if !isEmptyDoc(d.src) {
		var err error
		if loc, err = d.locateIn(d.src, q); err != nil {
			return err
		}
		src = loc.node
	}
	multiLine := false
	if src != nil && src.Style&yaml.FlowStyle != 0 && len(src.Content) > 0 {
		open, end, err := d.flowExtent(src, loc.ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
		multiLine = bytes.IndexByte(d.body[open:end], '\n') >= 0
	}
	if src != nil && (len(src.Content) == 0 || multiLine) {
		r, err := d.newReplacement(q, n)
		if err == nil {
			err = d.prepareCollection(r)
		}
		if err != nil {
			return err
		}
		d.supersede(q)
		if d.reps == nil {
			d.reps = map[string]*replacement{}
		}
		d.reps[q.String()] = r
		return nil
	}
	g := d.groups[q.String()]
	if g == nil {
		var err error
		if g, err = d.newGroup(q, src, loc.ctx, n); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
		if d.groups == nil {
			d.groups = map[string]*group{}
		}
		d.groups[q.String()] = g
	}
	g.keys[key] = true
	return nil
}

// newGroup places the insert group of the source mapping src (nil for an empty document): after
// the last child's subtree, or before a one-line flow mapping's closing bracket.
func (d *Document) newGroup(q Path, src *yaml.Node, c leafCtx, n *yaml.Node) (*group, error) {
	g := &group{parent: slices.Clone(q), node: n, keys: map[string]bool{}, offset: len(d.body)}
	switch {
	case src == nil:
	case src.Style&yaml.FlowStyle != 0:
		_, end, err := d.flowExtent(src, c)
		if err != nil {
			return nil, err
		}
		g.offset, g.flow = end-1, true
		for isBlank(d.body[g.offset-1]) {
			g.offset--
		}
	default:
		g.col = src.Content[0].Column - 1
		end, err := d.subtreeEnd(src, leafCtx{owner: g.col})
		if err != nil {
			return nil, err
		}
		g.offset = end
	}
	return g, nil
}

// renderGroup renders g's keys from the current tree.
func (d *Document) renderGroup(g *group) (*patch, error) {
	var lines []string
	var flow strings.Builder
	for _, k := range slices.Sorted(maps.Keys(g.keys)) {
		_, vn := child(g.node, Key(k))
		v, err := decodeValue(vn)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPatchVerification, g.parent, err)
		}
		if g.flow {
			flow.WriteString(flowSeparator + flowKey(k) + ": " + flowText(v))
			continue
		}
		lines = append(lines, d.pairLines(blockKey(k), v, g.col)...)
	}
	text := flow.String()
	if !g.flow {
		text = d.joinLines(lines, g.offset, g.offset)
	}
	return &patch{start: g.offset, end: g.offset, text: []byte(text), insert: true, col: g.col}, nil
}

// eofChompPatch returns the patch that sets strip chomping on a block scalar ending the body
// without a line break, as an insert after it adds one. Nil when there is none; the self-check guards the rest.
func (d *Document) eofChompPatch() *patch {
	if len(d.body) == 0 || d.body[len(d.body)-1] == '\n' || len(d.src.Content) == 0 {
		return nil
	}
	n, c := d.src.Content[0], leafCtx{owner: noOwner}
	for n.Style&yaml.FlowStyle == 0 && len(n.Content) > 0 {
		if n.Kind == yaml.MappingNode {
			c = leafCtx{owner: n.Content[len(n.Content)-2].Column - 1}
		} else {
			c = leafCtx{owner: d.dashColumn(n)}
		}
		n = n.Content[len(n.Content)-1]
	}
	if n.Kind != yaml.ScalarNode || n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) == 0 {
		return nil
	}
	s, err := d.scanSpan(n, c)
	if err != nil || s.end != len(d.body) {
		return nil
	}
	header := d.body[s.start:s.headerEnd]
	switch i := bytes.IndexByte(header, chompKeep); {
	case bytes.IndexByte(header, chompStrip) >= 0:
		return nil
	case i >= 0:
		return &patch{start: s.start + i, end: s.start + i + 1, text: []byte{chompStrip}}
	}
	return &patch{start: s.headerEnd, end: s.headerEnd, text: []byte{chompStrip}}
}

// subtreeEnd returns the start of the line after n's subtree: past its last leaf's text and the
// comment lines right below it that are indented deeper than c.owner.
func (d *Document) subtreeEnd(n *yaml.Node, c leafCtx) (int, error) {
	last, err := d.lastEnd(n, c)
	if err != nil {
		return 0, err
	}
	off := lineAfter(d.body, last)
	for off < len(d.body) {
		text := lineText(d.body, off)
		t := bytes.TrimLeft(text, " \t")
		if len(t) == 0 || t[0] != commentStart || leadingSpaces(text) <= c.owner {
			break
		}
		off = lineAfter(d.body, off)
	}
	return off, nil
}

// lastEnd returns where the text of n's last leaf ends, as the scanner ends it.
func (d *Document) lastEnd(n *yaml.Node, c leafCtx) (int, error) {
	switch {
	case n.Kind == yaml.ScalarNode || n.Kind == yaml.AliasNode:
		s, err := d.scanSpan(n, c)
		return s.end, err
	case n.Style&yaml.FlowStyle != 0 || c.flow:
		_, end, err := d.flowExtent(n, c)
		return end, err
	case n.Kind == yaml.MappingNode && len(n.Content) > 1:
		k := n.Content[len(n.Content)-2]
		return d.lastEnd(n.Content[len(n.Content)-1], leafCtx{owner: k.Column - 1})
	case n.Kind == yaml.SequenceNode && len(n.Content) > 0:
		return d.lastEnd(n.Content[len(n.Content)-1], leafCtx{owner: d.dashColumn(n)})
	}
	return 0, fmt.Errorf("%w: %d:%d: no end of subtree", ErrUnsupportedScalar, n.Line, n.Column)
}

// flowExtent returns the offsets of the flow collection n's opening bracket and of the byte after
// its matching closing bracket.
func (d *Document) flowExtent(n *yaml.Node, c leafCtx) (int, int, error) {
	b := d.body
	off, ok := d.offset(n.Line, n.Column)
	for ok && off < len(b) && (b[off] == anchorIndicator || b[off] == tagIndicator) {
		off = d.skipBlanks(d.tokenEnd(off, c))
	}
	if ok && off < len(b) && (b[off] == '[' || b[off] == '{') {
		if end, ok := flowEnd(b, off); ok {
			return off, end, nil
		}
	}
	return 0, 0, fmt.Errorf("%w: %d:%d: flow collection not found", ErrUnsupportedScalar, n.Line, n.Column)
}

// flowEnd returns the offset after the bracket that closes the one at open, honouring quotes,
// nesting and comments.
func flowEnd(b []byte, open int) (int, bool) {
	depth := 0
	for i := open; i < len(b); i++ {
		switch ch := b[i]; {
		case ch == '[' || ch == '{':
			depth++
		case ch == ']' || ch == '}':
			if depth--; depth == 0 {
				return i + 1, true
			}
		case (ch == doubleQuote || ch == singleQuote) && opensQuote(b[i-1]):
			end := quotedEnd(b, i, ch == doubleQuote)
			if end == i {
				return 0, false
			}
			i = end - 1
		case ch == commentStart && isSpace(b[i-1]):
			i = lineAfter(b, i) - 1
		}
	}
	return 0, false
}

// opensQuote reports whether a quote after prev starts a quoted scalar rather than sitting inside
// a plain one, as in `don't`.
func opensQuote(prev byte) bool {
	return isSpace(prev) || strings.IndexByte("[{,:", prev) >= 0
}

// isEmptyDoc reports whether the document node root holds no value text: no node, or the null
// that a lone "---" gives.
func isEmptyDoc(root *yaml.Node) bool {
	if len(root.Content) == 0 {
		return true
	}
	top := root.Content[0]
	return isNull(top) && top.Value == "" && top.Style == 0 && top.Anchor == ""
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == nullTag
}

// nest wraps value in one single-key mapping per segment of p.
func nest(p Path, value any) any {
	for i := len(p) - 1; i >= 0; i-- {
		k, _ := p[i].Key()
		value = map[string]any{k: value}
	}
	return value
}

// checkValue refuses a value Set does not write with ErrUnsupportedValue.
func checkValue(v any) error {
	switch v := v.(type) {
	case map[string]any:
		for _, sub := range v {
			if err := checkValue(sub); err != nil {
				return err
			}
		}
	case []any:
		for _, sub := range v {
			if err := checkValue(sub); err != nil {
				return err
			}
		}
	default:
		_, _, err := scalarFor(v)
		return err
	}
	return nil
}

// nodeFor builds the node tree of a value checkValue accepts; the scalar tags come from the Go types.
func nodeFor(v any) *yaml.Node {
	switch v := v.(type) {
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: mapTag}
		for _, k := range slices.Sorted(maps.Keys(v)) {
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: strTag, Value: k}, nodeFor(v[k]))
		}
		return n
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: seqTag}
		for _, item := range v {
			n.Content = append(n.Content, nodeFor(item))
		}
		return n
	}
	tag, text, _ := scalarFor(v)
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: text}
}

// setNode overwrites n in place with value's tree, keeping its anchor and position, so that the
// aliases of n resolve to the new value.
func setNode(n *yaml.Node, value any) {
	m := nodeFor(value)
	m.Anchor, m.Line, m.Column = n.Anchor, n.Line, n.Column
	*n = *m
}
