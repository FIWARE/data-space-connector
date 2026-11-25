package model

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Masterminds/semver/v3"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Leaf keys of a Kubernetes resources block; a table target is <path>.<requests|limits>.<cpu|memory>.
const (
	ResourceRequests = "requests"
	ResourceLimits   = "limits"
	ResourceCPU      = "cpu"
	ResourceMemory   = "memory"
)

// ResourceWildcard is the one path segment that stands for every key of a map, allowed only in an unsized path.
const ResourceWildcard = "*"

// ResourceTable is the bundle's resources: section, the figures a resource profile renders (ADR 0024).
type ResourceTable struct {
	// Answer is the choice foundation whose resolved value selects a row of Profiles.
	Answer string `yaml:"answer"`
	// MeasuredOn is the chart version the sizing guidance measured the figures on.
	MeasuredOn string `yaml:"measuredOn"`
	// Doc is a repo-relative documentation path, optionally with an anchor.
	Doc        string              `yaml:"doc"`
	Components []ResourceComponent `yaml:"components"`
	// Unsized are JVM components the profiles do not size; only the memoryLimit rule reads them.
	Unsized []ResourceComponent `yaml:"unsized"`
	// Profiles maps a choice value to the spec of every entry of Components, by component id.
	Profiles map[string]map[string]ResourceSpec `yaml:"profiles"`
}

// ResourceComponent is one workload's resources block.
type ResourceComponent struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	// Path is the resources block in roundtrip.ParsePath syntax; an unsized one may hold one ResourceWildcard.
	Path string `yaml:"path"`
	// FallbackPaths are resources blocks whose limit also covers Path, such as a map's common defaults.
	FallbackPaths []string `yaml:"fallbackPaths"`
	// EnabledKeys maps every switch that must be true for the workload to deploy to the chart's default.
	EnabledKeys map[string]bool `yaml:"enabledKeys"`
	// ChartMemoryLimit is the memory limit the chart sets by default; empty when it sets none.
	ChartMemoryLimit string `yaml:"chartMemoryLimit"`
}

// ResourceSpec is the requests and limits a profile renders for one component.
type ResourceSpec struct {
	Requests ResourceQuantities `yaml:"requests"`
	Limits   ResourceQuantities `yaml:"limits"`
}

// ResourceQuantities are Kubernetes quantities, kept as the strings the chart receives.
type ResourceQuantities struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

// checkResources validates the table against the compiled foundations; nil when there is no table.
func checkResources(t *ResourceTable, fs []CompiledFoundation, topLevelKeys []string) error {
	if t == nil {
		return nil
	}
	if err := checkResourceTable(t, fs, topLevelKeys); err != nil {
		return fmt.Errorf("resources: %w", err)
	}
	return nil
}

// addResourceTargets appends a const target per profile, component and quantity to the table's answer
// foundation, written only while that answer resolves to the profile; the table must have passed checkResources.
func addResourceTargets(fs []CompiledFoundation, t *ResourceTable) error {
	if t == nil {
		return nil
	}
	i := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == t.Answer })
	for _, name := range sortedKeys(t.Profiles) {
		for _, c := range t.Components {
			p, err := roundtrip.ParsePath(c.Path)
			if err != nil {
				return fmt.Errorf("resources: components (%s): path: %w", c.ID, err)
			}
			spec := t.Profiles[name][c.ID]
			for _, q := range []struct{ kind, name, value string }{
				{ResourceRequests, ResourceCPU, spec.Requests.CPU},
				{ResourceRequests, ResourceMemory, spec.Requests.Memory},
				{ResourceLimits, ResourceCPU, spec.Limits.CPU},
				{ResourceLimits, ResourceMemory, spec.Limits.Memory},
			} {
				fs[i].Targets = append(fs[i].Targets, CompiledTarget{
					Path:  slices.Concat(p, roundtrip.Path{roundtrip.Key(q.kind), roundtrip.Key(q.name)}),
					Fn:    FnConst,
					Value: q.value,
					Refs:  []string{t.Answer},
					Only:  name,
					Block: p,
				})
			}
		}
	}
	return nil
}

func checkResourceTable(t *ResourceTable, fs []CompiledFoundation, topLevelKeys []string) error {
	i := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == t.Answer })
	switch {
	case t.Answer == "":
		return fmt.Errorf("answer is missing")
	case i < 0:
		return fmt.Errorf("answer %q names no foundation", t.Answer)
	case fs[i].Type != TypeChoice:
		return fmt.Errorf("answer %s is a %s, not a %s", t.Answer, fs[i].Type, TypeChoice)
	}
	if _, err := semver.StrictNewVersion(t.MeasuredOn); err != nil {
		return fmt.Errorf("measuredOn %q is not semver", t.MeasuredOn)
	}
	if err := checkDoc(t.Doc); err != nil {
		return err
	}
	ids := map[string]string{}
	for _, list := range []struct {
		name       string
		components []ResourceComponent
		wildcard   bool
	}{{"components", t.Components, false}, {"unsized", t.Unsized, true}} {
		for j, c := range list.components {
			if err := checkResourceComponent(c, list.wildcard, topLevelKeys); err != nil {
				return fmt.Errorf("%s[%d] (%s): %w", list.name, j, c.ID, err)
			}
			if where, dup := ids[c.ID]; dup {
				return fmt.Errorf("%s[%d]: id %q is already declared in %s", list.name, j, c.ID, where)
			}
			ids[c.ID] = fmt.Sprintf("%s[%d]", list.name, j)
		}
	}
	options := fs[i].optionValues()
	for _, name := range sortedKeys(t.Profiles) {
		if !slices.Contains(options, name) {
			return fmt.Errorf("profiles: %q is not an option of %s", name, t.Answer)
		}
		if err := checkResourceProfile(t.Profiles[name], t); err != nil {
			return fmt.Errorf("profiles.%s: %w", name, err)
		}
	}
	return nil
}

func checkResourceComponent(c ResourceComponent, wildcard bool, topLevelKeys []string) error {
	if err := checkID(c.ID); err != nil {
		return err
	}
	if c.Label == "" {
		return fmt.Errorf("label is missing")
	}
	if err := checkResourcePath(c.Path, wildcard, topLevelKeys); err != nil {
		return fmt.Errorf("path: %w", err)
	}
	for i, p := range c.FallbackPaths {
		if err := checkResourcePath(p, false, topLevelKeys); err != nil {
			return fmt.Errorf("fallbackPaths[%d]: %w", i, err)
		}
	}
	for _, k := range sortedKeys(c.EnabledKeys) {
		if err := checkResourcePath(k, false, topLevelKeys); err != nil {
			return fmt.Errorf("enabledKeys %q: %w", k, err)
		}
	}
	if c.ChartMemoryLimit != "" {
		if _, err := resource.ParseQuantity(c.ChartMemoryLimit); err != nil {
			return fmt.Errorf("chartMemoryLimit %q is not a quantity", c.ChartMemoryLimit)
		}
	}
	return nil
}

// checkResourcePath refuses a path that does not parse, has an index, or has a root outside topLevelKeys; a
// ResourceWildcard segment only where wildcard allows it, once and not first.
func checkResourcePath(s string, wildcard bool, topLevelKeys []string) error {
	p, err := roundtrip.ParsePath(s)
	if err != nil {
		return err
	}
	if len(p) == 0 {
		return fmt.Errorf("is missing")
	}
	stars := 0
	for i, seg := range p {
		k, ok := seg.Key()
		switch {
		case !ok:
			return fmt.Errorf("%q: an index cannot address a resources block", s)
		case k != ResourceWildcard:
		case !wildcard:
			return fmt.Errorf("%q: %s is only allowed in an unsized path", s, ResourceWildcard)
		case i == 0:
			return fmt.Errorf("%q: %s cannot be the first segment", s, ResourceWildcard)
		default:
			stars++
		}
	}
	if stars > 1 {
		return fmt.Errorf("%q: %s may appear only once", s, ResourceWildcard)
	}
	if k, _ := p[0].Key(); !slices.Contains(topLevelKeys, k) {
		return fmt.Errorf("%q: root is not in topLevelKeys", s)
	}
	return nil
}

// checkResourceProfile requires a full spec, request at most limit, for every sized component and none other.
func checkResourceProfile(row map[string]ResourceSpec, t *ResourceTable) error {
	for _, c := range t.Components {
		spec, ok := row[c.ID]
		if !ok {
			return fmt.Errorf("component %s is missing", c.ID)
		}
		if err := checkResourceSpec(spec); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
	}
	for _, id := range sortedKeys(row) {
		if !slices.ContainsFunc(t.Components, func(c ResourceComponent) bool { return c.ID == id }) {
			return fmt.Errorf("%q names no entry of components", id)
		}
	}
	return nil
}

func checkResourceSpec(s ResourceSpec) error {
	for _, q := range []struct{ name, request, limit string }{
		{ResourceCPU, s.Requests.CPU, s.Limits.CPU},
		{ResourceMemory, s.Requests.Memory, s.Limits.Memory},
	} {
		req, err := parseQuantity(ResourceRequests, q.name, q.request)
		if err != nil {
			return err
		}
		lim, err := parseQuantity(ResourceLimits, q.name, q.limit)
		if err != nil {
			return err
		}
		if req.Cmp(lim) > 0 {
			return fmt.Errorf("%s.%s %s exceeds %s.%s %s", ResourceRequests, q.name, q.request, ResourceLimits, q.name, q.limit)
		}
	}
	return nil
}

func parseQuantity(kind, name, s string) (resource.Quantity, error) {
	if s == "" {
		return resource.Quantity{}, fmt.Errorf("%s.%s is missing", kind, name)
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("%s.%s %q is not a quantity", kind, name, s)
	}
	return q, nil
}

// sortedKeys returns m's keys sorted, so errors are reported in a stable order.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
