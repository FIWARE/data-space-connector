package trial

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// symlinkEntry stands for a symlink in a directory snapshot.
const symlinkEntry = "<symlink>"

// snapshot maps each entry of dir to its content, or to symlinkEntry.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink != 0 {
			out[e.Name()] = symlinkEntry
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(data)
	}
	return out
}

func TestWriteAll(t *testing.T) {
	const (
		kept     = "kept: true\n"
		keptMode = 0o600
	)
	outs := render(t)
	rendered := make(map[string]string, len(outs))
	for _, o := range outs {
		rendered[o.File] = string(o.Values)
	}
	trustAnchor, provider, consumer := outs[0].File, outs[1].File, outs[2].File
	existing := func(names ...string) func(t *testing.T, dir string) {
		return func(t *testing.T, dir string) {
			for _, n := range names {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(kept), keptMode); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		// outs edits a copy of the rendered outputs; nil writes them unchanged.
		outs    func([]Output)
		wantErr error
		// wantListed are the paths, relative to dir, the error must name.
		wantListed []string
		want       map[string]string
	}{
		{name: "empty dir writes all", want: rendered},
		{
			name:       "existing target writes nothing",
			setup:      existing(provider),
			wantErr:    fs.ErrExist,
			wantListed: []string{provider},
			want:       map[string]string{provider: kept},
		},
		{
			name:       "every existing target is listed",
			setup:      existing(trustAnchor, consumer),
			wantErr:    fs.ErrExist,
			wantListed: []string{trustAnchor, consumer},
			want:       map[string]string{trustAnchor: kept, consumer: kept},
		},
		{
			name: "dangling symlink target is refused",
			setup: func(t *testing.T, dir string) {
				if err := os.Symlink(filepath.Join(dir, "absent.yaml"), filepath.Join(dir, consumer)); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantErr:    fs.ErrExist,
			wantListed: []string{consumer},
			want:       map[string]string{consumer: symlinkEntry},
		},
		{
			name:    "failure in the middle removes created files",
			outs:    func(o []Output) { o[2].File = filepath.Join("absent", o[2].File) },
			wantErr: fs.ErrNotExist,
			want:    map[string]string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			in := slices.Clone(outs)
			if tc.outs != nil {
				tc.outs(in)
			}
			paths, err := WriteAll(dir, in)
			if (tc.wantErr == nil) != (err == nil) || !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			for _, rel := range tc.wantListed {
				if !strings.Contains(err.Error(), filepath.Join(dir, rel)) {
					t.Errorf("error %q does not list %s", err, rel)
				}
			}
			if tc.wantErr == nil {
				want := make([]string, len(in))
				for i, o := range in {
					want[i] = filepath.Join(dir, o.File)
				}
				if !slices.Equal(paths, want) {
					t.Errorf("paths = %v, want %v", paths, want)
				}
			}
			if got := snapshot(t, dir); !maps.Equal(got, tc.want) {
				t.Errorf("dir = %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(tc.want)))
			}
		})
	}
}
