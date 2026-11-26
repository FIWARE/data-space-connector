package model

import (
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// NoProxyService is an in-cluster service a proxied component calls, which NO_PROXY must bypass (ADR 0025).
type NoProxyService struct {
	// Name is a template over the reserved target refs only: the service name the chart defaults give.
	Name string `yaml:"name"`
	// Overrides are the value paths that rename the service, in the order the chart reads them.
	Overrides []string `yaml:"overrides"`
}

// CompiledNoProxyService is a validated NoProxyService.
type CompiledNoProxyService struct {
	Name      *template.Template
	Overrides []roundtrip.Path
}

// NoProxyServices returns the bundle's noProxyServices table in bundle order.
func (b Bundle) NoProxyServices() []CompiledNoProxyService {
	return b.noProxyServices
}

// Render returns the service name for a release in a namespace.
func (s CompiledNoProxyService) Render(release, namespace string) (string, error) {
	ns := map[string]any{"target": map[string]any{"release": release, "namespace": namespace}}
	var b strings.Builder
	if err := s.Name.Execute(&b, ns); err != nil {
		return "", err
	}
	return b.String(), nil
}

func compileNoProxyServices(ss []NoProxyService, topLevelKeys []string) ([]CompiledNoProxyService, error) {
	var compiled []CompiledNoProxyService
	for i, s := range ss {
		c, err := compileNoProxyService(i, s, topLevelKeys)
		if err != nil {
			return nil, fmt.Errorf("noProxyServices[%d] (%s): %w", i, s.Name, err)
		}
		compiled = append(compiled, c)
	}
	return compiled, nil
}

func compileNoProxyService(i int, s NoProxyService, topLevelKeys []string) (CompiledNoProxyService, error) {
	if s.Name == "" {
		return CompiledNoProxyService{}, fmt.Errorf("name is missing")
	}
	t, refs, err := parseTemplate(fmt.Sprintf("noProxyServices[%d]", i), s.Name)
	if err != nil {
		return CompiledNoProxyService{}, err
	}
	for _, r := range refs {
		if r != RefTargetRelease && r != RefTargetNamespace {
			return CompiledNoProxyService{}, fmt.Errorf("name reads %s; only %s and %s are allowed", r, RefTargetRelease, RefTargetNamespace)
		}
	}
	c := CompiledNoProxyService{Name: t}
	for j, o := range s.Overrides {
		p, err := roundtrip.ParsePath(o)
		if err != nil {
			return CompiledNoProxyService{}, fmt.Errorf("overrides[%d]: %w", j, err)
		}
		if len(p) == 0 {
			return CompiledNoProxyService{}, fmt.Errorf("overrides[%d] is empty", j)
		}
		if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
			return CompiledNoProxyService{}, fmt.Errorf("overrides[%d] %q: root is not in topLevelKeys", j, o)
		}
		c.Overrides = append(c.Overrides, p)
	}
	return c, nil
}
