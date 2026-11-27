package model

import (
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// anyText matches what one action rendered; lazy, so a later action or the anchored end decides where it stops.
const anyText = `(.*?)`

// At returns the compiled node at sub inside the element: a *template.Template, a literal scalar or a collection.
// The key field reads as its literal key. false when the element writes nothing there.
func (e CompiledElement) At(sub roundtrip.Path) (any, bool) {
	if len(sub) == 1 {
		if k, _ := sub[0].Key(); k == e.KeyField {
			return e.Key, true
		}
	}
	var v any = e.value
	for _, s := range sub {
		if k, ok := s.Key(); ok {
			m, isMap := v.(map[string]any)
			if v, ok = m[k]; !isMap || !ok {
				return nil, false
			}
			continue
		}
		i, _ := s.Index()
		l, isList := v.([]any)
		if !isList || i >= len(l) {
			return nil, false
		}
		v = l[i]
	}
	return v, true
}

// Leaves returns the paths inside the element of every scalar and empty collection its value writes, sorted; the key
// field is not among them.
func (e CompiledElement) Leaves() []roundtrip.Path {
	var out []roundtrip.Path
	var walk func(roundtrip.Path, any)
	walk = func(at roundtrip.Path, v any) {
		switch v := v.(type) {
		case map[string]any:
			if len(v) == 0 && len(at) > 0 {
				out = append(out, at)
			}
			for k, c := range v {
				walk(append(slices.Clone(at), roundtrip.Key(k)), c)
			}
		case []any:
			if len(v) == 0 {
				out = append(out, at)
			}
			for i, c := range v {
				walk(append(slices.Clone(at), roundtrip.Index(i)), c)
			}
		default:
			out = append(out, at)
		}
	}
	walk(nil, e.value)
	slices.SortFunc(out, func(a, b roundtrip.Path) int { return strings.Compare(a.String(), b.String()) })
	return out
}

// Invert reads back the answers that tpls, joined by sep, rendered as s, one string per answer id; false when s is
// not such a rendering or an id would take two values. A fullname action matches any text and answers nothing.
func Invert(tpls []*template.Template, sep, s string) (map[string]string, bool) {
	var pattern strings.Builder
	pattern.WriteString(`(?s)^`)
	var ids []string
	for i, t := range tpls {
		if i > 0 {
			pattern.WriteString(regexp.QuoteMeta(sep))
		}
		for _, n := range t.Root.Nodes {
			switch n := n.(type) {
			case *parse.TextNode:
				pattern.WriteString(regexp.QuoteMeta(string(n.Text)))
			case *parse.ActionNode:
				pattern.WriteString(anyText)
				id := ""
				if f := fieldOf(n); f != nil {
					id = strings.Join(f.Ident, ".")
				}
				ids = append(ids, id)
			}
		}
	}
	pattern.WriteString(`$`)
	m := regexp.MustCompile(pattern.String()).FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	out := map[string]string{}
	for i, id := range ids {
		if id == "" {
			continue
		}
		if v, ok := out[id]; ok && v != m[i+1] {
			return nil, false
		}
		out[id] = m[i+1]
	}
	return out, true
}
