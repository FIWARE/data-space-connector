package roundtrip

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	// leafShardSize leaves run per parallel subtest; the chart's values.yaml has about 1600.
	leafShardSize = 100
	// shortLeafStride keeps every Nth leaf under -short.
	shortLeafStride = 10
	sentinelString  = "dsc-config-sentinel"
	sentinelAlt     = "dsc-config-sentinel-alt"
	sentinelFloat   = 1.5
	// docCommentPrefix starts the helm-docs comment above a chart value.
	docCommentPrefix = "# --"
)

// leafState is what a re-parse must leave unchanged for every leaf but the edited one.
type leafState struct {
	path  string
	kind  yaml.Kind
	tag   string
	value string
}

func leafStates(leaves []Leaf) []leafState {
	out := make([]leafState, len(leaves))
	for i, l := range leaves {
		out[i] = leafState{path: l.Path.String(), kind: l.Node.Kind, tag: l.Node.Tag, value: l.Node.Value}
	}
	return out
}

// sentinelFor returns a value of the same Go type as cur that differs from it; a string for
// null and every other type.
func sentinelFor(cur any) any {
	switch v := cur.(type) {
	case bool:
		return !v
	case int:
		return v + 1
	case float64:
		if f := v + sentinelFloat; f != v && !math.IsNaN(f) {
			return f
		}
		return sentinelFloat
	case string:
		if v == sentinelString {
			return sentinelAlt
		}
	}
	return sentinelString
}

// corpusFile is one parsed corpus file shared read-only by its shards.
type corpusFile struct {
	src      []byte
	lines    []string
	leaves   []Leaf
	states   []leafState
	comments map[int][]string
	// allComments is every comment line of the tree in walk order, independent of any span.
	allComments []string
}

func loadCorpusFile(t *testing.T, path string) *corpusFile {
	t.Helper()
	src := readFile(t, path)
	d := mustParse(t, src)
	if len(d.headerRaw) > 0 || d.bom {
		t.Fatalf("corpus file carries a header or BOM; line numbers would shift")
	}
	f := &corpusFile{src: src, lines: strings.Split(string(src), lineFeed), leaves: Classify(d, nil), comments: map[int][]string{}}
	f.states = leafStates(f.leaves)
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n.LineComment != "" {
			f.comments[n.Line] = append(f.comments[n.Line], n.LineComment)
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(d.root)
	f.allComments = commentLines(d.root)
	return f
}

// commentLines returns the head, line and foot comment lines of the tree at n in walk order.
func commentLines(n *yaml.Node) []string {
	var out []string
	add := func(c string) {
		for _, l := range strings.Split(c, lineFeed) {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
	}
	add(n.HeadComment)
	add(n.LineComment)
	for _, c := range n.Content {
		out = append(out, commentLines(c)...)
	}
	add(n.FootComment)
	return out
}

// nextLeafLine returns the line of the first leaf after leaf i that starts below leaf i's value,
// or 0 when there is none. An edit of leaf i must end above it.
func (f *corpusFile) nextLeafLine(i int) int {
	for _, l := range f.leaves[i+1:] {
		if l.Line > f.leaves[i].Node.Line {
			return l.Line
		}
	}
	return 0
}

// editResult is what checkLeafEdit saw, for the named-case assertions.
type editResult struct {
	unsupported bool
	out         []string
	docComment  bool
}

// checkLeafEdit sets leaf i to a sentinel and checks the written file: only the leaf's own lines
// change, comments stay, and a re-parse sees the sentinel there and every other leaf unchanged.
func checkLeafEdit(t *testing.T, f *corpusFile, i int) editResult {
	t.Helper()
	leaf := f.leaves[i]
	cur, err := decodeValue(leaf.Node)
	if err != nil {
		t.Errorf("%s: decode: %v", leaf.Path, err)
		return editResult{}
	}
	sentinel := sentinelFor(cur)
	d := mustParse(t, f.src)
	if err := d.Set(leaf.Path, sentinel); errors.Is(err, ErrUnsupportedScalar) {
		return editResult{unsupported: true}
	} else if err != nil {
		t.Errorf("Set(%s): %v", leaf.Path, err)
		return editResult{}
	}
	if len(d.patches) != 1 {
		t.Errorf("Set(%s) recorded %d patches, want 1", leaf.Path, len(d.patches))
		return editResult{}
	}
	var p *patch
	for _, p = range d.patches {
	}
	out, err := d.Write()
	if err != nil {
		t.Errorf("Write after Set(%s): %v", leaf.Path, err)
		return editResult{}
	}

	first := bytes.Count(f.src[:p.start], []byte(lineFeed))
	last := bytes.Count(f.src[:p.end], []byte(lineFeed))
	outLines := strings.Split(string(out), lineFeed)
	after := len(f.lines) - last - 1
	if leaf.Node.Line-1 < first || leaf.Node.Line-1 > last {
		t.Errorf("%s: span lines %d-%d miss the node's line %d", leaf.Path, first+1, last+1, leaf.Node.Line)
	}
	if len(outLines) < first+after+1 || !equalLines(outLines[:first], f.lines[:first]) ||
		!equalLines(outLines[len(outLines)-after:], f.lines[last+1:]) {
		t.Errorf("%s: lines outside %d-%d changed", leaf.Path, first+1, last+1)
		return editResult{}
	}
	if next := f.nextLeafLine(i); next > 0 && last+1 >= next {
		t.Errorf("%s: span lines %d-%d reach the next leaf at line %d", leaf.Path, first+1, last+1, next)
	}
	changed := outLines[first : len(outLines)-after]
	if first == last && len(changed) != 1 {
		t.Errorf("%s: a single-line scalar changed %d lines", leaf.Path, len(changed))
	}
	region := strings.Join(changed, lineFeed)
	for ln := first + 1; ln <= last+1; ln++ {
		for _, c := range f.comments[ln] {
			if strings.Count(region, c) < strings.Count(strings.Join(f.lines[first:last+1], lineFeed), c) {
				t.Errorf("%s: line comment %q lost", leaf.Path, c)
			}
		}
	}

	again := mustParse(t, out)
	if got := commentLines(again.root); !equalLines(got, f.allComments) {
		t.Errorf("%s: comments changed: %d comment lines, want %d", leaf.Path, len(got), len(f.allComments))
	}
	n, err := again.Lookup(leaf.Path)
	if err != nil {
		t.Errorf("%s: re-parse Lookup: %v", leaf.Path, err)
		return editResult{}
	}
	if got, err := decodeValue(n); err != nil || !equalValue(got, sentinel) {
		t.Errorf("%s: re-parse gives %#v (%v), want %#v", leaf.Path, got, err, sentinel)
	}
	states := leafStates(Classify(again, nil))
	if len(states) != len(f.states) {
		t.Errorf("%s: re-parse has %d leaves, want %d", leaf.Path, len(states), len(f.states))
		return editResult{}
	}
	for j, s := range states {
		if j != i && s != f.states[j] {
			t.Errorf("%s: leaf %s changed to %+v", leaf.Path, f.states[j].path, s)
		}
	}
	doc := first > 0 && strings.HasPrefix(strings.TrimSpace(f.lines[first-1]), docCommentPrefix)
	return editResult{out: outLines, docComment: doc}
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isScalarLeaf(l Leaf) bool {
	return l.Node.Kind == yaml.ScalarNode || l.Node.Kind == yaml.AliasNode
}

// TestCorpusLeafEdits edits every scalar leaf of every corpus file on its own: editing one value
// changes exactly its lines and leaves its `# --` comment and line comment in place.
func TestCorpusLeafEdits(t *testing.T) {
	for _, path := range corpusFiles(t) {
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := loadCorpusFile(t, path)
			var (
				mu          sync.Mutex
				unsupported []string
				edited      int
				documented  int
			)
			t.Run("shards", func(t *testing.T) {
				for lo := 0; lo < len(f.leaves); lo += leafShardSize {
					hi := min(lo+leafShardSize, len(f.leaves))
					t.Run(fmt.Sprintf("%d-%d", lo, hi), func(t *testing.T) {
						t.Parallel()
						for i := lo; i < hi; i++ {
							if !isScalarLeaf(f.leaves[i]) || (testing.Short() && i%shortLeafStride != 0) {
								continue
							}
							r := checkLeafEdit(t, f, i)
							mu.Lock()
							if r.unsupported {
								unsupported = append(unsupported, fmt.Sprintf("%s (line %d)", f.leaves[i].Path, f.leaves[i].Line))
							}
							if r.out != nil {
								edited++
							}
							if r.docComment {
								documented++
							}
							mu.Unlock()
						}
					})
				}
			})
			if len(unsupported) > 0 {
				t.Logf("%d leaves refused with ErrUnsupportedScalar: %s", len(unsupported), strings.Join(unsupported, ", "))
			}
			if path == chartValues {
				if len(unsupported) > 0 {
					t.Errorf("the chart's own values.yaml must be fully editable")
				}
				if documented == 0 {
					t.Errorf("no edited leaf sits below a %q comment", docCommentPrefix)
				}
			}
			if edited == 0 && len(f.leaves) > 0 {
				t.Errorf("no leaf edited")
			}
		})
	}
}

// findLeaf returns the index of the scalar leaf on line ln whose path ends with suffix, after
// checking that the source line still holds want, so a shifted corpus fails loudly.
func findLeaf(t *testing.T, f *corpusFile, ln int, suffix, want string) int {
	t.Helper()
	if !strings.Contains(f.lines[ln-1], want) {
		t.Fatalf("line %d is %q, want it to hold %q", ln, f.lines[ln-1], want)
	}
	for i, l := range f.leaves {
		if l.Line == ln && isScalarLeaf(l) && strings.HasSuffix(l.Path.String(), suffix) {
			return i
		}
	}
	t.Fatalf("no leaf %s on line %d", suffix, ln)
	return 0
}

func TestNamedLeafEdits(t *testing.T) {
	t.Run("acr multi-line description", func(t *testing.T) {
		const keyLine = 912
		f := loadCorpusFile(t, chartValues)
		r := checkLeafEdit(t, f, findLeaf(t, f, keyLine, ".description", "description:"))
		if r.out == nil {
			t.Fatalf("edit refused or failed")
		}
		valueLine := strings.Repeat(" ", strings.Index(f.lines[keyLine], "OpenID")) + sentinelString
		if r.out[keyLine-1] != f.lines[keyLine-1] || r.out[keyLine] != valueLine || len(r.out) != len(f.lines)-1 {
			t.Errorf("lines %d-%d = %q, want the key line and %q", keyLine, keyLine+1, r.out[keyLine-1:keyLine+1], valueLine)
		}
	})
	consumer := filepath.Join(k3sDir, "consumer.yaml")
	for _, tt := range []struct{ shadowed, live int }{{816, 817}, {845, 846}} {
		t.Run(fmt.Sprintf("duplicate optionalClientScopes line %d", tt.live), func(t *testing.T) {
			f := loadCorpusFile(t, consumer)
			if !strings.Contains(f.lines[tt.shadowed-1], "optionalClientScopes: []") {
				t.Fatalf("line %d is %q, want the shadowed optionalClientScopes", tt.shadowed, f.lines[tt.shadowed-1])
			}
			r := checkLeafEdit(t, f, findLeaf(t, f, tt.live, "optionalClientScopes[0]", "optionalClientScopes: ["))
			if r.out == nil {
				t.Fatalf("edit refused or failed")
			}
			if r.out[tt.shadowed-1] != f.lines[tt.shadowed-1] || r.out[tt.live-1] == f.lines[tt.live-1] {
				t.Errorf("line %d = %q, line %d = %q; want only the live line changed",
					tt.shadowed, r.out[tt.shadowed-1], tt.live, r.out[tt.live-1])
			}
		})
	}
}
