package roundtrip

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// mergeTag is the tag yaml.v3 resolves a plain `<<` key to.
const mergeTag = "!!merge"

// Class is how a front end treats a leaf on load; CT-3 supplies the classifier that sets it.
type Class int

// The classes of a leaf; the zero value is ClassUnknown.
const (
	ClassUnknown Class = iota
	ClassRecognized
	ClassOwned
)

// String returns the lower-case class name used in load reports.
func (c Class) String() string {
	switch c {
	case ClassUnknown:
		return "unknown"
	case ClassRecognized:
		return "recognized"
	case ClassOwned:
		return "owned"
	}
	return fmt.Sprintf("Class(%d)", int(c))
}

// Leaf is a scalar, empty collection, alias or merge-key value found by Classify.
// Line is 1-based: the key's line in a mapping, the node's own line otherwise.
type Leaf struct {
	Path  Path
	Node  *yaml.Node
	Line  int
	Class Class
}

// Classifier assigns a Class to the leaf at a path.
type Classifier func(Path, *yaml.Node) Class

// DefaultClassifier classifies every leaf as ClassUnknown.
func DefaultClassifier(Path, *yaml.Node) Class {
	return ClassUnknown
}

// SkipReason is why Classify left a key/value pair out.
type SkipReason int

// The reasons a pair is skipped.
const (
	// SkipShadowed marks a duplicate key that a later key of the same mapping shadows.
	SkipShadowed SkipReason = iota
	// SkipNonScalarKey marks a pair whose key is an alias or complex key, which no Path addresses.
	SkipNonScalarKey
)

// SkippedPair is a pair Classify skips with its subtree; its bytes stay in the document.
// Line is the key's body line; ShadowedBy is the live key's body line, 0 unless shadowed.
type SkippedPair struct {
	Parent     Path
	Key, Value *yaml.Node
	Line       int
	ShadowedBy int
	Reason     SkipReason
}

// Classify returns doc's leaves in document order, classified by c (DefaultClassifier when nil).
// Aliases are not followed. Shadowed duplicate keys and pairs whose key is not a scalar (an
// alias or complex key, which no Path addresses) are skipped with their subtrees; see Skipped.
func Classify(doc *Document, c Classifier) []Leaf {
	if c == nil {
		c = DefaultClassifier
	}
	var leaves []Leaf
	walkPairs(doc, func(p Path, n *yaml.Node, line int) {
		leaves = append(leaves, Leaf{Path: p, Node: n, Line: line, Class: c(p, n)})
	}, func(SkippedPair) {})
	return leaves
}

// Skipped returns the pairs Classify skips, in document order. Pairs nested inside a skipped
// pair are not listed again.
func Skipped(doc *Document) []SkippedPair {
	var skipped []SkippedPair
	walkPairs(doc, func(Path, *yaml.Node, int) {}, func(s SkippedPair) {
		skipped = append(skipped, s)
	})
	return skipped
}

// walkPairs walks doc's current tree in document order, calling leaf for each leaf and skip for
// each skipped pair, whose subtree it does not enter.
func walkPairs(doc *Document, leaf func(Path, *yaml.Node, int), skip func(SkippedPair)) {
	if len(doc.root.Content) == 0 {
		return
	}
	var walk func(p Path, n *yaml.Node, line int)
	walk = func(p Path, n *yaml.Node, line int) {
		if n.Kind == yaml.AliasNode || n.Kind == yaml.ScalarNode || len(n.Content) == 0 {
			leaf(p, n, line)
			return
		}
		if n.Kind == yaml.SequenceNode {
			for i, item := range n.Content {
				walk(append(p[:len(p):len(p)], Index(i)), item, item.Line)
			}
			return
		}
		live := livePairs(n)
		for i := range live {
			key, value := n.Content[2*i], n.Content[2*i+1]
			switch {
			// livePairs gives every collection key the same identity, so they are never "shadowed".
			case key.Kind == yaml.MappingNode || key.Kind == yaml.SequenceNode:
				skip(SkippedPair{Parent: p, Key: key, Value: value, Line: key.Line, Reason: SkipNonScalarKey})
				continue
			case !live[i]:
				skip(SkippedPair{Parent: p, Key: key, Value: value, Line: key.Line,
					ShadowedBy: shadowingKey(n, live, i).Line, Reason: SkipShadowed})
				continue
			case key.Kind != yaml.ScalarNode:
				skip(SkippedPair{Parent: p, Key: key, Value: value, Line: key.Line, Reason: SkipNonScalarKey})
				continue
			}
			sub := append(p[:len(p):len(p)], Key(key.Value))
			if key.Tag == mergeTag {
				leaf(sub, value, key.Line)
				continue
			}
			walk(sub, value, key.Line)
		}
	}
	top := doc.root.Content[0]
	walk(nil, top, top.Line)
}

// shadowingKey returns the live key of mapping m that shadows its i-th pair.
func shadowingKey(m *yaml.Node, live []bool, i int) *yaml.Node {
	key := m.Content[2*i]
	for j := i + 1; j < len(live); j++ {
		if k := m.Content[2*j]; live[j] && k.Kind == key.Kind && k.Value == key.Value {
			return k
		}
	}
	return key
}
