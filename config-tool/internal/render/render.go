// Package render turns profile answers into a values tree whose leaves carry provenance (ADR 0020).
package render

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// The separators that follow a path's prefix in roundtrip.Path.String syntax.
const (
	keySeparator = '.'
	indexOpen    = '['
)

// Options tune a render; the zero value renders without a loaded file.
type Options struct {
	// Overrides is the loaded file whose hand edits pin leaves; nil when nothing is loaded.
	Overrides *load.Result
}

// Render evaluates b's foundation targets over p's answers. It errors on an answer b does not
// declare and on a derived answer its type refuses; an unanswered target, or one whose requires are, writes nothing.
func Render(b model.Bundle, p profile.Profile, opts Options) (Tree, error) {
	r, err := resolve(b, p)
	if err != nil {
		return Tree{}, err
	}
	leaves := []Leaf{}
	incomplete := map[string]bool{}
	// blocks maps a leaf's path to the block its target writes as one unit.
	blocks := map[string]roundtrip.Path{}
	for _, decl := range b.Foundations {
		f, _ := b.Foundation(decl.ID)
		for i, t := range f.Targets {
			if t.Element != nil || t.Each != nil {
				continue
			}
			if !all(r.answers, t.Refs) || !all(r.answers, t.Requires) {
				for _, l := range lists(t.Path) {
					incomplete[l.String()] = true
				}
				continue
			}
			if t.Only != "" && r.answers[f.ID] != any(t.Only) {
				continue
			}
			v, err := evaluate(t, r.answers, r.ns)
			if err != nil {
				return Tree{}, fmt.Errorf("render: %s targets[%d]: %w", f.ID, i, err)
			}
			leaves = append(leaves, Leaf{Path: slices.Clone(t.Path), Value: v, Provenance: Provenance{
				Foundation: f.ID, Answers: append([]string{}, t.Refs...), Fn: string(t.Fn), Target: i,
			}})
			if t.Block != nil {
				blocks[t.Path.String()] = t.Block
			}
		}
	}
	// An owned list is written whole or not at all, so a fragment never replaces the chart's list.
	leaves = slices.DeleteFunc(leaves, func(l Leaf) bool {
		return slices.ContainsFunc(lists(l.Path), func(k roundtrip.Path) bool { return incomplete[k.String()] })
	})
	if opts.Overrides != nil {
		pin(leaves, blocks, opts.Overrides)
	}
	// Element and each leaves are pinned by content, not by the hand edits pin reads: a loaded list holds them.
	elements, withheld, err := composeLists(b, r, opts)
	if err != nil {
		return Tree{}, err
	}
	each, err := eachLeaves(b, r, opts)
	if err != nil {
		return Tree{}, err
	}
	leaves = slices.Concat(leaves, elements, each)
	sortLeaves(leaves)
	return Tree{Leaves: leaves, Withheld: withheld}, nil
}

// Answers returns p's given answers plus the derived defaults, keyed by foundation id; it errors as Render does.
// Reserved refs such as target.environment are not answers and are left out.
func Answers(b model.Bundle, p profile.Profile) (map[string]any, error) {
	r, err := resolve(b, p)
	return r.answers, err
}

// resolution is a profile's resolved answers.
type resolution struct {
	// answers are the given answers plus the derived defaults, flat; never a reserved ref.
	answers map[string]any
	// ns is answers nested by id segment plus the reserved refs, the namespace templates run against.
	ns map[string]any
	// reserved holds the reserved refs that have a value.
	reserved map[string]bool
}

// resolved reports whether every id is an answer or a reserved ref with a value.
func (r resolution) resolved(ids []string) bool {
	return !slices.ContainsFunc(ids, func(id string) bool { _, ok := r.answers[id]; return !ok && !r.reserved[id] })
}

// resolve returns p's given answers plus the derived defaults, seeding the reserved target refs from p's target. It errors when a resolved answer does not start with its prefix.
func resolve(b model.Bundle, p profile.Profile) (resolution, error) {
	given := p.Answers
	for _, id := range slices.Sorted(maps.Keys(given)) {
		if _, ok := b.Foundation(id); !ok {
			return resolution{}, fmt.Errorf("render: answer %q is not declared by bundle %s %s", id, b.Chart, b.Version)
		}
	}
	r := resolution{answers: map[string]any{}, ns: map[string]any{}, reserved: map[string]bool{}}
	for ref, v := range map[string]string{
		model.FieldTargetEnvironment: string(p.Target.Environment),
		model.RefTargetRelease:       p.Target.ReleaseName,
		model.RefTargetNamespace:     p.Target.Namespace,
	} {
		if v != "" {
			r.reserved[ref] = true
			setNested(r.ns, ref, v)
		}
	}
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		v, ok := given[id]
		if !ok {
			if !defaults(f, r) {
				continue
			}
			s, err := execute(f.DefaultTemplate(), r.ns)
			if err != nil {
				return resolution{}, fmt.Errorf("render: default of %s: %w", id, err)
			}
			if err := f.Check(s); err != nil {
				return resolution{}, fmt.Errorf("render: derived %s %w", id, err)
			}
			v = s
		}
		if err := checkPrefix(f, v, r); err != nil {
			return resolution{}, err
		}
		r.answers[id] = v
		setNested(r.ns, id, v)
	}
	return r, nil
}

// defaults reports whether f's default applies: it has one, its default refs are resolved and DefaultWhen holds.
func defaults(f model.CompiledFoundation, r resolution) bool {
	if f.DefaultTemplate() == nil || !r.resolved(f.DefaultRefs()) {
		return false
	}
	return f.DefaultWhen == nil || f.DefaultWhen.Holds(r.answers)
}

// checkPrefix refuses v when it does not start with f's prefix; a prefix over unresolved answers is not checked.
func checkPrefix(f model.CompiledFoundation, v any, r resolution) error {
	if f.PrefixTemplate() == nil || !r.resolved(f.PrefixRefs()) {
		return nil
	}
	prefix, err := execute(f.PrefixTemplate(), r.ns)
	if err != nil {
		return fmt.Errorf("render: prefix of %s: %w", f.ID, err)
	}
	if s, ok := v.(string); !ok || !strings.HasPrefix(s, prefix) {
		return fmt.Errorf("render: %s must start with %q, which %s sets", f.ID, prefix, strings.Join(f.PrefixRefs(), ", "))
	}
	return nil
}

func all(answers map[string]any, ids []string) bool {
	return !slices.ContainsFunc(ids, func(id string) bool { _, ok := answers[id]; return !ok })
}

// lists returns every prefix of p that is indexed into, i.e. the owned lists p writes into.
func lists(p roundtrip.Path) []roundtrip.Path {
	var ls []roundtrip.Path
	for k := 1; k < len(p); k++ {
		if _, ok := p[k].Index(); ok {
			ls = append(ls, p[:k])
		}
	}
	return ls
}

// edit is a hand edit of the loaded file: an unknown or skipped leaf (ADR 0014 §4).
type edit struct {
	path string
	// emptyMap is set for an empty mapping, which a path needing a mapping there merges into.
	emptyMap bool
}

// conflicts reports whether e stands where the path key needs to write.
func (e edit) conflicts(key string) bool {
	if e.emptyMap && isUnder(key, e.path) && (e.path == "" || key[len(e.path)] == keySeparator) {
		return false
	}
	return overlaps(key, e.path)
}

// pin marks every leaf that overlaps a hand edit of r and keeps the loaded scalar in its place.
// An owned list a hand edit touches is pinned whole without kept values, so the overrides layer
// keeps its list and Nested() never writes a fragment of it. A hand edit of one leaf of a block
// (blocks, by leaf path) pins the whole block with kept values, so the file's block stays as written
// and a rendered request never meets a hand-set limit below it.
func pin(leaves []Leaf, blocks map[string]roundtrip.Path, r *load.Result) {
	var edits []edit
	for _, e := range r.Report.Unknown {
		p, err := roundtrip.ParsePath(e.Path)
		n, lerr := r.Doc.Lookup(p)
		empty := err == nil && lerr == nil && n.Kind == yaml.MappingNode && len(n.Content) == 0
		edits = append(edits, edit{path: e.Path, emptyMap: empty})
	}
	for _, e := range r.Report.Skipped {
		edits = append(edits, edit{path: e.Path})
	}
	hit := func(p roundtrip.Path) bool {
		key := slices.Concat(r.Root, p).String()
		return slices.ContainsFunc(edits, func(e edit) bool { return e.conflicts(key) })
	}
	// A block is pinned only when an edit hits one of its own leaves; other keys under it merge.
	hitBlocks := map[string]bool{}
	for _, l := range leaves {
		if b, ok := blocks[l.Path.String()]; ok && hit(l.Path) {
			hitBlocks[b.String()] = true
		}
	}
	inHitBlock := func(p roundtrip.Path) bool {
		b, ok := blocks[p.String()]
		return ok && hitBlocks[b.String()]
	}
	for i := range leaves {
		l := &leaves[i]
		inList := slices.ContainsFunc(lists(l.Path), hit)
		if !inList && !hit(l.Path) && !inHitBlock(l.Path) {
			continue
		}
		l.Provenance.Pinned, l.Provenance.Rendered = true, l.Value
		l.Value, l.Provenance.Kept = nil, false
		if !inList {
			l.Value, l.Provenance.Kept = kept(r.Doc, slices.Concat(r.Root, l.Path))
		}
	}
}

// kept decodes the loaded scalar at p with its YAML type. Report values are capped previews, so
// they are never used. ok is false when p holds no scalar, runs through an alias or is missing.
func kept(doc *roundtrip.Document, p roundtrip.Path) (v any, ok bool) {
	n, err := doc.Lookup(p)
	if err != nil || n.Kind != yaml.ScalarNode {
		return nil, false
	}
	if err := n.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// overlaps reports whether paths a and b are equal or one is a segment prefix of the other.
func overlaps(a, b string) bool {
	return a == b || isUnder(a, b) || isUnder(b, a)
}

// isUnder reports whether path p lies strictly below path q; every path lies below the root "".
func isUnder(p, q string) bool {
	if len(p) <= len(q) || !strings.HasPrefix(p, q) {
		return false
	}
	return q == "" || p[len(q)] == keySeparator || p[len(q)] == indexOpen
}
