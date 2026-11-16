// Package model reads the question-model bundles, one per supported chart version.
package model

import (
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// bundlePattern matches the bundle file of every version directory.
const bundlePattern = "*/bundle.yaml"

// Bundle is the question model of one chart version.
type Bundle struct {
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	// Latest marks the default version; Versions sets it, bundle files never do.
	Latest bool `yaml:"-"`
}

// versioned pairs a bundle with its parsed version for ordering.
type versioned struct {
	bundle  Bundle
	version *semver.Version
}

// Versions loads every bundle in fsys, highest semver first. Latest marks the highest release,
// or the highest pre-release when there is no release, so an rc never becomes the default.
// A bundle whose version is not strict MAJOR.MINOR.PATCH semver (Helm's library, so both agree on
// validity and order) or differs from its directory name is an error.
func Versions(fsys fs.FS) ([]Bundle, error) {
	files, err := fs.Glob(fsys, bundlePattern)
	if err != nil {
		return nil, err
	}
	found := make([]versioned, 0, len(files))
	for _, file := range files {
		v, err := readBundle(fsys, file)
		if err != nil {
			return nil, err
		}
		found = append(found, v)
	}
	slices.SortFunc(found, func(a, b versioned) int { return b.version.Compare(a.version) })
	bundles := make([]Bundle, len(found))
	for i, v := range found {
		bundles[i] = v.bundle
	}
	if len(found) > 0 {
		latest := 0
		if i := slices.IndexFunc(found, isRelease); i >= 0 {
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

func isRelease(v versioned) bool {
	return v.version.Prerelease() == ""
}

func readBundle(fsys fs.FS, file string) (versioned, error) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return versioned{}, err
	}
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.Chart == "" {
		return versioned{}, fmt.Errorf("%s: chart is missing", file)
	}
	v, err := semver.StrictNewVersion(b.Version)
	if err != nil {
		return versioned{}, fmt.Errorf("%s: version %q is not semver: %w", file, b.Version, err)
	}
	if dir := path.Dir(file); b.Version != dir {
		return versioned{}, fmt.Errorf("%s: version %q does not match its directory %q", file, b.Version, dir)
	}
	return versioned{bundle: b, version: v}, nil
}
