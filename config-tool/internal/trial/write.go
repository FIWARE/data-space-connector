package trial

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// ExistsError refuses a write because some targets exist; it matches fs.ErrExist.
type ExistsError struct {
	Paths []string
}

func (e *ExistsError) Error() string {
	return "trial: refusing to overwrite " + strings.Join(e.Paths, ", ")
}

func (e *ExistsError) Unwrap() error { return fs.ErrExist }

// WriteAll creates every output as a new file in dir and returns the written paths, in order.
// It writes all or nothing: an existing target, symlinks included, refuses the whole call with an
// *ExistsError listing all of them, and a failure removes every file the call created.
func WriteAll(dir string, outs []Output) ([]string, error) {
	paths := make([]string, len(outs))
	var existing []string
	for i, o := range outs {
		paths[i] = filepath.Join(dir, o.File)
		_, err := os.Lstat(paths[i])
		switch {
		case err == nil:
			existing = append(existing, paths[i])
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
	}
	if len(existing) > 0 {
		return nil, &ExistsError{existing}
	}
	created := make([]string, 0, len(outs))
	for i, o := range outs {
		// O_EXCL keeps a file created since the check from being clobbered.
		f, err := os.OpenFile(paths[i], os.O_WRONLY|os.O_CREATE|os.O_EXCL, roundtrip.ValuesFileMode)
		if err == nil {
			created = append(created, paths[i])
			_, err = f.Write(o.Values)
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
		if err != nil {
			for _, p := range created {
				_ = os.Remove(p)
			}
			if errors.Is(err, fs.ErrExist) {
				return nil, &ExistsError{[]string{paths[i]}}
			}
			return nil, err
		}
	}
	return paths, nil
}
