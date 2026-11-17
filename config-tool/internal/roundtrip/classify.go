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

// Classify returns doc's leaves in document order, classified by c (DefaultClassifier when nil).
// Aliases are not followed. Shadowed duplicate keys and pairs whose key is not a scalar (an
// alias or complex key, which no Path addresses) are skipped with their subtrees.
func Classify(doc *Document, c Classifier) []Leaf {
	if c == nil {
		c = DefaultClassifier
	}
	if len(doc.root.Content) == 0 {
		return nil
	}
	var leaves []Leaf
	var walk func(p Path, n *yaml.Node, line int)
	walk = func(p Path, n *yaml.Node, line int) {
		if n.Kind == yaml.AliasNode || n.Kind == yaml.ScalarNode || len(n.Content) == 0 {
			leaves = append(leaves, Leaf{Path: p, Node: n, Line: line, Class: c(p, n)})
			return
		}
		if n.Kind == yaml.SequenceNode {
			for i, item := range n.Content {
				walk(append(p[:len(p):len(p)], Index(i)), item, item.Line)
			}
			return
		}
		for i, live := range livePairs(n) {
			key, value := n.Content[2*i], n.Content[2*i+1]
			if !live || key.Kind != yaml.ScalarNode {
				continue
			}
			sub := append(p[:len(p):len(p)], Key(key.Value))
			if key.Tag == mergeTag {
				leaves = append(leaves, Leaf{Path: sub, Node: value, Line: key.Line, Class: c(sub, value)})
				continue
			}
			walk(sub, value, key.Line)
		}
	}
	top := doc.root.Content[0]
	walk(nil, top, top.Line)
	return leaves
}
