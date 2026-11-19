package recent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const chartVersion = "10.16.0"

var fixedNow = time.Date(2026, 11, 19, 13, 4, 5, 0, time.FixedZone("CET", 3600))

// newStore returns a store in a fresh temp dir and the dir profiles are created in.
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	return &Store{Dir: filepath.Join(t.TempDir(), appDir), Now: func() time.Time { return fixedNow }}, t.TempDir()
}

// touch creates the profile name in dir and returns its path.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name+profile.FileSuffix)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("p%02d", i)
	}
	return out
}

func TestAddOrder(t *testing.T) {
	overCap := numbered(MaxRecent + 2)
	wantCapped := slices.Clone(overCap[2:])
	slices.Reverse(wantCapped)
	tests := []struct {
		name string
		adds []string
		want []string
	}{
		{name: "newest first", adds: []string{"a", "b", "c"}, want: []string{"c", "b", "a"}},
		{name: "re-add moves to front", adds: []string{"a", "b", "c", "a"}, want: []string{"a", "c", "b"}},
		{name: "unclean path dedups", adds: []string{"a", "b", "./a"}, want: []string{"a", "b"}},
		{name: "capped at MaxRecent", adds: overCap, want: wantCapped},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			for _, n := range tc.adds {
				clean := filepath.Base(n)
				touch(t, dir, clean)
				if err := s.Add(Entry{Path: dir + string(filepath.Separator) + n + profile.FileSuffix}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := s.List()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(names(got), tc.want) {
				t.Errorf("List = %v, want %v", names(got), tc.want)
			}
		})
	}
}

func TestAddFillsEntry(t *testing.T) {
	s, dir := newStore(t)
	path := touch(t, dir, "provider")
	if err := s.Add(Entry{Path: path, Name: "ignored", ChartVersion: chartVersion, SavedAt: "ignored"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{Path: path, Name: "provider", ChartVersion: chartVersion, SavedAt: "2026-11-19T12:04:05Z"}}
	if !slices.Equal(got, want) {
		t.Errorf("List = %+v, want %+v", got, want)
	}
}

func TestAddRelativePath(t *testing.T) {
	s, dir := newStore(t)
	t.Chdir(dir)
	rel := "provider" + profile.FileSuffix
	touch(t, dir, "provider")
	if err := s.Add(Entry{Path: rel}); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != want {
		t.Errorf("List = %+v, want one entry with path %s", got, want)
	}
}

func TestListDropsMissingFiles(t *testing.T) {
	tests := []struct {
		name    string
		removed []string
		want    []string
	}{
		{name: "none removed", want: []string{"c", "b", "a"}},
		{name: "middle removed", removed: []string{"b"}, want: []string{"c", "a"}},
		{name: "all removed", removed: []string{"a", "b", "c"}, want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			for _, n := range []string{"a", "b", "c"} {
				if err := s.Add(Entry{Path: touch(t, dir, n)}); err != nil {
					t.Fatal(err)
				}
			}
			for _, n := range tc.removed {
				if err := os.Remove(filepath.Join(dir, n+profile.FileSuffix)); err != nil {
					t.Fatal(err)
				}
			}
			got, err := s.List()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(names(got), tc.want) {
				t.Errorf("List = %v, want %v", names(got), tc.want)
			}
		})
	}
}

func TestListMissingStore(t *testing.T) {
	s, _ := newStore(t)
	got, err := s.List()
	if err != nil || len(got) != 0 {
		t.Errorf("List = %v, %v; want empty, nil", got, err)
	}
}

func TestCorruptStore(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "not yaml", content: "entries: [\n"},
		{name: "entries not a list", content: "entries: provider\n"},
		{name: "top level a list", content: "- path: /a\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			writeStore(t, s, tc.content, fileMode)
			if _, err := s.List(); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("List err = %v, want ErrCorrupt", err)
			}
			// Add resets the store, so one corrupt file does not disable the list for good.
			if err := s.Add(Entry{Path: touch(t, dir, "a")}); !errors.Is(err, ErrReset) {
				t.Fatalf("Add err = %v, want ErrReset", err)
			}
			got, err := s.List()
			if err != nil || !slices.Equal(names(got), []string{"a"}) {
				t.Errorf("List after Add = %v, %v; want [a], nil", names(got), err)
			}
		})
	}
}

func TestUnreadableStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a 000 file")
	}
	const unreadable fs.FileMode = 0o000
	s, dir := newStore(t)
	other := touch(t, dir, "kept")
	content := fmt.Sprintf("entries:\n  - path: %s\n    name: kept\n", other)
	path := writeStore(t, s, content, unreadable)
	t.Cleanup(func() { _ = os.Chmod(path, fileMode) })

	if _, err := s.List(); err == nil || errors.Is(err, ErrCorrupt) {
		t.Fatalf("List err = %v, want a read error that is not ErrCorrupt", err)
	}
	if err := s.Add(Entry{Path: touch(t, dir, "a")}); err == nil || errors.Is(err, ErrReset) {
		t.Fatalf("Add err = %v, want a read error that is not ErrReset", err)
	}
	if err := os.Chmod(path, fileMode); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil || !slices.Equal(names(got), []string{"kept"}) {
		t.Errorf("List after failed Add = %v, %v; want [kept], nil", names(got), err)
	}
}

// writeStore writes content as the store file with mode and returns its path.
func writeStore(t *testing.T, s *Store, content string, mode fs.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(s.Dir, dirMode); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Dir, FileName)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	s, dir := newStore(t)
	if err := s.Add(Entry{Path: touch(t, dir, "a")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path string
		want fs.FileMode
	}{
		{path: s.Dir, want: dirMode},
		{path: filepath.Join(s.Dir, FileName), want: fileMode},
	}
	for _, tc := range tests {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != tc.want {
			t.Errorf("%s mode = %v, want %v", tc.path, info.Mode().Perm(), tc.want)
		}
	}
}

func TestDefaultDir(t *testing.T) {
	const override = "/tmp/dsc-config-home"
	tests := []struct {
		name       string
		env        map[string]string
		linuxOnly  bool
		wantExact  string
		wantAppDir bool
		wantErr    bool
	}{
		{name: "override set", env: map[string]string{ConfigHomeEnv: override}, wantExact: override},
		{name: "override empty", env: map[string]string{ConfigHomeEnv: ""}, wantAppDir: true},
		{
			name:      "no config dir",
			env:       map[string]string{ConfigHomeEnv: "", "XDG_CONFIG_HOME": "", "HOME": ""},
			linuxOnly: true,
			wantErr:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.linuxOnly && runtime.GOOS != "linux" {
				t.Skip("os.UserConfigDir only fails this way on Linux")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, err := DefaultDir()
			switch {
			case tc.wantErr:
				if err == nil {
					t.Errorf("DefaultDir = %q, want an error", got)
				}
			case err != nil:
				t.Fatal(err)
			case tc.wantExact != "" && got != tc.wantExact:
				t.Errorf("DefaultDir = %q, want %q", got, tc.wantExact)
			case tc.wantAppDir && (!filepath.IsAbs(got) || filepath.Base(got) != appDir):
				t.Errorf("DefaultDir = %q, want an absolute path ending in %q", got, appDir)
			}
		})
	}
}

func TestConcurrentAdd(t *testing.T) {
	s, dir := newStore(t)
	paths := make([]string, MaxRecent)
	for i, n := range numbered(MaxRecent) {
		paths[i] = touch(t, dir, n)
	}
	errs := make(chan error, len(paths))
	for _, p := range paths {
		go func() { errs <- s.Add(Entry{Path: p}) }()
	}
	for range paths {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(paths) {
		t.Errorf("List has %d entries, want %d; a concurrent Add was lost", len(got), len(paths))
	}
}
