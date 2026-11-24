package render

import (
	"encoding/json"
	"reflect"
	"slices"

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
	}{c.Path.String(), c.Status, opt(c.Before, c.HasBefore), opt(c.After, c.HasAfter), opt(c.Kept, c.HasKept)})
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
	for _, l := range next.Leaves {
		key := l.Path.String()
		p, had := before[key]
		delete(before, key)
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
		slices.SortFunc(cs, func(x, y Change) int { return comparePaths(x.Path, y.Path) })
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
