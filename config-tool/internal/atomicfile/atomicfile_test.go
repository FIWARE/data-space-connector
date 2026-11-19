package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

const (
	fileName = "a.yaml"
	newMode  = fs.FileMode(0o644)
	keptMode = fs.FileMode(0o640)
)

var (
	oldData = []byte("old\n")
	newData = []byte("new\n")
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name      string
		existing  bool
		overwrite bool
		wantMode  fs.FileMode
		wantErr   error
	}{
		{name: "new file gets the mode", wantMode: newMode},
		{name: "new file with overwrite", overwrite: true, wantMode: newMode},
		{name: "replace keeps the mode", existing: true, overwrite: true, wantMode: keptMode},
		{name: "existing without overwrite", existing: true, wantErr: ErrExists},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, fileName)
			want := newData
			if tc.existing {
				if err := os.WriteFile(path, oldData, keptMode); err != nil {
					t.Fatal(err)
				}
				// WriteFile's mode is subject to the umask.
				if err := os.Chmod(path, keptMode); err != nil {
					t.Fatal(err)
				}
				if tc.wantErr != nil {
					want = oldData
				}
			}
			err := Write(path, newData, newMode, tc.overwrite)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Write err = %v, want %v", err, tc.wantErr)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("content = %q, want %q", got, want)
			}
			if tc.wantErr == nil && runtime.GOOS != "windows" {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != tc.wantMode {
					t.Errorf("mode = %v, want %v", info.Mode().Perm(), tc.wantMode)
				}
			}
			assertEntries(t, dir, fileName)
		})
	}
}

func TestWriteRefuses(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, dir, path string)
		wantErr error
	}{
		{
			name: "symlink",
			prepare: func(t *testing.T, dir, path string) {
				target := filepath.Join(dir, "target.yaml")
				if err := os.WriteFile(target, oldData, newMode); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantErr: ErrSymlink,
		},
		{
			name: "directory",
			prepare: func(t *testing.T, _, path string) {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrNotRegular,
		},
		{
			name: "missing directory",
			prepare: func(t *testing.T, dir, _ string) {
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: fs.ErrNotExist,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "d")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, fileName)
			tc.prepare(t, dir, path)
			before := entries(t, parent, dir)
			if err := Write(path, newData, newMode, true); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Write err = %v, want %v", err, tc.wantErr)
			}
			if after := entries(t, parent, dir); !slices.Equal(before, after) {
				t.Errorf("entries after = %v, want %v (temp file left behind?)", after, before)
			}
		})
	}
}

// TestWriteRemovesTempOnLateError fails the final rename, after the temp file was written.
func TestWriteRemovesTempOnLateError(t *testing.T) {
	errRename := errors.New("rename failed")
	t.Cleanup(func() { rename = os.Rename })
	rename = func(string, string) error { return errRename }

	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, oldData, newMode); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, newData, newMode, true); !errors.Is(err, errRename) {
		t.Fatalf("Write err = %v, want %v", err, errRename)
	}
	assertEntries(t, dir, fileName)
	if got, _ := os.ReadFile(path); string(got) != string(oldData) {
		t.Errorf("content = %q, want the old %q", got, oldData)
	}
}

// TestWriteWithoutOverwritePublish covers a file created after the existence check, before
// publishing, and the placeholder fallback on filesystems without hard links.
func TestWriteWithoutOverwritePublish(t *testing.T) {
	race := func(newname string) {
		if err := os.WriteFile(newname, oldData, newMode); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name     string
		link     func(oldname, newname string) error
		wantErr  error
		wantData []byte
	}{
		{
			name: "race before link",
			link: func(oldname, newname string) error {
				race(newname)
				return os.Link(oldname, newname)
			},
			wantErr:  ErrExists,
			wantData: oldData,
		},
		{
			name:     "no hard links",
			link:     func(string, string) error { return errors.ErrUnsupported },
			wantData: newData,
		},
		{
			name: "no hard links, race before placeholder",
			link: func(_, newname string) error {
				race(newname)
				return errors.ErrUnsupported
			},
			wantErr:  ErrExists,
			wantData: oldData,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { link = os.Link })
			link = tt.link

			dir := t.TempDir()
			path := filepath.Join(dir, fileName)
			if err := Write(path, newData, newMode, false); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Write err = %v, want %v", err, tt.wantErr)
			}
			assertEntries(t, dir, fileName)
			if got, _ := os.ReadFile(path); string(got) != string(tt.wantData) {
				t.Errorf("content = %q, want %q", got, tt.wantData)
			}
			if info, err := os.Stat(path); err != nil {
				t.Fatal(err)
			} else if runtime.GOOS != "windows" && info.Mode().Perm() != newMode {
				t.Errorf("mode = %v, want %v", info.Mode().Perm(), newMode)
			}
		})
	}
}

// TestWritePlaceholderRemovedOnRenameError leaves neither placeholder nor temp file behind.
func TestWritePlaceholderRemovedOnRenameError(t *testing.T) {
	errRename := errors.New("rename failed")
	t.Cleanup(func() { link, rename = os.Link, os.Rename })
	link = func(string, string) error { return errors.ErrUnsupported }
	rename = func(string, string) error { return errRename }

	dir := t.TempDir()
	if err := Write(filepath.Join(dir, fileName), newData, newMode, false); !errors.Is(err, errRename) {
		t.Fatalf("Write err = %v, want %v", err, errRename)
	}
	assertEntries(t, dir)
}

// entries lists the names in each dir, or nil for a missing one, for before/after comparison.
func entries(t *testing.T, dirs ...string) []string {
	t.Helper()
	var names []string
	for _, dir := range dirs {
		list, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range list {
			names = append(names, filepath.Join(dir, e.Name()))
		}
	}
	return names
}

func assertEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range list {
		got = append(got, e.Name())
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s holds %v, want %v", dir, got, want)
	}
}
