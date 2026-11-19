// Package atomicfile replaces files so a reader sees either the old or the new content, never a mix.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// tempPattern names the temp file next to the target; the leading dot hides it in listings.
const tempPattern = ".%s.tmp-*"

var (
	// ErrExists is returned when the path exists and overwrite is false.
	ErrExists = errors.New("file already exists")
	// ErrSymlink is returned when the path is a symlink; replacing it would cut the link.
	ErrSymlink = errors.New("path is a symlink")
	// ErrNotRegular is returned when the path exists but is not a regular file.
	ErrNotRegular = errors.New("path is not a regular file")
)

// rename is os.Rename; tests replace it to fail after the temp file exists.
var rename = os.Rename

// Write writes data to path through a synced temp file in the same directory and a rename.
// A new file gets mode; a replaced one keeps its own. The directory must exist; no temp file
// is left behind on error.
func Write(path string, data []byte, mode fs.FileMode, overwrite bool) (err error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s: %w", path, ErrSymlink)
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s: %w", path, ErrNotRegular)
	case !overwrite:
		return fmt.Errorf("%s: %w", path, ErrExists)
	default:
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), fmt.Sprintf(tempPattern, filepath.Base(path)))
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	// CreateTemp always uses 0600, so the mode is set explicitly.
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return rename(tmp.Name(), path)
}
