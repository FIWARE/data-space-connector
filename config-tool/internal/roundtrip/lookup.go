package roundtrip

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Lookup returns the node at p. A duplicated key resolves to its last occurrence. It gives
// ErrNotFound for a missing key or index and ErrAliasPath when p runs through an alias.
func (d *Document) Lookup(p Path) (*yaml.Node, error) {
	if len(d.root.Content) == 0 {
		return nil, fmt.Errorf("%w: %s: empty document", ErrNotFound, p)
	}
	n := d.root.Content[0]
	for i, seg := range p {
		if n.Kind == yaml.AliasNode {
			return nil, fmt.Errorf("%w: %s passes through alias *%s at %s", ErrAliasPath, p, n.Value, p[:i])
		}
		next := child(n, seg)
		if next == nil {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, p[:i+1])
		}
		n = next
	}
	return n, nil
}

// child returns the node seg selects below n, or nil. Merge keys are not resolved.
func child(n *yaml.Node, seg Segment) *yaml.Node {
	if i, ok := seg.Index(); ok {
		if n.Kind != yaml.SequenceNode || i >= len(n.Content) {
			return nil
		}
		return n.Content[i]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	k, _ := seg.Key()
	for i := len(n.Content) - 2; i >= 0; i -= 2 {
		if key := n.Content[i]; key.Kind == yaml.ScalarNode && key.Value == k {
			return n.Content[i+1]
		}
	}
	return nil
}

// livePairs reports, per key/value pair of mapping m, whether it is live: no later key of the
// same kind and value shadows it. This is yaml.v3's duplicate rule, resolved last-wins as Helm does.
func livePairs(m *yaml.Node) []bool {
	type keyID struct {
		kind  yaml.Kind
		value string
	}
	live := make([]bool, len(m.Content)/2)
	seen := make(map[keyID]bool, len(live))
	for i := len(live) - 1; i >= 0; i-- {
		key := m.Content[2*i]
		id := keyID{key.Kind, key.Value}
		live[i] = !seen[id]
		seen[id] = true
	}
	return live
}

// decodeValue decodes n into Go values as Helm sees them: shadowed duplicate keys are dropped
// first, so yaml.v3's duplicate-key error cannot occur. n itself is left unchanged.
func decodeValue(n *yaml.Node) (any, error) {
	var v any
	if err := withoutShadowed(n, map[*yaml.Node]*yaml.Node{}).Decode(&v); err != nil {
		return nil, fmt.Errorf("roundtrip: %w", err)
	}
	return v, nil
}

// withoutShadowed deep-copies n without shadowed pairs. Alias targets are copied too, even
// when they sit in a shadowed pair, so every alias still resolves; copies memoizes shared nodes.
func withoutShadowed(n *yaml.Node, copies map[*yaml.Node]*yaml.Node) *yaml.Node {
	if c, ok := copies[n]; ok {
		return c
	}
	c := *n
	copies[n] = &c
	c.Content = nil
	if n.Alias != nil {
		c.Alias = withoutShadowed(n.Alias, copies)
	}
	var live []bool
	if n.Kind == yaml.MappingNode {
		live = livePairs(n)
	}
	for i, sub := range n.Content {
		if live != nil && !live[i/2] {
			continue
		}
		c.Content = append(c.Content, withoutShadowed(sub, copies))
	}
	return &c
}
