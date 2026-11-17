package roundtrip

import (
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// mergeAndWrite applies the Merges to src in order and returns the written bytes.
func mergeAndWrite(t *testing.T, src string, overrides ...map[string]any) string {
	t.Helper()
	d := mustParse(t, []byte(src))
	for _, o := range overrides {
		if err := Merge(d, o); err != nil {
			t.Fatalf("Merge(%v): %v", o, err)
		}
	}
	return string(mustWrite(t, d))
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		overrides []map[string]any
		want      string
	}{
		{name: "nested override and insert", src: "a:\n  b: 1 # c\n  c:\n    d: x\nz: 0\n", overrides: []map[string]any{{"a": m{"b": 2, "c": m{"e": true}}, "f": l{1}}},
			want: "a:\n  b: 2 # c\n  c:\n    d: x\n    e: true\nz: 0\nf:\n  - 1\n"},
		{name: "map over a scalar", src: "a: 5\n", overrides: []map[string]any{{"a": m{"b": 1}}}, want: "a:\n  b: 1\n"},
		{name: "scalar over a map", src: "a:\n  b: 1\n", overrides: []map[string]any{{"a": "s"}}, want: "a: s\n"},
		{name: "list replaced whole", src: "a: # c\n  - 1\n  - 2\n", overrides: []map[string]any{{"a": l{3}}}, want: "a: # c\n  - 3\n"},
		{name: "nil writes null", src: "a: 1\n", overrides: []map[string]any{{"a": nil}}, want: "a: null\n"},
		{name: "empty map over a missing key", src: "a: 1\n", overrides: []map[string]any{{"b": m{}}}, want: "a: 1\nb: {}\n"},
		{name: "empty map over a map", src: "a:\n  b: 1\n", overrides: []map[string]any{{"a": m{}}}, want: "a:\n  b: 1\n"},
		{name: "second merge changes the type of an edited subtree", src: "a:\n  b:\n    x: 1\n",
			overrides: []map[string]any{{"a": m{"b": m{"x": 2, "w": 3}}}, {"a": m{"b": "s"}}}, want: "a:\n  b: s\n"},
		{name: "second merge edits below the first", src: "a: 1\n",
			overrides: []map[string]any{{"b": m{"c": 1}}, {"b": m{"d": 2}, "a": m{"e": 3}}}, want: "a:\n  e: 3\nb:\n  c: 1\n  d: 2\n"},
		{name: "empty document", src: "", overrides: []map[string]any{{"b": 1, "a": m{"c": "x"}}}, want: "a:\n  c: x\nb: 1\n"},
		{name: "alias to a mapping, changed", src: "base: &b\n  a: 1\n  b: 2\nm: *b\nn: *b\n", overrides: []map[string]any{{"m": m{"b": 3}}},
			want: "base: &b\n  a: 1\n  b: 2\nm:\n  a: 1\n  b: 3\nn: *b\n"},
		{name: "alias to a mapping, equal", src: "base: &b\n  a: 1\n  b: 2\nm: *b\nn: *b\n", overrides: []map[string]any{{"m": m{"a": 1}}},
			want: "base: &b\n  a: 1\n  b: 2\nm: *b\nn: *b\n"},
		{name: "alias to a mapping with non-string keys", src: "base: &b {1: x, a: y, ~: v}\nm: *b\n", overrides: []map[string]any{{"m": m{"a": "z"}}},
			want: "base: &b {1: x, a: y, ~: v}\nm:\n  \"1\": x\n  a: z\n  \"null\": v\n"},
		{name: "alias to a mapping with nested non-string keys", src: "base: &b\n  p: {1: x, a: y}\nm: *b\n", overrides: []map[string]any{{"m": m{"p": m{"a": "z"}}}},
			want: "base: &b\n  p: {1: x, a: y}\nm:\n  p:\n    \"1\": x\n    a: z\n"},
		{name: "merge key override", src: "base: &a\n  x: 1\nm:\n  <<: *a\n  w: 2\n", overrides: []map[string]any{{"m": m{"x": 1}}},
			want: "base: &a\n  x: 1\nm:\n  <<: *a\n  w: 2\n  x: 1\n"},
		{name: "map over a map inherited through a merge key", src: "base: &a\n  x:\n    y: 0\n    z: 2\nm:\n  <<: *a\n  w: 1\n",
			overrides: []map[string]any{{"m": m{"x": m{"y": 1}}}},
			want:      "base: &a\n  x:\n    y: 0\n    z: 2\nm:\n  <<: *a\n  w: 1\n  x:\n    \"y\": 1\n    z: 2\n"},
		{name: "map over a scalar inherited through a merge key", src: "base: &a\n  x: 0\nm:\n  <<: *a\n",
			overrides: []map[string]any{{"m": m{"x": m{"y": 1}}}},
			want:      "base: &a\n  x: 0\nm:\n  <<: *a\n  x:\n    \"y\": 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeAndWrite(t, tt.src, tt.overrides...); got != tt.want {
				t.Errorf("Write =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestMergeIdempotent(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		overrides map[string]any
	}{
		{name: "merge key", src: "base: &a\n  x: 1\nm:\n  <<: *a\n  w: 2\n", overrides: m{"m": m{"x": 1, "v": 2}}},
		{name: "map inherited through a merge key", src: "base: &a\n  x:\n    y: 0\n    z: 2\nm:\n  <<: *a\n  w: 1\n", overrides: m{"m": m{"x": m{"y": 1}}}},
		{name: "nested", src: "a:\n  b: 1\n", overrides: m{"a": m{"c": m{"d": l{1, m{"e": "f"}}}}, "g": true}},
		{name: "type change", src: "a: 5 # c\n", overrides: m{"a": m{"b": 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := mergeAndWrite(t, tt.src, tt.overrides)
			if first == tt.src {
				t.Fatalf("Merge changed nothing")
			}
			if again := mergeAndWrite(t, tt.src, tt.overrides); again != first {
				t.Errorf("two Merges of one input differ:\n%q\n%q", first, again)
			}
			d := mustParse(t, []byte(first))
			if err := Merge(d, tt.overrides); err != nil || pendingEdits(d) != 0 {
				t.Errorf("Merge on the output: %v, %d edits; want none", err, pendingEdits(d))
			}
		})
	}
}

// TestMergeMissingSubtreeLikeSet checks that a missing subtree merged key by key gives the bytes
// of one Set of the subtree as a map.
func TestMergeMissingSubtreeLikeSet(t *testing.T) {
	const src = "a:\n  b: 1\n"
	sub := m{"x": m{"y": l{1, 2}, "z": "s"}, "w": nil}
	merged := mergeAndWrite(t, src, m{"a": m{"c": sub}})
	set := applyEdits(t, src, edit{"a.c", sub})
	keyByKey := applyEdits(t, src, edit{"a.c.w", nil}, edit{"a.c.x.z", "s"}, edit{"a.c.x.y", l{1, 2}})
	if merged != set || keyByKey != set {
		t.Errorf("Merge, Set and key-by-key Set differ:\n%q\n%q\n%q", merged, set, keyByKey)
	}
}

func TestMergeErrors(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		overrides map[string]any
		wantErr   error
	}{
		{name: "root is a list", src: "- a\n", overrides: m{"a": 1}, wantErr: ErrNotMapping},
		{name: "merge key value", src: "base: &a {x: 1}\nm:\n  <<: *a\n", overrides: m{"m": m{"<<": 1}}, wantErr: ErrAliasPath},
		{name: "unsupported value", src: "a: 1\n", overrides: m{"a": uint8(1)}, wantErr: ErrUnsupportedValue},
		{name: "alias keys colliding as strings", src: "base: &b {1.0: x, 1: y}\nm: *b\n", overrides: m{"m": m{"a": 1}}, wantErr: ErrUnsupportedValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Merge(mustParse(t, []byte(tt.src)), tt.overrides); !errors.Is(err, tt.wantErr) {
				t.Errorf("Merge = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestMergeKeyLookup(t *testing.T) {
	d := mustParse(t, []byte("base: &a\n  x: 1\nm:\n  <<: *a\n  w: 2\n"))
	if _, err := d.Lookup(Path{Key("m"), Key("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup(m.x) = %v, want ErrNotFound", err)
	}
}

// findPath returns the path of the first live key named key, from the leaves below it.
func findPath(t *testing.T, d *Document, key string) Path {
	t.Helper()
	for _, leaf := range Classify(d, nil) {
		for i, seg := range leaf.Path {
			if k, ok := seg.Key(); ok && k == key {
				return leaf.Path[:i+1]
			}
		}
	}
	t.Fatalf("no key %q", key)
	return nil
}

// splice replaces the lines [from, to] (0-based, inclusive) of src with repl.
func splice(src []string, from, to int, repl ...string) string {
	out := append(slices.Clone(src[:from]), repl...)
	return strings.Join(append(out, src[to+1:]...), lineFeed)
}

// indexOfLine returns the 0-based index of the first line at or after from equal to text.
func indexOfLine(t *testing.T, lines []string, from int, text string) int {
	t.Helper()
	for i := from; i < len(lines); i++ {
		if lines[i] == text {
			return i
		}
	}
	t.Fatalf("line %q not found", text)
	return 0
}

// TestMultiLineFlowInChart rewrites the chart's multi-line flow values as multi-line JSON.
func TestMultiLineFlowInChart(t *testing.T) {
	src := readFile(t, chartValues)
	lines := strings.Split(string(src), lineFeed)
	const (
		contextsLine = "          additionalContexts: ["
		mappingsLine = "            \"mappings\": ["
		scopesLine   = "          scopeMappings: {"
	)
	contexts := indexOfLine(t, lines, 0, contextsLine)
	contextsEnd := indexOfLine(t, lines, contexts, "          ]")
	mappings := indexOfLine(t, lines, contexts, mappingsLine)
	mappingsEnd := indexOfLine(t, lines, mappings, "            ]")
	scopes := indexOfLine(t, lines, contexts, scopesLine)
	scopesEnd := indexOfLine(t, lines, scopes, "          }")
	if !strings.Contains(lines[contexts-1], docCommentPrefix) {
		t.Fatalf("no `# --` comment above additionalContexts")
	}

	t.Run("set additionalContexts", func(t *testing.T) {
		d := mustParse(t, src)
		if err := d.Set(findPath(t, d, "additionalContexts"), l{m{"a": "<b>"}}); err != nil {
			t.Fatal(err)
		}
		want := splice(lines, contexts, contextsEnd, contextsLine, "            {", "              \"a\": \"<b>\"", "            }", "          ]")
		if got := string(mustWrite(t, d)); got != want {
			t.Errorf("output differs: %s", firstDiff([]byte(got), []byte(want)))
		}
	})
	t.Run("merge scopeMappings.mappings", func(t *testing.T) {
		d := mustParse(t, src)
		over := nest(findPath(t, d, "scopeMappings"), m{"mappings": l{m{"scopes": l{"s"}}}})
		if err := Merge(d, over.(map[string]any)); err != nil {
			t.Fatal(err)
		}
		want := splice(lines, mappings, mappingsEnd, mappingsLine, "              {", "                \"scopes\": [", "                  \"s\"",
			"                ]", "              }", "            ]")
		if got := string(mustWrite(t, d)); got != want {
			t.Errorf("output differs: %s", firstDiff([]byte(got), []byte(want)))
		}
	})
	t.Run("merge a new key into scopeMappings", func(t *testing.T) {
		d := mustParse(t, src)
		path := findPath(t, d, "scopeMappings")
		if err := Merge(d, nest(path, m{"extra": true}).(map[string]any)); err != nil {
			t.Fatal(err)
		}
		out := mustWrite(t, d)
		got := strings.Split(string(out), lineFeed)
		after := len(lines) - scopesEnd
		if !slices.Equal(got[:scopes+1], lines[:scopes+1]) || !slices.Equal(got[len(got)-after:], lines[scopesEnd:]) {
			t.Errorf("lines outside the flow mapping changed")
		}
		if body := strings.Join(got[scopes:len(got)-after+1], lineFeed); !strings.Contains(body, "            \"extra\": true,") {
			t.Errorf("new key not rendered as JSON inside the mapping:\n%s", body)
		}
		n, err := mustParse(t, out).Lookup(append(path, Key("extra")))
		if err != nil || n.Value != "true" {
			t.Errorf("Lookup(extra) = %v, %v", n, err)
		}
	})
}

// TestSetDuplicatedFlowList replaces the live one of the two optionalClientScopes in consumer.yaml.
func TestSetDuplicatedFlowList(t *testing.T) {
	src := readFile(t, filepath.Join(k3sDir, "consumer.yaml"))
	lines := strings.Split(string(src), lineFeed)
	const (
		shadowed = "        optionalClientScopes: []"
		live     = "        optionalClientScopes: [\"marketplace-credential\", \"legal-person-credential\"]"
	)
	i := indexOfLine(t, lines, 0, live)
	if lines[i-1] != shadowed {
		t.Fatalf("line before %d is %q", i+1, lines[i-1])
	}
	d := mustParse(t, src)
	var target Path
	for _, leaf := range Classify(d, nil) {
		if leaf.Line == i+1 {
			target = leaf.Path[:len(leaf.Path)-1]
			break
		}
	}
	if err := d.Set(target, l{"x", "y z"}); err != nil {
		t.Fatal(err)
	}
	want := splice(lines, i, i, "        optionalClientScopes: [x, y z]")
	if got := string(mustWrite(t, d)); got != want {
		t.Errorf("output differs: %s", firstDiff([]byte(got), []byte(want)))
	}
}

// referenceMerge is an independent deep merge of decoded values: maps merge, anything else wins.
func referenceMerge(base, over any) any {
	bm, ok1 := base.(map[string]any)
	om, ok2 := over.(map[string]any)
	if !ok1 || !ok2 {
		return over
	}
	out := map[string]any{}
	for k, v := range bm {
		out[k] = v
	}
	for k, v := range om {
		if cur, ok := out[k]; ok {
			out[k] = referenceMerge(cur, v)
		} else {
			out[k] = v
		}
	}
	return out
}

// lineDiff returns the lines only in a and only in b, by longest common subsequence.
func lineDiff(a, b []string) (removed, added []string) {
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			removed, i = append(removed, a[i]), i+1
		default:
			added, j = append(added, b[j]), j+1
		}
	}
	return append(removed, a[i:]...), append(added, b[j:]...)
}

// TestMergeProvider merges a nested override tree into k3s/provider.yaml: only the overridden and
// added lines change, and the result equals a reference deep merge of the decoded values.
func TestMergeProvider(t *testing.T) {
	src := readFile(t, filepath.Join(k3sDir, "provider.yaml"))
	overrides := m{
		"cert-manager": m{"enabled": true},
		"decentralizedIam": m{"vcAuthentication": m{
			"managedPostgres":      m{"config": m{"volume": m{"size": "2Gi"}, "users": m{"ngb": l{"login"}}}},
			"trusted-issuers-list": m{"deployment": m{"resources": m{"limits": m{"cpu": "1"}}}},
		}},
		"zz-added": m{"enabled": true, "list": l{"a"}},
	}
	d := mustParse(t, src)
	if err := Merge(d, overrides); err != nil {
		t.Fatal(err)
	}
	out := mustWrite(t, d)

	removed, added := lineDiff(strings.Split(string(src), lineFeed), strings.Split(string(out), lineFeed))
	wantRemoved := []string{"  enabled: false", "            - createdb", "            cpu: 300m"}
	wantAdded := []string{"  enabled: true", "            - login", "          size: 2Gi", "            cpu: \"1\"",
		"zz-added:", "  enabled: true", "  list:", "    - a"}
	slices.Sort(removed)
	slices.Sort(added)
	slices.Sort(wantRemoved)
	slices.Sort(wantAdded)
	if !slices.Equal(removed, wantRemoved) || !slices.Equal(added, wantAdded) {
		t.Errorf("diff:\nremoved %q\nadded   %q", removed, added)
	}

	before, err := decodeValue(mustParse(t, src).root)
	if err != nil {
		t.Fatal(err)
	}
	after, err := decodeValue(mustParse(t, out).root)
	if err != nil {
		t.Fatal(err)
	}
	if want := referenceMerge(before, maps.Clone(overrides)); !equalValue(after, want) {
		t.Errorf("re-parsed tree differs from the reference merge")
	}
}
