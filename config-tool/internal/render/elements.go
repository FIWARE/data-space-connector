package render

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Origins of a leaf in a list the render writes keyed elements into but whose element it does not own (ADR 0025).
const (
	// OriginBase marks an element of the list's lists-table base, written because Helm replaces lists.
	OriginBase = "base"
	// OriginLoaded marks a foreign element of the loaded list, written unchanged.
	OriginLoaded = "loaded"
)

// Reasons a mechanism is withheld.
const (
	// WithheldCollision: Element would share the unique Field's Value with the foreign element ForeignKey of List.
	WithheldCollision = "collision"
	// WithheldEdited: the loaded file holds something other than a sequence or null at List, a hand edit.
	WithheldEdited = "edited"
)

// nullTag is the YAML tag of a null scalar, written as `key:`, `key: ~` or `key: null`.
const nullTag = "!!null"

// Withheld is a mechanism none of whose targets rendered because of its element Element in List.
// Mechanism is empty for an element target without one; Field, Value and ForeignKey are set for a collision only.
type Withheld struct {
	Reason     string `json:"reason"`
	Mechanism  string `json:"mechanism"`
	Element    string `json:"element"`
	List       string `json:"list"`
	Field      string `json:"field,omitempty"`
	Value      any    `json:"value,omitempty"`
	ForeignKey string `json:"foreignKey,omitempty"`
}

// group is what renders or is withheld as a unit: a named mechanism, or one element target without one.
type group struct {
	mechanism  string
	foundation string
	target     int
}

// ownedElement is one rendered element target.
type ownedElement struct {
	foundation string
	target     int
	refs       []string
	group      group
	key        string
	value      map[string]any
}

// listState is what the loaded file holds at a lists-table path.
type listState int

const (
	// listAbsent: nothing is loaded there, so the base applies.
	listAbsent listState = iota
	// listLoaded: a sequence whose elements the owned ones merge into.
	listLoaded
	// listEdited: something other than a sequence or null, a hand edit that withholds every element written there.
	listEdited
)

// entry is one element of a composed list; owner is nil for a base or loaded foreign element.
type entry struct {
	value  any
	owner  *ownedElement
	origin string
	// pinned is set on an owned key whose loaded element differs from the rendered one, which value then holds.
	pinned bool
}

// composeLists writes every lists-table list that a ready element target writes into, and returns the
// mechanisms withheld for an edited list or a unique collision with a foreign element. A mechanism with an unready
// target renders nothing either, so no half-written mechanism is left behind.
func composeLists(b model.Bundle, r resolution, opts Options) ([]Leaf, []Withheld, error) {
	table := b.ListTable()
	owned := make([][]*ownedElement, len(table))
	unready := map[group]bool{}
	for _, decl := range b.Foundations {
		f, _ := b.Foundation(decl.ID)
		for i, t := range f.Targets {
			if t.Element == nil {
				continue
			}
			g := group{mechanism: t.Mechanism}
			if g.mechanism == "" {
				g = group{foundation: f.ID, target: i}
			}
			if !all(r.answers, t.Refs) || !all(r.answers, t.Requires) {
				unready[g] = true
				continue
			}
			v, err := t.Element.Expand(func(tpl *template.Template) (string, error) { return execute(tpl, r.ns) })
			if err != nil {
				return nil, nil, fmt.Errorf("render: %s targets[%d]: %w", f.ID, i, err)
			}
			k := slices.IndexFunc(table, func(l model.CompiledList) bool { return slices.Equal(l.Path, t.Path) })
			owned[k] = append(owned[k], &ownedElement{foundation: f.ID, target: i, refs: t.Refs, group: g, key: t.Element.Key, value: v})
		}
	}
	states := make([]listState, len(table))
	loaded := make([][]any, len(table))
	for k, l := range table {
		if len(owned[k]) > 0 && opts.Overrides != nil {
			states[k], loaded[k] = loadedList(opts.Overrides, l.Path)
		}
	}
	withheld := map[group]Withheld{}
	active := func(k int) []*ownedElement {
		return slices.DeleteFunc(slices.Clone(owned[k]), func(o *ownedElement) bool {
			_, w := withheld[o.group]
			return unready[o.group] || w
		})
	}
	for k, l := range table {
		if states[k] != listEdited {
			continue
		}
		for _, o := range owned[k] {
			if _, ok := withheld[o.group]; !ok && !unready[o.group] {
				withheld[o.group] = Withheld{Reason: WithheldEdited, Mechanism: o.group.mechanism, Element: o.key, List: l.Path.String()}
			}
		}
	}
	// One pass suffices: withholding restores a loaded owned element as foreign, but an unpinned one equals its
	// rendered value, whose literal unique values no other owned element shares (model.checkElements), and a
	// pinned one already counts as foreign.
	for k, l := range table {
		for _, w := range collisions(l, compose(l, states[k], loaded[k], active(k))) {
			if _, ok := withheld[w.group]; !ok {
				withheld[w.group] = w.Withheld
			}
		}
	}
	var leaves []Leaf
	for k, l := range table {
		act := active(k)
		if len(act) == 0 {
			continue
		}
		for i, e := range compose(l, states[k], loaded[k], act) {
			leaves = append(leaves, entryLeaves(l, i, e, act[0].foundation)...)
		}
	}
	return leaves, sortedWithheld(withheld), nil
}

// loadedList reads the loaded file at the chart-relative list path p. A null there is an empty list, not an absent
// one: Helm deletes a key set to null, chart default included, so writing the base would revive what the user removed.
func loadedList(o *load.Result, p roundtrip.Path) (listState, []any) {
	n, err := o.Doc.Lookup(slices.Concat(o.Root, p))
	if errors.Is(err, roundtrip.ErrNotFound) {
		return listAbsent, nil
	}
	if err == nil && n.Kind == yaml.ScalarNode && n.Tag == nullTag {
		return listLoaded, nil
	}
	if err != nil || n.Kind != yaml.SequenceNode {
		return listEdited, nil
	}
	var elems []any
	if err := n.Decode(&elems); err != nil {
		return listEdited, nil
	}
	return listLoaded, elems
}

// compose is the list written for l: the loaded elements with owned keys replaced in place and the other owned
// elements appended, or, when nothing is loaded there, the base followed by the owned elements in bundle order.
func compose(l model.CompiledList, state listState, loaded []any, owned []*ownedElement) []entry {
	var out []entry
	placed := map[string]bool{}
	if state == listLoaded {
		for _, e := range loaded {
			key, _ := l.KeyOf(e)
			i := slices.IndexFunc(owned, func(o *ownedElement) bool { return o.key == key })
			if i < 0 {
				out = append(out, entry{value: e, origin: OriginLoaded})
				continue
			}
			o := owned[i]
			placed[o.key] = true
			if reflect.DeepEqual(e, any(o.value)) {
				out = append(out, entry{value: o.value, owner: o})
			} else {
				out = append(out, entry{value: e, owner: o, pinned: true})
			}
		}
	} else {
		for _, e := range l.Base {
			out = append(out, entry{value: e, origin: OriginBase})
		}
	}
	for _, o := range owned {
		if !placed[o.key] {
			out = append(out, entry{value: o.value, owner: o})
		}
	}
	return out
}

// collision is a Withheld with the group it withholds.
type collision struct {
	Withheld
	group group
}

// collisions lists every owned element that shares a unique field's value with a foreign one; a pinned owned
// element holds the user's content, so it counts as foreign.
func collisions(l model.CompiledList, entries []entry) []collision {
	var foreign []any
	for _, e := range entries {
		if e.owner == nil || e.pinned {
			foreign = append(foreign, e.value)
		}
	}
	var out []collision
	for _, e := range entries {
		if e.owner == nil || e.pinned {
			continue
		}
		u, i, ok := l.Collision(e.owner.value, foreign)
		if !ok {
			continue
		}
		key, _ := l.KeyOf(foreign[i])
		out = append(out, collision{group: e.owner.group, Withheld: Withheld{
			Reason: WithheldCollision, Mechanism: e.owner.group.mechanism, Element: e.owner.key, List: l.Path.String(), Field: u, Value: e.owner.value[u], ForeignKey: key,
		}})
	}
	return out
}

// entryLeaves flattens the i-th element of l into leaves. A base or loaded foreign element is attributed to owner,
// the foundation whose element makes the list be written, so a diff groups it there. On a pinned owned element
// Rendered is the whole rendered element.
func entryLeaves(l model.CompiledList, i int, e entry, owner string) []Leaf {
	key, _ := l.KeyOf(e.value)
	prov := Provenance{Foundation: owner, Answers: []string{}, Element: key, Origin: e.origin}
	if e.owner != nil {
		prov = Provenance{Foundation: e.owner.foundation, Answers: append([]string{}, e.owner.refs...), Target: e.owner.target, Element: key}
		if e.pinned {
			prov.Pinned, prov.Kept, prov.Rendered = true, true, e.owner.value
		}
	}
	var leaves []Leaf
	flattenElement(append(slices.Clone(l.Path), roundtrip.Index(i)), e.value, func(p roundtrip.Path, v any) {
		leaves = append(leaves, Leaf{Path: p, Value: v, Provenance: prov})
	})
	return leaves
}

// flattenElement calls leaf for every scalar and every empty collection below v, at its path under p.
func flattenElement(p roundtrip.Path, v any, leaf func(roundtrip.Path, any)) {
	switch v := v.(type) {
	case map[string]any:
		if len(v) == 0 {
			leaf(p, map[string]any{})
		}
		for k, c := range v {
			flattenElement(append(slices.Clone(p), roundtrip.Key(k)), c, leaf)
		}
	case []any:
		if len(v) == 0 {
			leaf(p, []any{})
		}
		for i, c := range v {
			flattenElement(append(slices.Clone(p), roundtrip.Index(i)), c, leaf)
		}
	default:
		leaf(p, v)
	}
}

func sortedWithheld(ws map[group]Withheld) []Withheld {
	var out []Withheld
	for _, w := range ws {
		out = append(out, w)
	}
	slices.SortFunc(out, func(a, b Withheld) int {
		return cmp.Or(cmp.Compare(a.List, b.List), cmp.Compare(a.Mechanism, b.Mechanism), cmp.Compare(a.Element, b.Element))
	})
	return out
}

// eachLeaves writes every ready each target into the elements of the loaded list that hold its Within mapping;
// nothing without a loaded file. A loaded value that differs from the rendered one pins the leaf as a hand edit.
func eachLeaves(b model.Bundle, r resolution, opts Options) ([]Leaf, error) {
	o := opts.Overrides
	if o == nil {
		return nil, nil
	}
	var leaves []Leaf
	for _, decl := range b.Foundations {
		f, _ := b.Foundation(decl.ID)
		for i, t := range f.Targets {
			if t.Each == nil || !all(r.answers, t.Refs) || !all(r.answers, t.Requires) {
				continue
			}
			n, err := o.Doc.Lookup(slices.Concat(o.Root, t.Path))
			if err != nil || n.Kind != yaml.SequenceNode {
				continue
			}
			v, err := evaluate(t, r.answers, r.ns)
			if err != nil {
				return nil, fmt.Errorf("render: %s targets[%d]: %w", f.ID, i, err)
			}
			for j := range n.Content {
				elem := slices.Concat(t.Path, roundtrip.Path{roundtrip.Index(j)})
				p := slices.Concat(elem, t.Each)
				if within, err := o.Doc.Lookup(slices.Concat(o.Root, elem, t.Within)); err != nil || within.Kind != yaml.MappingNode {
					continue
				}
				l := Leaf{Path: p, Value: v, Provenance: Provenance{Foundation: f.ID, Answers: append([]string{}, t.Refs...), Fn: string(t.Fn), Target: i}}
				at := slices.Concat(o.Root, p)
				if _, err := o.Doc.Lookup(at); err == nil {
					if kv, ok := kept(o.Doc, at); !ok || !reflect.DeepEqual(kv, v) {
						l.Value, l.Provenance.Pinned, l.Provenance.Kept, l.Provenance.Rendered = kv, true, ok, v
					}
				}
				leaves = append(leaves, l)
			}
		}
	}
	return leaves, nil
}
