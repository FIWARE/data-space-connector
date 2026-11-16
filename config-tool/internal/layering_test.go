package internal

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/FIWARE/data-space-connector/config-tool"

// frontEnds are the only packages under internal/ allowed to speak HTTP; everything else is core.
var frontEnds = []string{"server", "cli", "launcher"}

// deniedImports returns the import paths core packages must not use directly.
// Transitive imports are fine: client-go pulls net/http in on its own.
func deniedImports() []string {
	denied := []string{"net/http"}
	for _, fe := range frontEnds {
		denied = append(denied, modulePath+"/internal/"+fe)
	}
	return denied
}

func isDenied(path string) bool {
	for _, d := range deniedImports() {
		if path == d || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	return false
}

// layeringViolations lists "file: import" for every core file under root that imports a denied path.
func layeringViolations(root string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (isFrontEnd(root, path) || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if isDenied(importPath) {
				rel, _ := filepath.Rel(root, path)
				violations = append(violations, filepath.ToSlash(rel)+": "+importPath)
			}
		}
		return nil
	})
	return violations, err
}

func isFrontEnd(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	for _, fe := range frontEnds {
		if filepath.ToSlash(rel) == fe {
			return true
		}
	}
	return false
}

func TestCoreDoesNotImportFrontEnds(t *testing.T) {
	violations, err := layeringViolations(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("core package imports HTTP or a front end: %s", v)
	}
}

func TestLayeringViolations(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "core without HTTP",
			files: map[string]string{"profile/p.go": `import "fmt"`},
		},
		{
			name:  "core importing net/http",
			files: map[string]string{"render/r.go": `import "net/http"`},
			want:  []string{"render/r.go: net/http"},
		},
		{
			name:  "new core package importing an http subpackage",
			files: map[string]string{"brandnew/sub/n.go": `import "net/http/httputil"`},
			want:  []string{"brandnew/sub/n.go: net/http/httputil"},
		},
		{
			name:  "core importing a front end",
			files: map[string]string{"model/m.go": `import "` + modulePath + `/internal/server"`},
			want:  []string{"model/m.go: " + modulePath + "/internal/server"},
		},
		{
			name: "front ends may use HTTP",
			files: map[string]string{
				"server/s.go":     `import "net/http"`,
				"launcher/l.go":   `import "net/http"`,
				"cli/c.go":        `import "` + modulePath + `/internal/server"`,
				"server/sub/x.go": `import "net/http"`,
			},
		},
		{
			name:  "core tests may use HTTP",
			files: map[string]string{"cluster/c_test.go": `import "net/http/httptest"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, imports := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				src := "package " + filepath.Base(filepath.Dir(path)) + "\n\n" + imports + "\n"
				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := layeringViolations(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("violations = %v, want %v", got, tc.want)
			}
		})
	}
}
