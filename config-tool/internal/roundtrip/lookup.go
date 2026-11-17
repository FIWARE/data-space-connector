package roundtrip

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Lookup returns the node at p. A duplicated key resolves to its last occurrence. It gives
// ErrNotFound for a missing key or index and ErrAliasPath when p runs through an alias.
func (d *Document) Lookup(p Path) (*yaml.Node, error) {
	loc, err := d.locate(p)
	return loc.node, err
}

// located is a node found by locate, with the context its value text is scanned in.
type located struct {
	node *yaml.Node
	ctx  leafCtx
	// merge is set when the path runs through a `<<` key.
	merge bool
}

// locate resolves p like Lookup and tracks the owning column and flow context on the way.
func (d *Document) locate(p Path) (located, error) {
	if len(d.root.Content) == 0 {
		return located{}, fmt.Errorf("%w: %s: empty document", ErrNotFound, p)
	}
	loc := located{node: d.root.Content[0], ctx: leafCtx{owner: noOwner}}
	for i, seg := range p {
		n := loc.node
		if n.Kind == yaml.AliasNode {
			return located{}, fmt.Errorf("%w: %s passes through alias *%s at %s", ErrAliasPath, p, n.Value, p[:i])
		}
		key, next := child(n, seg)
		if next == nil {
			return located{}, fmt.Errorf("%w: %s", ErrNotFound, p[:i+1])
		}
		switch {
		case n.Style&yaml.FlowStyle != 0:
			loc.ctx.flow = true
		case loc.ctx.flow:
		case key != nil:
			loc.ctx.owner = key.Column - 1
		default:
			// Every `-` of a block sequence sits at the sequence's own column.
			loc.ctx.owner = n.Column - 1
		}
		loc.merge = loc.merge || (key != nil && key.Tag == mergeTag)
		loc.node = next
	}
	return loc, nil
}

// child returns the key and value seg selects below n, or nil values; key is nil for an index.
// Merge keys are not resolved.
func child(n *yaml.Node, seg Segment) (key, value *yaml.Node) {
	if i, ok := seg.Index(); ok {
		if n.Kind != yaml.SequenceNode || i >= len(n.Content) {
			return nil, nil
		}
		return nil, n.Content[i]
	}
	if n.Kind != yaml.MappingNode {
		return nil, nil
	}
	k, _ := seg.Key()
	for i := len(n.Content) - 2; i >= 0; i -= 2 {
		if key := n.Content[i]; key.Kind == yaml.ScalarNode && key.Value == k {
			return key, n.Content[i+1]
		}
	}
	return nil, nil
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
