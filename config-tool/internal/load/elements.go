package load

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	noteNotInElement = "not a field of the %s element %s writes"
	noteNotRendered  = "is not what %s writes here"
	noteMissing      = "the element lacks %s, which %s writes"
	noteInactive     = "%s is not answered, so %s does not write here"
	noteCollision    = "%s is withheld: %s takes %s %v in %s"
	noteEdited       = "%s is withheld: %s is not a list"
	noteConflict     = "%s is withheld: the chart writes its own while %s are set"
)

// elementRule is a loaded node an owned element or each target writes (ADR 0025): a list element whose key field
// matches the target's key, or the each leaf below one loaded element. At is absolute.
type elementRule struct {
	at         roundtrip.Path
	foundation string
	target     model.CompiledTarget
	// missing is the note on an element that lacks a field the target writes; it lands on the key leaf.
	missing string
	// inactive is the note on a rule whose target the render leaves out for an unresolved answer; it lands on the
	// key leaf, or on the each leaf.
	inactive string
}

// elementRules lists the loaded nodes b's element and each targets write, in bundle order.
func elementRules(doc *roundtrip.Document, b model.Bundle, root roundtrip.Path) []elementRule {
	var rules []elementRule
	for _, f := range b.CompiledFoundations() {
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
				if within, err := doc.Lookup(slices.Concat(elem, t.Within)); err == nil && within.Kind == yaml.MappingNode &&
					!EachBlocked(doc, elem, t) {
					rules = append(rules, elementRule{at: slices.Concat(elem, t.Each), foundation: f.ID, target: t})
				}
			}
		}
	}
	return rules
}

// EachBlocked reports whether a loaded node strictly between t.Within and the each leaf below elem is not a
// mapping, so writing the leaf would replace a value the tool does not own. A missing node does not block.
func EachBlocked(doc *roundtrip.Document, elem roundtrip.Path, t model.CompiledTarget) bool {
	for k := len(t.Within) + 1; k < len(t.Each); k++ {
		n, err := doc.Lookup(slices.Concat(elem, t.Each[:k]))
		if errors.Is(err, roundtrip.ErrNotFound) {
			return false
		}
		if err != nil || n.Kind != yaml.MappingNode {
			return true
		}
	}
	return false
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

// deactivate marks every active rule whose target the render would leave out under answers, as composeLists and
// eachLeaves do: a ref or requires that does not resolve, also on another target of its mechanism. It reports
// whether it marked one; the load then classifies again, as the marked rules' answers no longer count.
func deactivate(rules []elementRule, b model.Bundle, answers map[string]*Answer) bool {
	resolved := resolvable(b, answers)
	changed := false
	for i, r := range rules {
		if r.inactive != "" {
			continue
		}
		id, ok := unresolved(b, r.target, resolved)
		if !ok {
			continue
		}
		rules[i].inactive, changed = fmt.Sprintf(noteInactive, id, r.foundation), true
	}
	return changed
}

// unresolved returns the first answer id t or a target of its mechanism needs that is not in resolved.
func unresolved(b model.Bundle, t model.CompiledTarget, resolved map[string]any) (string, bool) {
	targets := []model.CompiledTarget{t}
	if t.Mechanism != "" {
		targets = nil
		for _, f := range b.CompiledFoundations() {
			for _, o := range f.Targets {
				if o.Element != nil && o.Mechanism == t.Mechanism {
					targets = append(targets, o)
				}
			}
		}
	}
	for _, o := range targets {
		for _, id := range slices.Concat(o.Refs, o.Requires) {
			if _, ok := resolved[id]; !ok {
				return id, true
			}
		}
	}
	return "", false
}

// withhold marks the rules of every group composeLists withholds and deactivate does not catch: a mechanism whose
// model.CompiledMechanism.Conflict holds, and one with an element in a loaded list that is not a sequence, or whose
// unpinned element shares a unique field's value with a foreign one (model.CompiledList.Collision). It reports
// whether it marked one; the load then classifies again.
func withhold(doc *roundtrip.Document, root roundtrip.Path, rules []elementRule, b model.Bundle, c *classifier) bool {
	resolved := resolvable(b, c.answers)
	off := map[string]bool{}
	for _, r := range rules {
		if r.inactive != "" && r.target.Element != nil {
			off[groupOf(r.foundation, r.target)] = true
		}
	}
	notes := map[string]string{}
	withheld := func(foundation string, t model.CompiledTarget, format string, args ...any) {
		g := groupOf(foundation, t)
		if _, ok := notes[g]; !ok {
			notes[g] = fmt.Sprintf(format, append([]any{cmp.Or(t.Mechanism, foundation)}, args...)...)
		}
	}
	for _, m := range b.Mechanisms() {
		paths, ok := m.Conflict(doc, root)
		if !ok || off[m.ID] {
			continue
		}
		for _, f := range b.CompiledFoundations() {
			for _, t := range f.Targets {
				if _, unready := unresolved(b, t, resolved); t.Mechanism == m.ID && !unready {
					withheld(f.ID, t, noteConflict, strings.Join(paths, ", "))
				}
			}
		}
	}
	for _, l := range b.ListTable() {
		var owned []elementRule
		for _, f := range b.CompiledFoundations() {
			for _, t := range f.Targets {
				if t.Element == nil || !slices.Equal(t.Path, l.Path) || off[groupOf(f.ID, t)] {
					continue
				}
				if _, ok := unresolved(b, t, resolved); !ok {
					owned = append(owned, elementRule{foundation: f.ID, target: t})
				}
			}
		}
		at := slices.Concat(root, l.Path)
		n, err := doc.Lookup(at)
		if len(owned) == 0 || errors.Is(err, roundtrip.ErrNotFound) ||
			(err == nil && n.Kind == yaml.ScalarNode && n.ShortTag() == nullTag) {
			continue
		}
		var elems []any
		if err != nil || n.Kind != yaml.SequenceNode || n.Decode(&elems) != nil {
			for _, o := range owned {
				withheld(o.foundation, o.target, noteEdited, l.Path)
			}
			continue
		}
		// As compose does: a loaded element under an owned key is the owned one unless pinned, then it is foreign;
		// an owned key the list lacks is appended.
		var foreign []any
		checked := map[string]bool{}
		loaded := map[string]bool{}
		for j, e := range elems {
			key, _ := l.KeyOf(e)
			if !slices.ContainsFunc(owned, func(o elementRule) bool { return o.target.Element.Key == key }) {
				foreign = append(foreign, e)
				continue
			}
			loaded[key] = true
			if c.elementPinned[append(slices.Clone(at), roundtrip.Index(j)).String()] {
				foreign = append(foreign, e)
			} else {
				checked[key] = true
			}
		}
		for _, o := range owned {
			key := o.target.Element.Key
			if loaded[key] && !checked[key] {
				continue
			}
			unique := map[string]any{}
			for _, u := range l.Unique {
				if v, ok := o.target.Element.At(roundtrip.Path{roundtrip.Key(u)}); ok {
					unique[u] = v
				}
			}
			if u, i, ok := l.Collision(unique, foreign); ok {
				other, _ := l.KeyOf(foreign[i])
				withheld(o.foundation, o.target, noteCollision, other, u, unique[u], l.Path)
			}
		}
	}
	changed := false
	for i, r := range rules {
		if r.target.Element == nil || r.inactive != "" {
			continue
		}
		if note, ok := notes[groupOf(r.foundation, r.target)]; ok {
			rules[i].inactive, changed = note, true
		}
	}
	return changed
}

// groupOf identifies what renders or is withheld as a unit, as the render's group does: a mechanism, or one element
// target without one.
func groupOf(foundation string, t model.CompiledTarget) string {
	if t.Mechanism != "" {
		return t.Mechanism
	}
	return foundation + " " + t.Path.String() + "[" + t.Element.Key + "]"
}

// resolvable mirrors the render's resolve over what the seeded profile will hold: the foundations-stage answers,
// plus every default whose refs resolve. A default's value is unknown here (nil), and a reserved ref counts as
// resolved, as the load does not know the Stage 1 target (ADR 0025).
func resolvable(b model.Bundle, answers map[string]*Answer) map[string]any {
	out := map[string]any{}
	for _, a := range answers {
		if a.Stage == string(model.StageFoundations) {
			out[a.ID] = a.Value
		}
	}
	// A default ref that no foundation declares is a reserved one.
	unset := func(r string) bool {
		_, answered := out[r]
		_, declared := b.Foundation(r)
		return !answered && declared
	}
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		if _, ok := out[id]; ok || f.DefaultTemplate() == nil {
			continue
		}
		if !slices.ContainsFunc(f.DefaultRefs(), unset) &&
			(f.DefaultWhen == nil || f.DefaultWhen.Holds(out)) {
			out[id] = nil
		}
	}
	return out
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
	onKey := r.target.Each != nil || isKeyField(sub, r.target.Element.KeyField)
	texts, note := c.readElement(r, sub, n)
	if note == "" && r.missing != "" && r.target.Element != nil && onKey {
		note = r.missing
	}
	if note != "" {
		c.notes[key] = note
		c.elementPinned[r.at.String()] = true
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
	// A leaf that matches still is not the render's while its rule is inactive; a content note above is more specific.
	if r.inactive != "" {
		if onKey {
			c.notes[key] = r.inactive
		}
		return false
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
		// The render compares typed values, so an unquoted 123 is not the string "123" a template writes.
		if n.ShortTag() != c.renderedTag(t) {
			return nil, notRendered
		}
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

// renderedTag is the YAML tag of what t's templates render: a string, except an each copy of a bool answer.
func (c *classifier) renderedTag(t model.CompiledTarget) string {
	if t.Element == nil && t.Fn == model.FnCopy {
		if f, ok := c.foundation(t.Refs[0]); ok && f.Type == model.TypeBool {
			return boolTag
		}
	}
	return strTag
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
