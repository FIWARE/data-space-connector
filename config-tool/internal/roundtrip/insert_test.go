package roundtrip

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// edit is one Set in a test sequence.
type edit struct {
	path  string
	value any
}

// applyEdits runs the Sets on src and returns the written bytes.
func applyEdits(t *testing.T, src string, edits ...edit) string {
	t.Helper()
	d := mustParse(t, []byte(src))
	for _, e := range edits {
		if err := d.Set(mustPath(t, e.path), e.value); err != nil {
			t.Fatalf("Set(%s, %#v): %v", e.path, e.value, err)
		}
	}
	return string(mustWrite(t, d))
}

type m = map[string]any

type l = []any

func TestSetInsert(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		edits []edit
		want  string
	}{
		{name: "top-level key", src: "a: 1\n", edits: []edit{{"b", 2}}, want: "a: 1\nb: 2\n"},
		{name: "nested parents created", src: "a: 1\n", edits: []edit{{"b.c.d", 1}}, want: "a: 1\nb:\n  c:\n    d: 1\n"},
		{name: "into nested mapping", src: "a:\n  b: 1\nx: 2\n", edits: []edit{{"a.c", "v"}}, want: "a:\n  b: 1\n  c: v\nx: 2\n"},
		{name: "after foot comment of the last child", src: "a:\n  b:\n    c: 1\n    # d: 2\nx:", edits: []edit{{"a.e", 1}},
			want: "a:\n  b:\n    c: 1\n    # d: 2\n  e: 1\nx:"},
		{name: "before commented-out examples", src: "a:\n  # -- b doc\n  b: 1\n  # -- an example\n  # c: 2\n\nx: 1\n", edits: []edit{{"a.d", 2}},
			want: "a:\n  # -- b doc\n  b: 1\n  d: 2\n  # -- an example\n  # c: 2\n\nx: 1\n"},
		{name: "before a blank line", src: "a:\n  b: 1\n\n  # c\nx: 1\n", edits: []edit{{"a.d", 2}}, want: "a:\n  b: 1\n  d: 2\n\n  # c\nx: 1\n"},
		{name: "after keep-chomped block", src: "a:\n  s: |+\n    x\n\nb: 1\n", edits: []edit{{"a.t", 1}}, want: "a:\n  s: |+\n    x\n\n  t: 1\nb: 1\n"},
		{name: "after literal with hash lines", src: "a:\n  s: |\n    # not a comment\n    echo\nx: 1", edits: []edit{{"a.t", 1}},
			want: "a:\n  s: |\n    # not a comment\n    echo\n  t: 1\nx: 1"},
		{name: "after multi-line plain", src: "a:\n  s: one\n    two\nx: 1\n", edits: []edit{{"a.t", 1}}, want: "a:\n  s: one\n    two\n  t: 1\nx: 1\n"},
		{name: "after nested list", src: "a:\n  l:\n    - x\n    - y: 1\n      z: 2\nb: 1\n", edits: []edit{{"a.t", 1}},
			want: "a:\n  l:\n    - x\n    - y: 1\n      z: 2\n  t: 1\nb: 1\n"},
		{name: "after implicit null with comment", src: "a:\n  b: # c\nx: 1\n", edits: []edit{{"a.t", 1}}, want: "a:\n  b: # c\n  t: 1\nx: 1\n"},
		{name: "into compact item", src: "l:\n  - a: 1\n    b: 2\n  - x\n", edits: []edit{{"l[0].c", 3}}, want: "l:\n  - a: 1\n    b: 2\n    c: 3\n  - x\n"},
		{name: "sibling order", src: "p:\n  x: 1\n", edits: []edit{{"p.b", 2}, {"p.a", 1}}, want: "p:\n  x: 1\n  a: 1\n  b: 2\n"},
		{name: "nested and outer insert at one offset", src: "a:\n  b:\n    c: 1\n", edits: []edit{{"a.z", 2}, {"a.b.d", 1}, {"w", 3}},
			want: "a:\n  b:\n    c: 1\n    d: 1\n  z: 2\nw: 3\n"},
		{name: "map with sorted keys", src: "a: 1\n", edits: []edit{{"b", m{"z": 1, "w": l{"p", m{"q": true, "o": nil}}, "x": m{}}}},
			want: "a: 1\nb:\n  w:\n    - p\n    - o: null\n      q: true\n  x: {}\n  z: 1\n"},
		{name: "keys and strings that need quoting", src: "a: 1\n", edits: []edit{{"b", m{"x: y": "true", "<<": "8080", "": "a\tb"}}},
			want: "a: 1\nb:\n  \"\": \"a\\tb\"\n  \"<<\": \"8080\"\n  'x: y': \"true\"\n"},
		{name: "multi-line string as literal", src: "a: 1\n", edits: []edit{{"b", m{"s": "x\ny\n", "t": "x\ny", "u": "x\n\n"}}},
			want: "a: 1\nb:\n  s: |\n    x\n    y\n  t: |-\n    x\n    y\n  u: |+\n    x\n\n"},
		{name: "list of lists", src: "a: 1\n", edits: []edit{{"b", l{l{1, 2}, l{}}}}, want: "a: 1\nb:\n  - - 1\n    - 2\n  - []\n"},
		{name: "empty flow mapping to block", src: "a: {} # c\nb: 1\n", edits: []edit{{"a.x", 1}}, want: "a: # c\n  x: 1\nb: 1\n"},
		{name: "implicit null to block", src: "a:\nb: 1\n", edits: []edit{{"a.c", 1}}, want: "a:\n  c: 1\nb: 1\n"},
		{name: "explicit null to block", src: "a: null # c\n", edits: []edit{{"a.c", 1}}, want: "a: # c\n  c: 1\n"},
		{name: "tilde to block", src: "a: ~\n", edits: []edit{{"a.c", 1}}, want: "a:\n  c: 1\n"},
		{name: "anchored null to block", src: "a: &x\nb: *x\n", edits: []edit{{"a.c", 1}}, want: "a: &x\n  c: 1\nb: *x\n"},
		{name: "append into flow mapping", src: "a: {x: 1} # c\n", edits: []edit{{"a.z", 3}, {"a.w", "p,q"}}, want: "a: {x: 1, w: \"p,q\", z: 3} # c\n"},
		{name: "append into flow mapping in flow sequence", src: "l: [{x: 1}, 2]\n", edits: []edit{{"l[0].w", m{"z": l{1}}}}, want: "l: [{x: 1, w: {z: [1]}}, 2]\n"},
		{name: "append nested into empty flow in flow", src: "a: {b: {}}\n", edits: []edit{{"a.b.c", 1}}, want: "a: {b: {c: 1}}\n"},
		{name: "edits below an inserted key", src: "z: 1\n", edits: []edit{{"a.b", 1}, {"a.c", 2}, {"a.b", 3}}, want: "z: 1\na:\n  b: 3\n  c: 2\n"},
		{name: "edit inside an inserted map", src: "z: 1\n", edits: []edit{{"x", m{"v": 1, "w": 2}}, {"x.v", "s"}}, want: "z: 1\nx:\n  v: s\n  w: 2\n"},
		{name: "edit inside a replacing map", src: "x: 1\n", edits: []edit{{"x", m{"v": 1, "w": 2}}, {"x.v", "s"}, {"x.u.t", true}},
			want: "x:\n  u:\n    t: true\n  v: s\n  w: 2\n"},
		{name: "null pending value gains a key", src: "a: 5\n", edits: []edit{{"a", nil}, {"a.b", 1}}, want: "a:\n  b: 1\n"},
		{name: "supersede insert by scalar", src: "a:\n  b:\n    x: 1\n", edits: []edit{{"a.b.c", 2}, {"a.b", "s"}}, want: "a:\n  b: s\n"},
		{name: "supersede insert by map", src: "a:\n  b:\n    x: 1\n", edits: []edit{{"a.b.c", 2}, {"a.b", m{"w": 3}}}, want: "a:\n  b:\n    w: 3\n"},
		{name: "supersede replacement by scalar", src: "a:\n  b:\n    x: 1\n", edits: []edit{{"a.b.x", 5}, {"a.b", "s"}}, want: "a:\n  b: s\n"},
		{name: "ancestor then descendant", src: "a:\n  b:\n    x: 1\n", edits: []edit{{"a.b", m{"w": 3}}, {"a.b.x", 5}}, want: "a:\n  b:\n    w: 3\n    x: 5\n"},
		{name: "no trailing newline", src: "a: 1", edits: []edit{{"b", m{"c": 2}}}, want: "a: 1\nb:\n  c: 2"},
		{name: "no trailing newline nested", src: "a:\n  b: 1", edits: []edit{{"a.c", 2}}, want: "a:\n  b: 1\n  c: 2"},
		{name: "after a clipped block at EOF", src: "a: |\n  x", edits: []edit{{"b", 1}}, want: "a: |-\n  x\nb: 1"},
		{name: "after a kept block at EOF", src: "a:\n  s: >+ # c\n    x", edits: []edit{{"a.t", 1}}, want: "a:\n  s: >- # c\n    x\n  t: 1"},
		{name: "after an indented block at EOF", src: "l:\n  - |2\n     x", edits: []edit{{"b", 1}}, want: "l:\n  - |2-\n     x\nb: 1"},
		{name: "after a stripped block at EOF", src: "a: |-\n  x", edits: []edit{{"b", 1}}, want: "a: |-\n  x\nb: 1"},
		{name: "empty document", src: "", edits: []edit{{"a.b", 1}, {"c", 2}}, want: "a:\n  b: 1\nc: 2\n"},
		{name: "comment-only document", src: "# only a comment\n", edits: []edit{{"a", 1}}, want: "# only a comment\na: 1\n"},
		{name: "comment-only document without newline", src: "# c", edits: []edit{{"a", 1}}, want: "# c\na: 1"},
		{name: "leading marker only", src: "---\n", edits: []edit{{"a", 1}}, want: "---\na: 1\n"},
		{name: "before trailing start marker", src: "a: 1\n---\n", edits: []edit{{"b", 2}}, want: "a: 1\nb: 2\n---\n"},
		{name: "before trailing end marker", src: "a: 1\n...\n", edits: []edit{{"b", 2}}, want: "a: 1\nb: 2\n...\n"},
		{name: "CRLF", src: "a: 1\r\nk: v\r\n", edits: []edit{{"b", m{"c": "x\ny"}}, {"k", l{1}}},
			want: "a: 1\r\nk:\r\n  - 1\r\nb:\r\n  c: |-\r\n    x\r\n    y\r\n"},
		{name: "BOM and header kept", src: "\xef\xbb\xbf# Generated by dsc-config 1 for chart 2\n# Keys under a \"dsc-config:owned\" marker are re-rendered on every save.\n\na: 1\n",
			edits: []edit{{"b", 2}}, want: "\xef\xbb\xbf# Generated by dsc-config 1 for chart 2\n# Keys under a \"dsc-config:owned\" marker are re-rendered on every save.\n\na: 1\nb: 2\n"},
		{name: "merge key insert", src: "base: &a\n  x: 1\nm:\n  <<: *a\n  y: 2\n", edits: []edit{{"m.x", 5}}, want: "base: &a\n  x: 1\nm:\n  <<: *a\n  y: 2\n  x: 5\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyEdits(t, tt.src, tt.edits...); got != tt.want {
				t.Errorf("Write =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestSetReplaceCollection(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		path  string
		value any
		want  string
	}{
		{name: "scalar to map", src: "k: v # c\nx: 1\n", path: "k", value: m{"a": 1}, want: "k: # c\n  a: 1\nx: 1\n"},
		{name: "map to scalar", src: "k: # c\n  a: 1\nx: 1\n", path: "k", value: "s", want: "k: s # c\nx: 1\n"},
		{name: "map to scalar at EOF", src: "k:\n  a: 1", path: "k", value: "s", want: "k: s"},
		{name: "scalar to map at EOF", src: "k: v", path: "k", value: m{"a": 1}, want: "k:\n  a: 1"},
		{name: "plain below key to map", src: "k:\n  long plain\n  text\nx: 1\n", path: "k", value: m{"a": 1}, want: "k:\n  a: 1\nx: 1\n"},
		{name: "plain below key to empty list", src: "k:\n  long plain\n  text\nx: 1\n", path: "k", value: l{}, want: "k: []\nx: 1\n"},
		{name: "multi-line plain to map", src: "k: one\n  two # c\nx: 1\n", path: "k", value: m{"a": 1}, want: "k: # c\n  a: 1\nx: 1\n"},
		{name: "literal to map", src: "k: |\n  x\ny: 1\n", path: "k", value: m{"a": 1}, want: "k:\n  a: 1\ny: 1\n"},
		{name: "literal to empty map", src: "k: | # c\n  x\ny: 1\n", path: "k", value: m{}, want: "k: {} # c\ny: 1\n"},
		{name: "map to map", src: "k:\n  # inner comment\n  a: 1\n  b:\n    c: 2\n  # foot\n# top-level comment\nx: 1\n", path: "k", value: m{"z": 1},
			want: "k:\n  z: 1\n# top-level comment\nx: 1\n"},
		{name: "list to list", src: "k: # c\n  - a\n  - b\nx: 1\n", path: "k", value: l{"c"}, want: "k: # c\n  - c\nx: 1\n"},
		{name: "list to empty list", src: "k: # c\n  - a\n", path: "k", value: l{}, want: "k: [] # c\n"},
		{name: "map to list", src: "k:\n  a: 1\n", path: "k", value: l{1, m{"b": 2}}, want: "k:\n  - 1\n  - b: 2\n"},
		{name: "empty list to list", src: "k: [] # c\n", path: "k", value: l{1}, want: "k: # c\n  - 1\n"},
		{name: "flow list stays flow", src: "k: [a, b] # c\n", path: "k", value: l{"c"}, want: "k: [c] # c\n"},
		{name: "flow map stays flow", src: "k: {a: 1}\n", path: "k", value: m{"b": l{"x y", "p,q"}}, want: "k: {b: [x y, \"p,q\"]}\n"},
		{name: "anchored flow map", src: "k: &x {a: 1}\ny: *x\n", path: "k", value: m{"a": 2}, want: "k: &x {a: 2}\ny: *x\n"},
		{name: "anchored map to scalar", src: "k: &x\n  a: 1\n", path: "k", value: "s", want: "k: &x s\n"},
		{name: "anchored map with comment to scalar", src: "k: &x # c\n  a: 1\n", path: "k", value: "s", want: "k: &x s # c\n"},
		{name: "tagged map to scalar", src: "k: !!map\n  a: 1\n", path: "k", value: "s", want: "k: s\n"},
		{name: "tagged map stays tagged", src: "k: !!map\n  a: 1\n", path: "k", value: m{"b": 1}, want: "k: !!map\n  b: 1\n"},
		{name: "tagged scalar to map", src: "k: !!str v\n", path: "k", value: m{"a": 1}, want: "k:\n  a: 1\n"},
		{name: "anchored tagged scalar to map", src: "k: &x !!str v\n", path: "k", value: m{"a": 1}, want: "k: &x\n  a: 1\n"},
		{name: "tagged anchored scalar to map", src: "k: !!str &x v\n", path: "k", value: m{"a": 1}, want: "k: &x\n  a: 1\n"},
		{name: "tagged scalar to string", src: "k: !!str v\n", path: "k", value: "w", want: "k: !!str w\n"},
		{name: "multi-line flow to scalar", src: "k: [\n  1,\n  2\n]  # c\nx: 1\n", path: "k", value: "s", want: "k: s  # c\nx: 1\n"},
		{name: "multi-line flow to list", src: "a:\n  k: [\n    1, # one\n    2\n  ]\n", path: "a.k", value: l{3, m{"b": "<x>"}},
			want: "a:\n  k: [\n    3,\n    {\n      \"b\": \"<x>\"\n    }\n  ]\n"},
		{name: "alias leaf to map", src: "b: &b {a: 1}\nm: *b # c\n", path: "m", value: m{"a": 2}, want: "b: &b {a: 1}\nm: # c\n  a: 2\n"},
		{name: "compact item to scalar", src: "l:\n  - a: 1\n    b: 2\n  - x\n", path: "l[0]", value: "s", want: "l:\n  - s\n  - x\n"},
		{name: "compact item to map", src: "l:\n  - a: 1\n    b: 2\n  - x\n", path: "l[0]", value: m{"c": 3, "d": 4}, want: "l:\n  - c: 3\n    d: 4\n  - x\n"},
		{name: "compact nested list", src: "l:\n- - a\n  - b\n", path: "l[0]", value: l{"c"}, want: "l:\n- - c\n"},
		{name: "compact item at EOF", src: "l:\n  - a: 1", path: "l[0]", value: l{1}, want: "l:\n  - - 1"},
		{name: "anchored item to scalar", src: "l:\n  - &x\n    a: 1\n", path: "l[0]", value: "s", want: "l:\n  - &x s\n"},
		{name: "item on the next line to scalar", src: "l:\n  - # c\n    a: 1\n  - x\n", path: "l[0]", value: "s", want: "l:\n  - s # c\n  - x\n"},
		{name: "scalar item to map", src: "l:\n  - x\n  - y\n", path: "l[0]", value: m{"a": 1, "b": 2}, want: "l:\n  - a: 1\n    b: 2\n  - y\n"},
		{name: "flow item to map", src: "l: [x, y]\n", path: "l[1]", value: m{"a": 1}, want: "l: [x, {a: 1}]\n"},
		{name: "duplicate key last wins", src: "a: [y]\na: [z]\n", path: "a", value: l{"w"}, want: "a: [y]\na: [w]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyEdits(t, tt.src, edit{tt.path, tt.value}); got != tt.want {
				t.Errorf("Write =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestSetInsertErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		edits   []edit
		wantErr error
	}{
		{name: "index out of range", src: "a: [x, y]\n", edits: []edit{{"a[5]", 1}}, wantErr: ErrNotFound},
		{name: "index below a missing parent", src: "a: 1\n", edits: []edit{{"b.c[0]", 1}}, wantErr: ErrNotFound},
		{name: "key below a scalar", src: "a: 5\n", edits: []edit{{"a.b", 1}}, wantErr: ErrNotMapping},
		{name: "key below a pending scalar", src: "a: 5\n", edits: []edit{{"a", "s"}, {"a.b", 1}}, wantErr: ErrNotMapping},
		{name: "key below a pending list", src: "z: 1\n", edits: []edit{{"a", l{1}}, {"a.b", 1}}, wantErr: ErrNotMapping},
		{name: "through an alias to a mapping", src: "base: &b\n  a: 1\nm: *b\n", edits: []edit{{"m.b", 3}}, wantErr: ErrAliasPath},
		{name: "merge key value", src: "base: &a {x: 1}\nm:\n  <<: *a\n", edits: []edit{{"m.<<", 1}}, wantErr: ErrAliasPath},
		{name: "empty path in an empty document", src: "", edits: []edit{{"", 1}}, wantErr: ErrNotFound},
		{name: "root mapping replaced", src: "a: 1\n", edits: []edit{{"", l{1}}}, wantErr: ErrUnsupportedScalar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, []byte(tt.src))
			last := len(tt.edits) - 1
			for _, e := range tt.edits[:last] {
				if err := d.Set(mustPath(t, e.path), e.value); err != nil {
					t.Fatalf("Set(%s): %v", e.path, err)
				}
			}
			before := pendingEdits(d)
			e := tt.edits[last]
			if err := d.Set(mustPath(t, e.path), e.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Set(%s) = %v, want %v", e.path, err, tt.wantErr)
			}
			if pendingEdits(d) != before {
				t.Errorf("a refused Set changed the pending edits")
			}
		})
	}
}

// TestSetInsertIdempotent writes a key, re-parses the output and sets the same value again.
func TestSetInsertIdempotent(t *testing.T) {
	tests := []edit{
		{"a.b", 1}, {"new.deep.key", m{"x": l{1, "two"}}}, {"list", l{m{"a": 1}}}, {"a.empty", m{}},
	}
	const src = "a:\n  c: 1 # c\nz: 2\n"
	for _, e := range tests {
		t.Run(e.path, func(t *testing.T) {
			first := applyEdits(t, src, e)
			if first == src {
				t.Fatalf("Set(%s) changed nothing", e.path)
			}
			if again := applyEdits(t, first, e); again != first {
				t.Errorf("Set again =\n%q\nwant\n%q", again, first)
			}
			d := mustParse(t, []byte(first))
			if err := d.Set(mustPath(t, e.path), e.value); err != nil || pendingEdits(d) != 0 {
				t.Errorf("Set again: %v, %d edits; want no edit", err, pendingEdits(d))
			}
		})
	}
}

func TestSetInsertDeterministic(t *testing.T) {
	const src = "a:\n  b: 1\nl: []\n"
	edits := []edit{{"x.y", 1}, {"a.c", m{"q": l{1, 2}, "p": "s"}}, {"l", l{m{"k": "v"}}}, {"a.b", m{"n": nil}}, {"x.w", true}}
	first := applyEdits(t, src, edits...)
	for range 3 {
		if got := applyEdits(t, src, edits...); got != first {
			t.Fatalf("same edits gave different bytes:\n%s\n---\n%s", first, got)
		}
	}
}

func TestLayoutDetection(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "two spaces, indented dashes", src: "a:\n  b:\n    c: 1\nl:\n  - x\n",
			want: "new:\n  m:\n    - 1\n    - p: 2\n      q:\n        - 3\n"},
		{name: "four spaces, indented dashes", src: "a:\n    b:\n        c: 1\nl:\n    - x\n",
			want: "new:\n    m:\n        - 1\n        - p: 2\n          q:\n              - 3\n"},
		{name: "two spaces, indentless dashes", src: "a:\n  b: 1\nl:\n- x\nk:\n  m:\n  - y\n",
			want: "new:\n  m:\n  - 1\n  - p: 2\n    q:\n    - 3\n"},
		{name: "four spaces, indentless dashes", src: "a:\n    b: 1\nl:\n- x\n",
			want: "new:\n    m:\n    - 1\n    - p: 2\n      q:\n      - 3\n"},
		{name: "no nesting falls back to two", src: "a: 1\n", want: "new:\n  m:\n    - 1\n    - p: 2\n      q:\n        - 3\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyEdits(t, tt.src, edit{"new", m{"m": l{1, m{"p": 2, "q": l{3}}}}})
			if want := tt.src + tt.want; got != want {
				t.Errorf("Write =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestWriteRefusesChangedNeighbour covers edits whose output would change a value they do not
// own; the self-check refuses them.
func TestWriteRefusesChangedNeighbour(t *testing.T) {
	tests := []struct {
		name string
		src  string
		edit edit
	}{
		// The merge key would inherit from a scalar, which does not decode.
		{name: "anchor used by a merge key", src: "a: &x\n  p: 1\nm:\n  <<: *x\n", edit: edit{"a", "s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, []byte(tt.src))
			if err := d.Set(mustPath(t, tt.edit.path), tt.edit.value); err != nil {
				t.Fatal(err)
			}
			if got, err := d.Write(); !errors.Is(err, ErrPatchVerification) || got != nil {
				t.Fatalf("Write = %q, %v; want no bytes and ErrPatchVerification", got, err)
			}
		})
	}
}

// TestReplaceAnchoredValue checks that aliases follow a replaced anchor site.
func TestReplaceAnchoredValue(t *testing.T) {
	d := mustParse(t, []byte("k: &x {a: 1}\ny: *x\n"))
	if err := d.Set(Path{Key("k")}, m{"a": 2}); err != nil {
		t.Fatal(err)
	}
	again := mustParse(t, mustWrite(t, d))
	n, err := again.Lookup(Path{Key("y")})
	if err != nil {
		t.Fatal(err)
	}
	if v, err := decodeValue(n); err != nil || !equalValue(v, m{"a": 2}) {
		t.Errorf("y = %v, %v; want {a: 2}", v, err)
	}
}

// TestReplaceKeepsKeyLine sets users.admin in the chart's values.yaml, whose key line carries a
// line comment, to a new list.
func TestReplaceKeepsKeyLine(t *testing.T) {
	src := readFile(t, chartValues)
	const keyLine = "          admin: # maintainer"
	d := mustParse(t, src)
	var target Path
	for _, leaf := range Classify(d, nil) {
		if len(leaf.Path) < 2 {
			continue
		}
		if k, _ := leaf.Path[len(leaf.Path)-2].Key(); k == "admin" && leaf.Line == lineOf(src, keyLine)+1 {
			target = leaf.Path[:len(leaf.Path)-1]
		}
	}
	if target == nil {
		t.Fatalf("users.admin not found")
	}
	if err := d.Set(target, l{"login"}); err != nil {
		t.Fatal(err)
	}
	got := mustWrite(t, d)
	want := bytes.Replace(src, []byte(keyLine+"\n            - superuser\n            - createdb\n"), []byte(keyLine+"\n            - login\n"), 1)
	if bytes.Equal(want, src) || !bytes.Equal(got, want) {
		t.Errorf("output differs from the expected two-line change\n%s", firstDiff(got, want))
	}
}

// lineOf returns the 1-based line of the first line equal to text, or 0.
func lineOf(src []byte, text string) int {
	for i, line := range strings.Split(string(src), lineFeed) {
		if line == text {
			return i + 1
		}
	}
	return 0
}

// firstDiff describes the first line where got and want differ.
func firstDiff(got, want []byte) string {
	g, w := strings.Split(string(got), lineFeed), strings.Split(string(want), lineFeed)
	for i := range min(len(g), len(w)) {
		if g[i] != w[i] {
			return fmt.Sprintf("line %d: got %q, want %q", i+1, g[i], w[i])
		}
	}
	return "lengths differ"
}
