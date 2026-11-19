// Package recent keeps the list of recently saved or opened profiles in the user config directory.
package recent

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/atomicfile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	// ConfigHomeEnv overrides DefaultDir, so tests of the real binary keep off the user's list.
	ConfigHomeEnv = "DSC_CONFIG_HOME"
	// MaxRecent caps the list; Add drops the oldest entries beyond it.
	MaxRecent = 10
	// FileName is the store file inside the store directory.
	FileName = "recent.yaml"

	appDir     = "dsc-config"
	yamlIndent = 2
)

// The store is owner-only: the list reveals where the user keeps profiles.
const (
	dirMode  fs.FileMode = 0o700
	fileMode fs.FileMode = 0o600
)

// ErrCorrupt marks a store file that is readable but does not parse.
var ErrCorrupt = errors.New("recent list is corrupt")

// ErrReset is wrapped by Add's error when it replaced a corrupt store; the new entry is still written.
var ErrReset = errors.New("recent list was corrupt and has been reset")

// Entry is one recent profile. It holds a path and metadata only, never profile content.
type Entry struct {
	Path string `json:"path" yaml:"path"`
	// Name is the file base name without profile.FileSuffix.
	Name         string `json:"name" yaml:"name"`
	ChartVersion string `json:"chartVersion" yaml:"chartVersion"`
	// SavedAt is RFC 3339 UTC.
	SavedAt string `json:"savedAt" yaml:"savedAt"`
	// Role is filled from Epic 2 on; empty until then.
	Role string `json:"role,omitempty" yaml:"role,omitempty"`
}

// file is the on-disk shape of FileName.
type file struct {
	Entries []Entry `yaml:"entries"`
}

// Store is the recent list in Dir. Methods are safe for concurrent use within one process.
type Store struct {
	Dir string
	// Now stamps SavedAt; nil means time.Now.
	Now func() time.Time

	mu sync.Mutex
}

// DefaultDir is $DSC_CONFIG_HOME when set, else os.UserConfigDir()/dsc-config.
func DefaultDir() (string, error) {
	if dir := os.Getenv(ConfigHomeEnv); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("recent list: %w (set %s)", err, ConfigHomeEnv)
	}
	return filepath.Join(base, appDir), nil
}

// List returns the entries newest first, without those whose file is gone.
// A missing store is empty; an unreadable one is an error, wrapping ErrCorrupt when it does not parse.
func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list()
}

// Add puts e first, made absolute, replacing an entry of the same path, and fills Name and SavedAt.
// It also drops entries whose file is gone. A corrupt store is replaced and reported through ErrReset;
// any other read error fails Add and leaves the store untouched.
func (s *Store) Add(e Entry) error {
	abs, err := filepath.Abs(e.Path)
	if err != nil {
		return err
	}
	e.Path = abs
	e.Name = strings.TrimSuffix(filepath.Base(abs), profile.FileSuffix)
	e.SavedAt = s.now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	defer s.mu.Unlock()
	entries, listErr := s.list()
	reset := errors.Is(listErr, ErrCorrupt)
	if listErr != nil && !reset {
		return listErr
	}
	entries = slices.DeleteFunc(entries, func(old Entry) bool { return samePath(old.Path, abs) })
	entries = append([]Entry{e}, entries...)
	if len(entries) > MaxRecent {
		entries = entries[:MaxRecent]
	}
	if err := s.write(entries); err != nil {
		return err
	}
	if reset {
		return fmt.Errorf("%w: %w", ErrReset, listErr)
	}
	return nil
}

func (s *Store) path() string { return filepath.Join(s.Dir, FileName) }

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s *Store) list() ([]Entry, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", s.path(), ErrCorrupt, err)
	}
	return slices.DeleteFunc(f.Entries, func(e Entry) bool {
		if e.Path == "" {
			return true
		}
		_, err := os.Stat(e.Path)
		return errors.Is(err, fs.ErrNotExist)
	}), nil
}

func (s *Store) write(entries []Entry) error {
	if err := os.MkdirAll(s.Dir, dirMode); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(file{Entries: entries}); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return atomicfile.Write(s.path(), buf.Bytes(), fileMode, true)
}

// samePath compares a stored path, which a hand edit may have left relative or unclean, with abs.
func samePath(stored, abs string) bool {
	storedAbs, err := filepath.Abs(stored)
	return err == nil && storedAbs == abs
}
