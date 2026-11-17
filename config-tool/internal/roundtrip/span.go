package roundtrip

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	anchorIndicator = '&'
	tagIndicator    = '!'
	aliasIndicator  = '*'
	commentStart    = '#'
	singleQuote     = '\''
	doubleQuote     = '"'
	literalHeader   = '|'
	foldedHeader    = '>'
	chompStrip      = '-'
	chompKeep       = '+'
	sequenceDash    = '-'
	// noOwner is the owning column of the root node, which no line has to be indented past.
	noOwner = -1
	// checkKey is the key under which a span's text is decoded on its own.
	checkKey = "k: "
)

// spanStyle is how a leaf's value text is written in the source.
type spanStyle int

const (
	styleEmpty spanStyle = iota
	stylePlain
	styleSingle
	styleDouble
	styleBlock
	styleAlias
)

// leafCtx is where a leaf sits: the 0-based column a continuation or block-content line must be
// indented past (the key's column, or the `-` column of a sequence item), and whether the leaf is
// inside a flow collection.
type leafCtx struct {
	owner int
	flow  bool
}

// valueSpan locates a leaf's value text in the body. Offsets are byte offsets into the body.
type valueSpan struct {
	style spanStyle
	// node is the node's first token: an anchor, a tag or the value text.
	node int
	// [tagStart, tagEnd) is the tag token and the whitespace after it; empty when there is none.
	tagStart, tagEnd int
	// [start, end) is the value text; a block scalar's runs from its indicator to its last
	// content line, and an implicit null's is empty.
	start, end int
	// Block scalar only: the end of the indicator token, the end of its line's text (before any
	// line break) and the start of the first content line.
	headerEnd, lineEnd, contentStart int
}

// scanSpan finds the value text of the scalar or alias leaf n and checks it by decoding it on its
// own. A span it cannot place exactly gives ErrUnsupportedScalar.
func (d *Document) scanSpan(n *yaml.Node, c leafCtx) (valueSpan, error) {
	off, ok := d.offset(n.Line, n.Column)
	if !ok {
		return valueSpan{}, fmt.Errorf("%w: position %d:%d outside the body", ErrUnsupportedScalar, n.Line, n.Column)
	}
	s := valueSpan{node: off}
	s.start = d.skipProperties(&s, c)
	b := d.body
	switch {
	case n.Kind == yaml.AliasNode:
		s.style, s.end = styleAlias, d.tokenEnd(s.start, c)
	case n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0:
		s.style = styleBlock
		d.scanBlock(&s, c)
	case n.Style&yaml.SingleQuotedStyle != 0:
		s.style, s.end = styleSingle, quotedEnd(b, s.start, false)
	case n.Style&yaml.DoubleQuotedStyle != 0:
		s.style, s.end = styleDouble, quotedEnd(b, s.start, true)
	case d.valueAbsent(d.skipBlanks(s.start), c):
		s.style, s.end = styleEmpty, s.start
	default:
		s.style, s.end = stylePlain, d.plainEnd(s.start, c)
	}
	if err := d.checkSpan(n, s, c); err != nil {
		return valueSpan{}, fmt.Errorf("%w: %d:%d: %v", ErrUnsupportedScalar, n.Line, n.Column, err)
	}
	return s, nil
}

// skipProperties moves past the anchor and tag tokens at s.node and returns the start of the
// value text. When no value text follows on the line, it stops right after the last token, so an
// inserted value lands before the whitespace and any comment.
func (d *Document) skipProperties(s *valueSpan, c leafCtx) int {
	off := s.node
	for off < len(d.body) && (d.body[off] == anchorIndicator || d.body[off] == tagIndicator) {
		tok := off
		end := d.tokenEnd(off, c)
		next := d.skipBlanks(end)
		if d.valueAbsent(next, c) {
			next = end
		}
		if d.body[tok] == tagIndicator {
			s.tagStart, s.tagEnd = tok, next
		}
		off = next
		if next == end {
			break
		}
	}
	return off
}

// valueAbsent reports whether no value text starts at off: the line, or the flow entry, ends there.
func (d *Document) valueAbsent(off int, c leafCtx) bool {
	if off >= len(d.body) {
		return true
	}
	switch ch := d.body[off]; {
	case ch == '\n' || ch == '\r' || ch == commentStart:
		return true
	case c.flow && isFlowIndicator(ch):
		return true
	}
	return false
}

func (d *Document) skipBlanks(off int) int {
	for off < len(d.body) && isBlank(d.body[off]) {
		off++
	}
	return off
}

// tokenEnd returns the end of the anchor, tag or alias token at off.
func (d *Document) tokenEnd(off int, c leafCtx) int {
	for off < len(d.body) && !isSpace(d.body[off]) && !(c.flow && isFlowIndicator(d.body[off])) {
		off++
	}
	return off
}

// quotedEnd returns the offset after the closing quote of the quoted scalar at off, or off when
// it is unclosed. Single quotes escape themselves by doubling; double quotes use backslashes.
func quotedEnd(b []byte, off int, double bool) int {
	for i := off + 1; i < len(b); i++ {
		switch {
		case double && b[i] == '\\':
			i++
		case double && b[i] == doubleQuote:
			return i + 1
		case !double && b[i] == singleQuote:
			if i+1 < len(b) && b[i+1] == singleQuote {
				i++
				continue
			}
			return i + 1
		}
	}
	return off
}

// plainEnd returns the end of the plain scalar starting at off. In block context it continues
// over following lines indented past the owner, up to a comment, a line at or left of the
// owner, or EOF; trailing blank lines are not part of it.
func (d *Document) plainEnd(off int, c leafCtx) int {
	end, comment := d.plainLineEnd(off, c)
	if comment || c.flow {
		return end
	}
	for next := lineAfter(d.body, end); next < len(d.body); {
		text := lineText(d.body, next)
		trimmed := bytes.TrimLeft(text, " \t")
		if len(trimmed) == 0 {
			next = lineAfter(d.body, next)
			continue
		}
		if leadingSpaces(text) <= c.owner || trimmed[0] == commentStart {
			break
		}
		end, comment = d.plainLineEnd(next+len(text)-len(trimmed), c)
		if comment {
			break
		}
		next = lineAfter(d.body, end)
	}
	return end
}

// plainLineEnd returns where plain text starting at off ends on its line, trimmed of trailing
// whitespace, and whether a comment ends it.
func (d *Document) plainLineEnd(off int, c leafCtx) (int, bool) {
	b := d.body
	end := off
	comment := false
	for i := off; i < len(b) && b[i] != '\n'; i++ {
		ch := b[i]
		if ch == commentStart && i > off && isBlank(b[i-1]) {
			comment = true
			break
		}
		if c.flow && isFlowIndicator(ch) {
			break
		}
		if ch == ':' && (i+1 == len(b) || isSpace(b[i+1]) || (c.flow && isFlowIndicator(b[i+1]))) {
			break
		}
		if !isSpace(ch) {
			end = i + 1
		}
	}
	return end, comment
}

// scanBlock fills the block-scalar fields of s: the indicator token, its line and the content
// lines indented past the owner. Trailing blank lines are included only under keep chomping.
func (d *Document) scanBlock(s *valueSpan, c leafCtx) {
	b := d.body
	s.headerEnd = s.start
	for s.headerEnd < len(b) && !isSpace(b[s.headerEnd]) {
		s.headerEnd++
	}
	s.lineEnd = s.headerEnd + len(lineText(b, s.headerEnd))
	s.end = s.lineEnd
	s.contentStart = s.lineEnd
	keep := bytes.IndexByte(b[s.start:s.headerEnd], chompKeep) >= 0
	first := true
	for next := lineAfter(b, s.lineEnd); next < len(b); next = lineAfter(b, next) {
		text := lineText(b, next)
		blank := len(bytes.TrimLeft(text, " \t")) == 0
		if !blank && leadingSpaces(text) <= c.owner {
			break
		}
		if first {
			s.contentStart, first = next, false
		}
		if !blank || keep {
			s.end = next + len(text)
		}
	}
	if first || s.end < s.contentStart {
		s.contentStart = s.end
	}
}

// checkSpan decodes the span's text on its own: it must give n's value and carry no comment, as
// a span that swallowed a comment still decodes to the same value.
func (d *Document) checkSpan(n *yaml.Node, s valueSpan, c leafCtx) error {
	b := d.body
	switch s.style {
	case styleEmpty:
		if n.Value != "" || s.start == 0 || !d.emptyAllowedAt(s) {
			return fmt.Errorf("no value text at the node")
		}
		return nil
	case styleAlias:
		if got := string(b[s.start:s.end]); got != string(aliasIndicator)+n.Value {
			return fmt.Errorf("alias text %q", got)
		}
		return nil
	}
	if s.end <= s.start {
		return fmt.Errorf("empty or unclosed value text")
	}
	text := b[s.start:s.end]
	if s.style == styleBlock {
		text = blockText(b[s.start:s.headerEnd], b[s.contentStart:s.end], s.contentStart < s.end)
	}
	got, ok := d.decodeAt(text, s.end, c)
	if !ok || got.Kind != yaml.ScalarNode || got.Value != n.Value {
		return fmt.Errorf("text %q does not decode to the node's value", text)
	}
	return nil
}

// emptyAllowedAt reports whether a value can be inserted at the empty span s: right after a `:`
// or `-` indicator, or after an anchor or tag token.
func (d *Document) emptyAllowedAt(s valueSpan) bool {
	if s.start > s.node {
		return true
	}
	prev := d.body[s.start-1]
	return prev == ':' || prev == '-'
}

// decodeAt decodes text as the value of a leaf in context c, followed by the blank lines that
// follow offset end in the body. It returns the scalar it decodes to; ok is false when the text
// does not decode to exactly one scalar, or carries a comment.
func (d *Document) decodeAt(text []byte, end int, c leafCtx) (*yaml.Node, bool) {
	var src bytes.Buffer
	src.WriteString(strings.Repeat(" ", max(c.owner, 0)))
	src.WriteString(checkKey)
	if c.flow {
		src.WriteByte('[')
	}
	src.Write(text)
	if c.flow {
		src.WriteByte(']')
	}
	src.Write(d.blankAfter(end))
	var root yaml.Node
	if err := yaml.Unmarshal(src.Bytes(), &root); err != nil || anyComment(&root) {
		return nil, false
	}
	if len(root.Content) != 1 || len(root.Content[0].Content) != 2 {
		return nil, false
	}
	v := root.Content[0].Content[1]
	if c.flow {
		if v.Kind != yaml.SequenceNode || len(v.Content) != 1 {
			return nil, false
		}
		v = v.Content[0]
	}
	if v.Kind != yaml.ScalarNode {
		return nil, false
	}
	return v, true
}

// blankAfter returns the line break after offset end and the blank lines that follow it, when
// only whitespace follows end on its line; a block scalar's chomping depends on them.
func (d *Document) blankAfter(end int) []byte {
	if len(bytes.TrimSpace(lineText(d.body, end))) != 0 {
		return nil
	}
	stop := lineAfter(d.body, end)
	for stop < len(d.body) && len(bytes.TrimSpace(lineText(d.body, stop))) == 0 {
		stop = lineAfter(d.body, stop)
	}
	return d.body[end:stop]
}

// blockText joins a block scalar's header and content lines the way they are decoded, without
// whatever followed the header on its line.
func blockText(header, content []byte, hasContent bool) []byte {
	text := append([]byte(nil), header...)
	if hasContent {
		text = append(append(text, '\n'), content...)
	}
	return text
}

func anyComment(n *yaml.Node) bool {
	if hasComments(n) {
		return true
	}
	for _, c := range n.Content {
		if anyComment(c) {
			return true
		}
	}
	return false
}

// lineText returns the rest of the line at off, without its line break.
func lineText(b []byte, off int) []byte {
	end := bytes.IndexByte(b[off:], '\n')
	if end < 0 {
		end = len(b) - off
	}
	return bytes.TrimSuffix(b[off:off+end], []byte("\r"))
}

// lineAfter returns the start of the line after the one holding off, len(b) when there is none.
func lineAfter(b []byte, off int) int {
	end := bytes.IndexByte(b[off:], '\n')
	if end < 0 {
		return len(b)
	}
	return off + end + 1
}

func leadingSpaces(text []byte) int {
	return len(text) - len(bytes.TrimLeft(text, " "))
}

func isBlank(ch byte) bool {
	return ch == ' ' || ch == '\t'
}

func isSpace(ch byte) bool {
	return isBlank(ch) || ch == '\n' || ch == '\r'
}

func isFlowIndicator(ch byte) bool {
	return ch == ',' || ch == '[' || ch == ']' || ch == '{' || ch == '}'
}
