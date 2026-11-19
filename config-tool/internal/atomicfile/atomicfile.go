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

// rename and link are os.Rename and os.Link; tests replace them to act after the temp file exists.
var (
	rename = os.Rename
	link   = os.Link
)

// Write writes data to path through a synced temp file in the same directory, published by a
// rename, or without overwrite by a hard link, which fails on a file created since the check
// (an exclusive placeholder plus rename where the filesystem has no hard links).
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
	if overwrite {
		return rename(tmp.Name(), path)
	}
	err = link(tmp.Name(), path)
	switch {
	case err == nil:
		// path is published; a temp name that fails to go away is only clutter.
		_ = os.Remove(tmp.Name())
		return nil
	case errors.Is(err, fs.ErrExist):
		return fmt.Errorf("%s: %w", path, ErrExists)
	default:
		// No hard links here (FAT, exFAT, some network and FUSE mounts).
		return claimAndRename(tmp.Name(), path, mode)
	}
}

// claimAndRename creates path exclusively as an empty placeholder, then renames tmp over it.
// A racing writer still gets ErrExists; a reader may briefly see the empty placeholder.
func claimAndRename(tmp, path string, mode fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s: %w", path, ErrExists)
	}
	if err != nil {
		return err
	}
	if err = f.Close(); err == nil {
		err = rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}
