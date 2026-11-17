package roundtrip

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// itemIndent is the width of `- `, after which a compact sequence item's first entry starts.
const itemIndent = 2

// replacedSpan is where a value replaced around its key line sits. The inline part is the value
// text starting on the key (or `-`) line, the block part the whole lines below it.
type replacedSpan struct {
	// point is the offset after the `:` or `-` and any anchor and tag tokens; props is set when
	// there are such tokens.
	point            int
	props            bool
	tagStart, tagEnd int
	inStart, inEnd   int
	blStart, blEnd   int
	// compact is set for a sequence item whose collection starts on the `-` line; its value is
	// then all of [point, blEnd).
	compact bool
	// flow is set when the inline part is a non-empty flow collection, multiLine when it also
	// crosses lines; inFlow when the value sits inside a flow collection.
	flow, multiLine, inFlow bool
	// childCol and dashCol are the columns of a new block mapping's keys and of a new block
	// sequence's dashes; lineIndent is the indentation of the line holding inStart.
	childCol, dashCol, lineIndent int
}

// replacedSpan locates r's source value as described on replacedSpan.
func (d *Document) replacedSpan(r *replacement) (*replacedSpan, error) {
	n, c, b := r.src, r.ctx, d.body
	var mark int
	var err error
	switch {
	case r.key != nil:
		mark, err = d.afterColon(r.key, c)
	case r.item && c.flow:
		var ok bool
		if mark, ok = d.offset(n.Line, n.Column); !ok {
			err = fmt.Errorf("%w: %d:%d outside the body", ErrUnsupportedScalar, n.Line, n.Column)
		}
	case r.item:
		mark, err = d.afterDash(n)
	default:
		err = fmt.Errorf("%w: the root is only replaced by a scalar", ErrUnsupportedScalar)
	}
	if err != nil {
		return nil, err
	}
	s := &replacedSpan{inFlow: c.flow}
	s.point, s.tagStart, s.tagEnd = d.valuePoint(mark, c)
	s.props = s.point != mark
	s.inStart, s.inEnd = s.point, s.point
	s.blStart = lineAfter(b, s.point)
	s.blEnd = s.blStart
	l := d.layout()
	s.childCol, s.dashCol = c.owner+l.unit, c.owner+l.unit
	if l.indentless && r.key != nil {
		s.dashCol = c.owner
	}
	switch {
	case n.Kind == yaml.ScalarNode || n.Kind == yaml.AliasNode:
		err = d.scalarParts(s, n, c, mark)
	case n.Style&yaml.FlowStyle != 0 || c.flow:
		s.inStart, s.inEnd, err = d.flowExtent(n, c)
		s.flow = len(n.Content) > 0
		s.multiLine = bytes.IndexByte(b[s.inStart:s.inEnd], '\n') >= 0
		s.blStart = lineAfter(b, s.inEnd)
		s.blEnd = s.blStart
	case r.item && n.Content[0].Line == d.lineAt(mark):
		s.compact = true
		s.blEnd, err = d.subtreeEnd(n, c)
		s.childCol = d.column(s.point)
		if !isBlank(b[s.point-1]) {
			s.childCol++
		}
		s.dashCol = s.childCol
	default:
		s.blEnd, err = d.subtreeEnd(n, c)
	}
	if err != nil {
		return nil, err
	}
	s.lineIndent = leadingSpaces(lineText(b, lineStart(b, s.inStart)))
	return s, nil
}

// scalarParts fills s for the scalar or alias n, using the scanner's span of its text.
func (d *Document) scalarParts(s *replacedSpan, n *yaml.Node, c leafCtx, mark int) error {
	sp, err := d.scanSpan(n, c)
	if err != nil {
		return err
	}
	b := d.body
	switch {
	case sp.style == styleEmpty:
	case sp.style == styleBlock:
		s.inStart, s.inEnd = sp.start, sp.headerEnd
		s.blStart = lineAfter(b, sp.lineEnd)
		s.blEnd = s.blStart
		if sp.contentStart < sp.end {
			s.blStart, s.blEnd = sp.contentStart, lineAfter(b, sp.end)
		}
	case bytes.IndexByte(b[mark:sp.start], '\n') < 0:
		s.inStart, s.inEnd = sp.start, sp.end
		s.blStart = lineAfter(b, sp.end)
		s.blEnd = s.blStart
	default:
		s.blStart, s.blEnd = lineStart(b, sp.start), lineAfter(b, sp.end)
	}
	return nil
}

// afterColon returns the offset after the `:` that follows the key node's text.
func (d *Document) afterColon(key *yaml.Node, c leafCtx) (int, error) {
	b := d.body
	off, ok := d.offset(key.Line, key.Column)
	for ok && off < len(b) && (b[off] == anchorIndicator || b[off] == tagIndicator) {
		off = d.skipBlanks(d.tokenEnd(off, c))
	}
	end := off
	switch {
	case !ok:
	case key.Style&yaml.SingleQuotedStyle != 0:
		end = quotedEnd(b, off, false)
	case key.Style&yaml.DoubleQuotedStyle != 0:
		end = quotedEnd(b, off, true)
	default:
		end, _ = d.plainLineEnd(off, c)
	}
	end = d.skipBlanks(end)
	if !ok || end >= len(b) || b[end] != ':' {
		return 0, fmt.Errorf("%w: %d:%d: no `:` after the key", ErrUnsupportedScalar, key.Line, key.Column)
	}
	return end + 1, nil
}

// afterDash returns the offset after the `-` of the block sequence item n. The dash precedes n's
// first token on its line, or ends an earlier line, followed at most by a comment.
func (d *Document) afterDash(n *yaml.Node) (int, error) {
	b := d.body
	off, ok := d.offset(n.Line, n.Column)
	for ok && off > 0 && isBlank(b[off-1]) {
		off--
	}
	if ok && off > 0 && b[off-1] == sequenceDash {
		return off, nil
	}
	for ok && off > 0 && b[off-1] == '\n' {
		ls := lineStart(b, off-1)
		text := lineText(b, ls)
		if i := commentIndex(text); i >= 0 {
			text = text[:i]
		}
		text = bytes.TrimRight(text, " \t")
		if t := bytes.TrimSpace(text); len(t) == 0 {
			off = ls
			continue
		}
		if i := len(text) - 1; text[i] == sequenceDash && (i == 0 || isBlank(text[i-1])) {
			return ls + len(text), nil
		}
		break
	}
	return 0, fmt.Errorf("%w: %d:%d: no `-` before the item", ErrUnsupportedScalar, n.Line, n.Column)
}

// commentIndex returns where a comment starts in a line holding no quoted text, or -1.
func commentIndex(text []byte) int {
	for i, ch := range text {
		if ch == commentStart && (i == 0 || isBlank(text[i-1])) {
			return i
		}
	}
	return -1
}

// valuePoint returns the offset after the anchor and tag tokens following off, and the tag token's
// range with the whitespace after it. When no value text follows a token on its line, the point
// stays right after the token; without tokens it is off itself.
func (d *Document) valuePoint(off int, c leafCtx) (point, tagStart, tagEnd int) {
	b := d.body
	point = off
	for next := d.skipBlanks(off); next < len(b) && (b[next] == anchorIndicator || b[next] == tagIndicator); {
		end := d.tokenEnd(next, c)
		after := d.skipBlanks(end)
		absent := d.valueAbsent(after, c)
		if b[next] == tagIndicator {
			tagStart, tagEnd = next, after
			if absent {
				tagEnd = end
			}
		}
		point = end
		if absent {
			break
		}
		point, next = after, after
	}
	return point, tagStart, tagEnd
}

// renderReplacement renders r's current value over its source value.
func (d *Document) renderReplacement(r *replacement) ([]*patch, error) {
	if r.scalar {
		return []*patch{r.leaf}, nil
	}
	v, err := decodeValue(r.cur)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrPatchVerification, r.path, err)
	}
	s, b := r.span, d.body
	from, prefix := s.inStart, ""
	if s.tagStart < s.tagEnd && !sameKind(r.src, v) {
		// A tag of the old kind would not decode the new value; an anchor after it stays.
		from, prefix = s.tagStart, string(b[s.tagEnd:s.inStart])
	}
	inline := !isBlockCollection(v) || s.inFlow || s.flow
	switch {
	case s.compact:
		text := d.inlineText(s, v)
		if !inline {
			lines := d.blockLines(v, s.childCol)
			lines[0] = lines[0][s.childCol:]
			text = strings.Join(lines, d.eol)
		}
		return []*patch{{start: from, end: s.blEnd, text: []byte(d.lead(from, prefix) + text + d.lineEnd(s.blEnd))}}, nil
	case inline:
		p := &patch{start: from, end: s.inEnd, text: []byte(d.lead(from, prefix) + d.inlineText(s, v))}
		if s.blStart < s.blEnd {
			return combine(p, d.removeLines(s.blStart, s.blEnd)), nil
		}
		return []*patch{p}, nil
	case r.item && !s.props:
		// The first entry goes on the `-` line, before any line comment, the rest below it.
		col := d.column(from)
		if !isBlank(b[from-1]) {
			col++
		}
		lines := d.blockLines(v, col)
		p := &patch{start: from, end: s.inEnd, text: []byte(d.lead(from, "") + lines[0][col:])}
		if len(lines) == 1 {
			if s.blStart == s.blEnd {
				return []*patch{p}, nil
			}
			return combine(p, d.removeLines(s.blStart, s.blEnd)), nil
		}
		return combine(p, &patch{start: s.blStart, end: s.blEnd, text: []byte(d.joinLines(lines[1:], s.blStart, s.blEnd))}), nil
	}
	var ps []*patch
	if from < s.inEnd {
		keep := strings.TrimRight(prefix, " \t")
		if keep == "" && from > 0 && isBlank(b[from-1]) {
			from--
		}
		ps = append(ps, &patch{start: from, end: s.inEnd, text: []byte(keep)})
	}
	col := s.childCol
	if _, ok := v.([]any); ok {
		col = s.dashCol
	}
	return append(ps, &patch{start: s.blStart, end: s.blEnd, text: []byte(d.joinLines(d.blockLines(v, col), s.blStart, s.blEnd))}), nil
}

// combine returns a and b as one patch when b starts where a ends, which two patches starting at
// one offset could not express.
func combine(a, b *patch) []*patch {
	if b.start != a.end {
		return []*patch{a, b}
	}
	return []*patch{{start: a.start, end: b.end, text: append(append([]byte(nil), a.text...), b.text...)}}
}

// inlineText writes v where an inline value goes: flow inside a flow collection, multi-line JSON
// over a flow collection that crossed lines.
func (d *Document) inlineText(s *replacedSpan, v any) string {
	switch {
	case !isBlockCollection(v) && !s.inFlow:
		return inlineScalar(v)
	case isBlockCollection(v) && s.multiLine:
		if text, ok := d.jsonText(v, s.lineIndent); ok {
			return text
		}
	}
	return flowText(v)
}

// lead returns prefix plus the space that must separate inline text inserted at from from the
// token before it, so that `key:v` cannot read as one scalar.
func (d *Document) lead(from int, prefix string) string {
	if from == 0 {
		return prefix
	}
	junction := d.body[from-1]
	if prefix != "" {
		junction = prefix[len(prefix)-1]
	}
	if isBlank(junction) || junction == '[' || junction == '{' || junction == '\n' {
		return prefix
	}
	return prefix + " "
}

// removeLines removes the whole lines [start, end). At an EOF without a line break it also takes
// the line break before them, so the file still ends without one.
func (d *Document) removeLines(start, end int) *patch {
	if end == len(d.body) && d.unterminated() && start > 0 {
		start--
		if start > 0 && d.body[start-1] == '\r' {
			start--
		}
	}
	return &patch{start: start, end: end}
}

// joinLines joins lines that replace the whole lines [start, end), keeping the file's line ending
// and its lack of a final line break.
func (d *Document) joinLines(lines []string, start, end int) string {
	text := strings.Join(lines, d.eol) + d.lineEnd(end)
	if start == len(d.body) && d.unterminated() {
		text = d.eol + text
	}
	return text
}

// lineEnd is the line break text ending at end needs: none at an EOF without one.
func (d *Document) lineEnd(end int) string {
	if end == len(d.body) && d.unterminated() {
		return ""
	}
	return d.eol
}

func (d *Document) unterminated() bool {
	return len(d.body) > 0 && d.body[len(d.body)-1] != '\n'
}

// lineAt returns the 1-based line holding body offset off.
func (d *Document) lineAt(off int) int {
	return sort.SearchInts(d.lineStarts, off+1)
}

// column returns the 0-based rune column of body offset off.
func (d *Document) column(off int) int {
	return utf8.RuneCount(d.body[lineStart(d.body, off):off])
}

func lineStart(b []byte, off int) int {
	return bytes.LastIndexByte(b[:off], '\n') + 1
}

// sameKind reports whether the decoded value v is of n's kind, so that n's tag still applies.
func sameKind(n *yaml.Node, v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any:
		return n.Kind == yaml.MappingNode
	case []any:
		return n.Kind == yaml.SequenceNode
	}
	return n.Kind == yaml.ScalarNode
}
