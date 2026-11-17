package roundtrip

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// sentinelKey is a key no corpus file uses.
const sentinelKey = "dsc-config-sentinel-key"

// collectionAt is a mapping or sequence of a corpus file, reached without passing an alias.
type collectionAt struct {
	path Path
	node *yaml.Node
}

// collections lists the live collections below n in walk order; shadowed pairs and merge keys
// are skipped.
func collections(n *yaml.Node, p Path) []collectionAt {
	var out []collectionAt
	switch n.Kind {
	case yaml.MappingNode:
		out = append(out, collectionAt{path: p, node: n})
		last := map[string]int{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			last[n.Content[i].Value] = i
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag == mergeTag || last[k.Value] != i {
				continue
			}
			out = append(out, collections(n.Content[i+1], append(slices.Clone(p), Key(k.Value)))...)
		}
	case yaml.SequenceNode:
		out = append(out, collectionAt{path: p, node: n})
		for i, c := range n.Content {
			out = append(out, collections(c, append(slices.Clone(p), Index(i)))...)
		}
	}
	return out
}

// lookupValue decodes the value at p in a fresh parse of out.
func lookupValue(t *testing.T, out []byte, p Path) any {
	t.Helper()
	n, err := mustParse(t, out).Lookup(p)
	if err != nil {
		t.Errorf("re-parse Lookup(%s): %v", p, err)
		return nil
	}
	v, err := decodeValue(n)
	if err != nil {
		t.Errorf("decode %s: %v", p, err)
	}
	return v
}

// checkInsert adds sentinelKey below the mapping c: a block mapping gains exactly one line.
func checkInsert(t *testing.T, f *corpusFile, c collectionAt) {
	t.Helper()
	p := append(slices.Clone(c.path), Key(sentinelKey))
	d := mustParse(t, f.src)
	if err := d.Set(p, sentinelString); err != nil {
		t.Errorf("Set(%s): %v", p, err)
		return
	}
	out, err := d.Write()
	if err != nil {
		t.Errorf("Write after Set(%s): %v", p, err)
		return
	}
	if got := lookupValue(t, out, p); got != sentinelString {
		t.Errorf("%s = %v after re-parse", p, got)
	}
	if c.node.Style&yaml.FlowStyle != 0 {
		return
	}
	lines := strings.Split(string(out), lineFeed)
	added := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, sentinelKey) })
	if len(lines) != len(f.lines)+1 || added < 0 || !onlyEOFChomp(slices.Delete(slices.Clone(lines), added, added+1), f.lines) {
		t.Errorf("Set(%s) changed more than one added line", p)
		return
	}
	if want := sentinelKey + ": " + sentinelString; strings.TrimLeft(lines[added], " -") != want {
		t.Errorf("Set(%s) added %q, want %q", p, lines[added], want)
	}
}

// onlyEOFChomp reports whether got equals want, apart from a strip indicator set on the block
// scalar that ends a file without a final line break.
func onlyEOFChomp(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	i := 0
	for i < len(got) && got[i] == want[i] {
		i++
	}
	if i == len(got) || want[len(want)-1] == "" {
		return slices.Equal(got, want)
	}
	g, w := got[i], want[i]
	j := 0
	for j < len(w) && j < len(g) && g[j] == w[j] {
		j++
	}
	ok := j < len(g) && g[j] == chompStrip && (g[j+1:] == w[j:] || (j < len(w) && w[j] == chompKeep && g[j+1:] == w[j+1:]))
	return ok && slices.Equal(got[i+1:], want[i+1:])
}

// checkKindChange sets the collection c to a scalar; it reports whether the scanner refused it.
func checkKindChange(t *testing.T, f *corpusFile, c collectionAt) (unsupported bool) {
	t.Helper()
	d := mustParse(t, f.src)
	if err := d.Set(c.path, sentinelString); errors.Is(err, ErrUnsupportedScalar) {
		return true
	} else if err != nil {
		t.Errorf("Set(%s): %v", c.path, err)
		return false
	}
	out, err := d.Write()
	if err != nil {
		t.Errorf("Write after Set(%s): %v", c.path, err)
		return false
	}
	if got := lookupValue(t, out, c.path); got != sentinelString {
		t.Errorf("%s = %v after re-parse", c.path, got)
	}
	return false
}

// TestCorpusCollectionEdits inserts a key into every mapping of every corpus file and sets every
// collection to a scalar, each on its own; the write self-check guards the rest of the file.
func TestCorpusCollectionEdits(t *testing.T) {
	for _, path := range corpusFiles(t) {
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := loadCorpusFile(t, path)
			cs := collections(mustParse(t, f.src).root.Content[0], nil)
			var (
				mu          sync.Mutex
				unsupported []string
			)
			t.Run("shards", func(t *testing.T) {
				for lo := 0; lo < len(cs); lo += leafShardSize {
					hi := min(lo+leafShardSize, len(cs))
					t.Run(fmt.Sprintf("%d-%d", lo, hi), func(t *testing.T) {
						t.Parallel()
						for i := lo; i < hi; i++ {
							if testing.Short() && i%shortLeafStride != 0 {
								continue
							}
							c := cs[i]
							if c.node.Kind == yaml.MappingNode {
								checkInsert(t, f, c)
							}
							if len(c.path) == 0 || !checkKindChange(t, f, c) {
								continue
							}
							mu.Lock()
							unsupported = append(unsupported, fmt.Sprintf("%s (line %d)", c.path, c.node.Line))
							mu.Unlock()
						}
					})
				}
			})
			if len(unsupported) > 0 {
				t.Logf("%d collections refused with ErrUnsupportedScalar: %s", len(unsupported), strings.Join(unsupported, ", "))
				if path == chartValues {
					t.Errorf("the chart's own values.yaml must be fully editable")
				}
			}
		})
	}
}
