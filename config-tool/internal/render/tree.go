package render

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Tree is a rendered values tree: one scalar per leaf, sorted by path.
type Tree struct {
	Leaves []Leaf `json:"leaves"`
	// Withheld are the mechanisms left out for a collision with a foreign list element (ADR 0025).
	Withheld []Withheld `json:"withheld,omitempty"`
}

// Leaf is one chart key the render writes. Path is chart-relative.
type Leaf struct {
	Path       roundtrip.Path
	Value      any
	Provenance Provenance
}

// Provenance says which foundation target wrote a leaf and whether a hand edit pinned it.
type Provenance struct {
	Foundation string `json:"foundation"`
	// Answers are the answer ids the target references; empty for const.
	Answers []string `json:"answers"`
	Fn      string   `json:"fn"`
	// Target is the index of the target in its foundation's targets list.
	Target int  `json:"target"`
	Pinned bool `json:"pinned"`
	// Kept is set on a pinned leaf whose loaded node is a scalar, which Value then holds.
	Kept bool `json:"kept"`
	// Rendered is what the render would have written; set only on a pinned leaf.
	Rendered any `json:"rendered,omitempty"`
	// Element is the key of the list element the leaf lies in, for a list written by keyed elements.
	Element string `json:"element,omitempty"`
	// Origin is OriginBase or OriginLoaded on a leaf of an element the render keeps but does not own.
	Origin string `json:"origin,omitempty"`
}

// MarshalJSON writes the path in roundtrip.ParsePath syntax.
func (l Leaf) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path       string     `json:"path"`
		Value      any        `json:"value"`
		Provenance Provenance `json:"provenance"`
	}{l.Path.String(), l.Value, l.Provenance})
}

// Nested builds the map/slice form of t. A pinned leaf without a kept value is left out, so the
// overrides layer keeps its subtree; a list index nothing writes is nil.
func (t Tree) Nested() map[string]any {
	root := map[string]any{}
	for _, l := range t.Leaves {
		if l.Provenance.Pinned && !l.Provenance.Kept {
			continue
		}
		root = insert(root, l.Path, l.Value).(map[string]any)
	}
	return root
}

// insert sets p below node to v, creating the maps and slices on the way.
func insert(node any, p roundtrip.Path, v any) any {
	if len(p) == 0 {
		return v
	}
	if i, ok := p[0].Index(); ok {
		s, _ := node.([]any)
		for len(s) <= i {
			s = append(s, nil)
		}
		s[i] = insert(s[i], p[1:], v)
		return s
	}
	k, _ := p[0].Key()
	m, _ := node.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	m[k] = insert(m[k], p[1:], v)
	return m
}

// comparePaths orders segment-wise: indices numerically, keys bytewise, an index before a key.
func comparePaths(a, b roundtrip.Path) int {
	for i := range min(len(a), len(b)) {
		ai, aIdx := a[i].Index()
		bi, bIdx := b[i].Index()
		switch {
		case aIdx && bIdx:
			if c := cmp.Compare(ai, bi); c != 0 {
				return c
			}
		case aIdx != bIdx:
			if aIdx {
				return -1
			}
			return 1
		default:
			ak, _ := a[i].Key()
			bk, _ := b[i].Key()
			if c := cmp.Compare(ak, bk); c != 0 {
				return c
			}
		}
	}
	return cmp.Compare(len(a), len(b))
}

func sortLeaves(ls []Leaf) {
	slices.SortFunc(ls, func(a, b Leaf) int { return comparePaths(a.Path, b.Path) })
}
