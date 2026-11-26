package render

import (
	"cmp"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Status says how a leaf changed between two renders.
type Status string

// Change statuses; an unchanged leaf has none and is left out of the diff.
const (
	StatusAdded   Status = "added"
	StatusChanged Status = "changed"
	StatusRemoved Status = "removed"
	// StatusPinned is a hand-edited leaf the render would have changed but keeps as loaded.
	StatusPinned Status = "pinned"
)

// TreeDiff is the change between two trees, grouped by foundation.
type TreeDiff struct {
	Groups []Group `json:"groups"`
}

// Group holds one foundation's changes, sorted by path.
type Group struct {
	Foundation string   `json:"foundation"`
	Changes    []Change `json:"changes"`
}

// Change is one changed leaf. For a pinned leaf Before and After are the rendered values, not the
// written one, which is Kept. HasBefore, HasAfter and HasKept tell an absent value from a nil one.
type Change struct {
	Path      roundtrip.Path
	Status    Status
	Before    any
	HasBefore bool
	After     any
	HasAfter  bool
	Kept      any
	HasKept   bool
	// Element selects a pinned keyed element, as field=key; Path is then its list and the values whole elements.
	Element string
}

// MarshalJSON writes the path in roundtrip.ParsePath syntax and omits the absent values.
func (c Change) MarshalJSON() ([]byte, error) {
	opt := func(v any, ok bool) *any {
		if !ok {
			return nil
		}
		return &v
	}
	return json.Marshal(struct {
		Path   string `json:"path"`
		Status Status `json:"status"`
		Before *any   `json:"before,omitempty"`
		After  *any   `json:"after,omitempty"`
		Kept   *any   `json:"kept,omitempty"`
	}{c.Path.String() + elementSelector(c.Element), c.Status, opt(c.Before, c.HasBefore), opt(c.After, c.HasAfter), opt(c.Kept, c.HasKept)})
}

// Diff compares prev with next. Groups follow b's foundation order; a foundation b does not declare
// (prev rendered against another bundle) comes after them, by id. A pinned leaf is reported only
// when its rendered value changed, so a hand edit is not reported on every render.
func Diff(b model.Bundle, prev, next Tree) TreeDiff {
	before := map[string]Leaf{}
	for _, l := range prev.Leaves {
		before[l.Path.String()] = l
	}
	groups := map[string][]Change{}
	add := func(f string, c Change) { groups[f] = append(groups[f], c) }
	var pinned []*pinnedElement
	for _, l := range next.Leaves {
		key := l.Path.String()
		p, had := before[key]
		delete(before, key)
		if e := pinnedElementOf(b, l, &pinned); e != nil {
			e.kept = insert(e.kept, l.Path[len(e.at):], l.Value)
			continue
		}
		if l.Provenance.Pinned {
			was, wasOK := rendered(p, had)
			if !wasOK || !reflect.DeepEqual(was, l.Provenance.Rendered) {
				add(l.Provenance.Foundation, Change{Path: l.Path, Status: StatusPinned, Before: was, HasBefore: wasOK,
					After: l.Provenance.Rendered, HasAfter: true, Kept: l.Value, HasKept: l.Provenance.Kept})
			}
			continue
		}
		was, wasOK := written(p, had)
		switch {
		case !wasOK:
			add(l.Provenance.Foundation, Change{Path: l.Path, Status: StatusAdded, After: l.Value, HasAfter: true})
		case !reflect.DeepEqual(was, l.Value):
			add(l.Provenance.Foundation, Change{Path: l.Path, Status: StatusChanged, Before: was, HasBefore: true, After: l.Value, HasAfter: true})
		}
	}
	for _, e := range pinned {
		was, wasOK := e.renderedIn(b, prev, before)
		if !wasOK || !reflect.DeepEqual(was, e.rendered) {
			add(e.foundation, Change{Path: e.list.Path, Element: e.list.Key + elementKeySep + e.key, Status: StatusPinned,
				Before: was, HasBefore: wasOK, After: e.rendered, HasAfter: true, Kept: e.kept, HasKept: true})
		}
	}
	for _, p := range before {
		was, wasOK := written(p, true)
		if !wasOK {
			was, wasOK = rendered(p, true)
		}
		add(p.Provenance.Foundation, Change{Path: p.Path, Status: StatusRemoved, Before: was, HasBefore: wasOK})
	}
	var order, extra []string
	for _, f := range b.Foundations {
		if _, ok := groups[f.ID]; ok {
			order = append(order, f.ID)
		}
	}
	for f := range groups {
		if !slices.Contains(order, f) {
			extra = append(extra, f)
		}
	}
	slices.Sort(extra)
	d := TreeDiff{Groups: []Group{}}
	for _, f := range append(order, extra...) {
		cs := groups[f]
		slices.SortFunc(cs, func(x, y Change) int {
			return cmp.Or(comparePaths(x.Path, y.Path), strings.Compare(x.Element, y.Element))
		})
		d.Groups = append(d.Groups, Group{Foundation: f, Changes: cs})
	}
	return d
}

// written is the value l puts in the values file; false when there is no leaf or it keeps no scalar.
func written(l Leaf, ok bool) (any, bool) {
	if !ok || l.Provenance.Pinned && !l.Provenance.Kept {
		return nil, false
	}
	return l.Value, true
}

// rendered is the value the render computed for l, kept or not; false when there is no leaf.
func rendered(l Leaf, ok bool) (any, bool) {
	if !ok {
		return nil, false
	}
	if l.Provenance.Pinned {
		return l.Provenance.Rendered, true
	}
	return l.Value, true
}

// elementKeySep joins a keyed element's key field and key in Change.Element.
const elementKeySep = "="

// pinnedElement is an owned keyed element the loaded file holds with other content, reported as one change.
type pinnedElement struct {
	list       model.CompiledList
	at         roundtrip.Path
	key        string
	foundation string
	rendered   any
	kept       any
}

// pinnedElementOf returns the entry of pinned for l's element, adding it on its first leaf; nil when l is not a leaf
// of a pinned owned element.
func pinnedElementOf(b model.Bundle, l Leaf, pinned *[]*pinnedElement) *pinnedElement {
	if !l.Provenance.Pinned || l.Provenance.Element == "" || l.Provenance.Origin != "" {
		return nil
	}
	list, at, ok := elementOf(b, l.Path)
	if !ok {
		return nil
	}
	for _, e := range *pinned {
		if slices.Equal(e.at, at) {
			return e
		}
	}
	e := &pinnedElement{list: list, at: at, key: l.Provenance.Element, foundation: l.Provenance.Foundation, rendered: l.Provenance.Rendered}
	*pinned = append(*pinned, e)
	return e
}

// renderedIn is what prev rendered for e's element, wherever it sat in the list, and drops its leaves from before so
// they are not reported as removed; false when prev has no such element.
func (e *pinnedElement) renderedIn(b model.Bundle, prev Tree, before map[string]Leaf) (any, bool) {
	var was any
	found := false
	for _, l := range prev.Leaves {
		list, at, ok := elementOf(b, l.Path)
		if !ok || !slices.Equal(list.Path, e.list.Path) || l.Provenance.Element != e.key || l.Provenance.Origin != "" {
			continue
		}
		delete(before, l.Path.String())
		// A pinned leaf carries the whole rendered element; an unpinned one is a piece of it.
		if l.Provenance.Pinned {
			was = l.Provenance.Rendered
		} else {
			was = insert(was, l.Path[len(at):], l.Value)
		}
		found = true
	}
	return was, found
}

// elementOf returns the lists-table list that holds p in one of its elements, and that element's path.
func elementOf(b model.Bundle, p roundtrip.Path) (model.CompiledList, roundtrip.Path, bool) {
	for _, l := range b.ListTable() {
		n := len(l.Path)
		if len(p) <= n || !slices.Equal(p[:n], l.Path) {
			continue
		}
		if _, ok := p[n].Index(); ok {
			return l, p[:n+1], true
		}
	}
	return model.CompiledList{}, nil, false
}

// elementSelector is the [field=key] suffix of a keyed element's path; empty for a leaf.
func elementSelector(sel string) string {
	if sel == "" {
		return ""
	}
	return "[" + sel + "]"
}
