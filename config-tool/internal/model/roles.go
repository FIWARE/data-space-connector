package model

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Group is a feature group of the catalogue (FEATURE_MODEL.md#concepts).
type Group string

// Feature groups, in catalogue order.
const (
	GroupIdentity      Group = "identity"
	GroupAccess        Group = "access"
	GroupData          Group = "data"
	GroupContracts     Group = "contracts"
	GroupMarketplace   Group = "marketplace"
	GroupInterop       Group = "interop"
	GroupConsent       Group = "consent"
	GroupOperator      Group = "operator"
	GroupObservability Group = "observability"
)

var knownGroups = []Group{
	GroupIdentity, GroupAccess, GroupData, GroupContracts, GroupMarketplace,
	GroupInterop, GroupConsent, GroupOperator, GroupObservability,
}

// Feature is one entry of the feature catalogue.
type Feature struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Summary string `yaml:"summary"`
	Group   Group  `yaml:"group"`
	// Chart is the chart whose values the feature sets; readBundle defaults it to the bundle's chart.
	Chart    string   `yaml:"chart"`
	Requires []string `yaml:"requires"`
	// Keys are roundtrip paths, each naming its whole subtree; features may share keys.
	Keys []string `yaml:"keys"`
	// Consequence is shown when the user turns off a feature the role recommends; optional, Summary stands in.
	Consequence string `yaml:"consequence"`
}

// Offer says how a role preset offers a feature.
type Offer string

// Offers a preset may make.
const (
	OfferRecommended Offer = "recommended"
	OfferAvailable   Offer = "available"
	OfferNotOffered  Offer = "not-offered"
)

var knownOffers = []Offer{OfferRecommended, OfferAvailable, OfferNotOffered}

// Role ids of the presets.
const (
	RoleConsumer         = "consumer"
	RoleProvider         = "provider"
	RoleConsumerProvider = "consumer-provider"
	RoleOperator         = "operator"
	RoleCustom           = "custom"
)

// KnownRoles lists the role ids in the order a bundle declares them.
var KnownRoles = []string{RoleConsumer, RoleProvider, RoleConsumerProvider, RoleOperator, RoleCustom}

// Role is a role preset: the chart it deploys and how it offers every catalogue feature.
type Role struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Summary string `yaml:"summary"`
	Chart   string `yaml:"chart"`
	// Floor is nil when the footprint depends on the selection (custom).
	Floor    *Floor           `yaml:"floor"`
	Docs     string           `yaml:"docs"`
	Features map[string]Offer `yaml:"features"`
}

// Floor is a role's minimum footprint as Kubernetes quantities, measured with one chart version.
type Floor struct {
	Memory       string `yaml:"memory"`
	CPU          string `yaml:"cpu"`
	MeasuredWith string `yaml:"measuredWith"`
}

// RoleMatrix maps the role guide's component matrix to feature ids.
type RoleMatrix struct {
	// Components maps a matrix "Component" cell to a feature id.
	Components map[string]string `yaml:"components"`
	// Infrastructure lists matrix rows that are not features.
	Infrastructure []string `yaml:"infrastructure"`
}

// keyWildcard is refused in feature keys: a key already names its whole subtree.
const keyWildcard = "*"

// ToggleSuffix ends the answer id of every recognize rule at stage features: <feature id>.enabled.
const ToggleSuffix = ".enabled"

var (
	floorMemory = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(Mi|Gi)$`)
	floorCPU    = regexp.MustCompile(`^[0-9]+m$`)
)

// ErrNoRoles is returned by RoleCatalogue when no bundle declares roles.
var ErrNoRoles = errors.New("model: no bundle declares roles")

// RoleCatalogue returns the bundle the roles of chart@version come from: its own bundle when that
// declares roles, else a Latest bundle with roles (highest first), else the highest bundle with roles.
// An empty version means the chart's Latest bundle.
func RoleCatalogue(bundles []Bundle, chart, version string) (Bundle, error) {
	for _, b := range bundles {
		if b.Chart == chart && (b.Version == version || version == "" && b.Latest) && len(b.Roles) > 0 {
			return b, nil
		}
	}
	var best *Bundle
	for i := range bundles {
		b := &bundles[i]
		if len(b.Roles) == 0 {
			continue
		}
		if best == nil || b.Latest && !best.Latest || b.Latest == best.Latest && newer(*b, *best) {
			best = b
		}
	}
	if best == nil {
		return Bundle{}, ErrNoRoles
	}
	return *best, nil
}

// newer reports whether a's version is above b's; Versions has validated both.
func newer(a, b Bundle) bool {
	va, errA := semver.StrictNewVersion(a.Version)
	vb, errB := semver.StrictNewVersion(b.Version)
	if errA != nil || errB != nil {
		return errB != nil && errA == nil
	}
	return va.GreaterThan(vb)
}

// checkCatalogue validates features, roles and roleMatrix of b and fills each feature's default chart.
func checkCatalogue(b *Bundle) error {
	if err := checkFeatures(b.Features, b.Chart); err != nil {
		return err
	}
	if err := checkRoles(b.Roles, b.Features); err != nil {
		return err
	}
	if err := checkRoleMatrix(b.RoleMatrix, b.Roles, b.Features); err != nil {
		return err
	}
	return checkFeatureRules(b.rules, b.Features)
}

func checkFeatures(features []Feature, chart string) error {
	seen := map[string]int{}
	for i := range features {
		f := &features[i]
		if f.Chart == "" {
			f.Chart = chart
		}
		if f.ID == "" {
			return fmt.Errorf("features[%d]: id is missing", i)
		}
		if j, dup := seen[f.ID]; dup {
			return fmt.Errorf("features[%d]: id %q is already used by features[%d]", i, f.ID, j)
		}
		seen[f.ID] = i
		if err := checkFeature(*f); err != nil {
			return fmt.Errorf("features[%d] (%s): %w", i, f.ID, err)
		}
	}
	for i, f := range features {
		for _, r := range f.Requires {
			if _, ok := seen[r]; !ok {
				return fmt.Errorf("features[%d] (%s): requires %q, which is not in the catalogue", i, f.ID, r)
			}
			if r == f.ID {
				return fmt.Errorf("features[%d] (%s): requires itself", i, f.ID)
			}
		}
	}
	return nil
}

func checkFeature(f Feature) error {
	switch {
	case f.Title == "":
		return errors.New("title is missing")
	case f.Summary == "":
		return errors.New("summary is missing")
	case f.Consequence != "" && strings.TrimSpace(f.Consequence) == "":
		return errors.New("consequence is blank")
	case f.Group == "":
		return errors.New("group is missing")
	case !slices.Contains(knownGroups, f.Group):
		return fmt.Errorf("group %q is unknown", f.Group)
	}
	return checkKeys(f.Keys)
}

// checkKeys refuses a malformed, wildcard, repeated or nested key within one feature.
func checkKeys(keys []string) error {
	paths := make([]roundtrip.Path, 0, len(keys))
	for i, k := range keys {
		p, err := roundtrip.ParsePath(k)
		if err != nil {
			return fmt.Errorf("keys[%d]: %w", i, err)
		}
		if len(p) == 0 {
			return fmt.Errorf("keys[%d] is empty", i)
		}
		for _, s := range p {
			if name, ok := s.Key(); ok && strings.Contains(name, keyWildcard) {
				return fmt.Errorf("keys[%d]: %q holds %q; a key already names its whole subtree", i, k, keyWildcard)
			}
		}
		for j, q := range paths {
			switch {
			case slices.Equal(p, q):
				return fmt.Errorf("keys[%d]: %q is already listed at keys[%d]", i, k, j)
			case isUnder(p, q):
				return fmt.Errorf("keys[%d]: %q is under keys[%d] %q", i, k, j, keys[j])
			case isUnder(q, p):
				return fmt.Errorf("keys[%d]: keys[%d] %q is under %q", i, j, keys[j], k)
			}
		}
		paths = append(paths, p)
	}
	return nil
}

// isUnder reports whether p lies strictly inside the subtree q names.
func isUnder(p, q roundtrip.Path) bool {
	return len(p) > len(q) && slices.Equal(p[:len(q)], q)
}

func checkRoles(roles []Role, features []Feature) error {
	if len(roles) == 0 {
		return nil
	}
	if len(features) == 0 {
		return errors.New("roles need features")
	}
	ids := make([]string, len(roles))
	for i, r := range roles {
		ids[i] = r.ID
	}
	if !slices.Equal(ids, KnownRoles) {
		return fmt.Errorf("roles are %v, want %v", ids, KnownRoles)
	}
	for i, r := range roles {
		if err := checkRole(r, features); err != nil {
			return fmt.Errorf("roles[%d] (%s): %w", i, r.ID, err)
		}
	}
	return nil
}

func checkRole(r Role, features []Feature) error {
	switch {
	case r.Title == "":
		return errors.New("title is missing")
	case r.Summary == "":
		return errors.New("summary is missing")
	case r.Chart == "":
		return errors.New("chart is missing")
	case r.Docs == "":
		return errors.New("docs is missing")
	}
	if err := checkFloor(r.Floor); err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(r.Features)) {
		if !slices.ContainsFunc(features, func(f Feature) bool { return f.ID == id }) {
			return fmt.Errorf("feature %q is not in the catalogue", id)
		}
		if o := r.Features[id]; !slices.Contains(knownOffers, o) {
			return fmt.Errorf("feature %s: offer %q is unknown", id, o)
		}
	}
	for _, f := range features {
		o, ok := r.Features[f.ID]
		if !ok {
			return fmt.Errorf("feature %s is missing", f.ID)
		}
		if o == OfferNotOffered {
			continue
		}
		if f.Chart != r.Chart {
			return fmt.Errorf("offers %s of chart %s, want only features of chart %s", f.ID, f.Chart, r.Chart)
		}
		if r.ID == RoleCustom && o == OfferRecommended {
			return fmt.Errorf("recommends %s; %s must recommend nothing", f.ID, RoleCustom)
		}
		for _, req := range f.Requires {
			switch ro := r.Features[req]; {
			case o == OfferRecommended && ro != OfferRecommended:
				return fmt.Errorf("recommends %s but not its requirement %s", f.ID, req)
			case ro == OfferNotOffered:
				return fmt.Errorf("offers %s but not its requirement %s", f.ID, req)
			}
		}
	}
	return nil
}

func checkFloor(f *Floor) error {
	if f == nil {
		return nil
	}
	if !floorMemory.MatchString(f.Memory) {
		return fmt.Errorf("floor.memory %q does not match %s", f.Memory, floorMemory)
	}
	if !floorCPU.MatchString(f.CPU) {
		return fmt.Errorf("floor.cpu %q does not match %s", f.CPU, floorCPU)
	}
	if _, err := semver.StrictNewVersion(f.MeasuredWith); err != nil {
		return fmt.Errorf("floor.measuredWith %q is not semver: %w", f.MeasuredWith, err)
	}
	return nil
}

func checkRoleMatrix(m *RoleMatrix, roles []Role, features []Feature) error {
	if m == nil {
		return nil
	}
	if len(roles) == 0 {
		return errors.New("roleMatrix needs roles")
	}
	for _, c := range slices.Sorted(maps.Keys(m.Components)) {
		id := m.Components[c]
		if !slices.ContainsFunc(features, func(f Feature) bool { return f.ID == id }) {
			return fmt.Errorf("roleMatrix.components[%q]: %q is not in the catalogue", c, id)
		}
	}
	for i, c := range m.Infrastructure {
		if _, ok := m.Components[c]; ok {
			return fmt.Errorf("roleMatrix.infrastructure[%d]: %q is also a component", i, c)
		}
	}
	return nil
}

// checkFeatureRules makes every rule at stage features a toggle of a catalogue feature, once a catalogue exists.
func checkFeatureRules(rules []CompiledRule, features []Feature) error {
	if len(features) == 0 {
		return nil
	}
	for i, r := range rules {
		if r.Stage != StageFeatures {
			continue
		}
		id, ok := strings.CutSuffix(r.Answer, ToggleSuffix)
		if !ok || !slices.ContainsFunc(features, func(f Feature) bool { return f.ID == id }) {
			return fmt.Errorf("recognize[%d]: answer %q is not <catalogue id>%s", i, r.Answer, ToggleSuffix)
		}
		if r.Kind != KindToggle {
			return fmt.Errorf("recognize[%d]: kind %s at stage %s, want %s", i, r.Kind, StageFeatures, KindToggle)
		}
	}
	return nil
}

// checkCharts runs the checks that need every bundle: a role's or feature's chart is shipped, and a
// feature's keys start with a top-level key of its chart (its own bundle, else that chart's Latest).
func checkCharts(bundles []Bundle) error {
	latest := map[string]Bundle{}
	for _, b := range bundles {
		if b.Latest {
			latest[b.Chart] = b
		}
	}
	for _, b := range bundles {
		file := path.Join(b.Version, bundleName)
		for i, r := range b.Roles {
			if _, ok := latest[r.Chart]; !ok {
				return fmt.Errorf("%s: roles[%d] (%s): chart %q has no shipped bundle", file, i, r.ID, r.Chart)
			}
		}
		for i, f := range b.Features {
			ref, ok := latest[f.Chart]
			if !ok {
				return fmt.Errorf("%s: features[%d] (%s): chart %q has no shipped bundle", file, i, f.ID, f.Chart)
			}
			if f.Chart == b.Chart {
				ref = b
			}
			if err := checkKeyRoots(f.Keys, ref); err != nil {
				return fmt.Errorf("%s: features[%d] (%s): %w", file, i, f.ID, err)
			}
		}
	}
	return nil
}

// checkKeyRoots refuses a key whose first segment is not in ref's topLevelKeys, unless none are listed.
func checkKeyRoots(keys []string, ref Bundle) error {
	if len(ref.TopLevelKeys) == 0 {
		return nil
	}
	for i, k := range keys {
		// checkKeys has parsed every key already.
		p, _ := roundtrip.ParsePath(k)
		if root, _ := p[0].Key(); !slices.Contains(ref.TopLevelKeys, root) {
			return fmt.Errorf("keys[%d]: %q is not in topLevelKeys of %s %s", i, root, ref.Chart, ref.Version)
		}
	}
	return nil
}
