package roundtrip

import (
	"bytes"
	"errors"
	"math"
	"path/filepath"
	"testing"
)

const nonASCIIFixture = "non-ascii.yaml"

func mustPath(t *testing.T, s string) Path {
	t.Helper()
	p, err := ParsePath(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// pendingEdits counts the pending replacements and insert groups.
func pendingEdits(d *Document) int {
	return len(d.reps) + len(d.groups)
}

// setAndWrite applies one Set to src and returns the written bytes.
func setAndWrite(t *testing.T, src, path string, value any) string {
	t.Helper()
	d := mustParse(t, []byte(src))
	if err := d.Set(mustPath(t, path), value); err != nil {
		t.Fatalf("Set(%s, %#v): %v", path, value, err)
	}
	return string(mustWrite(t, d))
}

func TestSetStyles(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		path  string
		value any
		want  string
	}{
		{name: "plain to plain", src: "a: foo # c\nb: 1\n", path: "a", value: "bar", want: "a: bar # c\nb: 1\n"},
		{name: "plain needs quoting", src: "a: foo\n", path: "a", value: "x: y", want: "a: 'x: y'\n"},
		{name: "plain needs double quotes", src: "a: foo\n", path: "a", value: "x\ny", want: "a: \"x\\ny\"\n"},
		{name: "plain two lines", src: "a: foo\n  bar\nb: 1\n", path: "a", value: "x", want: "a: x\nb: 1\n"},
		{name: "plain three lines with blank", src: "a: foo\n  bar\n\n  baz\nb: 1\n", path: "a", value: "x", want: "a: x\nb: 1\n"},
		{name: "plain multi-line then comment line", src: "a: foo\n  bar\n# c\nb: 1\n", path: "a", value: "x", want: "a: x\n# c\nb: 1\n"},
		{name: "plain multi-line at EOF", src: "a: foo\n  bar", path: "a", value: "x", want: "a: x"},
		{name: "plain multi-line comment on continuation", src: "a: foo\n  bar # c\nb: 1\n", path: "a", value: "x", want: "a: x # c\nb: 1\n"},
		{name: "plain multi-line sequence item", src: "l:\n  - foo\n    bar\n  - z\n", path: "l[0]", value: "x", want: "l:\n  - x\n  - z\n"},
		{name: "anchored list deeper than its anchor", src: "k: &a\n    - foo\n    - z\n", path: "k[0]", value: "w", want: "k: &a\n    - w\n    - z\n"},
		{name: "anchored list multi-line item", src: "longkey: &anchor\n  - foo\n    bar\n", path: "longkey[0]", value: "x", want: "longkey: &anchor\n  - x\n"},
		{name: "anchored list with comment", src: "k: &a # c\n  # d\n  - foo\n", path: "k[0]", value: "x", want: "k: &a # c\n  # d\n  - x\n"},
		{name: "tagged list block item", src: "key: !!seq\n  - |\n    x\n", path: "key[0]", value: "y\n", want: "key: !!seq\n  - |\n    y\n"},
		{name: "compact nested list multi-line item", src: "l:\n- - foo\n    bar\n  - z\n", path: "l[0][0]", value: "x", want: "l:\n- - x\n  - z\n"},
		{name: "plain starting below the key", src: "a:\n  foo\n  bar\nb: 1\n", path: "a", value: "x", want: "a:\n  x\nb: 1\n"},
		{name: "plain in CRLF file", src: "a: foo # c\r\nb: 1\r\n", path: "a", value: "x", want: "a: x # c\r\nb: 1\r\n"},
		{name: "single quoted", src: "a: 'it''s' # c\n", path: "a", value: "x'y", want: "a: 'x''y' # c\n"},
		{name: "single quoted needs double", src: "a: 'x'\n", path: "a", value: "x\ny", want: "a: \"x\\ny\"\n"},
		{name: "double quoted with escapes", src: "a: \"t\\tq\\\"\" # c\n", path: "a", value: "x\ny\"", want: "a: \"x\\ny\\\"\" # c\n"},
		{name: "literal", src: "a: | # c\n  one\n  two\nb: 1\n", path: "a", value: "x\ny\n", want: "a: | # c\n  x\n  y\nb: 1\n"},
		{name: "literal strip", src: "a: |-\n  one\nb: 1\n", path: "a", value: "two", want: "a: |-\n  two\nb: 1\n"},
		{name: "literal changes chomping", src: "a: |\n  one\nb: 1\n", path: "a", value: "x", want: "a: |-\n  x\nb: 1\n"},
		{name: "literal indentation indicator", src: "a: |2-\n    one\nb: 1\n", path: "a", value: "  x", want: "a: |2-\n    x\nb: 1\n"},
		{name: "literal nested", src: "m:\n  a: |\n    one\n", path: "m.a", value: "x\n", want: "m:\n  a: |\n    x\n"},
		{name: "literal in CRLF file", src: "a: |\r\n  one\r\nb: 1\r\n", path: "a", value: "x\ny\n", want: "a: |\r\n  x\r\n  y\r\nb: 1\r\n"},
		{name: "literal cannot hold leading space", src: "a: |\n  one\n", path: "a", value: "  x\n", want: "a: \"  x\\n\"\n"},
		{name: "literal to number", src: "a: | # c\n  one\nb: 1\n", path: "a", value: 5, want: "a: 5 # c\nb: 1\n"},
		{name: "folded", src: "a: >\n  one\n  two\nb: 1\n", path: "a", value: "x y\nz\n", want: "a: >\n  x y\n\n  z\nb: 1\n"},
		{name: "folded keep", src: "a: >+\n  one\n\nb: 1\n", path: "a", value: "x\n\n", want: "a: >+\n  x\n\nb: 1\n"},
		{name: "flow sequence item", src: "a: [x, y]\n", path: "a[1]", value: "z", want: "a: [x, z]\n"},
		{name: "flow sequence item needs quoting", src: "a: [x, y]\n", path: "a[0]", value: "p,q", want: "a: [\"p,q\", y]\n"},
		{name: "flow mapping value", src: "a: {k: v, l: w}\n", path: "a.k", value: "z", want: "a: {k: z, l: w}\n"},
		{name: "anchored scalar", src: "a: &x foo # c\nb: *x\n", path: "a", value: "bar", want: "a: &x bar # c\nb: *x\n"},
		{name: "tagged scalar", src: "a: !!str 8080\n", path: "a", value: "9090", want: "a: !!str 9090\n"},
		{name: "tag dropped on type change", src: "a: !!str 8080\n", path: "a", value: 9090, want: "a: 9090\n"},
		{name: "tag dropped before anchor", src: "a: !!str &x 80\nb: *x\n", path: "a", value: 90, want: "a: &x 90\nb: *x\n"},
		{name: "bool-looking string stays quoted", src: "a: \"true\"\n", path: "a", value: "false", want: "a: \"false\"\n"},
		{name: "YAML 1.1 bool string quoted", src: "a: x\n", path: "a", value: "yes", want: "a: \"yes\"\n"},
		{name: "implicit null", src: "a:\nb: 1\n", path: "a", value: "v", want: "a: v\nb: 1\n"},
		{name: "implicit null with comment", src: "a:   # c\n", path: "a", value: "v", want: "a: v   # c\n"},
		{name: "implicit null sequence item", src: "l:\n  - \n  - x\n", path: "l[0]", value: "v", want: "l:\n  - v \n  - x\n"},
		{name: "implicit null at EOF", src: "a: 1\nb:", path: "b", value: "v", want: "a: 1\nb: v"},
		{name: "anchored implicit null", src: "a: &x\nb: *x\n", path: "a", value: "v", want: "a: &x v\nb: *x\n"},
		{name: "tagged empty string", src: "a: !!str\n", path: "a", value: 5, want: "a: 5\n"},
		{name: "root scalar", src: "foo # c\n", path: "", value: "bar", want: "bar # c\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setAndWrite(t, tt.src, tt.path, tt.value); got != tt.want {
				t.Errorf("Write = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetNonASCII(t *testing.T) {
	src := string(readFile(t, filepath.Join(testdataDir, nonASCIIFixture)))
	tests := []struct {
		path    string
		value   any
		oldLine string
		newLine string
	}{
		{path: "ä", value: "z", oldLine: "ä: x # line comment", newLine: "ä: z # line comment"},
		{path: "flow.ö[0]", value: "ü", oldLine: "  ö: [é, x]", newLine: "  ö: [ü, x]"},
		{path: "flow.ö[1]", value: "ß", oldLine: "  ö: [é, x]", newLine: "  ö: [é, ß]"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := setAndWrite(t, src, tt.path, tt.value)
			want := string(bytes.Replace([]byte(src), []byte(tt.oldLine+"\n"), []byte(tt.newLine+"\n"), 1))
			if got != want || got == src {
				t.Errorf("Write = %q, want %q", got, want)
			}
		})
	}
}

func TestSetTypes(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		value any
		want  string
	}{
		{name: "int over quoted string", src: "x: \"8080\"\n", value: 9090, want: "x: 9090\n"},
		{name: "bool-looking string over plain", src: "x: a\n", value: "true", want: "x: \"true\"\n"},
		{name: "number-looking string over int", src: "x: 8080\n", value: "8080", want: "x: \"8080\"\n"},
		{name: "int64", src: "x: 1\n", value: int64(2), want: "x: 2\n"},
		{name: "bool", src: "x: false # c\n", value: true, want: "x: true # c\n"},
		{name: "nil", src: "x: 1\n", value: nil, want: "x: null\n"},
		{name: "float written as integer", src: "x: 1\n", value: float64(2), want: "x: 2\n"},
		{name: "float", src: "x: 1\n", value: 1.5, want: "x: 1.5\n"},
		{name: "NaN", src: "x: 1\n", value: math.NaN(), want: "x: .nan\n"},
		{name: "negative infinity", src: "x: 1\n", value: math.Inf(-1), want: "x: -.inf\n"},
		{name: "null-looking string", src: "x: 1\n", value: "null", want: "x: \"null\"\n"},
		{name: "empty string", src: "x: 1\n", value: "", want: "x: \"\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setAndWrite(t, tt.src, "x", tt.value); got != tt.want {
				t.Errorf("Write = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetEqualValueAddsNoPatch(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		path  string
		value any
	}{
		{name: "float equal to int", src: "port: 8080\n", path: "port", value: 8080.0},
		{name: "same string", src: "a: 'x'\n", path: "a", value: "x"},
		{name: "null over implicit null", src: "a:\n", path: "a", value: nil},
		{name: "alias resolved value", src: "b: &a 1\nc: *a\n", path: "c", value: 1},
		{name: "inherited duplicate", src: "a: [y]\na: [z]\n", path: "a[0]", value: "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, []byte(tt.src))
			if err := d.Set(mustPath(t, tt.path), tt.value); err != nil {
				t.Fatalf("Set: %v", err)
			}
			if n := pendingEdits(d); n != 0 {
				t.Errorf("%d edits recorded, want none", n)
			}
			if got := string(mustWrite(t, d)); got != tt.src {
				t.Errorf("Write = %q, want the source", got)
			}
		})
	}
}

func TestSetNaNTwice(t *testing.T) {
	d := mustParse(t, []byte("x: 1\n"))
	for range 2 {
		if err := d.Set(Path{Key("x")}, math.NaN()); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	if n := pendingEdits(d); n != 1 {
		t.Errorf("%d edits, want 1", n)
	}
	if got := string(mustWrite(t, d)); got != "x: .nan\n" {
		t.Errorf("Write = %q", got)
	}
}

func TestSetTwiceLastWins(t *testing.T) {
	d := mustParse(t, []byte("a: 1 # c\nb: |\n  x\n"))
	for _, v := range []any{2, "three", 4} {
		if err := d.Set(Path{Key("a")}, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []any{"y\nz\n", 5, "w\n"} {
		if err := d.Set(Path{Key("b")}, v); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := string(mustWrite(t, d)), "a: 4 # c\nb: |\n  w\n"; got != want {
		t.Errorf("Write = %q, want %q", got, want)
	}
	if n, err := d.Lookup(Path{Key("a")}); err != nil || n.Value != "4" {
		t.Errorf("Lookup(a) = %v, %v; want 4", n, err)
	}
}

func TestApplyPatchesConflict(t *testing.T) {
	body := []byte("a: 1234\n")
	tests := []struct {
		name    string
		patches []*patch
		wantErr bool
		want    string
	}{
		{name: "overlapping", patches: []*patch{{start: 3, end: 6}, {start: 5, end: 7}}, wantErr: true},
		{name: "nested", patches: []*patch{{start: 3, end: 7}, {start: 4, end: 5}}, wantErr: true},
		{name: "same start", patches: []*patch{{start: 3, end: 3}, {start: 3, end: 5}}, wantErr: true},
		{name: "adjacent", patches: []*patch{{start: 5, end: 7, text: []byte("y")}, {start: 3, end: 5, text: []byte("x")}}, want: "a: xy\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyPatches(body, tt.patches)
			if tt.wantErr {
				if !errors.Is(err, ErrPatchConflict) || got != nil {
					t.Fatalf("applyPatches = %q, %v; want ErrPatchConflict", got, err)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Errorf("applyPatches = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// TestSelfCheckRefusesWrongType feeds Write a patch whose text has the wrong type.
func TestSelfCheckRefusesWrongType(t *testing.T) {
	d := mustParse(t, []byte("x: \"8080\"\n"))
	if err := d.Set(Path{Key("x")}, 9090); err != nil {
		t.Fatal(err)
	}
	d.reps["x"].leaf.text = []byte(`"9090"`)
	if got, err := d.Write(); !errors.Is(err, ErrPatchVerification) || got != nil {
		t.Fatalf("Write = %q, %v; want no bytes and ErrPatchVerification", got, err)
	}
}

func TestSetDuplicateKey(t *testing.T) {
	if got, want := setAndWrite(t, "a: [y]\na: [z]\n", "a[0]", "x"), "a: [y]\na: [x]\n"; got != want {
		t.Errorf("Write = %q, want %q", got, want)
	}
}

func TestSetAliases(t *testing.T) {
	const src = "b: &a 1\nc: *a\nd: [*a, x]\n"
	tests := []struct {
		path  string
		value any
		want  string
	}{
		{path: "c", value: 2, want: "b: &a 1\nc: 2\nd: [*a, x]\n"},
		{path: "d[0]", value: 2, want: "b: &a 1\nc: *a\nd: [2, x]\n"},
		{path: "b", value: 2, want: "b: &a 2\nc: *a\nd: [*a, x]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := setAndWrite(t, src, tt.path, tt.value); got != tt.want {
				t.Errorf("Write = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetErrors(t *testing.T) {
	const src = "base: &b {x: 1}\nm:\n  <<: *b\n  y: 2\nref: *b\nlist: [1]\nmap: {}\n"
	tests := []struct {
		name    string
		path    Path
		value   any
		wantErr error
	}{
		{name: "index out of range", path: Path{Key("list"), Index(1)}, value: 1, wantErr: ErrNotFound},
		{name: "index below a missing key", path: Path{Key("nope"), Index(0)}, value: 1, wantErr: ErrNotFound},
		{name: "key below a scalar", path: Path{Key("m"), Key("y"), Key("z")}, value: 1, wantErr: ErrNotMapping},
		{name: "key below a sequence", path: Path{Key("list"), Key("z")}, value: 1, wantErr: ErrNotMapping},
		{name: "through an alias", path: Path{Key("ref"), Key("x")}, value: 2, wantErr: ErrAliasPath},
		{name: "merge key value", path: Path{Key("m"), Key("<<")}, value: 2, wantErr: ErrAliasPath},
		{name: "unsupported type", path: Path{Key("m"), Key("y")}, value: uint(3), wantErr: ErrUnsupportedValue},
		{name: "unsupported type in a map", path: Path{Key("m"), Key("y")}, value: map[string]any{"a": []any{uint(3)}}, wantErr: ErrUnsupportedValue},
		{name: "alias leaf below", path: Path{Key("ref"), Key("x"), Key("y")}, value: 2, wantErr: ErrAliasPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, []byte(src))
			if err := d.Set(tt.path, tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Set = %v, want %v", err, tt.wantErr)
			}
			if got := string(mustWrite(t, d)); got != src {
				t.Errorf("a refused Set changed the output: %q", got)
			}
		})
	}
}

func TestSetUnsupportedScalar(t *testing.T) {
	// A flow scalar that crosses lines is not scanned, so its edit is refused, not guessed.
	d := mustParse(t, []byte("a: [x\n  y]\n"))
	if err := d.Set(Path{Key("a"), Index(0)}, "z"); !errors.Is(err, ErrUnsupportedScalar) {
		t.Fatalf("Set = %v, want ErrUnsupportedScalar", err)
	}
}

func TestSetDeterministic(t *testing.T) {
	src := readFile(t, filepath.Join(testdataDir, "anchors.yaml"))
	edits := []struct {
		path  string
		value any
	}{
		{"provider.replicas", 3}, {"defaults.image", "quay.io/fiware/app:2.0"}, {"provider.items[1]", "x"}, {"defaults.replicas", nil},
	}
	var first []byte
	for range 3 {
		d := mustParse(t, src)
		for _, e := range edits {
			if err := d.Set(mustPath(t, e.path), e.value); err != nil {
				t.Fatalf("Set(%s): %v", e.path, err)
			}
		}
		out := mustWrite(t, d)
		if first == nil {
			first = out
		} else if !bytes.Equal(out, first) {
			t.Fatalf("same edits gave different bytes:\n%s\n---\n%s", first, out)
		}
	}
}
