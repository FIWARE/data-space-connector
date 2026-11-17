package roundtrip

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"
)

// Merge deep-merges overrides into doc through Set: maps merge recursively, the override wins
// otherwise. Sorted key order makes equal overrides give equal bytes. A map over an alias to a
// mapping, or over a map inherited through `<<`, merges with that value as Helm sees it; an alias is
// replaced only on change. Edits before an error stay pending.
func Merge(doc *Document, overrides map[string]any) error {
	if !isEmptyDoc(doc.root) && doc.root.Content[0].Kind != yaml.MappingNode {
		return ErrNotMapping
	}
	return doc.merge(nil, overrides)
}

func (d *Document) merge(p Path, overrides map[string]any) error {
	for _, k := range slices.Sorted(maps.Keys(overrides)) {
		q := append(p[:len(p):len(p)], Key(k))
		v := overrides[k]
		sub, isMap := v.(map[string]any)
		if !isMap {
			if err := d.Set(q, v); err != nil {
				return err
			}
			continue
		}
		n, err := d.Lookup(q)
		switch {
		case errors.Is(err, ErrNotFound):
			err = d.mergeInherited(p, k, sub)
		case err != nil:
		case n.Kind == yaml.MappingNode:
			err = d.merge(q, sub)
		case n.Kind == yaml.AliasNode && n.Alias != nil && n.Alias.Kind == yaml.MappingNode:
			err = d.mergeAlias(q, n, sub)
		default:
			err = d.Set(q, v)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// mergeAlias merges over the resolved value of the alias n at p, as Helm sees it.
func (d *Document) mergeAlias(p Path, n *yaml.Node, over map[string]any) error {
	cur, err := decodeValue(n)
	if err != nil {
		return err
	}
	if cur, err = stringKeys(cur); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnsupportedValue, p, err)
	}
	base, _ := cur.(map[string]any)
	return d.Set(p, deepMerge(base, over))
}

// mergeInherited sets the missing key k under p to sub, deep-merged with the map that p
// inherits for k through a merge key `<<`, so the inherited siblings are not lost.
func (d *Document) mergeInherited(p Path, k string, sub map[string]any) error {
	q := append(p[:len(p):len(p)], Key(k))
	parent, err := d.Lookup(p)
	if err != nil || !hasMergeKey(parent) {
		return d.Set(q, sub)
	}
	cur, err := decodeValue(parent)
	if err != nil {
		return err
	}
	if cur, err = stringKeys(cur); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnsupportedValue, p, err)
	}
	inherited, ok := cur.(map[string]any)[k].(map[string]any)
	if !ok {
		return d.Set(q, sub)
	}
	return d.Set(q, deepMerge(inherited, sub))
}

// hasMergeKey reports whether mapping n has a `<<` key.
func hasMergeKey(n *yaml.Node) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Tag == mergeTag {
			return true
		}
	}
	return false
}

// stringKeys turns every map[any]any in v into a map[string]any, its keys formatted as Helm's
// JSON conversion does (`1` → "1", `~` → "null"). Two keys with one string form give an error.
func stringKeys(v any) (any, error) {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, sub := range v {
			var err error
			if out[k], err = stringKeys(sub); err != nil {
				return nil, err
			}
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, sub := range v {
			s := nullText
			if k != nil {
				s = fmt.Sprint(k)
			}
			if _, dup := out[s]; dup {
				return nil, fmt.Errorf("keys collide as %q", s)
			}
			var err error
			if out[s], err = stringKeys(sub); err != nil {
				return nil, err
			}
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, sub := range v {
			var err error
			if out[i], err = stringKeys(sub); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return v, nil
}

// deepMerge returns base with over merged in, over winning; neither input is changed.
func deepMerge(base, over map[string]any) map[string]any {
	out := maps.Clone(base)
	for k, v := range over {
		if om, ok := v.(map[string]any); ok {
			if bm, ok := out[k].(map[string]any); ok {
				out[k] = deepMerge(bm, om)
				continue
			}
		}
		out[k] = v
	}
	return out
}
