package model

import (
	"errors"
	"fmt"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// MechanismNone marks a caCoverage component the tool has no CA mechanism for yet (ADR 0025).
const MechanismNone = "none"

// Keys of the caCoverage and noProxyCoverage message templates.
const (
	// MessageComponent is the label of the one component a withheld finding concerns.
	MessageComponent = "component"
	// MessageElement is the key of the owned element a withheld mechanism would have written.
	MessageElement = "element"
	// MessageList is the list path of a withheld element.
	MessageList = "list"
	// MessageField is the unique field a collision shares.
	MessageField = "field"
	// MessageValue is the value of that field.
	MessageValue = "value"
	// MessageForeign is the key of the foreign element a collision meets.
	MessageForeign = "foreign"
	// MessagePaths are the loaded paths of a conflict.
	MessagePaths = "paths"
	// MessageIndex is the index of an APISIX route.
	MessageIndex = "index"
	// MessageURI is the uri of an APISIX route; empty when it has none.
	MessageURI = "uri"
	// MessageService is the effective name of a service NO_PROXY misses.
	MessageService = "service"
	// MessagePath is the value path that renamed that service.
	MessagePath = "path"
)

// RuleCACoverage checks that every deployed component trusts the private CA through its mechanism (ADR 0025).
// The rule's severity applies to a mechanism not in effect and to a route that does not verify; a withheld mechanism
// and a component without one are warnings, since the tool cannot tell whether a hand-made setup trusts the CA.
type RuleCACoverage struct {
	Components []CACoverageComponent `yaml:"components"`
	Routes     *CACoverageRoutes     `yaml:"routes"`
	// The message templates of the findings other than a mechanism not in effect, which is the rule's message.
	WithheldCollision string `yaml:"withheldCollision"`
	WithheldEdited    string `yaml:"withheldEdited"`
	WithheldConflict  string `yaml:"withheldConflict"`
	None              string `yaml:"none"`
	Route             string `yaml:"route"`
}

// CACoverageComponent is one component that calls a host the private CA may sign. It has exactly one of Mechanism,
// whose element targets must all be in effect, or MechanismNone, and Paths, scalar targets that must be.
type CACoverageComponent struct {
	ID        string   `yaml:"id"`
	Label     string   `yaml:"label"`
	Mechanism string   `yaml:"mechanism"`
	Paths     []string `yaml:"paths"`
	// EnabledKeys maps every switch that must be true for the component to deploy to the chart's default.
	EnabledKeys map[string]bool `yaml:"enabledKeys"`
}

// CACoverageRoutes are the APISIX routes whose Verify key, an each target, must not end up false.
type CACoverageRoutes struct {
	Path   string `yaml:"path"`
	Verify string `yaml:"verify"`
	// URI is the key of a route that names it in a finding.
	URI string `yaml:"uri"`
	// Component is the components entry that deploys the routes; while it does not, they are not checked.
	Component string `yaml:"component"`
}

// RuleNoProxyCoverage warns per noProxyServices row whose effective name the final Answer does not bypass.
// The rule's message covers a renamed service; Unrenamed one with the name the chart defaults give.
type RuleNoProxyCoverage struct {
	Answer    string `yaml:"answer"`
	Unrenamed string `yaml:"unrenamed"`
}

// RuleValueSet holds while the chart values, rendered over loaded, set a non-empty value at Path.
type RuleValueSet struct {
	Path string `yaml:"path"`
}

var (
	collisionKeys = []string{MessageComponent, MessageElement, MessageList, MessageField, MessageValue, MessageForeign}
	editedKeys    = []string{MessageComponent, MessageElement, MessageList}
	conflictKeys  = []string{MessageComponent, MessagePaths}
	routeKeys     = []string{MessageIndex, MessageURI}
)

func checkCACoverage(r CheckRule, c ruleContext) error {
	k := r.CACoverage
	if len(k.Components) == 0 {
		return errors.New("caCoverage: components is empty")
	}
	var ids []string
	for i, comp := range k.Components {
		if err := checkCAComponent(comp, ids, c); err != nil {
			return fmt.Errorf("caCoverage: components[%d]: %w", i, err)
		}
		ids = append(ids, comp.ID)
	}
	if k.Routes != nil {
		if err := checkCARoutes(*k.Routes, ids, c); err != nil {
			return fmt.Errorf("caCoverage: routes: %w", err)
		}
		if err := checkTemplate(r.ID, "caCoverage: route", k.Route, routeKeys); err != nil {
			return err
		}
	}
	for _, m := range []struct {
		name, s string
		keys    []string
	}{
		{"message", r.Message, []string{MessageComponents}},
		{"caCoverage: withheldCollision", k.WithheldCollision, collisionKeys},
		{"caCoverage: withheldEdited", k.WithheldEdited, editedKeys},
		{"caCoverage: withheldConflict", k.WithheldConflict, conflictKeys},
		{"caCoverage: none", k.None, []string{MessageComponents}},
	} {
		if err := checkTemplate(r.ID, m.name, m.s, m.keys); err != nil {
			return err
		}
	}
	return nil
}

func checkCAComponent(comp CACoverageComponent, ids []string, c ruleContext) error {
	switch {
	case comp.ID == "" || comp.Label == "":
		return errors.New("id and label are required")
	case slices.Contains(ids, comp.ID):
		return fmt.Errorf("id %q is already used", comp.ID)
	case len(comp.EnabledKeys) == 0:
		return fmt.Errorf("%s: enabledKeys is empty", comp.ID)
	case (comp.Mechanism == "") == (len(comp.Paths) == 0):
		return fmt.Errorf("%s: has exactly one of mechanism and paths", comp.ID)
	}
	for _, key := range sortedKeys(comp.EnabledKeys) {
		if err := checkResourcePath(key, false, c.topLevelKeys); err != nil {
			return fmt.Errorf("%s: enabledKeys: %w", comp.ID, err)
		}
	}
	if comp.Mechanism != "" && comp.Mechanism != MechanismNone &&
		!hasTarget(c.fs, func(t CompiledTarget) bool { return t.Mechanism == comp.Mechanism }) {
		return fmt.Errorf("%s: mechanism %q is no target's mechanism", comp.ID, comp.Mechanism)
	}
	for _, s := range comp.Paths {
		p, err := roundtrip.ParsePath(s)
		if err != nil {
			return fmt.Errorf("%s: paths: %w", comp.ID, err)
		}
		if !hasTarget(c.fs, func(t CompiledTarget) bool { return t.Element == nil && t.Each == nil && slices.Equal(t.Path, p) }) {
			return fmt.Errorf("%s: paths: %q is no scalar target's path", comp.ID, s)
		}
	}
	return nil
}

func checkCARoutes(rt CACoverageRoutes, ids []string, c ruleContext) error {
	p, err := roundtrip.ParsePath(rt.Path)
	if err != nil {
		return err
	}
	v, err := roundtrip.ParsePath(rt.Verify)
	switch {
	case err != nil:
		return err
	case rt.URI == "":
		return errors.New("uri is missing")
	case !slices.Contains(ids, rt.Component):
		return fmt.Errorf("component %q names no entry of components", rt.Component)
	case !hasTarget(c.fs, func(t CompiledTarget) bool { return slices.Equal(t.Path, p) && slices.Equal(t.Each, v) }):
		return fmt.Errorf("%s below every element of %s is no each target", rt.Verify, rt.Path)
	}
	return nil
}

func checkNoProxyCoverage(r CheckRule, fs []CompiledFoundation) error {
	k := r.NoProxyCoverage
	if !slices.ContainsFunc(fs, func(f CompiledFoundation) bool { return f.ID == k.Answer }) {
		return fmt.Errorf("noProxyCoverage: answer %q names no foundation", k.Answer)
	}
	if err := checkTemplate(r.ID, "message", r.Message, []string{MessageService, MessagePath}); err != nil {
		return err
	}
	return checkTemplate(r.ID, "noProxyCoverage: unrenamed", k.Unrenamed, []string{MessageService})
}

func checkValueSet(r CheckRule, topLevelKeys []string) error {
	if err := checkResourcePath(r.ValueSet.Path, false, topLevelKeys); err != nil {
		return fmt.Errorf("valueSet: path: %w", err)
	}
	return nil
}

// hasTarget reports whether any target of fs satisfies ok.
func hasTarget(fs []CompiledFoundation, ok func(CompiledTarget) bool) bool {
	return slices.ContainsFunc(fs, func(f CompiledFoundation) bool { return slices.ContainsFunc(f.Targets, ok) })
}
