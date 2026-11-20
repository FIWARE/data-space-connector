package profile

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

const (
	// FormatVersion is the only schema version of .dscprofile.yaml this build reads and writes.
	FormatVersion = 1
	// FileSuffix ends every profile path; the API reads and writes no other file.
	FileSuffix = ".dscprofile.yaml"
	// Header is the comment line every written profile starts with.
	Header = "# dsc-config profile: the answer set values.yaml is rendered from. It never holds a secret value.\n"
	// TargetFieldPrefix starts the field ids of the Target stage, e.g. "target.namespace".
	TargetFieldPrefix = "target."
	// yamlIndent is the indent of written profiles.
	yamlIndent = 2
)

// Field ids reported by Profile.Validate for errors outside Target's fields.
const (
	FieldFormat        = "format"
	FieldModel         = "model"
	FieldOpen          = "open"
	FieldSourceFile    = "source.file"
	FieldSourceSHA256  = "source.sha256"
	answersFieldPrefix = "answers."
	secretsFieldPrefix = "secrets."
)

// sha256Hex is a SHA-256 digest as written to source.sha256.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// targetFields are the Target keys an open question may name, without TargetFieldPrefix.
var targetFields = []string{
	FieldType, FieldContext, FieldNamespace, FieldCreateNamespace,
	FieldReleaseName, FieldChart, FieldChartVersion, FieldEnvironment,
}

// Profile is the answer set stored in a .dscprofile.yaml (ADR 0015). It never holds a secret value.
type Profile struct {
	Format int      `json:"format" yaml:"format"`
	Model  ModelRef `json:"model" yaml:"model"`
	// Catalogue is the bundle the role presets came from; nil when that is Model (ADR 0015 §7).
	Catalogue *ModelRef `json:"catalogue,omitempty" yaml:"catalogue,omitempty"`
	Target    Target    `json:"target" yaml:"target"`
	Role      string    `json:"role,omitempty" yaml:"role,omitempty"`
	// Features maps catalogue feature ids to their state; not-offered features are absent.
	Features map[string]FeatureState `json:"features,omitempty" yaml:"features,omitempty"`
	// Answers maps answer ids to scalars; no answer id exists yet, so Validate refuses every key.
	Answers map[string]any `json:"answers" yaml:"answers"`
	Open    []OpenQuestion `json:"open" yaml:"open"`
	// Source references the loaded values file; nil when nothing was loaded.
	Source *Source `json:"source,omitempty" yaml:"source,omitempty"`
	// Secrets is filled from Epic 8 on; until then Validate refuses every entry.
	Secrets map[string]SecretRef `json:"secrets" yaml:"secrets"`
}

// ModelRef names the question-model bundle a profile was built with.
type ModelRef struct {
	Chart    string `json:"chart" yaml:"chart"`
	Version  string `json:"version" yaml:"version"`
	Revision int    `json:"revision" yaml:"revision"`
}

// OpenQuestion marks a field the user deliberately left open; a mapping leaves room for a note.
type OpenQuestion struct {
	Field string `json:"field" yaml:"field"`
}

// Source is the values file the overrides layer lives in: metadata only, never its content.
type Source struct {
	// File is a plain base name, resolved against the profile's directory.
	File string `json:"file" yaml:"file"`
	// Root is the mapping the chart's values sit under; "" is the whole file, never "detect".
	Root   string `json:"root" yaml:"root"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

// SecretRef records how a secret is provided, never its value (SECRETS.md rule 1).
type SecretRef struct {
	Mode         string            `json:"mode" yaml:"mode"`
	Keys         []string          `json:"keys" yaml:"keys"`
	Fingerprints map[string]string `json:"fingerprints" yaml:"fingerprints"`
}

// Marshal encodes p deterministically: header, FormatVersion, `open` sorted, empty maps as {}.
func Marshal(p Profile) ([]byte, error) {
	p.Format = FormatVersion
	p.Open = slices.SortedFunc(slices.Values(p.Open), func(a, b OpenQuestion) int { return cmp.Compare(a.Field, b.Field) })
	if p.Answers == nil {
		p.Answers = map[string]any{}
	}
	if p.Secrets == nil {
		p.Secrets = map[string]SecretRef{}
	}
	var buf bytes.Buffer
	buf.WriteString(Header)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(p); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal decodes a profile strictly: unknown keys, a second document and a format other than
// FormatVersion are errors. It does not run Validate.
func Unmarshal(data []byte) (Profile, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var p Profile
	if err := dec.Decode(&p); err != nil {
		if errors.Is(err, io.EOF) {
			return Profile{}, errors.New("profile is empty")
		}
		return Profile{}, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Profile{}, errors.New("profile holds more than one YAML document")
	}
	if p.Format != FormatVersion {
		return Profile{}, fmt.Errorf("profile format %d is not supported, want %d", p.Format, FormatVersion)
	}
	return p, nil
}

// Validate returns the structural errors that make save and open refuse p (ADR 0015 §5).
// Per-field format and "is required" errors of Target are left to the stage, so a half-finished
// profile is valid.
func (p Profile) Validate(bundles []model.Bundle) []FieldError {
	var errs []FieldError
	if p.Format != FormatVersion {
		errs = append(errs, FieldError{FieldFormat, fmt.Sprintf("must be %d", FormatVersion)})
	}
	for _, e := range p.Target.Validate() {
		if (e.Field == FieldType || e.Field == FieldEnvironment) && e.Message != msgRequired {
			errs = append(errs, FieldError{TargetFieldPrefix + e.Field, e.Message})
		}
	}
	errs = append(errs, p.validateModel(bundles)...)
	errs = append(errs, p.validateRole(bundles)...)
	errs = append(errs, p.validateOpen()...)
	errs = append(errs, p.Source.validate()...)
	for _, k := range sortedKeys(p.Answers) {
		errs = append(errs, FieldError{answersFieldPrefix + k, "is not a known answer id"})
	}
	for _, k := range sortedKeys(p.Secrets) {
		errs = append(errs, FieldError{secretsFieldPrefix + k, "secrets are recorded from Epic 8 on"})
	}
	return errs
}

// validateModel checks that the model bundle is shipped and agrees with the target's chart and version.
func (p Profile) validateModel(bundles []model.Bundle) []FieldError {
	var errs []FieldError
	chartShipped := slices.ContainsFunc(bundles, func(b model.Bundle) bool { return b.Chart == p.Target.Chart })
	if p.Target.Chart != "" && !chartShipped {
		errs = append(errs, FieldError{TargetFieldPrefix + FieldChart, fmt.Sprintf("chart %q is not shipped", p.Target.Chart)})
	}
	if v := p.Target.ChartVersion; v != "" && chartShipped && findBundle(bundles, p.Target.Chart, v) == nil {
		errs = append(errs, FieldError{TargetFieldPrefix + FieldChartVersion, fmt.Sprintf("version %q of chart %q is not shipped", v, p.Target.Chart)})
	}

	m := p.Model
	errs = append(errs, checkRef(FieldModel, m, bundles)...)
	if m.Chart != p.Target.Chart {
		errs = append(errs, FieldError{FieldModel, fmt.Sprintf("chart %q does not match target.chart %q", m.Chart, p.Target.Chart)})
	}
	if v := p.Target.ChartVersion; v != "" && m.Version != v {
		errs = append(errs, FieldError{FieldModel, fmt.Sprintf("version %q does not match target.chartVersion %q", m.Version, v)})
	}
	return errs
}

// checkRef refuses a bundle reference that is not shipped or whose revision this build cannot read;
// a catalogue must also declare roles.
func checkRef(field string, m ModelRef, bundles []model.Bundle) []FieldError {
	switch b := findBundle(bundles, m.Chart, m.Version); {
	case b == nil:
		return []FieldError{{field, fmt.Sprintf("bundle %s@%s is not shipped; migrating between bundles comes with US-9.3", m.Chart, m.Version)}}
	case m.Revision < 1:
		return []FieldError{{field, "revision must be positive"}}
	case m.Revision > b.Revision:
		return []FieldError{{field, fmt.Sprintf("revision %d of bundle %s@%s is newer than the shipped revision %d; migrating between bundles comes with US-9.3", m.Revision, m.Chart, m.Version, b.Revision)}}
	case field == FieldCatalogue && len(b.Roles) == 0:
		return []FieldError{{field, fmt.Sprintf("bundle %s@%s declares no roles", m.Chart, m.Version)}}
	}
	return nil
}

// validateOpen refuses unknown and repeated field ids.
func (p Profile) validateOpen() []FieldError {
	var errs []FieldError
	seen := map[string]bool{}
	for _, q := range p.Open {
		name, ok := strings.CutPrefix(q.Field, TargetFieldPrefix)
		switch {
		case !ok || !slices.Contains(targetFields, name):
			errs = append(errs, FieldError{FieldOpen, fmt.Sprintf("%q is not a known field", q.Field)})
		case seen[q.Field]:
			errs = append(errs, FieldError{FieldOpen, fmt.Sprintf("%q is listed more than once", q.Field)})
		}
		seen[q.Field] = true
	}
	return errs
}

// validate refuses a source file that is not a plain base name, so open reads only next to the profile.
func (s *Source) validate() []FieldError {
	if s == nil {
		return nil
	}
	var errs []FieldError
	if !isBaseName(s.File) {
		errs = append(errs, FieldError{FieldSourceFile, "must be a plain file name in the profile's directory"})
	}
	if !sha256Hex.MatchString(s.SHA256) {
		errs = append(errs, FieldError{FieldSourceSHA256, "must be 64 lowercase hex digits"})
	}
	return errs
}

// isBaseName reports whether f names a file in the current directory on every OS; ':' is refused
// because "C:x" is drive-relative on Windows.
func isBaseName(f string) bool {
	return f != "" && f != "." && f != ".." && !strings.ContainsAny(f, `/\:`) && filepath.Base(f) == f
}

func findBundle(bundles []model.Bundle, chart, version string) *model.Bundle {
	i := slices.IndexFunc(bundles, func(b model.Bundle) bool { return b.Chart == chart && b.Version == version })
	if i < 0 {
		return nil
	}
	return &bundles[i]
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
