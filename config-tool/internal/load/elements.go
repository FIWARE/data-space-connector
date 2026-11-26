package load

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	noteNotInElement = "not a field of the %s element %s writes"
	noteNotRendered  = "is not what %s writes here"
	noteMissing      = "the element lacks %s, which %s writes"
)

// elementRule is a loaded node an owned element or each target writes (ADR 0025): a list element whose key field
// matches the target's key, or the each leaf below one loaded element. At is absolute.
type elementRule struct {
	at         roundtrip.Path
	foundation string
	target     model.CompiledTarget
	// missing is the note on an element that lacks a field the target writes; it lands on the key leaf.
	missing string
}

// elementRules lists the loaded nodes b's element and each targets write, in bundle order.
func elementRules(doc *roundtrip.Document, b model.Bundle, root roundtrip.Path) []elementRule {
	var rules []elementRule
	for _, decl := range b.Foundations {
		f, _ := b.Foundation(decl.ID)
		for _, t := range f.Targets {
			if t.Element == nil && t.Each == nil {
				continue
			}
			list := slices.Concat(root, t.Path)
			n, err := doc.Lookup(list)
			if err != nil || n.Kind != yaml.SequenceNode {
				continue
			}
			for j := range n.Content {
				elem := append(slices.Clone(list), roundtrip.Index(j))
				if t.Element != nil {
					k, err := doc.Lookup(append(slices.Clone(elem), roundtrip.Key(t.Element.KeyField)))
					if err == nil && k.Kind == yaml.ScalarNode && k.ShortTag() == strTag && k.Value == t.Element.Key {
						rules = append(rules, elementRule{at: elem, foundation: f.ID, target: t,
							missing: missingLeaf(doc, elem, t.Element, f.ID)})
					}
					continue
				}
				at := slices.Concat(elem, t.Each)
				if parent, err := doc.Lookup(at[:len(at)-1]); err == nil && parent.Kind == yaml.MappingNode {
					rules = append(rules, elementRule{at: at, foundation: f.ID, target: t})
				}
			}
		}
	}
	return rules
}

// missingLeaf is the note on the loaded element at elem when it lacks a leaf e writes; "" when it has them all.
func missingLeaf(doc *roundtrip.Document, elem roundtrip.Path, e *model.CompiledElement, foundation string) string {
	for _, sub := range e.Leaves() {
		if _, err := doc.Lookup(slices.Concat(elem, sub)); err != nil {
			return fmt.Sprintf(noteMissing, sub, foundation)
		}
	}
	return ""
}

// element returns the rule whose element holds p, or whose each leaf is p, and p's path below it.
func (c *classifier) element(p roundtrip.Path) (elementRule, roundtrip.Path, bool) {
	for _, r := range c.elements {
		if len(p) < len(r.at) || !slices.Equal(p[:len(r.at)], r.at) {
			continue
		}
		if r.target.Each != nil && len(p) != len(r.at) {
			continue
		}
		return r, p[len(r.at):], true
	}
	return elementRule{}, nil, false
}

// deriveElement back-derives the answers r's target rendered as n and reports whether n is that rendering. The
// first loaded element in document order sets an answer; a leaf that disagrees with it gets a note, so it is pinned.
func (c *classifier) deriveElement(p roundtrip.Path, r elementRule, sub roundtrip.Path, n *yaml.Node) bool {
	key := p.String()
	texts, note := c.readElement(r, sub, n)
	if note == "" && r.missing != "" && r.target.Element != nil && isKeyField(sub, r.target.Element.KeyField) {
		note = r.missing
	}
	if note != "" {
		c.notes[key] = note
		return false
	}
	ids := make([]string, 0, len(texts))
	values := map[string]any{}
	for id, s := range texts {
		f, ok := c.foundation(id)
		if !ok {
			// A reserved ref such as target.namespace is not an answer.
			continue
		}
		v := typed(f, s)
		if f.Check(v) != nil {
			c.notes[key] = fmt.Sprintf(noteType, id)
			return false
		}
		if a, set := c.answers[id]; set && a.Value != v {
			c.notes[key] = fmt.Sprintf(noteDiffers, id, a.Value)
			return false
		}
		ids, values[id] = append(ids, id), v
	}
	slices.Sort(ids)
	for _, id := range ids {
		a, set := c.answers[id]
		if !set {
			f, _ := c.foundation(id)
			a = &Answer{ID: id, Stage: string(f.Stage), Value: values[id], From: FromOwned}
			c.answers[id] = a
			c.order = append(c.order, id)
		}
		a.Paths = append(a.Paths, key)
	}
	if len(ids) > 0 {
		c.leafAnswer[key] = ids[0]
	}
	c.elementOwned[key] = true
	return true
}

// readElement matches n against what r's target writes at sub: the texts of the answers it rendered from, by id,
// or the note on why n is not that rendering.
func (c *classifier) readElement(r elementRule, sub roundtrip.Path, n *yaml.Node) (map[string]string, string) {
	t := r.target
	var want any
	if t.Element != nil {
		v, ok := t.Element.At(sub)
		if !ok {
			return nil, fmt.Sprintf(noteNotInElement, t.Element.Key, r.foundation)
		}
		want = v
	} else if t.Fn == model.FnConst {
		want = t.Value
	} else {
		want = t.Templates()
	}
	notRendered := fmt.Sprintf(noteNotRendered, r.foundation)
	switch w := want.(type) {
	case *template.Template:
		want = []*template.Template{w}
	case map[string]any, []any:
		// The walk reaches a collection only as an empty leaf, which matches only an empty one of the same kind.
		if (n.Kind != yaml.MappingNode && n.Kind != yaml.SequenceNode) || len(n.Content) > 0 ||
			reflect.ValueOf(w).Len() > 0 || (n.Kind == yaml.MappingNode) != (reflect.TypeOf(w).Kind() == reflect.Map) {
			return nil, notRendered
		}
		return nil, ""
	}
	if n.Kind != yaml.ScalarNode {
		return nil, notRendered
	}
	if tpls, ok := want.([]*template.Template); ok {
		texts, ok := model.Invert(tpls, t.Sep, n.Value)
		if !ok {
			return nil, notRendered
		}
		return texts, ""
	}
	var got any
	if err := n.Decode(&got); err != nil || !reflect.DeepEqual(got, want) {
		return nil, notRendered
	}
	return nil, ""
}

func isKeyField(sub roundtrip.Path, field string) bool {
	if len(sub) != 1 {
		return false
	}
	k, ok := sub[0].Key()
	return ok && k == field
}

// typed is the answer value of text s as f's type stores it.
func typed(f model.CompiledFoundation, s string) any {
	if f.Type == model.TypeBool {
		if b, err := strconv.ParseBool(s); err == nil {
			return b
		}
	}
	return s
}
