package roundtrip

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	strTag   = "!!str"
	boolTag  = "!!bool"
	intTag   = "!!int"
	floatTag = "!!float"
	// defaultIndentUnit indents a block scalar's content past its owner when the source shows none.
	defaultIndentUnit = 2
)

// patch is a pending replacement of the body bytes [start, end), recorded for one path.
type patch struct {
	start, end int
	text       []byte
	// span and ctx describe the source value, so a later Set of the same path re-renders it.
	span valueSpan
	ctx  leafCtx
}

// candidate is one way to write a new value: an inline text, or a block scalar's header and
// content lines.
type candidate struct {
	head    []byte
	content []byte
	block   bool
}

// Set replaces the scalar or alias leaf at p with value: a string, bool, int, int64, float64 or
// nil. The edit is a byte patch rendered at Write; a value equal to the current one adds none.
// Errors: ErrNotFound, ErrNotScalar, ErrAliasPath, ErrUnsupportedValue, ErrUnsupportedScalar.
func (d *Document) Set(p Path, value any) error {
	tag, text, err := scalarFor(value)
	if err != nil {
		return err
	}
	loc, err := d.locate(p)
	if err != nil {
		return err
	}
	if loc.merge {
		return fmt.Errorf("%w: %s sets a merge key's value", ErrAliasPath, p)
	}
	n := loc.node
	if n.Kind != yaml.ScalarNode && n.Kind != yaml.AliasNode {
		return fmt.Errorf("%w: %s", ErrNotScalar, p)
	}
	if cur, err := decodeValue(n); err == nil && equalValue(cur, value) {
		return nil
	}
	key := p.String()
	pt, ok := d.patches[key]
	if !ok {
		s, err := d.scanSpan(n, loc.ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		pt = &patch{span: s, ctx: loc.ctx}
	}
	if err := d.render(pt, value); err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	if d.patches == nil {
		d.patches = map[string]*patch{}
	}
	d.patches[key] = pt
	*n = yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: text, Anchor: n.Anchor, Line: n.Line, Column: n.Column,
		HeadComment: n.HeadComment, LineComment: n.LineComment, FootComment: n.FootComment}
	return nil
}

// scalarFor returns the tag and canonical text of a Go value; the tag comes from the Go type, so
// the self-check catches text written with the wrong type.
func scalarFor(v any) (string, string, error) {
	switch v := v.(type) {
	case nil:
		return nullTag, "null", nil
	case string:
		return strTag, v, nil
	case bool:
		return boolTag, strconv.FormatBool(v), nil
	case int, int64:
		return intTag, fmt.Sprint(v), nil
	case float64:
		return floatTag, marshalScalar(v), nil
	}
	return "", "", fmt.Errorf("%w: %T", ErrUnsupportedValue, v)
}

// marshalScalar is yaml.v3's encoding of a single scalar, without the final line break.
func marshalScalar(v any) string {
	out, err := yaml.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("roundtrip: marshal %T: %v", v, err))
	}
	return strings.TrimSuffix(string(out), lineFeed)
}

// render sets pt's range and text for value: the first candidate that decodes back to value in
// place, with the tag kept, else with the tag removed. None fitting gives ErrUnsupportedScalar.
func (d *Document) render(pt *patch, value any) error {
	s, b := pt.span, d.body
	type props struct {
		start  int
		prefix []byte
	}
	tries := []props{{start: s.start}}
	if s.tagStart < s.tagEnd {
		// Removing the tag keeps any anchor written after it.
		tries = append(tries, props{start: s.tagStart, prefix: b[s.tagEnd:s.start]})
	}
	var middle []byte
	if s.style == styleBlock {
		middle = b[s.headerEnd:s.lineEnd]
	}
	for _, t := range tries {
		lead := append([]byte(nil), t.prefix...)
		if s.style == styleEmpty {
			// scanSpan guarantees an empty span is preceded by an indicator or a token.
			junction := b[t.start-1]
			if len(lead) > 0 {
				junction = lead[len(lead)-1]
			}
			if !isBlank(junction) {
				// Without the space, `key:v` would read as one plain scalar.
				lead = append(lead, ' ')
			}
		}
		for _, c := range d.candidates(s, pt.ctx, value, t.start == s.start && s.tagStart < s.tagEnd) {
			head := append(append([]byte(nil), lead...), c.head...)
			check := append(append([]byte(nil), b[s.node:t.start]...), blockText(head, c.content, c.block)...)
			got, ok := d.decodeAt(check, s.end, pt.ctx)
			if !ok {
				continue
			}
			if v, err := decodeValue(got); err != nil || !equalValue(v, value) {
				continue
			}
			text := append(head, middle...)
			if c.block {
				text = append(append(text, d.eol...), c.content...)
			}
			pt.start, pt.end, pt.text = t.start, s.end, text
			return nil
		}
	}
	return fmt.Errorf("%w: no style writes %#v here", ErrUnsupportedScalar, value)
}

// candidates lists the texts that may write value over s, best first: the original style where it
// can hold the value, then yaml.v3's choice. Non-strings are always written plain. tagged is set
// when the text follows a kept tag.
func (d *Document) candidates(s valueSpan, c leafCtx, value any, tagged bool) []candidate {
	str, isString := value.(string)
	if !isString {
		return []candidate{{head: []byte(marshalScalar(value))}}
	}
	double := candidate{head: []byte(strconv.Quote(str))}
	switch s.style {
	case styleDouble:
		return []candidate{double}
	case styleSingle:
		return []candidate{{head: []byte(singleQuoted(str))}, double}
	case styleBlock:
		return []candidate{d.blockCandidate(s, c, str), double}
	}
	var out []candidate
	enc := marshalScalar(str)
	// yaml.v3 quotes YAML 1.1 bools such as `yes` and `y`, which Helm's parser still reads as
	// bools, so plain is taken only where it agrees; a kept tag fixes the type itself. The decode
	// check refuses plain text that does not fit the context, e.g. `a,b` in a flow sequence.
	if enc == str || tagged {
		out = append(out, candidate{head: []byte(str)})
	}
	if strings.HasPrefix(enc, string(singleQuote)) {
		out = append(out, candidate{head: []byte(singleQuoted(str))})
	}
	return append(out, double)
}

// singleQuoted writes s in single quotes; a string that cannot be written that way fails the
// decode check and falls through to double quotes.
func singleQuoted(s string) string {
	q := string(singleQuote)
	return q + strings.ReplaceAll(s, q, q+q) + q
}

// blockCandidate writes str as a block scalar with the source's indicator, indentation indicator
// and content indentation. The chomping indicator is kept when it can hold the string's trailing
// line breaks, otherwise changed to one that can.
func (d *Document) blockCandidate(s valueSpan, c leafCtx, str string) candidate {
	header := d.body[s.start:s.headerEnd]
	body := strings.TrimRight(str, lineFeed)
	trailing := len(str) - len(body)
	chomp := byte(0)
	if i := bytes.IndexAny(header, string([]byte{chompStrip, chompKeep})); i >= 0 {
		chomp = header[i]
	}
	fits := (chomp == chompStrip && trailing == 0) || (chomp == 0 && trailing == 1) || (chomp == chompKeep && trailing > 0)
	if !fits {
		header = bytes.Map(func(r rune) rune {
			if r == chompStrip || r == chompKeep {
				return -1
			}
			return r
		}, header)
		switch {
		case trailing == 0:
			header = append(header, chompStrip)
		case trailing > 1:
			header = append(header, chompKeep)
		}
	}
	indent := strings.Repeat(" ", d.blockIndent(s, c))
	lines := strings.Split(body, lineFeed)
	var out []string
	for i, l := range lines {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+l)
		if d.body[s.start] == foldedHeader && i < len(lines)-1 {
			// A single line break between two text lines folds into a space; a blank line keeps it.
			out = append(out, "")
		}
	}
	content := strings.Join(out, d.eol)
	if trailing > 1 {
		content += strings.Repeat(d.eol, trailing-1)
	}
	return candidate{head: header, content: []byte(content), block: body != "" || trailing > 1}
}

// blockIndent returns the content indentation of the block scalar s: from its indentation
// indicator, else from its first content line, else one default unit past the owner.
func (d *Document) blockIndent(s valueSpan, c leafCtx) int {
	for _, ch := range d.body[s.start+1 : s.headerEnd] {
		if ch >= '1' && ch <= '9' {
			return max(c.owner, 0) + int(ch-'0')
		}
	}
	for off := s.contentStart; off < s.end; off = lineAfter(d.body, off) {
		if text := lineText(d.body, off); len(bytes.TrimSpace(text)) != 0 {
			return leadingSpaces(text)
		}
	}
	return max(c.owner, 0) + defaultIndentUnit
}

// applyPatches returns body with ps applied. Overlapping spans give ErrPatchConflict.
func applyPatches(body []byte, ps []*patch) ([]byte, error) {
	ps = slices.Clone(ps)
	slices.SortFunc(ps, func(a, b *patch) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	var out bytes.Buffer
	last := 0
	for i, p := range ps {
		if i > 0 && (p.start < ps[i-1].end || p.start == ps[i-1].start) {
			return nil, fmt.Errorf("%w: [%d, %d) and [%d, %d)", ErrPatchConflict, ps[i-1].start, ps[i-1].end, p.start, p.end)
		}
		out.Write(body[last:p.start])
		out.Write(p.text)
		last = p.end
	}
	out.Write(body[last:])
	return out.Bytes(), nil
}

// sortedPatches returns the pending patches in path order, for a deterministic apply.
func (d *Document) sortedPatches() []*patch {
	keys := make([]string, 0, len(d.patches))
	for k := range d.patches {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	ps := make([]*patch, len(keys))
	for i, k := range keys {
		ps[i] = d.patches[k]
	}
	return ps
}

// verify re-parses out and compares its values with the edited in-memory tree.
func (d *Document) verify(out []byte) error {
	again, err := Parse(out)
	if err != nil {
		return fmt.Errorf("%w: output does not parse: %v", ErrPatchVerification, err)
	}
	got, err := decodeValue(again.root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPatchVerification, err)
	}
	want, err := decodeValue(d.root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPatchVerification, err)
	}
	if !equalValue(got, want) {
		return fmt.Errorf("%w: written values differ from the edited tree", ErrPatchVerification)
	}
	return nil
}

// equalValue compares decoded values as Helm sees them: numbers by value whatever their Go type
// (NaN equals NaN), maps and lists element-wise; a string never equals a number, bool or null.
func equalValue(a, b any) bool {
	if x, ok := numberOf(a); ok {
		y, ok := numberOf(b)
		return ok && x.equal(y)
	}
	switch a := a.(type) {
	case map[string]any:
		b, ok := b.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if w, ok := b[k]; !ok || !equalValue(v, w) {
				return false
			}
		}
		return true
	case map[any]any:
		b, ok := b.(map[any]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if w, ok := b[k]; !ok || !equalValue(v, w) {
				return false
			}
		}
		return true
	case []any:
		b, ok := b.([]any)
		return ok && slices.EqualFunc(a, b, equalValue)
	}
	return reflect.DeepEqual(a, b)
}

// number is an integer (i set) or a float.
type number struct {
	i *big.Int
	f float64
}

func numberOf(v any) (number, bool) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return number{i: big.NewInt(rv.Int())}, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return number{i: new(big.Int).SetUint64(rv.Uint())}, true
	case reflect.Float32, reflect.Float64:
		return number{f: rv.Float()}, true
	}
	return number{}, false
}

func (n number) equal(m number) bool {
	if n.i != nil && m.i != nil {
		return n.i.Cmp(m.i) == 0
	}
	x, y := n.float(), m.float()
	return x == y || (math.IsNaN(x) && math.IsNaN(y))
}

func (n number) float() float64 {
	if n.i == nil {
		return n.f
	}
	f, _ := new(big.Float).SetInt(n.i).Float64()
	return f
}
