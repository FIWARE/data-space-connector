package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
)

func execute(t *testing.T, runGUI GUIRunner, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand(runGUI)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestCommands(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErr    error
		wantOutput []string
	}{
		{
			name:       "version",
			args:       []string{"--version"},
			wantOutput: []string{"dsc-config " + version.Version + " (" + version.Commit + ")\n"},
		},
		{
			name:       "generate help documents its flags",
			args:       []string{"generate", "--help"},
			wantOutput: []string{"-p, --profile", "-o, --output"},
		},
		{
			name:    "generate is not implemented yet",
			args:    []string{"generate", "-p", "profile.yaml", "-o", "values.yaml"},
			wantErr: render.ErrNotImplemented,
		},
		{
			name:    "no subcommand starts the GUI",
			wantErr: ErrGUIUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := execute(t, stubGUI, tc.args...)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(out, want) {
					t.Errorf("output does not contain %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestGenerateRequiresProfile(t *testing.T) {
	_, err := execute(t, stubGUI, "generate")
	if err == nil || !strings.Contains(err.Error(), flagProfile) {
		t.Errorf("error = %v, want a missing --%s error", err, flagProfile)
	}
}

func TestRootPassesContextToGUI(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, true)
	var got bool
	root := NewRootCommand(func(ctx context.Context) error {
		got, _ = ctx.Value(key{}).(bool)
		return nil
	})
	if code := run(ctx, root, nil, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !got {
		t.Error("GUI runner did not receive the command context")
	}
}

func TestRunReportsErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), NewRootCommand(stubGUI), []string{"generate", "-p", "profile.yaml"}, &stderr)
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), render.ErrNotImplemented.Error()) {
		t.Errorf("stderr = %q, want the render error", stderr.String())
	}
}

func TestWriteOutput(t *testing.T) {
	const (
		content  = "global: {}\n"
		existing = "kept: true\n"
		readOnly = 0o444
	)
	writeExisting := func(mode os.FileMode) func(t *testing.T, path string) {
		return func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(existing), mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	symlink := func(withTarget bool) func(t *testing.T, path string) {
		return func(t *testing.T, path string) {
			target := filepath.Join(filepath.Dir(path), "target.yaml")
			if withTarget {
				writeExisting(outputFileMode)(t, target)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}
	}
	// An empty wantFile means nothing may be readable at the path afterwards.
	tests := []struct {
		name     string
		setup    func(t *testing.T, path string)
		sub      string
		wantErr  error
		wantFile string
	}{
		{"new file", func(*testing.T, string) {}, "", nil, content},
		{"existing file is kept", writeExisting(outputFileMode), "", fs.ErrExist, existing},
		{"read-only file is kept", writeExisting(readOnly), "", fs.ErrExist, existing},
		{"symlink is kept", symlink(true), "", fs.ErrExist, existing},
		{"dangling symlink is kept", symlink(false), "", fs.ErrExist, ""},
		{"missing directory", func(*testing.T, string) {}, "absent", fs.ErrNotExist, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.sub, "values.yaml")
			tc.setup(t, path)
			err := writeOutput(&bytes.Buffer{}, path, []byte(content))
			if (tc.wantErr == nil) != (err == nil) || !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			got, err := os.ReadFile(path)
			if tc.wantFile == "" {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("read %q (err %v), want nothing at the path", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.wantFile {
				t.Errorf("file = %q, want %q", got, tc.wantFile)
			}
		})
	}
}

func TestWriteOutputStdout(t *testing.T) {
	var out bytes.Buffer
	if err := writeOutput(&out, stdoutPath, []byte("a: 1\n")); err != nil {
		t.Fatal(err)
	}
	if out.String() != "a: 1\n" {
		t.Errorf("stdout = %q", out.String())
	}
}
