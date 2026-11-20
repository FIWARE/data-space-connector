// Package trial loads the local trial descriptor and renders its releases into values files (ADR 0016).
package trial

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	// DescriptorFile is the descriptor's path in the trial file system.
	DescriptorFile = "trial.yaml"
	// TrialProfileName is the profile named in the generated header of every trial output.
	TrialProfileName = "local-trial"

	outputExt = ".yaml"
	// bannerFormat follows the generated header; its arguments are chart, chart version and source file.
	bannerFormat = "# Development configuration from the dsc-config local trial for chart %s@%s, equivalent to %s.\n" +
		"# Self-signed certificates, chart-deployed databases and example identities: not for production.\n\n"
	// lineBreaks are the characters yaml.v3 ends a comment line at; a field holding one would leave the comment.
	lineBreaks = "\n\r\u0085  "
)

// Release is one Helm release of the trial, in install order.
type Release struct {
	Role   string `json:"role" yaml:"role"`
	Source string `json:"source" yaml:"source"`
	// Values is the bundled values file's path in the trial file system.
	Values string         `json:"values" yaml:"values"`
	Target profile.Target `json:"target" yaml:"target"`
}

// File is the name of the values file written for r.
func (r Release) File() string {
	return r.Target.ReleaseName + outputExt
}

// Trial is the decoded descriptor.
type Trial struct {
	Releases []Release `json:"releases" yaml:"releases"`
}

// Output is the rendered values file of one release.
type Output struct {
	Release Release
	File    string
	Values  []byte
}

// Load reads and checks DescriptorFile in fsys; every release must be a valid development target
// whose values file exists, and release names must be unique.
func Load(fsys fs.FS) (Trial, error) {
	data, err := fs.ReadFile(fsys, DescriptorFile)
	if err != nil {
		return Trial{}, fmt.Errorf("trial: %w", err)
	}
	t, err := decode(data)
	if err != nil {
		return Trial{}, fmt.Errorf("trial: %s: %w", DescriptorFile, err)
	}
	if len(t.Releases) == 0 {
		return Trial{}, errors.New("trial: no releases")
	}
	// File derives from the release name, so this also keeps the output names apart.
	files := make(map[string]bool, len(t.Releases))
	for i, r := range t.Releases {
		if err := checkRelease(fsys, r); err != nil {
			return Trial{}, fmt.Errorf("trial: release %d (%s): %w", i+1, r.Role, err)
		}
		if files[r.File()] {
			return Trial{}, fmt.Errorf("trial: duplicate release %q", r.Target.ReleaseName)
		}
		files[r.File()] = true
	}
	return t, nil
}

// decode decodes a single-document descriptor strictly.
func decode(data []byte) (Trial, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var t Trial
	if err := dec.Decode(&t); err != nil && !errors.Is(err, io.EOF) {
		return Trial{}, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Trial{}, errors.New("more than one YAML document")
	}
	return t, nil
}

func checkRelease(fsys fs.FS, r Release) error {
	if r.Role == "" {
		return errors.New("role is required")
	}
	// Target.Validate only checks the chart is named: the trust-anchor chart has no model bundle.
	if errs := r.Target.Validate(); len(errs) > 0 {
		return fmt.Errorf("target: %w", errs[0])
	}
	if r.Target.Environment != profile.EnvDevelopment {
		return fmt.Errorf("environment is %q, want %q", r.Target.Environment, profile.EnvDevelopment)
	}
	for _, f := range []string{r.Source, r.Target.Chart, r.Target.ChartVersion} {
		if strings.ContainsAny(f, lineBreaks) {
			return fmt.Errorf("%q contains a line break", f)
		}
	}
	if _, err := fs.Stat(fsys, r.Values); err != nil {
		return fmt.Errorf("values: %w", err)
	}
	return nil
}

// Render returns one output per release of t, in descriptor order: the generated header, the
// development banner and the bundled bytes of fsys, unchanged.
func Render(t Trial, fsys fs.FS, toolVersion string) ([]Output, error) {
	if strings.ContainsAny(toolVersion, lineBreaks) {
		return nil, errors.New("trial: tool version contains a line break")
	}
	out := make([]Output, 0, len(t.Releases))
	for _, r := range t.Releases {
		src, err := fs.ReadFile(fsys, r.Values)
		if err != nil {
			return nil, fmt.Errorf("trial: %w", err)
		}
		h := roundtrip.Header{ToolVersion: toolVersion, Profile: TrialProfileName}
		// Loading reads the header's chart version as a DefaultChart version (ADR 0016 §6).
		if r.Target.Chart == load.DefaultChart {
			h.ChartVersion = r.Target.ChartVersion
		}
		var b bytes.Buffer
		b.Write(h.Render())
		fmt.Fprintf(&b, bannerFormat, r.Target.Chart, r.Target.ChartVersion, r.Source)
		b.Write(src)
		out = append(out, Output{Release: r, File: r.File(), Values: b.Bytes()})
	}
	return out, nil
}
