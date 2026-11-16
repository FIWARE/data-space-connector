// Package model reads the question-model bundles, one per supported chart version.
package model

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"

	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// bundlePattern matches the bundle file of every version directory.
const bundlePattern = "*/bundle.yaml"

// semverPrefix is the "v" golang.org/x/mod/semver insists on and chart versions leave out.
const semverPrefix = "v"

// Bundle is the question model of one chart version.
type Bundle struct {
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	// Latest marks the default version; Versions sets it, bundle files never do.
	Latest bool `yaml:"-"`
}

// Versions loads every bundle in fsys, highest semver first. Latest marks the highest release,
// or the highest pre-release when there is no release, so an rc never becomes the default.
// A bundle whose version is not semver or differs from its directory name is an error.
func Versions(fsys fs.FS) ([]Bundle, error) {
	files, err := fs.Glob(fsys, bundlePattern)
	if err != nil {
		return nil, err
	}
	bundles := make([]Bundle, 0, len(files))
	for _, file := range files {
		b, err := readBundle(fsys, file)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, b)
	}
	sort.Slice(bundles, func(i, j int) bool {
		return semver.Compare(semverPrefix+bundles[i].Version, semverPrefix+bundles[j].Version) > 0
	})
	if len(bundles) > 0 {
		latest := 0
		if i := slices.IndexFunc(bundles, isRelease); i >= 0 {
			latest = i
		}
		bundles[latest].Latest = true
	}
	return bundles, nil
}

// Charts lists the distinct charts of bundles in their first-seen order.
func Charts(bundles []Bundle) []string {
	charts := []string{}
	for _, b := range bundles {
		if !slices.Contains(charts, b.Chart) {
			charts = append(charts, b.Chart)
		}
	}
	return charts
}

func isRelease(b Bundle) bool {
	return semver.Prerelease(semverPrefix+b.Version) == ""
}

func readBundle(fsys fs.FS, file string) (Bundle, error) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return Bundle{}, err
	}
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return Bundle{}, fmt.Errorf("%s: %w", file, err)
	}
	dir := path.Dir(file)
	switch {
	case b.Chart == "":
		return Bundle{}, fmt.Errorf("%s: chart is missing", file)
	case !semver.IsValid(semverPrefix + b.Version):
		return Bundle{}, fmt.Errorf("%s: version %q is not semver", file, b.Version)
	case b.Version != dir:
		return Bundle{}, fmt.Errorf("%s: version %q does not match its directory %q", file, b.Version, dir)
	}
	return b, nil
}
