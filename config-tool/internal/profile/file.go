package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/atomicfile"
)

// fileMode is the mode of a new profile; it holds no secrets, so it is world-readable.
const fileMode fs.FileMode = 0o644

// ErrSuffix is returned for a path that does not end in FileSuffix.
var ErrSuffix = errors.New("profile path must end in " + FileSuffix)

// ReadFile reads and strictly decodes the profile at path; it does not run Validate.
func ReadFile(path string) (Profile, error) {
	if err := checkSuffix(path); err != nil {
		return Profile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	p, err := Unmarshal(data)
	if err != nil {
		return Profile{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// WriteFile writes p atomically to path; an existing file is replaced only with overwrite
// (otherwise atomicfile.ErrExists) and keeps its mode. It does not run Validate.
func WriteFile(path string, p Profile, overwrite bool) error {
	if err := checkSuffix(path); err != nil {
		return err
	}
	data, err := Marshal(p)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, data, fileMode, overwrite)
}

func checkSuffix(path string) error {
	if !strings.HasSuffix(path, FileSuffix) {
		return fmt.Errorf("%s: %w", path, ErrSuffix)
	}
	return nil
}
