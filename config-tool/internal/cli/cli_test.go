package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/atomicfile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/internal/trial"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
	trialdata "github.com/FIWARE/data-space-connector/config-tool/trial"
)

// validProfile is a complete profile against the shipped 10.16.0 bundle.
const validProfile = `format: 1
model: {chart: data-space-connector, version: 10.16.0, revision: 1}
target:
  type: file
  namespace: provider
  createNamespace: true
  releaseName: provider
  chart: data-space-connector
  chartVersion: 10.16.0
  environment: production
answers: {}
open: []
secrets: {}
`

// writeProfile writes content to a profile file in a temp dir and returns its path.
func writeProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.dscprofile.yaml")
	if err := os.WriteFile(path, []byte(content), roundtrip.ValuesFileMode); err != nil {
		t.Fatal(err)
	}
	return path
}

var errFakeGUI = errors.New("fake GUI")

func fakeGUI(context.Context, GUIOptions) error {
	return errFakeGUI
}

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
			wantOutput: []string{"-p, --profile", "-o, --output", "--" + flagTrial, "--" + flagOutputDir},
		},
		{
			name:    "generate is not implemented yet",
			args:    []string{"generate", "-p", writeProfile(t, validProfile), "-o", filepath.Join(t.TempDir(), "values.yaml")},
			wantErr: render.ErrNotImplemented,
		},
		{
			name:    "no subcommand starts the GUI",
			wantErr: errFakeGUI,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := execute(t, fakeGUI, tc.args...)
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

func TestGenerateRefusesProfile(t *testing.T) {
	tests := []struct {
		name string
		// content is written to a temp profile; "" leaves the path missing.
		content string
		path    string
		wantErr error
		want    []string
	}{
		{name: "missing profile", wantErr: fs.ErrNotExist},
		{name: "wrong suffix", path: "values.yaml", wantErr: profile.ErrSuffix},
		{name: "not a profile", content: "global: {}\n", want: []string{"field global not found"}},
		{
			name:    "unknown bundle",
			content: strings.ReplaceAll(validProfile, "10.16.0", "0.0.1"),
			want:    []string{"invalid profile", "model: bundle data-space-connector@0.0.1 is not shipped", "target.chartVersion"},
		},
		{
			name:    "malformed target",
			content: strings.Replace(validProfile, "namespace: provider", "namespace: Prod_NS", 1),
			want:    []string{"invalid profile", "target.namespace: must consist of lowercase"},
		},
		{
			name:    "unanswered target",
			content: strings.Replace(validProfile, "  type: file\n", "", 1),
			want:    []string{"target.type: is required"},
		},
		{
			name:    "structural error",
			content: strings.Replace(validProfile, "answers: {}", "answers: {db.password: x}", 1),
			want:    []string{"answers.db.password: is not a known answer id"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "absent.dscprofile.yaml")
			switch {
			case tc.path != "":
				path = tc.path
			case tc.content != "":
				path = writeProfile(t, tc.content)
			}
			_, err := execute(t, fakeGUI, "generate", "-p", path)
			if err == nil {
				t.Fatal("generate succeeded, want an error")
			}
			if errors.Is(err, render.ErrNotImplemented) {
				t.Fatalf("error = %v, want the profile refused before rendering", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
			for _, want := range append(tc.want, path) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestGenerateRequiresProfile(t *testing.T) {
	_, err := execute(t, fakeGUI, "generate")
	if err == nil || !strings.Contains(err.Error(), flagProfile) {
		t.Errorf("error = %v, want a missing --%s error", err, flagProfile)
	}
}

// executeSplit runs the command tree like execute but keeps stdout and stderr apart.
func executeSplit(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCommand(fakeGUI)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

// trialOutputs renders the embedded trial as generate --trial does.
func trialOutputs(t *testing.T) []trial.Output {
	t.Helper()
	tr, err := trial.Load(trialdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	outs, err := trial.Render(tr, trialdata.FS, version.Version)
	if err != nil {
		t.Fatal(err)
	}
	return outs
}

func TestGenerateTrial(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, err := executeSplit(t, "generate", "--"+flagTrial, "--"+flagOutputDir, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range trialOutputs(t) {
		path := filepath.Join(dir, o.File)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, o.Values) {
			t.Errorf("%s differs from the rendered trial output", o.File)
		}
		if !strings.Contains(stdout, path+"\n") {
			t.Errorf("stdout does not list %s:\n%s", path, stdout)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != len(trialOutputs(t)) {
		t.Errorf("dir holds %d entries, want %d", len(entries), len(trialOutputs(t)))
	}
	if !strings.Contains(stdout, quickStartURL) {
		t.Errorf("stdout does not point to the Quick Start:\n%s", stdout)
	}
	assertTrialWarnings(t, stderr)
}

// assertTrialWarnings fails t unless stderr carries every production warning.
func assertTrialWarnings(t *testing.T, stderr string) {
	t.Helper()
	for _, w := range trial.Warnings() {
		if !strings.Contains(stderr, warningPrefix+w.Message) {
			t.Errorf("stderr does not carry warning %s:\n%s", w.ID, stderr)
		}
	}
}

func TestGenerateTrialRefusesFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"trial with profile", []string{"--" + flagTrial, "--" + flagProfile, "x.dscprofile.yaml"}, []string{flagTrial, flagProfile}},
		{"trial with output", []string{"--" + flagTrial, "--" + flagOutput, "values.yaml"}, []string{flagTrial, flagOutput}},
		{"profile with output dir", []string{"--" + flagProfile, "x.dscprofile.yaml", "--" + flagOutputDir, "."}, []string{flagProfile, flagOutputDir}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			_, err := execute(t, fakeGUI, append([]string{"generate"}, tc.args...)...)
			if err == nil {
				t.Fatal("generate succeeded, want the flags refused")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("generate wrote %d entries, want none", len(entries))
			}
		})
	}
}

func TestGenerateTrialKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	args := []string{"generate", "--" + flagTrial, "--" + flagOutputDir, dir}
	if _, _, err := executeSplit(t, args...); err != nil {
		t.Fatal(err)
	}
	first := make(map[string][]byte)
	for _, o := range trialOutputs(t) {
		// Mark each file so a rewrite with the same bytes would still show.
		path := filepath.Join(dir, o.File)
		marked := append(slices.Clone(o.Values), "# edited\n"...)
		if err := os.WriteFile(path, marked, roundtrip.ValuesFileMode); err != nil {
			t.Fatal(err)
		}
		first[path] = marked
	}
	stdout, stderr, err := executeSplit(t, args...)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("error = %v, want %v", err, fs.ErrExist)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing written", stdout)
	}
	assertTrialWarnings(t, stderr)
	for path, want := range first {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not list %s", err, path)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, want) {
			t.Errorf("%s changed", path)
		}
	}
}

func TestRootPassesContextToGUI(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, true)
	var got bool
	root := NewRootCommand(func(ctx context.Context, _ GUIOptions) error {
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

func TestNoBrowserFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"default opens a browser", nil, false},
		{"--no-browser only prints the URL", []string{"--" + flagNoBrowser}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got GUIOptions
			_, err := execute(t, func(_ context.Context, opts GUIOptions) error {
				got = opts
				return nil
			}, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got.NoBrowser != tc.want {
				t.Errorf("NoBrowser = %v, want %v", got.NoBrowser, tc.want)
			}
		})
	}
}

func TestRunGUIPrintsLoopbackURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if err := runGUI(ctx, GUIOptions{NoBrowser: true, Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "http://127.0.0.1:") || !strings.Contains(stdout.String(), "/?token=") {
		t.Errorf("stdout = %q, want the loopback token URL", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestRunReportsErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), NewRootCommand(fakeGUI), []string{"generate", "-p", writeProfile(t, validProfile)}, &stderr)
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
				writeExisting(roundtrip.ValuesFileMode)(t, target)
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
		{"existing file is kept", writeExisting(roundtrip.ValuesFileMode), "", fs.ErrExist, existing},
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

func TestReplaceOutput(t *testing.T) {
	const (
		original = "# kept header comment\nglobal:\n  # kept comment\n  domain: old.example # kept line comment\nother: [1, 2]\n"
		edited   = "# kept header comment\nglobal:\n  # kept comment\n  domain: new.example # kept line comment\nother: [1, 2]\n"
		mode     = 0o600
	)
	errEdit := errors.New("edit failed")
	setDomain := func(doc *roundtrip.Document) error {
		return doc.Set(roundtrip.Path{roundtrip.Key("global"), roundtrip.Key("domain")}, "new.example")
	}
	failEdit := func(*roundtrip.Document) error { return errEdit }
	writeFile := func(t *testing.T, path string) {
		if err := os.WriteFile(path, []byte(original), mode); err != nil {
			t.Fatal(err)
		}
	}
	symlink := func(t *testing.T, path string) {
		target := filepath.Join(filepath.Dir(path), "target.yaml")
		writeFile(t, target)
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	tests := []struct {
		name     string
		setup    func(t *testing.T, path string)
		edit     func(*roundtrip.Document) error
		wantErr  error
		wantFile string
	}{
		{"edit keeps untouched bytes", writeFile, setDomain, nil, edited},
		{"failing edit leaves the file", writeFile, failEdit, errEdit, original},
		{"symlink is refused", symlink, setDomain, atomicfile.ErrSymlink, original},
		{"missing file", func(*testing.T, string) {}, setDomain, fs.ErrNotExist, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "values.yaml")
			tc.setup(t, path)
			err := replaceOutput(path, tc.edit)
			if (tc.wantErr == nil) != (err == nil) || !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantFile != "" {
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != tc.wantFile {
					t.Errorf("file = %q, want %q", got, tc.wantFile)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != mode {
					t.Errorf("mode = %v, want %v", info.Mode().Perm(), os.FileMode(mode))
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".") {
					t.Errorf("temp file %q left behind", e.Name())
				}
			}
		})
	}
}
