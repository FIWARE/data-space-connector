package roundtrip

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	repoRoot          = "../../.."
	k3sDir            = repoRoot + "/k3s"
	chartValues       = repoRoot + "/charts/data-space-connector/values.yaml"
	trustAnchorValues = repoRoot + "/charts/trust-anchor/values.yaml"
	testdataDir       = "testdata"
	multiDocFixture   = "multi-document.yaml"
)

// requiredCorpus guards against a moved k3s directory letting the corpus test pass vacuously.
var requiredCorpus = []string{
	"provider.yaml", "consumer.yaml", "trust-anchor.yaml",
	"provider-eidas.yaml", "consumer-gaia-x.yaml", "consent-provider.yaml",
}

var testHeader = Header{ToolVersion: "v0.2.0", ChartVersion: "10.16.0", Profile: "provider-dev", ProfileHash: "sha256:0f1e2d"}

// corpusFiles returns the chart values files a user may load: k3s/*.yaml and both charts' values.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(k3sDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range requiredCorpus {
		if _, err := os.Stat(filepath.Join(k3sDir, name)); err != nil {
			t.Fatalf("corpus file missing: %v", err)
		}
	}
	return append(files, chartValues, trustAnchorValues)
}

// fixtureFiles returns the testdata fixtures.
func fixtureFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(testdataDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures in %s: %v", testdataDir, err)
	}
	return files
}

func readFile(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func toCRLF(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
}

func mustParse(t *testing.T, src []byte) *Document {
	t.Helper()
	d, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

func mustWrite(t *testing.T, d *Document) []byte {
	t.Helper()
	out, err := d.Write()
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return out
}

// assertRoundTrip checks Parse→Write identity, SetHeader output and idempotence for src,
// which carries no BOM and no generated header.
func assertRoundTrip(t *testing.T, src []byte) {
	t.Helper()
	if got := mustWrite(t, mustParse(t, src)); !bytes.Equal(got, src) {
		t.Fatalf("Parse→Write changed the bytes (%d → %d)", len(src), len(got))
	}

	d := mustParse(t, src)
	d.SetHeader(testHeader)
	withHeader := mustWrite(t, d)
	wantHeader := testHeader.Render()
	if d.eol == crlf {
		wantHeader = toCRLF(wantHeader)
	}
	if want := append(wantHeader, src...); !bytes.Equal(withHeader, want) {
		t.Fatalf("SetHeader→Write:\n got %q\nwant %q", head(withHeader), head(want))
	}

	again := mustParse(t, withHeader)
	if again.Header() != testHeader {
		t.Errorf("Header() after re-parse = %+v, want %+v", again.Header(), testHeader)
	}
	if got := mustWrite(t, again); !bytes.Equal(got, withHeader) {
		t.Errorf("Parse(Write(x))→Write differs from Write(x)")
	}
	if n := bytes.Count(withHeader, []byte(HeaderPrefix)); n != 1 {
		t.Errorf("header written %d times, want 1", n)
	}
}

// head shortens b for failure messages.
func head(b []byte) []byte {
	const maxShown = 300
	return b[:min(len(b), maxShown)]
}

func TestCorpusRoundTrip(t *testing.T) {
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			assertRoundTrip(t, readFile(t, path))
		})
	}
}

func TestFixtures(t *testing.T) {
	for _, path := range fixtureFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src := readFile(t, path)
			if filepath.Base(path) == multiDocFixture {
				if _, err := Parse(src); !errors.Is(err, ErrMultiDocument) {
					t.Fatalf("Parse error = %v, want ErrMultiDocument", err)
				}
				return
			}
			assertCommentPositions(t, src)
			if bom := []byte(utf8BOM); bytes.HasPrefix(src, bom) {
				d := mustParse(t, src)
				if got := mustWrite(t, d); !bytes.Equal(got, src) {
					t.Fatalf("Parse→Write changed the bytes")
				}
				d.SetHeader(testHeader)
				want := append(append(bom, testHeader.Render()...), src[len(bom):]...)
				if got := mustWrite(t, d); !bytes.Equal(got, want) {
					t.Fatalf("SetHeader→Write = %q, want %q", head(got), head(want))
				}
				return
			}
			assertRoundTrip(t, src)
		})
	}
}

// assertCommentPositions checks that a fixture exercises head, line and foot comments.
func assertCommentPositions(t *testing.T, src []byte) {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		t.Fatal(err)
	}
	var headC, lineC, footC bool
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		headC = headC || n.HeadComment != ""
		lineC = lineC || n.LineComment != ""
		footC = footC || n.FootComment != ""
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)
	if !headC || !lineC || !footC {
		t.Errorf("fixture lacks a comment position: head=%v line=%v foot=%v", headC, lineC, footC)
	}
}

func TestManifestsNeverDamaged(t *testing.T) {
	var manifests []string
	entries, err := os.ReadDir(k3sDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		err := filepath.WalkDir(filepath.Join(k3sDir, e.Name()), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
				manifests = append(manifests, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(manifests) == 0 {
		t.Fatalf("no manifests under %s/*/", k3sDir)
	}
	for _, path := range manifests {
		t.Run(strings.TrimPrefix(path, k3sDir+"/"), func(t *testing.T) {
			src := readFile(t, path)
			d, err := Parse(src)
			if errors.Is(err, ErrMultiDocument) {
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := mustWrite(t, d); !bytes.Equal(got, src) {
				t.Fatalf("Parse→Write changed the bytes")
			}
		})
	}
}

func TestHeaderDetection(t *testing.T) {
	const body = "a: 1\n"
	line2 := HeaderOwnedLine + "\n"
	rendered := string(testHeader.Render())
	tests := []struct {
		name       string
		src        string
		wantSplit  bool
		wantHeader Header
		wantBody   string
	}{
		{name: "exact", src: rendered + body, wantSplit: true, wantHeader: testHeader, wantBody: body},
		{name: "header only", src: rendered, wantSplit: true, wantHeader: testHeader, wantBody: ""},
		{name: "line 1 only", src: HeaderPrefix + "v1\n" + body, wantBody: HeaderPrefix + "v1\n" + body},
		{name: "line 2 differs", src: HeaderPrefix + "v1\n# other\n\n" + body, wantBody: HeaderPrefix + "v1\n# other\n\n" + body},
		{name: "line 3 not blank", src: HeaderPrefix + "v1\n" + line2 + "# user\n" + body, wantBody: HeaderPrefix + "v1\n" + line2 + "# user\n" + body},
		{name: "line 3 missing", src: HeaderPrefix + "v1\n" + line2, wantBody: HeaderPrefix + "v1\n" + line2},
		{name: "prefix without space", src: "# Generated by dsc-configX\n" + line2 + "\n" + body, wantBody: "# Generated by dsc-configX\n" + line2 + "\n" + body},
		{name: "user comment under header", src: rendered + "# user comment\n" + body, wantSplit: true, wantHeader: testHeader, wantBody: "# user comment\n" + body},
		{name: "header in the middle", src: body + rendered + "b: 2\n", wantBody: body + rendered + "b: 2\n"},
		{name: "CRLF", src: string(toCRLF([]byte(rendered + body))), wantSplit: true, wantHeader: testHeader, wantBody: "a: 1\r\n"},
		{
			name: "hand-edited line 1", src: HeaderPrefix + "v1 (edited by hand)\n" + line2 + "\n" + body,
			wantSplit: true, wantBody: body,
		},
		{
			name: "other field order", src: HeaderPrefix + "v1 from dev for chart 10.0.0 — profile hash abc\n" + line2 + "\n" + body,
			wantSplit: true, wantHeader: Header{ToolVersion: "v1", Profile: "dev for chart 10.0.0", ProfileHash: "abc"}, wantBody: body,
		},
		{
			name: "profile containing a dash", src: string(Header{ToolVersion: "v1", Profile: "a — b", ProfileHash: "h"}.Render()) + body,
			wantSplit: true, wantHeader: Header{ToolVersion: "v1", Profile: "a — b", ProfileHash: "h"}, wantBody: body,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, []byte(tt.src))
			if got := mustWrite(t, d); string(got) != tt.src {
				t.Fatalf("Parse→Write = %q, want %q", got, tt.src)
			}
			if split := len(d.headerRaw) > 0; split != tt.wantSplit {
				t.Errorf("header split = %v, want %v", split, tt.wantSplit)
			}
			if d.Header() != tt.wantHeader {
				t.Errorf("Header() = %+v, want %+v", d.Header(), tt.wantHeader)
			}
			if string(d.body) != tt.wantBody {
				t.Errorf("body = %q, want %q", d.body, tt.wantBody)
			}

			d.SetHeader(testHeader)
			want := []byte(rendered + tt.wantBody)
			if d.eol == crlf {
				want = append(toCRLF([]byte(rendered)), tt.wantBody...)
			}
			got := mustWrite(t, d)
			if !bytes.Equal(got, want) {
				t.Errorf("SetHeader→Write = %q, want %q", got, want)
			}
			if !tt.wantSplit && !strings.Contains(tt.src, rendered) {
				return
			}
			if n := bytes.Count(got, []byte(HeaderPrefix)); n != 1+strings.Count(tt.wantBody, HeaderPrefix) {
				t.Errorf("SetHeader→Write holds %d headers", n)
			}
		})
	}
}

func TestHeaderRenderParse(t *testing.T) {
	tests := []struct {
		name   string
		header Header
		line1  string
	}{
		{name: "all fields", header: testHeader, line1: "# Generated by dsc-config v0.2.0 for chart 10.16.0 from provider-dev — profile hash sha256:0f1e2d"},
		{name: "no chart version", header: Header{ToolVersion: "v1", Profile: "p", ProfileHash: "h"}, line1: "# Generated by dsc-config v1 from p — profile hash h"},
		{name: "chart only", header: Header{ChartVersion: "10.0.0"}, line1: "# Generated by dsc-config for chart 10.0.0"},
		{name: "hash only", header: Header{ProfileHash: "h"}, line1: "# Generated by dsc-config — profile hash h"},
		{name: "profile with separators", header: Header{ToolVersion: "v1", Profile: "x from y — z", ProfileHash: "h"}, line1: "# Generated by dsc-config v1 from x from y — z — profile hash h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.line1 + "\n" + HeaderOwnedLine + "\n\n"
			if got := string(tt.header.Render()); got != want {
				t.Fatalf("Render = %q, want %q", got, want)
			}
			if got := parseHeaderLine(tt.line1); got != tt.header {
				t.Errorf("parseHeaderLine = %+v, want %+v", got, tt.header)
			}
		})
	}
	if got := (Header{}).Render(); got != nil {
		t.Errorf("zero Header renders %q, want nothing", got)
	}
}

func TestEdgeCases(t *testing.T) {
	const bom = utf8BOM
	rendered := string(testHeader.Render())
	old := string(Header{ToolVersion: "v0.1.0", ProfileHash: "old"}.Render())
	tests := []struct {
		name          string
		src           string
		wantErr       error
		wantTail      string
		wantEOL       string
		wantSetHeader string
	}{
		{name: "empty", src: "", wantSetHeader: rendered},
		{name: "comment only", src: "# only a comment\n", wantSetHeader: rendered + "# only a comment\n"},
		{name: "no trailing newline", src: "a: 1\nb: 2", wantSetHeader: rendered + "a: 1\nb: 2"},
		{name: "CRLF", src: "a: 1\r\nb: 2\r\n", wantEOL: crlf, wantSetHeader: string(toCRLF([]byte(rendered))) + "a: 1\r\nb: 2\r\n"},
		{name: "BOM", src: bom + "a: 1\n", wantSetHeader: bom + rendered + "a: 1\n"},
		{name: "leading marker", src: "---\na: 1\n", wantSetHeader: rendered + "---\na: 1\n"},
		{name: "leading marker after comment", src: "# c\n---\na: 1\n", wantSetHeader: rendered + "# c\n---\na: 1\n"},
		{name: "leading marker only", src: "---\n", wantSetHeader: rendered + "---\n"},
		{name: "BOM and header", src: bom + old + "a: 1\n", wantSetHeader: bom + rendered + "a: 1\n"},
		{name: "CRLF and header", src: string(toCRLF([]byte(old + "a: 1\n"))), wantEOL: crlf, wantSetHeader: string(toCRLF([]byte(rendered + "a: 1\n")))},
		{name: "trailing start marker", src: "a: 1\n---\n", wantTail: "---\n"},
		{name: "trailing start marker without newline", src: "a: 1\n---", wantTail: "---"},
		{name: "trailing end marker", src: "a: 1\n...\n", wantTail: "...\n"},
		{name: "trailing end marker without newline", src: "a: 1\n...", wantTail: "..."},
		{name: "end then start marker", src: "a: 1\n...\n---\n", wantTail: "...\n---\n"},
		{name: "start then end marker", src: "a: 1\n---\n...\n", wantTail: "---\n...\n"},
		{name: "blank lines around tail", src: "a: 1\n\n---  \n\n", wantTail: "---  \n\n"},
		{name: "leading and trailing marker", src: "---\na: 1\n---\n", wantTail: "---\n"},
		{name: "comment after start marker", src: "a: 1\n---\n# c\n", wantErr: ErrMultiDocument},
		{name: "comment after end marker", src: "a: 1\n...\n# c\n", wantErr: ErrMultiDocument},
		{name: "second document", src: "a: 1\n---\nb: 2\n", wantErr: ErrMultiDocument},
		{name: "two trailing start markers", src: "a: 1\n---\n---\n", wantErr: ErrMultiDocument},
		{name: "marker with content", src: "a: 1\n--- b\n", wantErr: ErrMultiDocument},
		{name: "two leading markers", src: "---\n---\na: 1\n", wantErr: ErrMultiDocument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Parse([]byte(tt.src))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := mustWrite(t, d); string(got) != tt.src {
				t.Fatalf("Parse→Write = %q, want %q", got, tt.src)
			}
			if string(d.tail) != tt.wantTail {
				t.Errorf("tail = %q, want %q", d.tail, tt.wantTail)
			}
			wantEOL := tt.wantEOL
			if wantEOL == "" {
				wantEOL = lineFeed
			}
			if d.eol != wantEOL {
				t.Errorf("eol = %q, want %q", d.eol, wantEOL)
			}
			if tt.wantSetHeader == "" {
				return
			}
			d.SetHeader(testHeader)
			if got := mustWrite(t, d); string(got) != tt.wantSetHeader {
				t.Errorf("SetHeader→Write = %q, want %q", got, tt.wantSetHeader)
			}
		})
	}
}

func TestSetZeroHeaderRemovesHeader(t *testing.T) {
	d := mustParse(t, append(testHeader.Render(), "a: 1\n"...))
	d.SetHeader(Header{})
	if got := string(mustWrite(t, d)); got != "a: 1\n" {
		t.Errorf("Write = %q, want the body only", got)
	}
}

// TestOffsetMatchesKeys checks the Line/Column → byte mapping on every plain key, including
// keys after multi-byte runes, where yaml.v3's rune-counted Column differs from a byte column.
func TestOffsetMatchesKeys(t *testing.T) {
	files := append(corpusFiles(t), fixtureFiles(t)...)
	for _, path := range files {
		if filepath.Base(path) == multiDocFixture {
			continue
		}
		t.Run(path, func(t *testing.T) {
			d := mustParse(t, readFile(t, path))
			checked := 0
			var walk func(*yaml.Node)
			walk = func(n *yaml.Node) {
				if n.Kind == yaml.MappingNode {
					for i := 0; i < len(n.Content); i += 2 {
						k := n.Content[i]
						if k.Kind != yaml.ScalarNode || k.Style != 0 || k.Anchor != "" || k.Value == "" {
							continue
						}
						off, ok := d.offset(k.Line, k.Column)
						if !ok || !bytes.HasPrefix(d.body[off:], []byte(k.Value)) {
							t.Errorf("key %q at %d:%d does not map to its bytes", k.Value, k.Line, k.Column)
						}
						checked++
					}
				}
				for _, c := range n.Content {
					walk(c)
				}
			}
			walk(d.root)
			if checked == 0 {
				t.Errorf("no keys checked")
			}
		})
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, path := range fixtureFiles(f) {
		f.Add(readFile(f, path))
	}
	for _, seed := range []string{"", "a: 1\n---\n", "a: 1\r\n...", string(testHeader.Render()) + "a: 1\n"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		d, err := Parse(src)
		if err != nil {
			return
		}
		out, err := d.Write()
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if !bytes.Equal(out, src) {
			t.Fatalf("Parse→Write = %q, want %q", out, src)
		}
	})
}
