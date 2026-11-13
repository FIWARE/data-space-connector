package cli

import (
	"bytes"
	"context"
	"errors"
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
	const content = "global: {}\n"
	tests := []struct {
		name     string
		existing string
		path     func(dir string) string
		wantErr  bool
	}{
		{"new file", "", func(dir string) string { return filepath.Join(dir, "values.yaml") }, false},
		{"replaces a longer file", strings.Repeat("x", 4*len(content)), func(dir string) string { return filepath.Join(dir, "values.yaml") }, false},
		{"missing directory", "", func(dir string) string { return filepath.Join(dir, "absent", "values.yaml") }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tc.path(dir)
			if tc.existing != "" {
				if err := os.WriteFile(path, []byte(tc.existing), outputFileMode); err != nil {
					t.Fatal(err)
				}
			}
			err := writeOutput(&bytes.Buffer{}, path, []byte(content))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != content {
					t.Errorf("file = %q, want %q", got, content)
				}
			}
			leftovers, _ := filepath.Glob(filepath.Join(dir, tempPattern))
			if len(leftovers) != 0 {
				t.Errorf("temp files left behind: %v", leftovers)
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
