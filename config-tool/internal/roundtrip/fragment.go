package roundtrip

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// mergeKey is written quoted as a plain key, where it would read as a merge key.
	mergeKey = "<<"
	// flowSpecials force a plain string inside a flow collection into double quotes.
	flowSpecials = ",[]{}:#"
)

// layout is the indentation new block lines follow, detected from the source.
type layout struct {
	// unit is the most common positive indent step.
	unit int
	// indentless is set when block sequences put their dashes at the parent key's column.
	indentless bool
}

// layout returns the document's layout, detected once from the source tree.
func (d *Document) layout() layout {
	if d.lay == nil {
		l := d.detectLayout()
		d.lay = &l
	}
	return *d.lay
}

func (d *Document) detectLayout() layout {
	steps := map[int]int{}
	indentless, indented := 0, 0
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode && n.Style&yaml.FlowStyle == 0 {
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if v.Style&yaml.FlowStyle != 0 || len(v.Content) == 0 {
					continue
				}
				switch v.Kind {
				case yaml.MappingNode:
					if step := v.Content[0].Column - k.Column; step > 0 {
						steps[step]++
					}
				case yaml.SequenceNode:
					switch step := d.dashColumn(v) - (k.Column - 1); {
					case step == 0:
						indentless++
					case step > 0:
						indented++
						steps[step]++
					}
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	root := d.src
	if root == nil {
		root = d.root
	}
	walk(root)
	l := layout{unit: defaultIndentUnit, indentless: indentless > indented}
	best := 0
	for step, count := range steps {
		if count > best || (count == best && step < l.unit) {
			l.unit, best = step, count
		}
	}
	return l
}

// blockLines renders the non-empty map or list v as block lines whose entries start at column col.
// tail is set when the lines end the body without a final line break (see scalarLines).
func (d *Document) blockLines(v any, col int, tail bool) []string {
	var out []string
	if items, ok := v.([]any); ok {
		for i, item := range items {
			out = append(out, d.itemLines(item, col, tail && i == len(items)-1)...)
		}
		return out
	}
	es := entries(v)
	for i, e := range es {
		out = append(out, d.pairLines(blockKey(e.key), e.value, col, tail && i == len(es)-1)...)
	}
	return out
}

// pairLines renders `key: v` at column col; a nested collection goes one indent unit deeper, or a
// sequence at col itself in an indentless document.
func (d *Document) pairLines(key string, v any, col int, tail bool) []string {
	head := strings.Repeat(" ", col) + key + ":"
	if !isBlockCollection(v) {
		return d.scalarLines(head, v, col, tail)
	}
	l := d.layout()
	sub := col + l.unit
	if _, ok := v.([]any); ok && l.indentless {
		sub = col
	}
	return append([]string{head}, d.blockLines(v, sub, tail)...)
}

// itemLines renders the sequence item v with its dash at column col; a collection is written
// compact, its first entry on the dash line.
func (d *Document) itemLines(v any, col int, tail bool) []string {
	dash := strings.Repeat(" ", col) + string(sequenceDash)
	if !isBlockCollection(v) {
		return d.scalarLines(dash, v, col, tail)
	}
	lines := d.blockLines(v, col+itemIndent, tail)
	lines[0] = dash + " " + lines[0][col+itemIndent:]
	return lines
}

// scalarLines writes v after head: a multi-line string as a literal block one indent unit past
// col, anything else on the line. With tail set, a literal that owns its final line break is
// written double-quoted instead, as the body has no line break left to end it.
func (d *Document) scalarLines(head string, v any, col int, tail bool) []string {
	if s, ok := v.(string); ok {
		header, content, ok := literalBlock(s, strings.Repeat(" ", col+d.layout().unit))
		if ok && (!tail || header[len(header)-1] == chompStrip) {
			return append([]string{head + " " + header}, content...)
		}
	}
	return []string{head + " " + inlineScalar(v)}
}

// literalBlock writes s as a literal block scalar with content lines at indent, when yaml.v3 would
// choose literal style without an indentation indicator; ok is false otherwise.
func literalBlock(s, indent string) (header string, content []string, ok bool) {
	body := strings.TrimRight(s, lineFeed)
	enc := marshalScalar(s)
	first, _, _ := strings.Cut(enc, lineFeed)
	if body == "" || !strings.Contains(s, lineFeed) || first[0] != literalHeader || strings.ContainsAny(first, "123456789") {
		return "", nil, false
	}
	trailing := len(s) - len(body)
	header = string(literalHeader)
	switch {
	case trailing == 0:
		header += string(chompStrip)
	case trailing > 1:
		header += string(chompKeep)
	}
	for _, l := range strings.Split(body, lineFeed) {
		if l != "" {
			l = indent + l
		}
		content = append(content, l)
	}
	// Keep chomping owns the line breaks after the first as blank lines.
	for range trailing - 1 {
		content = append(content, "")
	}
	return header, content, true
}

// inlineScalar writes a scalar or an empty collection on one line in block context.
func inlineScalar(v any) string {
	switch v := v.(type) {
	case []any:
		return "[]"
	case map[string]any, map[any]any:
		return "{}"
	case string:
		if enc := marshalScalar(v); !strings.Contains(enc, lineFeed) {
			return enc
		}
		return strconv.Quote(v)
	}
	return marshalScalar(v)
}

// flowText writes v in flow style on one line.
func flowText(v any) string {
	var parts []string
	switch v := v.(type) {
	case []any:
		for _, item := range v {
			parts = append(parts, flowText(item))
		}
		return "[" + strings.Join(parts, flowSeparator) + "]"
	case map[string]any, map[any]any:
		for _, e := range entries(v) {
			parts = append(parts, flowKey(e.key)+": "+flowText(e.value))
		}
		return "{" + strings.Join(parts, flowSeparator) + "}"
	case string:
		return flowKey(v)
	}
	return marshalScalar(v)
}

// flowKey writes a scalar for flow context, quoting what would end a plain scalar there.
func flowKey(k any) string {
	s, ok := k.(string)
	if !ok {
		return marshalScalar(k)
	}
	enc := marshalScalar(s)
	if strings.Contains(enc, lineFeed) || (enc == s && strings.ContainsAny(s, flowSpecials)) {
		return strconv.Quote(s)
	}
	return enc
}

// blockKey writes a mapping key for block context.
func blockKey(k any) string {
	s, ok := k.(string)
	if !ok {
		return marshalScalar(k)
	}
	enc := marshalScalar(s)
	if strings.Contains(enc, lineFeed) || enc == mergeKey {
		return strconv.Quote(s)
	}
	return enc
}

// jsonText writes v as indented JSON, valid flow YAML: inner lines one indent unit deeper than
// indent, the closing bracket at indent. ok is false for a value JSON cannot hold.
func (d *Document) jsonText(v any, indent int) (string, bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(strings.Repeat(" ", indent), strings.Repeat(" ", d.layout().unit))
	if err := enc.Encode(v); err != nil {
		return "", false
	}
	return strings.ReplaceAll(strings.TrimSuffix(buf.String(), lineFeed), lineFeed, d.eol), true
}

type entry struct {
	key, value any
}

// entries returns a decoded mapping's pairs sorted by key.
func entries(v any) []entry {
	var out []entry
	switch m := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(m)) {
			out = append(out, entry{k, m[k]})
		}
	case map[any]any:
		for k, sub := range m {
			out = append(out, entry{k, sub})
		}
		slices.SortFunc(out, func(a, b entry) int { return cmp.Compare(fmt.Sprint(a.key), fmt.Sprint(b.key)) })
	}
	return out
}

// isBlockCollection reports whether v is a non-empty map or list.
func isBlockCollection(v any) bool {
	switch v := v.(type) {
	case map[string]any:
		return len(v) > 0
	case map[any]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	}
	return false
}

func isScalarValue(v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any, []any:
		return false
	}
	return true
}
