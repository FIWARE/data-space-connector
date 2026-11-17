package roundtrip

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

const duplicateKeysFixture = "duplicate-keys.yaml"

func TestPathStringParse(t *testing.T) {
	tests := []struct {
		name string
		path Path
		want string
	}{
		{name: "root", path: Path{}, want: ""},
		{name: "keys and index", path: Path{Key("a"), Key("b"), Index(0)}, want: "a.b[0]"},
		{name: "nested indexes", path: Path{Key("a"), Index(1), Index(12), Key("c")}, want: "a[1][12].c"},
		{name: "leading index", path: Path{Index(0), Key("a")}, want: "[0].a"},
		{name: "dotted key", path: Path{Key("a"), Key("x.y")}, want: `a."x.y"`},
		{name: "bracket key", path: Path{Key("x[0]")}, want: `"x[0]"`},
		{name: "closing bracket key", path: Path{Key("x]")}, want: `"x]"`},
		{name: "quote in key", path: Path{Key(`a"b`)}, want: `"a\"b"`},
		{name: "backslash in key", path: Path{Key(`a\b`)}, want: `a\b`},
		{name: "backslash and quote", path: Path{Key(`a\"`)}, want: `"a\\\""`},
		{name: "empty key", path: Path{Key("")}, want: `""`},
		{name: "empty key inside", path: Path{Key("a"), Key(""), Key("b")}, want: `a."".b`},
		{name: "merge key", path: Path{Key("m"), Key("<<")}, want: "m.<<"},
		{name: "spaces and unicode", path: Path{Key("größe x"), Key("☕")}, want: "größe x.☕"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.path.String(); got != tt.want {
				t.Fatalf("String = %q, want %q", got, tt.want)
			}
			got, err := ParsePath(tt.want)
			if err != nil {
				t.Fatalf("ParsePath: %v", err)
			}
			if !slices.Equal(got, tt.path) {
				t.Errorf("ParsePath = %#v, want %#v", got, tt.path)
			}
		})
	}
}

func TestParsePathInvalid(t *testing.T) {
	for _, s := range []string{
		`"a`, `a."b`, `"a\"`, `"a\x"`, `"a"b`, `a""`,
		`a[`, `a[]`, `a[x]`, `a[-1]`, `a[01]`, `a[+1]`, `a[99999999999999999999]`, `a[0]b`,
		`.a`, `a.`, `a..b`, `a]`, `a"b`, `a.[0]`,
	} {
		t.Run(s, func(t *testing.T) {
			if p, err := ParsePath(s); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("ParsePath = %#v, %v; want ErrInvalidPath", p, err)
			}
		})
	}
}

func FuzzPathRoundTrip(f *testing.F) {
	f.Add("a", uint16(0), "x.y")
	f.Add(`a\"`, uint16(3), "")
	f.Fuzz(func(t *testing.T, a string, i uint16, b string) {
		p := Path{Key(a), Index(int(i)), Key(b), Key(a)}
		got, err := ParsePath(p.String())
		if err != nil || !slices.Equal(got, p) {
			t.Fatalf("ParsePath(%q) = %#v, %v; want %#v", p.String(), got, err, p)
		}
	})
}

func TestLookup(t *testing.T) {
	const src = `a:
  b: [x, {c: 1}]
  "x.y": dotted
  "x[0]": bracket
base: &base {k: v}
ref: *base
seq:
  - *base
  - plain
m:
  <<: *base
  y: 2
`
	d := mustParse(t, []byte(src))
	tests := []struct {
		path      string
		wantKind  yaml.Kind
		wantValue string
		wantErr   error
	}{
		{path: "", wantKind: yaml.MappingNode},
		{path: "a.b[0]", wantKind: yaml.ScalarNode, wantValue: "x"},
		{path: "a.b[1].c", wantKind: yaml.ScalarNode, wantValue: "1"},
		{path: `a."x.y"`, wantKind: yaml.ScalarNode, wantValue: "dotted"},
		{path: `a."x[0]"`, wantKind: yaml.ScalarNode, wantValue: "bracket"},
		{path: "base.k", wantKind: yaml.ScalarNode, wantValue: "v"},
		{path: "ref", wantKind: yaml.AliasNode, wantValue: "base"},
		{path: "seq[0]", wantKind: yaml.AliasNode, wantValue: "base"},
		{path: "seq[1]", wantKind: yaml.ScalarNode, wantValue: "plain"},
		{path: "m.<<", wantKind: yaml.AliasNode, wantValue: "base"},
		{path: "ref.k", wantErr: ErrAliasPath},
		{path: "seq[0].k", wantErr: ErrAliasPath},
		{path: "m.<<.k", wantErr: ErrAliasPath},
		{path: "m.k", wantErr: ErrNotFound},
		{path: "a.missing", wantErr: ErrNotFound},
		{path: "a.b[2]", wantErr: ErrNotFound},
		{path: "a[0]", wantErr: ErrNotFound},
		{path: "a.b.c", wantErr: ErrNotFound},
		{path: "base.k.z", wantErr: ErrNotFound},
		{path: "x.y", wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			p, err := ParsePath(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			n, err := d.Lookup(p)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Lookup error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			if n.Kind != tt.wantKind || n.Value != tt.wantValue {
				t.Errorf("Lookup = kind %v value %q, want kind %v value %q", n.Kind, n.Value, tt.wantKind, tt.wantValue)
			}
		})
	}
	if _, err := mustParse(t, nil).Lookup(nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup on an empty document = %v, want ErrNotFound", err)
	}
}

// leafSummary is a leaf reduced to what the fixture tables compare.
type leafSummary struct {
	path string
	line int
	kind yaml.Kind
}

func summarize(leaves []Leaf) []leafSummary {
	var out []leafSummary
	for _, l := range leaves {
		out = append(out, leafSummary{l.Path.String(), l.Line, l.Node.Kind})
	}
	return out
}

func TestClassifyFixtures(t *testing.T) {
	const (
		scalar  = yaml.ScalarNode
		alias   = yaml.AliasNode
		mapping = yaml.MappingNode
	)
	tests := []struct {
		fixture string
		want    []leafSummary
	}{
		{fixture: "anchors.yaml", want: []leafSummary{
			{"defaults.image", 5, scalar},
			{"defaults.replicas", 6, scalar},
			{"provider.<<", 10, alias},
			{"provider.replicas", 11, scalar},
			{"provider.sidecar", 12, alias},
			{"provider.items[0].name", 14, scalar},
			{"provider.items[0].port", 14, scalar},
			{"provider.items[1]", 15, alias},
			{"consumer", 18, alias},
		}},
		{fixture: "tags.yaml", want: []leafSummary{
			{"explicitString", 2, scalar},
			{"explicitInt", 3, scalar},
			{"binary", 4, scalar},
			{"custom.key", 6, scalar},
			{"set.a", 7, scalar},
			{"set.b", 7, scalar},
			{"anchoredTagged", 8, scalar},
			{"reused", 9, alias},
		}},
		{fixture: "flow.yaml", want: []leafSummary{
			{"empty", 2, mapping},
			{"emptyList", 3, yaml.SequenceNode},
			{"inline[0]", 4, scalar},
			{"inline[1]", 4, scalar},
			{"inline[2]", 4, scalar},
			{"nested.a[0]", 5, scalar},
			{"nested.a[1]", 5, scalar},
			{"nested.b.c", 5, scalar},
			{"multiLine[0]", 7, scalar},
			{"multiLine[1]", 8, scalar},
			{"mapMulti.x", 11, scalar},
			{"mapMulti.y", 12, scalar},
		}},
		{fixture: duplicateKeysFixture, want: []leafSummary{
			{"a[0]", 3, scalar},
			{"b", 4, scalar},
			{"c.e", 8, alias},
			{"c.f", 9, scalar},
		}},
		{fixture: "non-scalar-keys.yaml", want: []leafSummary{
			{"x", 2, scalar},
			{"k", 4, scalar},
			{"plain", 9, scalar},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			d := mustParse(t, readFile(t, filepath.Join(testdataDir, tt.fixture)))
			leaves := Classify(d, nil)
			if got := summarize(leaves); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Classify =\n%v\nwant\n%v", got, tt.want)
			}
			assertLeavesResolve(t, d, leaves)
		})
	}
	if got := Classify(mustParse(t, []byte("# only a comment\n")), nil); got != nil {
		t.Errorf("Classify on an empty document = %v, want none", got)
	}
}

// assertLeavesResolve checks that Lookup finds every leaf's node at its path, with the default class.
func assertLeavesResolve(t *testing.T, d *Document, leaves []Leaf) {
	t.Helper()
	for _, l := range leaves {
		n, err := d.Lookup(l.Path)
		if err != nil || n != l.Node {
			t.Errorf("Lookup(%s) = %p, %v; want leaf node %p", l.Path, n, err, l.Node)
		}
		if l.Class != ClassUnknown {
			t.Errorf("leaf %s has class %v under DefaultClassifier", l.Path, l.Class)
		}
	}
}

// countLeaves counts leaves of a decoded tree independently of Classify.
func countLeaves(v any) int {
	n := 0
	switch v := v.(type) {
	case map[string]any:
		for _, sub := range v {
			n += countLeaves(sub)
		}
	case []any:
		for _, sub := range v {
			n += countLeaves(sub)
		}
	default:
		return 1
	}
	return max(n, 1)
}

func TestClassifyCorpus(t *testing.T) {
	sawConsumer := false
	for _, path := range corpusFiles(t) {
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
		// consumer.yaml holds duplicate keys that Decode(&any) refuses; it must be counted too.
		sawConsumer = sawConsumer || name == "k3s/consumer.yaml"
		t.Run(name, func(t *testing.T) {
			d := mustParse(t, readFile(t, path))
			v, err := decodeValue(d.root)
			if err != nil {
				t.Fatalf("decodeValue: %v", err)
			}
			leaves := Classify(d, nil)
			if want := countLeaves(v); len(leaves) != want {
				t.Errorf("Classify found %d leaves, decoded tree has %d", len(leaves), want)
			}
			assertLeavesResolve(t, d, leaves)
		})
	}
	if !sawConsumer {
		t.Error("k3s/consumer.yaml is not in the corpus")
	}
}

func TestDuplicateKeys(t *testing.T) {
	d := mustParse(t, readFile(t, filepath.Join(testdataDir, duplicateKeysFixture)))
	var raw any
	if err := d.root.Decode(&raw); err == nil {
		t.Fatal("fixture has no duplicate keys: Decode(&any) accepted it")
	}
	a, err := d.Lookup(Path{Key("a")})
	if err != nil {
		t.Fatal(err)
	}
	if a.Line != 3 || len(a.Content) != 1 {
		t.Errorf("Lookup(a) = line %d with %d items, want the second a", a.Line, len(a.Content))
	}
	got, err := decodeValue(d.root)
	if err != nil {
		t.Fatalf("decodeValue: %v", err)
	}
	want := map[string]any{
		"a": []any{"x"},
		"b": 1,
		"c": map[string]any{"e": map[string]any{"port": 80}, "f": 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decodeValue = %#v, want %#v", got, want)
	}
	if d.root.Decode(&raw) == nil {
		t.Error("decodeValue changed the source tree")
	}
}

func TestClassifyCustomClassifier(t *testing.T) {
	d := mustParse(t, []byte("owned: 1\nrecognized: [a]\nother: {}\n"))
	classes := map[string]Class{"owned": ClassOwned, "recognized[0]": ClassRecognized}
	var seen []string
	leaves := Classify(d, func(p Path, n *yaml.Node) Class {
		seen = append(seen, p.String())
		return classes[p.String()]
	})
	if want := []string{"owned", "recognized[0]", "other"}; !slices.Equal(seen, want) {
		t.Errorf("classifier called for %v, want %v", seen, want)
	}
	for _, l := range leaves {
		if l.Class != classes[l.Path.String()] {
			t.Errorf("leaf %s class = %v, want %v", l.Path, l.Class, classes[l.Path.String()])
		}
	}
}

func TestClassString(t *testing.T) {
	for c, want := range map[Class]string{
		ClassUnknown: "unknown", ClassRecognized: "recognized", ClassOwned: "owned", Class(7): "Class(7)",
	} {
		if got := c.String(); got != want {
			t.Errorf("Class(%d).String() = %q, want %q", int(c), got, want)
		}
	}
}

func TestClassifyMergeKeyIsOneLeaf(t *testing.T) {
	d := mustParse(t, []byte("m:\n  <<: [{x: 1}, {y: 2}]\n  w: 4\nq:\n  \"<<\": {z: 3}\n"))
	want := []leafSummary{{"m.<<", 2, yaml.SequenceNode}, {"m.w", 3, yaml.ScalarNode}, {"q.<<.z", 5, yaml.ScalarNode}}
	if got := summarize(Classify(d, nil)); !reflect.DeepEqual(got, want) {
		t.Errorf("Classify = %v, want %v", got, want)
	}
}

// skipSummary is a SkippedPair with its parent as a string and its key as source text.
type skipSummary struct {
	parent     string
	key        string
	line       int
	shadowedBy int
	reason     SkipReason
}

func TestSkipped(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		want []skipSummary
	}{
		{name: duplicateKeysFixture, src: readFile(t, filepath.Join(testdataDir, duplicateKeysFixture)), want: []skipSummary{
			{"", "a", 2, 3, SkipShadowed},
			{"", "c", 5, 7, SkipShadowed},
		}},
		{name: "non-scalar-keys.yaml", src: readFile(t, filepath.Join(testdataDir, "non-scalar-keys.yaml")), want: []skipSummary{
			{"", "k", 3, 0, SkipNonScalarKey},
			{"", "", 5, 0, SkipNonScalarKey},
			{"", "", 7, 0, SkipNonScalarKey},
		}},
		{name: "duplicate inside a shadowed subtree", src: []byte("a:\n  x: 1\n  x: 2\na: 3\n"), want: []skipSummary{
			{"", "a", 1, 4, SkipShadowed},
		}},
		{name: "duplicate inside a live subtree", src: []byte("b:\n  - y: 1\n    y: 2\n"), want: []skipSummary{
			{"b[0]", "y", 2, 3, SkipShadowed},
		}},
		{name: "none", src: []byte("a: 1\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []skipSummary
			for _, s := range Skipped(mustParse(t, tt.src)) {
				got = append(got, skipSummary{s.Parent.String(), s.Key.Value, s.Line, s.ShadowedBy, s.Reason})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Skipped =\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

func TestSkippedCorpus(t *testing.T) {
	d := mustParse(t, readFile(t, filepath.Join(k3sDir, "consumer.yaml")))
	if len(Skipped(d)) == 0 {
		t.Error("Skipped found no pair in k3s/consumer.yaml, which has duplicate keys")
	}
}
