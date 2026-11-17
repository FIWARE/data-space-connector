package roundtrip

import (
	"errors"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"
)

// Merge deep-merges overrides into doc through Set: maps merge recursively, the override wins
// otherwise. Sorted key order makes equal overrides give equal bytes. A map over an alias to a
// mapping merges with its resolved value, replacing the alias only on change. Edits before an error stay pending.
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
			err = d.Set(q, v)
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

// mergeAlias merges over over the resolved value of the alias n at p, as Helm sees it.
func (d *Document) mergeAlias(p Path, n *yaml.Node, over map[string]any) error {
	cur, err := decodeValue(n)
	if err != nil {
		return err
	}
	base, ok := cur.(map[string]any)
	if !ok {
		return d.Set(p, over)
	}
	return d.Set(p, deepMerge(base, over))
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
