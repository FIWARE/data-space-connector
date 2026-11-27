package check

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// caCoverage returns, over the deployed components of r in table order, one finding naming every component whose
// CA mechanism is not in effect, one warning per withheld mechanism, and one finding per route whose final verify
// key is false (ADR 0025).
func caCoverage(b model.Bundle, r model.CheckRule, tree render.Tree, v chartValues) ([]Finding, error) {
	k := r.CACoverage
	var missing []string
	var warnings []Finding
	deployed := map[string]bool{}
	for _, c := range k.Components {
		if !v.deployed(model.ResourceComponent{EnabledKeys: c.EnabledKeys}) {
			continue
		}
		deployed[c.ID] = true
		switch {
		case c.Mechanism != "":
			i := slices.IndexFunc(tree.Withheld, func(w render.Withheld) bool { return w.Mechanism == c.Mechanism })
			if i >= 0 {
				f, err := withheldFinding(r, c, tree.Withheld[i])
				if err != nil {
					return nil, err
				}
				warnings = append(warnings, f)
			} else if !mechanismInEffect(b, tree, c.Mechanism) {
				missing = append(missing, c.Label)
			}
		default:
			if !pathsInEffect(tree, c.Paths) {
				missing = append(missing, c.Label)
			}
		}
	}
	var out []Finding
	if len(missing) > 0 {
		msg, err := r.RenderMessage(map[string]string{model.MessageComponents: strings.Join(missing, labelSeparator)})
		if err != nil {
			return nil, err
		}
		out = append(out, finding(r, r.Field, "", msg))
	}
	if k.Routes != nil && deployed[k.Routes.Component] {
		fs, err := unverifiedRoutes(r, tree, v)
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
	}
	out = append(out, warnings...)
	return out, nil
}

// mechanismInEffect reports whether every target of mechanism m wrote at least one owned leaf, each in effect.
func mechanismInEffect(b model.Bundle, tree render.Tree, m string) bool {
	for _, f := range b.CompiledFoundations() {
		for i, t := range f.Targets {
			if t.Mechanism != m {
				continue
			}
			found := false
			for _, l := range tree.Leaves {
				if l.Provenance.Foundation != f.ID || l.Provenance.Target != i || l.Provenance.Origin != "" {
					continue
				}
				if !inEffect(l) {
					return false
				}
				found = true
			}
			if !found {
				return false
			}
		}
	}
	return true
}

// pathsInEffect reports whether the tree holds an owned leaf in effect at every one of paths.
func pathsInEffect(tree render.Tree, paths []string) bool {
	for _, s := range paths {
		// The bundle validated every path.
		p, _ := roundtrip.ParsePath(s)
		i := slices.IndexFunc(tree.Leaves, func(l render.Leaf) bool {
			return l.Provenance.Origin == "" && slices.Equal(l.Path, p)
		})
		if i < 0 || !inEffect(tree.Leaves[i]) {
			return false
		}
	}
	return true
}

// inEffect reports whether the chart receives what the tool rendered for l: it is not pinned, or the loaded file
// holds the rendered value. A pinned element leaf never is: its Rendered is the whole element.
func inEffect(l render.Leaf) bool {
	return !l.Provenance.Pinned || (l.Provenance.Kept && reflect.DeepEqual(l.Value, l.Provenance.Rendered))
}

// withheldFinding is the warning for component c, whose mechanism w withholds.
func withheldFinding(r model.CheckRule, c model.CACoverageComponent, w render.Withheld) (Finding, error) {
	k := r.CACoverage
	data := map[string]string{model.MessageComponent: c.Label, model.MessageElement: w.Element, model.MessageList: w.List}
	tpl := k.WithheldEdited
	switch w.Reason {
	case render.WithheldCollision:
		tpl = k.WithheldCollision
		data[model.MessageField], data[model.MessageValue], data[model.MessageForeign] = w.Field, fmt.Sprint(w.Value), w.ForeignKey
	case render.WithheldConflict:
		tpl = k.WithheldConflict
		data = map[string]string{model.MessageComponent: c.Label, model.MessagePaths: strings.Join(w.Conflict, labelSeparator)}
	}
	msg, err := model.RenderTemplate(r.ID, tpl, data)
	if err != nil {
		return Finding{}, err
	}
	return warning(r, msg), nil
}

// unverifiedRoutes returns one finding per route, rendered or loaded, whose verify key ends up false.
func unverifiedRoutes(r model.CheckRule, tree render.Tree, v chartValues) ([]Finding, error) {
	rt := r.CACoverage.Routes
	// The bundle validated both paths.
	routes, _ := roundtrip.ParsePath(rt.Path)
	verify, _ := roundtrip.ParsePath(rt.Verify)
	var out []Finding
	for _, i := range v.indices(routes) {
		route := append(slices.Clone(routes), roundtrip.Index(i))
		if !v.falseAt(slices.Concat(route, verify)) {
			continue
		}
		uri, _ := v.final(append(route, roundtrip.Key(rt.URI)))
		msg, err := model.RenderTemplate(r.ID, r.CACoverage.Route, map[string]string{
			model.MessageIndex: strconv.Itoa(i), model.MessageURI: fmt.Sprint(cmpNil(uri)),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, finding(r, r.Field, "", msg))
	}
	return out, nil
}

// cmpNil maps nil to the empty string, so an absent uri prints as nothing.
func cmpNil(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// noProxyCoverage returns one warning per deployed noProxyServices row whose effective name the resolved answer does
// not bypass for a Go client; none while the answer is unresolved. A row whose name needs the missing release name
// is skipped.
func noProxyCoverage(b model.Bundle, r model.CheckRule, p profile.Profile, answers map[string]any, v chartValues) ([]Finding, error) {
	noProxy, ok := answers[r.NoProxyCoverage.Answer].(string)
	if !ok {
		return nil, nil
	}
	var out []Finding
	for _, s := range b.NoProxyServices() {
		if !v.deployed(model.ResourceComponent{EnabledKeys: s.EnabledKeys}) {
			continue
		}
		name, path, known, err := effectiveName(s, p.Target, v)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.ID, err)
		}
		if !known || model.NoProxyBypasses(noProxy, name) {
			continue
		}
		tpl, data := r.NoProxyCoverage.Unrenamed, map[string]string{model.MessageService: name}
		if path != "" {
			tpl, data[model.MessagePath] = r.Message, path
		}
		msg, err := model.RenderTemplate(r.ID, tpl, data)
		if err != nil {
			return nil, err
		}
		out = append(out, finding(r, r.Field, "", msg))
	}
	return out, nil
}

// effectiveName is the name the chart gives service s under the final values, with the value path that renamed
// it: the first set override, else the name with a set nameOverride in place of the chart name; path is empty
// for the chart default. known is false when the name needs the release name and t has none.
func effectiveName(s model.CompiledNoProxyService, t profile.Target, v chartValues) (name, path string, known bool, err error) {
	for _, o := range s.Overrides {
		if n, ok := v.final(o); ok && n != nil && n != "" {
			return fmt.Sprint(n), o.String(), true, nil
		}
	}
	if s.ReadsRelease && t.ReleaseName == "" {
		return "", "", false, nil
	}
	if s.NameOverride != nil {
		if n, ok := v.final(s.NameOverride); ok && n != nil && n != "" {
			name, err = s.RenderNamed(t.ReleaseName, t.Namespace, fmt.Sprint(n))
			return name, s.NameOverride.String(), true, err
		}
	}
	name, err = s.Render(t.ReleaseName, t.Namespace)
	return name, "", true, err
}

// valueSet reports whether the final values hold a non-empty value at r's path.
func valueSet(r model.CheckRule, v chartValues) bool {
	// The bundle validated the path.
	p, _ := roundtrip.ParsePath(r.ValueSet.Path)
	val, ok := v.final(p)
	if !ok || val == nil {
		return false
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map:
		return rv.Len() > 0
	}
	return true
}

// warning is a finding of r that never blocks, whatever r's severity.
func warning(r model.CheckRule, message string) Finding {
	f := finding(r, r.Field, "", message)
	f.Severity = model.SeverityWarning
	return f
}

// final returns the value the chart receives at p from the values: the rendered leaf, else the loaded node.
func (v chartValues) final(p roundtrip.Path) (any, bool) {
	if val, ok := v.rendered(p); ok {
		return val, true
	}
	n, ok := v.loaded(p)
	if !ok {
		return nil, false
	}
	var out any
	if err := n.Decode(&out); err != nil {
		return nil, false
	}
	return out, true
}

// yaml11False are the plain scalars that Helm's YAML 1.1 reader takes for false and yaml.v3 decodes as strings.
var yaml11False = []string{"no", "off", "n"}

// falseAt reports whether the chart receives false at p. A loaded untagged plain scalar is read as YAML 1.1 reads it.
func (v chartValues) falseAt(p roundtrip.Path) bool {
	if val, ok := v.rendered(p); ok {
		return val == false
	}
	n, ok := v.loaded(p)
	switch {
	case !ok || n.Kind != yaml.ScalarNode:
		return false
	case n.ShortTag() == tagBool:
		var b bool
		return n.Decode(&b) == nil && !b
	case n.Style != 0:
		// Quoted or tagged: a string for YAML 1.1 too.
		return false
	}
	return slices.ContainsFunc(yaml11False, func(s string) bool { return strings.EqualFold(s, n.Value) })
}

// indices lists, sorted, the indices of the list at p in the rendered tree and the loaded file.
func (v chartValues) indices(p roundtrip.Path) []int {
	seen := map[int]bool{}
	for _, l := range v.leaves {
		if len(l.Path) > len(p) && slices.Equal(l.Path[:len(p)], p) {
			if i, ok := l.Path[len(p)].Index(); ok {
				seen[i] = true
			}
		}
	}
	if n, ok := v.loaded(p); ok && n.Kind == yaml.SequenceNode {
		for i := range n.Content {
			seen[i] = true
		}
	}
	out := make([]int, 0, len(seen))
	for i := range seen {
		out = append(out, i)
	}
	slices.Sort(out)
	return out
}
