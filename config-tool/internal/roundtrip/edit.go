package roundtrip

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
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
	// nullText is the canonical text of null.
	nullText = "null"
	// defaultIndentUnit indents a block scalar's content past its owner when the source shows none.
	defaultIndentUnit = 2
)

// patch is a concrete edit of the body bytes [start, end).
type patch struct {
	start, end int
	text       []byte
	// insert marks an insert group's zero-length patch: at equal offsets it follows replacements,
	// and of two groups the one at the deeper column col goes first.
	insert bool
	col    int
	// span and ctx describe a scalar's source text, so a later Set of the same path re-renders it.
	span valueSpan
	ctx  leafCtx
}

// replacement is a pending new value for one source node; Write renders it from the current tree.
type replacement struct {
	path Path
	// cur is the node in root, edited in place so that aliases to it see the new value; src is
	// the same node in the source tree.
	cur, src *yaml.Node
	// key is the source key node; nil for a sequence item or the root.
	key  *yaml.Node
	item bool
	ctx  leafCtx
	// scalar is set while source and new value are both scalars: leaf is then the in-place text
	// edit. Otherwise span places the new value around the key line.
	scalar bool
	leaf   *patch
	span   *replacedSpan
}

// candidate is one way to write a new value: an inline text, or a block scalar's header and
// content lines.
type candidate struct {
	head    []byte
	content []byte
	block   bool
}

// Set writes value at p: a string, bool, int, int64, float64 or nil, or a map[string]any or []any
// of them. A missing key is inserted, with any missing parent mappings; no sequence item is ever
// created. A collection, or a value of another kind, replaces the old value's text, and comments
// inside the old value's block lines go with it; the key line's anchor and line comment stay.
// A value equal to the current one adds no edit. Edits are rendered at Write.
// Errors: ErrNotFound, ErrNotMapping, ErrAliasPath, ErrUnsupportedValue, ErrUnsupportedScalar.
func (d *Document) Set(p Path, value any) error {
	if err := checkValue(value); err != nil {
		return err
	}
	if d.src == nil {
		d.src = cloneTree(d.root, map[*yaml.Node]*yaml.Node{})
	}
	if isEmptyDoc(d.root) {
		if len(p) == 0 {
			return fmt.Errorf("%w: empty document", ErrNotFound)
		}
		return d.insert(p, 0, nil, value)
	}
	nodes, err := d.resolve(p)
	if err != nil {
		return err
	}
	if i := len(nodes) - 1; i < len(p) {
		return d.insert(p, i, nodes[i], value)
	}
	return d.replace(p, nodes[len(p)], value)
}

// resolve walks p in the current tree and returns the nodes of its longest existing prefix, the
// top node first. A path through an alias or onto a merge key gives ErrAliasPath.
func (d *Document) resolve(p Path) ([]*yaml.Node, error) {
	n := d.root.Content[0]
	nodes := []*yaml.Node{n}
	for i, seg := range p {
		if n.Kind == yaml.AliasNode {
			return nil, fmt.Errorf("%w: %s passes through alias *%s at %s", ErrAliasPath, p, n.Value, p[:i])
		}
		key, next := child(n, seg)
		if key != nil && key.Tag == mergeTag {
			return nil, fmt.Errorf("%w: %s sets a merge key's value", ErrAliasPath, p)
		}
		if next == nil {
			break
		}
		nodes = append(nodes, next)
		n = next
	}
	return nodes, nil
}

// replace sets the existing node cur at p to value.
func (d *Document) replace(p Path, cur *yaml.Node, value any) error {
	if v, err := decodeValue(cur); err == nil && equalValue(v, value) {
		return nil
	}
	r, exact, inGroup := d.owner(p)
	if inGroup || (r != nil && !exact) {
		setNode(cur, value)
		return nil
	}
	if r == nil {
		var err error
		if r, err = d.newReplacement(p, cur); err != nil {
			return err
		}
	}
	var err error
	if isScalarValue(value) && (r.src.Kind == yaml.ScalarNode || r.src.Kind == yaml.AliasNode) {
		err = d.prepareScalar(r, value)
	} else {
		err = d.prepareCollection(r)
	}
	if err != nil {
		return err
	}
	d.supersede(p)
	if d.reps == nil {
		d.reps = map[string]*replacement{}
	}
	d.reps[p.String()] = r
	setNode(cur, value)
	return nil
}

// newReplacement starts a replacement of the source node at p, whose current node is cur.
func (d *Document) newReplacement(p Path, cur *yaml.Node) (*replacement, error) {
	loc, err := d.locateIn(d.src, p)
	if err != nil {
		return nil, err
	}
	r := &replacement{path: slices.Clone(p), cur: cur, src: loc.node, key: loc.key, ctx: loc.ctx}
	if len(p) > 0 {
		_, r.item = p[len(p)-1].Index()
	}
	return r, nil
}

// prepareScalar renders value over r's source scalar text in place.
func (d *Document) prepareScalar(r *replacement, value any) error {
	if r.leaf == nil {
		s, err := d.scanSpan(r.src, r.ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", r.path, err)
		}
		r.leaf = &patch{span: s, ctx: r.ctx}
	}
	if err := d.render(r.leaf, value); err != nil {
		return fmt.Errorf("%s: %w", r.path, err)
	}
	r.scalar = true
	return nil
}

// prepareCollection switches r to rendering around the key line, whose span it locates once.
func (d *Document) prepareCollection(r *replacement) error {
	if r.span == nil {
		s, err := d.replacedSpan(r)
		if err != nil {
			return fmt.Errorf("%s: %w", r.path, err)
		}
		r.span = s
	}
	r.scalar = false
	return nil
}

// owner finds the pending edit p lies in: a replacement at or above p (exact when at p), or a key
// inserted at or above p (inGroup). Everything below either is edited in the current tree only.
func (d *Document) owner(p Path) (r *replacement, exact, inGroup bool) {
	for j := 0; j <= len(p); j++ {
		q := p[:j].String()
		if r := d.reps[q]; r != nil {
			return r, j == len(p), false
		}
		if j == len(p) {
			break
		}
		if k, ok := p[j].Key(); ok && d.groups[q] != nil && d.groups[q].keys[k] {
			return nil, false, true
		}
	}
	return nil, false, false
}

// supersede drops the pending edits a new value at p replaces: replacements strictly below p and
// insert groups at or below it.
func (d *Document) supersede(p Path) {
	for k, r := range d.reps {
		if len(r.path) > len(p) && hasPrefix(r.path, p) {
			delete(d.reps, k)
		}
	}
	for k, g := range d.groups {
		if hasPrefix(g.parent, p) {
			delete(d.groups, k)
		}
	}
}

func hasPrefix(p, q Path) bool {
	return len(p) >= len(q) && slices.Equal(p[:len(q)], q)
}

// pendingPatches renders every pending edit from the current tree.
func (d *Document) pendingPatches() ([]*patch, error) {
	var ps []*patch
	for _, k := range slices.Sorted(maps.Keys(d.reps)) {
		rp, err := d.renderReplacement(d.reps[k])
		if err != nil {
			return nil, err
		}
		ps = append(ps, rp...)
	}
	for _, k := range slices.Sorted(maps.Keys(d.groups)) {
		gp, err := d.renderGroup(d.groups[k])
		if err != nil {
			return nil, err
		}
		ps = append(ps, gp)
	}
	if !slices.ContainsFunc(ps, func(p *patch) bool { return p.insert && p.start == len(d.body) }) {
		return ps, nil
	}
	cp := d.eofChompPatch()
	if cp == nil || slices.ContainsFunc(ps, func(p *patch) bool { return !p.insert && p.start <= cp.end && cp.start <= p.end }) {
		return ps, nil
	}
	return append(ps, cp), nil
}

// scalarFor returns the tag and canonical text of a Go value; the tag comes from the Go type, so
// the self-check catches text written with the wrong type.
func scalarFor(v any) (string, string, error) {
	switch v := v.(type) {
	case nil:
		return nullTag, nullText, nil
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

// applyPatches returns body with ps applied. Overlapping spans, and two replacements starting at
// one offset, give ErrPatchConflict.
func applyPatches(body []byte, ps []*patch) ([]byte, error) {
	ps = slices.Clone(ps)
	slices.SortStableFunc(ps, func(a, b *patch) int {
		switch {
		case a.start != b.start:
			return cmp.Compare(a.start, b.start)
		case a.insert != b.insert && a.insert:
			return 1
		case a.insert != b.insert:
			return -1
		case a.insert:
			return cmp.Compare(b.col, a.col)
		}
		return cmp.Compare(a.end, b.end)
	})
	var out bytes.Buffer
	last := 0
	for i, p := range ps {
		if i > 0 {
			prev := ps[i-1]
			if p.start < prev.end || (!p.insert && !prev.insert && p.start == prev.start) {
				return nil, fmt.Errorf("%w: [%d, %d) and [%d, %d)", ErrPatchConflict, prev.start, prev.end, p.start, p.end)
			}
		}
		out.Write(body[last:p.start])
		out.Write(p.text)
		last = p.end
	}
	out.Write(body[last:])
	return out.Bytes(), nil
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
