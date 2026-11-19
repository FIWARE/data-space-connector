package load

import (
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// maxRootDepth is how many keys below the document root Load looks for the chart's values, e.g.
// `dsc:` when an Argo CD or umbrella chart packages this chart as a dependency.
const maxRootDepth = 3

// ErrRoot means a requested root does not address a mapping of the file.
var ErrRoot = errors.New("load: root is not a mapping of the file")

// RootSource says how Report.Root was chosen.
type RootSource string

// The sources of the root.
const (
	RootDetected  RootSource = "detected"
	RootRequested RootSource = "requested"
)

// helmGlobalKey is the key Helm passes to every chart; any root may hold it besides the chart's own.
const helmGlobalKey = "global"

// candidate is a mapping that may hold the chart's values: score of its keys, global aside, are
// chart keys.
type candidate struct {
	path  roundtrip.Path
	score int
	keys  int
}

// chartKeys are the keys that mark the chart's values: the bundle's top-level keys and the
// first key of every rule.
func chartKeys(b model.Bundle) map[string]bool {
	keys := map[string]bool{}
	for _, k := range b.TopLevelKeys {
		keys[k] = true
	}
	for _, r := range b.Rules() {
		if k, ok := r.Path[0].Key(); ok {
			keys[k] = true
		}
	}
	return keys
}

// rootCandidates ranks the mappings at most maxRootDepth keys deep that hold a chart key. The
// chart's schema refuses unknown keys, so the best is the one with the largest share of chart
// keys; ties go to more chart keys, then the shallower one, then document order. No candidate
// lies below a chart key: that subtree belongs to the chart.
func rootCandidates(leaves []roundtrip.Leaf, chart map[string]bool) []candidate {
	var list []candidate
	index := map[string]int{}
	type pair struct{ mapping, key string }
	counted := map[pair]bool{}
	for _, l := range leaves {
		for d := 0; d <= maxRootDepth && d < len(l.Path); d++ {
			k, ok := l.Path[d].Key()
			if !ok {
				break
			}
			prefix := l.Path[:d].String()
			if seen := (pair{prefix, k}); !counted[seen] {
				counted[seen] = true
				i, ok := index[prefix]
				if !ok {
					i = len(list)
					index[prefix] = i
					list = append(list, candidate{path: slices.Clone(l.Path[:d])})
				}
				if k != helmGlobalKey {
					list[i].keys++
				}
				if chart[k] {
					list[i].score++
				}
			}
			if chart[k] {
				break
			}
		}
	}
	list = slices.DeleteFunc(list, func(c candidate) bool { return c.score == 0 })
	slices.SortStableFunc(list, func(a, b candidate) int {
		if share := b.score*a.keys - a.score*b.keys; share != 0 {
			return share
		}
		if a.score != b.score {
			return b.score - a.score
		}
		return len(a.path) - len(b.path)
	})
	return list
}

// checkRoot accepts the empty path and a key path to a mapping; anything else wraps ErrRoot.
func checkRoot(doc *roundtrip.Document, root roundtrip.Path) error {
	if len(root) == 0 {
		return nil
	}
	for _, s := range root {
		if _, ok := s.Key(); !ok {
			return fmt.Errorf("%w: %s holds a sequence index", ErrRoot, root)
		}
	}
	n, err := doc.Lookup(root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRoot, err)
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: %s is not a mapping", ErrRoot, root)
	}
	return nil
}

// under returns p relative to root; ok is false when p is not below root.
func under(p, root roundtrip.Path) (roundtrip.Path, bool) {
	if len(p) <= len(root) || !slices.Equal(p[:len(root)], root) {
		return nil, false
	}
	return p[len(root):], true
}
