package render

import (
	"encoding/json"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// TargetRef is one chart key an answer reaches; Component is the path's top-level key.
type TargetRef struct {
	Path      roundtrip.Path
	Component string
}

// MarshalJSON writes the path in roundtrip.ParsePath syntax.
func (r TargetRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path      string `json:"path"`
		Component string `json:"component"`
	}{r.Path.String(), r.Component})
}

// Targets lists the chart keys answerID fans out to, sorted by path and each once: the targets that reference or
// require it, or an answer whose default derives from it, transitively, and every target of a mechanism one of
// whose targets does, because a mechanism renders as a whole. A const without requires is not listed. Nil when b
// does not declare answerID.
func Targets(b model.Bundle, answerID string) []TargetRef {
	if _, ok := b.Foundation(answerID); !ok {
		return nil
	}
	reach := map[string]bool{answerID: true}
	reached := func(r string) bool { return reach[r] }
	// FoundationOrder puts every default after the answers it references, so one pass closes the set.
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		if f.DefaultTemplate() != nil && slices.ContainsFunc(f.DefaultRefs(), reached) {
			reach[id] = true
		}
	}
	targetReached := func(t model.CompiledTarget) bool {
		return slices.ContainsFunc(t.Refs, reached) || slices.ContainsFunc(t.Requires, reached)
	}
	mechanisms := map[string]bool{}
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		for _, t := range f.Targets {
			if t.Mechanism != "" && targetReached(t) {
				mechanisms[t.Mechanism] = true
			}
		}
	}
	refs := []TargetRef{}
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		for _, t := range f.Targets {
			if targetReached(t) || mechanisms[t.Mechanism] {
				k, _ := t.Path[0].Key()
				refs = append(refs, TargetRef{Path: slices.Clone(t.Path), Component: k})
			}
		}
	}
	slices.SortFunc(refs, func(a, b TargetRef) int { return comparePaths(a.Path, b.Path) })
	// A resources table writes each path once per profile, and keyed elements share their list's path.
	return slices.CompactFunc(refs, func(a, b TargetRef) bool { return comparePaths(a.Path, b.Path) == 0 })
}
