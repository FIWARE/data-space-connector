// Package model reads the question-model bundles, one per supported chart version.
package model

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	// bundleName is the bundle's file name inside its version directory.
	bundleName = "bundle.yaml"
	// bundlePattern matches the bundle file of every version directory.
	bundlePattern = "*/" + bundleName
)

// Bundle is the question model of one chart version.
type Bundle struct {
	Chart   string `yaml:"chart"`
	Version string `yaml:"version"`
	// Revision counts changes to the bundle within one chart version; a profile records it (ADR 0015 §6).
	Revision int `yaml:"revision"`
	// TopLevelKeys are the chart's top-level values keys; a load finds the chart's values by them
	// when another chart wraps them under a key of its own.
	TopLevelKeys []string `yaml:"topLevelKeys"`
	// Recognize maps chart keys back to answers when an existing values file is loaded.
	Recognize []Rule `yaml:"recognize"`
	// Features is the feature catalogue; Roles requires it.
	Features []Feature `yaml:"features"`
	// Roles are the role presets, absent or exactly KnownRoles in order.
	Roles []Role `yaml:"roles"`
	// RoleMatrix maps the role guide's component matrix to Features; nil when not declared.
	RoleMatrix *RoleMatrix `yaml:"roleMatrix"`
	// Foundations are the cross-cutting answers and the chart keys they fan out to (ADR 0020).
	Foundations []Foundation `yaml:"foundations"`
	// Resources is the resource-profile table (ADR 0024); nil when the bundle sizes nothing.
	Resources *ResourceTable `yaml:"resources"`
	// Checks are the L2 rules, written as rules: in the bundle (ADR 0021); Rules is taken by Recognize.
	Checks []CheckRule `yaml:"rules"`
	// Lists declares the lists element targets write into, with their base elements (ADR 0025).
	Lists []List `yaml:"lists"`
	// NoProxyServicesTable lists the in-cluster services the proxy.no default bypasses (ADR 0025).
	NoProxyServicesTable []NoProxyService `yaml:"noProxyServices"`
	// RequirementsTable lists the Secrets the rendered values reference and the user creates (ADR 0025).
	RequirementsTable []Requirement `yaml:"requirements"`
	// MechanismsTable declares when a mechanism is withheld for what the chart templates itself (ADR 0025).
	MechanismsTable []Mechanism `yaml:"mechanisms"`
	// Latest marks the default version; Versions sets it, bundle files never do.
	Latest bool `yaml:"-"`
	// rules is Recognize compiled; readBundle sets it after validating every rule.
	rules []CompiledRule
	// foundations is Foundations compiled and order their topological order; both nil without foundations.
	foundations []CompiledFoundation
	order       []string
	// lists is Lists compiled.
	lists []CompiledList
	// noProxyServices is NoProxyServicesTable compiled.
	noProxyServices []CompiledNoProxyService
	// requirements is RequirementsTable compiled.
	requirements []CompiledRequirement
	// mechanisms is MechanismsTable compiled.
	mechanisms []CompiledMechanism
}

// Rules returns the validated recognition rules in bundle order.
func (b Bundle) Rules() []CompiledRule {
	return b.rules
}

// Kind says how a recognition rule reads its leaf.
type Kind string

const (
	// KindToggle takes a !!bool scalar as the answer.
	KindToggle Kind = "toggle"
	// KindValue takes any scalar, optionally narrowed by the rule's pattern.
	KindValue Kind = "value"
)

// Stage is a wizard stage id; the values mirror ui/src/state/stageIds.ts.
type Stage string

// Stage ids, in wizard order.
const (
	StageTarget         Stage = "target"
	StageRole           Stage = "role"
	StageFoundations    Stage = "foundations"
	StageInfrastructure Stage = "infrastructure"
	StageFeatures       Stage = "features"
	StageConfiguration  Stage = "configuration"
	StageReview         Stage = "review"
	StageInstall        Stage = "install"
)

var (
	knownKinds  = []Kind{KindToggle, KindValue}
	knownStages = []Stage{
		StageTarget, StageRole, StageFoundations, StageInfrastructure,
		StageFeatures, StageConfiguration, StageReview, StageInstall,
	}
)

// patternGroups is the number of capture groups a rule pattern must have: the one that holds the answer.
const patternGroups = 1

// Rule maps one chart key to an answer, as written in a bundle's recognize list.
type Rule struct {
	// Path is an exact chart key in roundtrip.ParsePath syntax.
	Path   string `yaml:"path"`
	Answer string `yaml:"answer"`
	Stage  Stage  `yaml:"stage"`
	Kind   Kind   `yaml:"kind"`
	// Pattern is an optional regexp for value rules whose single capture group is the answer.
	Pattern string `yaml:"pattern"`
}

// CompiledRule is a validated Rule. Pattern is nil when the rule has none.
type CompiledRule struct {
	Path    roundtrip.Path
	Answer  string
	Stage   Stage
	Kind    Kind
	Pattern *regexp.Regexp
}

// compileRules validates rules and compiles them; an error names the offending rule's index.
func compileRules(rules []Rule) ([]CompiledRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	compiled := make([]CompiledRule, len(rules))
	seen := map[string]int{}
	// firstByAnswer keeps the first rule per answer, because the report gives an answer one stage and type.
	firstByAnswer := map[string]int{}
	for i, r := range rules {
		c, err := compileRule(r)
		if err != nil {
			return nil, fmt.Errorf("recognize[%d]: %w", i, err)
		}
		key := c.Path.String()
		if j, dup := seen[key]; dup {
			return nil, fmt.Errorf("recognize[%d]: path %q is already used by recognize[%d]", i, key, j)
		}
		seen[key] = i
		j, ok := firstByAnswer[c.Answer]
		switch {
		case !ok:
			firstByAnswer[c.Answer] = i
		case compiled[j].Stage != c.Stage:
			return nil, fmt.Errorf("recognize[%d]: answer %q has stage %s, recognize[%d] has %s", i, c.Answer, c.Stage, j, compiled[j].Stage)
		case compiled[j].Kind != c.Kind:
			return nil, fmt.Errorf("recognize[%d]: answer %q has kind %s, recognize[%d] has %s", i, c.Answer, c.Kind, j, compiled[j].Kind)
		}
		compiled[i] = c
	}
	return compiled, nil
}

func compileRule(r Rule) (CompiledRule, error) {
	p, err := roundtrip.ParsePath(r.Path)
	if err != nil {
		return CompiledRule{}, err
	}
	if len(p) == 0 {
		return CompiledRule{}, fmt.Errorf("path is missing")
	}
	if r.Answer == "" {
		return CompiledRule{}, fmt.Errorf("answer is missing")
	}
	if !slices.Contains(knownStages, r.Stage) {
		return CompiledRule{}, fmt.Errorf("stage %q is unknown", r.Stage)
	}
	if !slices.Contains(knownKinds, r.Kind) {
		return CompiledRule{}, fmt.Errorf("kind %q is unknown", r.Kind)
	}
	c := CompiledRule{Path: p, Answer: r.Answer, Stage: r.Stage, Kind: r.Kind}
	if r.Pattern == "" {
		return c, nil
	}
	if r.Kind != KindValue {
		return CompiledRule{}, fmt.Errorf("pattern is only allowed on %s rules", KindValue)
	}
	if c.Pattern, err = regexp.Compile(r.Pattern); err != nil {
		return CompiledRule{}, fmt.Errorf("pattern: %w", err)
	}
	if n := c.Pattern.NumSubexp(); n != patternGroups {
		return CompiledRule{}, fmt.Errorf("pattern %q has %d capture groups, want %d", r.Pattern, n, patternGroups)
	}
	return c, nil
}

// versioned pairs a bundle with its parsed version for ordering.
type versioned struct {
	bundle  Bundle
	version *semver.Version
}

// Versions loads every bundle in fsys, highest semver first. Per chart, Latest marks the highest
// release, or the highest pre-release when there is no release, so an rc never becomes the default.
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
	latest := map[string]int{}
	for i, v := range found {
		j, seen := latest[v.bundle.Chart]
		if !seen || (!isRelease(found[j]) && isRelease(v)) {
			latest[v.bundle.Chart] = i
		}
	}
	for _, i := range latest {
		bundles[i].Latest = true
	}
	if err := checkCharts(bundles); err != nil {
		return nil, err
	}
	return bundles, nil
}

// Chart is one chart with its bundled versions, in the order Versions returns them.
type Chart struct {
	Name     string
	Versions []Bundle
}

// Charts groups bundles by chart, in first-seen order, so a version is only offered for its chart.
func Charts(bundles []Bundle) []Chart {
	charts := []Chart{}
	for _, b := range bundles {
		i := slices.IndexFunc(charts, func(c Chart) bool { return c.Name == b.Chart })
		if i < 0 {
			charts = append(charts, Chart{Name: b.Chart})
			i = len(charts) - 1
		}
		charts[i].Versions = append(charts[i].Versions, b)
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
	// KnownFields makes a misspelled key an error instead of a silently dropped field.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var b Bundle
	if err := dec.Decode(&b); err != nil && !errors.Is(err, io.EOF) {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.Chart == "" {
		return versioned{}, fmt.Errorf("%s: chart is missing", file)
	}
	v, err := semver.StrictNewVersion(b.Version)
	if err != nil {
		return versioned{}, fmt.Errorf("%s: version %q is not semver: %w", file, b.Version, err)
	}
	if b.Revision < 1 {
		return versioned{}, fmt.Errorf("%s: revision is missing or not positive", file)
	}
	if dir := path.Dir(file); b.Version != dir {
		return versioned{}, fmt.Errorf("%s: version %q does not match its directory %q", file, b.Version, dir)
	}
	if err := checkTopLevelKeys(b.TopLevelKeys); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.rules, err = compileRules(b.Recognize); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if err := checkRuleRoots(b.rules, b.TopLevelKeys); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if err := checkCatalogue(&b); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.lists, err = compileLists(b.Lists, b.TopLevelKeys); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.noProxyServices, err = compileNoProxyServices(b.NoProxyServicesTable, b.TopLevelKeys); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.foundations, b.order, err = compileFoundations(b.Foundations, b.lists, b.Resources, b.TopLevelKeys); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.requirements, err = compileRequirements(b.RequirementsTable, b.foundations); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if b.mechanisms, err = compileMechanisms(b.MechanismsTable, b.foundations, b.TopLevelKeys); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if err := checkFoundationFeatures(b.foundations, b.Features, b.Chart); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if err := checkRecognizedFoundations(b.rules, b.foundations); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	if err := checkCheckRules(b.Checks, ruleContext{
		fs: b.foundations, features: b.Features, order: b.order, table: b.Resources, topLevelKeys: b.TopLevelKeys,
	}); err != nil {
		return versioned{}, fmt.Errorf("%s: %w", file, err)
	}
	return versioned{bundle: b, version: v}, nil
}

// checkTopLevelKeys refuses an empty or repeated key.
func checkTopLevelKeys(keys []string) error {
	for i, k := range keys {
		if k == "" {
			return fmt.Errorf("topLevelKeys[%d] is empty", i)
		}
		if j := slices.Index(keys, k); j < i {
			return fmt.Errorf("topLevelKeys[%d]: %q is already listed at topLevelKeys[%d]", i, k, j)
		}
	}
	return nil
}

// checkRuleRoots refuses a rule whose first key is not a top-level key, unless none are listed.
func checkRuleRoots(rules []CompiledRule, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	for i, r := range rules {
		if k, _ := r.Path[0].Key(); !slices.Contains(keys, k) {
			return fmt.Errorf("recognize[%d]: %q is not in topLevelKeys", i, k)
		}
	}
	return nil
}

// checkRecognizedFoundations refuses a rule that answers a foundation with another stage, or with a kind that
// cannot read its type: toggle for a bool, value for a string type, nothing for a list.
func checkRecognizedFoundations(rules []CompiledRule, fs []CompiledFoundation) error {
	for i, r := range rules {
		j := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == r.Answer })
		if j < 0 {
			continue
		}
		f := fs[j]
		if r.Stage != f.Stage {
			return fmt.Errorf("recognize[%d]: answer %q is a foundation of stage %s, not %s", i, r.Answer, f.Stage, r.Stage)
		}
		want := KindValue
		switch {
		case f.Type == TypeBool:
			want = KindToggle
		case !f.Type.IsString():
			return fmt.Errorf("recognize[%d]: answer %q is a %s, which a single leaf cannot answer", i, r.Answer, f.Type)
		}
		if r.Kind != want {
			return fmt.Errorf("recognize[%d]: answer %q is a %s, which only a %s rule reads", i, r.Answer, f.Type, want)
		}
	}
	return nil
}
