package check

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// YAML short tags the rule reads loaded scalars by.
const (
	tagBool = "!!bool"
	tagNull = "!!null"
)

// labelSeparator joins the labels of the unlimited components in a memoryLimit message.
const labelSeparator = ", "

// memoryLimit returns one finding naming every deployed component of r without a memory limit, in table order
// (components, then unsized); none when every one has a limit. Deployment is read from enabledKeys, never from
// features: the tool renders no enabled key until US-5.3, so the chart default decides (ADR 0024).
func memoryLimit(b model.Bundle, r model.CheckRule, v chartValues) ([]Finding, error) {
	var labels []string
	for _, c := range slices.Concat(b.Resources.Components, b.Resources.Unsized) {
		if !slices.Contains(r.MemoryLimit.Components, c.ID) || !v.deployed(c) {
			continue
		}
		p, err := roundtrip.ParsePath(c.Path)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %s: %w", r.ID, c.ID, err)
		}
		for _, in := range v.instances(c, p) {
			if !v.limited(c, in.path) {
				labels = append(labels, in.label)
			}
		}
	}
	if len(labels) == 0 {
		return nil, nil
	}
	msg, err := r.RenderMessage(map[string]string{model.MessageComponents: strings.Join(labels, labelSeparator)})
	if err != nil {
		return nil, err
	}
	return []Finding{finding(r, r.Field, "", msg)}, nil
}

// chartValues are the values the chart receives: the rendered tree over the loaded file.
type chartValues struct {
	leaves map[string]render.Leaf
	// res is the loaded file; nil when nothing is loaded.
	res *load.Result
}

func newChartValues(tree render.Tree, res *load.Result) chartValues {
	leaves := make(map[string]render.Leaf, len(tree.Leaves))
	for _, l := range tree.Leaves {
		leaves[l.Path.String()] = l
	}
	return chartValues{leaves: leaves, res: res}
}

// rendered returns the value the tree writes at p; a pinned leaf defers to the loaded file, so it is not one.
func (v chartValues) rendered(p roundtrip.Path) (any, bool) {
	l, ok := v.leaves[p.String()]
	if !ok || l.Provenance.Pinned {
		return nil, false
	}
	return l.Value, true
}

// loaded returns the node of the loaded file at chart path p.
func (v chartValues) loaded(p roundtrip.Path) (*yaml.Node, bool) {
	if v.res == nil {
		return nil, false
	}
	n, err := v.res.Doc.Lookup(slices.Concat(v.res.Root, p))
	return n, err == nil
}

// loadedTag returns the short tag of the loaded scalar at p; empty when p holds no scalar.
func (v chartValues) loadedTag(p roundtrip.Path) string {
	n, ok := v.loaded(p)
	if !ok || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.ShortTag()
}

// deployed reports whether every enabledKey of c is effectively true: rendered, else loaded, else its default.
// A value that is not a bool falls back to the default.
func (v chartValues) deployed(c model.ResourceComponent) bool {
	for key, def := range c.EnabledKeys {
		// The bundle validated every key.
		p, _ := roundtrip.ParsePath(key)
		on := def
		if r, ok := v.rendered(p); ok {
			if b, isBool := r.(bool); isBool {
				on = b
			}
		} else if n, ok := v.loaded(p); ok && n.Kind == yaml.ScalarNode && n.ShortTag() == tagBool {
			_ = n.Decode(&on)
		}
		if !on {
			return false
		}
	}
	return true
}

// instance is one workload of a component: a wildcard path yields one per key of its map.
type instance struct {
	label string
	path  roundtrip.Path
}

// instances expands the one wildcard segment of p over the keys of the map at its prefix, in the rendered tree
// and the loaded file, sorted; a loaded null key deploys nothing. Without a wildcard, c itself is the instance.
func (v chartValues) instances(c model.ResourceComponent, p roundtrip.Path) []instance {
	i := slices.Index(p, roundtrip.Key(model.ResourceWildcard))
	if i < 0 {
		return []instance{{c.Label, p}}
	}
	prefix := p[:i]
	keys := map[string]bool{}
	for _, l := range v.leaves {
		if len(l.Path) > i && slices.Equal(l.Path[:i], prefix) {
			if k, ok := l.Path[i].Key(); ok {
				keys[k] = true
			}
		}
	}
	if n, ok := v.loaded(prefix); ok && n.Kind == yaml.MappingNode {
		for j := 0; j+1 < len(n.Content); j += 2 {
			if k, val := n.Content[j], n.Content[j+1]; val.ShortTag() != tagNull {
				keys[k.Value] = true
			}
		}
	}
	var out []instance
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		out = append(out, instance{
			label: fmt.Sprintf("%s (%s)", c.Label, k),
			path:  slices.Concat(prefix, roundtrip.Path{roundtrip.Key(k)}, p[i+1:]),
		})
	}
	return out
}

// limited reports whether the resources block at p ends up with a memory limit: one set at p or a fallback, or the
// chart's own unless a loaded null at p, p.limits or p.limits.memory drops it, as Helm's coalesce does.
func (v chartValues) limited(c model.ResourceComponent, p roundtrip.Path) bool {
	if v.hasLimit(p) {
		return true
	}
	for _, fb := range c.FallbackPaths {
		if q, err := roundtrip.ParsePath(fb); err == nil && v.hasLimit(q) {
			return true
		}
	}
	if c.ChartMemoryLimit == "" {
		return false
	}
	for _, q := range []roundtrip.Path{p, memoryLimitPath(p)[:len(p)+1], memoryLimitPath(p)} {
		if v.loadedTag(q) == tagNull {
			return false
		}
	}
	return true
}

// hasLimit reports whether the values set a memory limit in the resources block at p.
func (v chartValues) hasLimit(p roundtrip.Path) bool {
	q := memoryLimitPath(p)
	if val, ok := v.rendered(q); ok {
		return val != nil
	}
	tag := v.loadedTag(q)
	return tag != "" && tag != tagNull
}

func memoryLimitPath(p roundtrip.Path) roundtrip.Path {
	return slices.Concat(p, roundtrip.Path{roundtrip.Key(model.ResourceLimits), roundtrip.Key(model.ResourceMemory)})
}
