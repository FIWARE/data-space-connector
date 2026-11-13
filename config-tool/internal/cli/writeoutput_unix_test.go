//go:build unix

package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWriteOutputMode(t *testing.T) {
	const restrictiveUmask = 0o077
	tests := []struct {
		name         string
		existingMode fs.FileMode
		want         fs.FileMode
	}{
		{"new file applies the umask", 0, outputFileMode &^ restrictiveUmask},
		{"existing private file stays private", 0o600, 0o600},
		{"existing group-writable file keeps its mode", 0o664, 0o664},
	}
	old := syscall.Umask(restrictiveUmask)
	defer syscall.Umask(old)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "values.yaml")
			if tc.existingMode != 0 {
				if err := os.WriteFile(path, nil, tc.existingMode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, tc.existingMode); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeOutput(&bytes.Buffer{}, path, []byte("a: 1\n")); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != tc.want {
				t.Errorf("mode = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWriteOutputFollowsSymlink(t *testing.T) {
	const content = "a: 1\n"
	tests := []struct {
		name       string
		withTarget bool
		wantErr    bool
	}{
		{"replaces the link target", true, false},
		{"dangling link", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "real.yaml")
			link := filepath.Join(dir, "values.yaml")
			if tc.withTarget {
				if err := os.WriteFile(target, []byte("old\n"), outputFileMode); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			err := writeOutput(&bytes.Buffer{}, link, []byte(content))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			info, err := os.Lstat(link)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("%s is no longer a symlink", link)
			}
			if tc.wantErr {
				return
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Errorf("target = %q, want %q", got, content)
			}
		})
	}
}
