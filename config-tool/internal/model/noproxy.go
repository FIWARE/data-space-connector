package model

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"golang.org/x/net/http/httpproxy"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// NoProxyService is an in-cluster service a proxied component calls, which NO_PROXY must bypass (ADR 0025).
type NoProxyService struct {
	// Name is a template over the reserved target refs only: the service name the chart defaults give.
	Name string `yaml:"name"`
	// Overrides are the value paths that rename the service, in the order the chart reads them.
	Overrides []string `yaml:"overrides"`
	// NameOverride is the value path whose value replaces the chart name in the name's fullname call, as Helm's
	// fullname does with nameOverride; an Overrides value still wins.
	NameOverride string `yaml:"nameOverride"`
	// EnabledKeys maps every switch that must be true for the service to deploy to the chart's default; none means
	// it always deploys.
	EnabledKeys map[string]bool `yaml:"enabledKeys"`
}

// CompiledNoProxyService is a validated NoProxyService.
type CompiledNoProxyService struct {
	Name      *template.Template
	Overrides []roundtrip.Path
	// NameOverride is nil when the row declares none.
	NameOverride roundtrip.Path
	EnabledKeys  map[string]bool
	// ReadsRelease is true when the name template reads the release name, so it cannot render without one.
	ReadsRelease bool
}

// NoProxyServices returns the bundle's noProxyServices table in bundle order.
func (b Bundle) NoProxyServices() []CompiledNoProxyService {
	return b.noProxyServices
}

// Render returns the service name for a release in a namespace with the chart defaults.
func (s CompiledNoProxyService) Render(release, namespace string) (string, error) {
	return s.RenderNamed(release, namespace, "")
}

// RenderNamed is Render with a nameOverride value; an empty one keeps the chart name.
func (s CompiledNoProxyService) RenderNamed(release, namespace, nameOverride string) (string, error) {
	t := s.Name
	if nameOverride != "" {
		// Clone keeps the shared compiled template free of the per-call function.
		c, err := s.Name.Clone()
		if err != nil {
			return "", err
		}
		t = c.Funcs(template.FuncMap{fullnameFunc: func(release, _ string) string { return Fullname(release, nameOverride) }})
	}
	ns := map[string]any{"target": map[string]any{"release": release, "namespace": namespace}}
	var b strings.Builder
	if err := t.Execute(&b, ns); err != nil {
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
	readsRelease := false
	for _, r := range refs {
		readsRelease = readsRelease || r == RefTargetRelease
		if r != RefTargetRelease && r != RefTargetNamespace {
			return CompiledNoProxyService{}, fmt.Errorf("name reads %s; only %s and %s are allowed", r, RefTargetRelease, RefTargetNamespace)
		}
	}
	c := CompiledNoProxyService{Name: t, EnabledKeys: s.EnabledKeys, ReadsRelease: readsRelease}
	for _, key := range sortedKeys(s.EnabledKeys) {
		if err := checkResourcePath(key, false, topLevelKeys); err != nil {
			return CompiledNoProxyService{}, fmt.Errorf("enabledKeys: %w", err)
		}
	}
	for j, o := range s.Overrides {
		p, err := overridePath(o, topLevelKeys)
		if err != nil {
			return CompiledNoProxyService{}, fmt.Errorf("overrides[%d]: %w", j, err)
		}
		c.Overrides = append(c.Overrides, p)
	}
	if s.NameOverride != "" {
		if !callsFullname(t) {
			return CompiledNoProxyService{}, fmt.Errorf("nameOverride %q: name has no %s call to rename", s.NameOverride, fullnameFunc)
		}
		if c.NameOverride, err = overridePath(s.NameOverride, topLevelKeys); err != nil {
			return CompiledNoProxyService{}, fmt.Errorf("nameOverride: %w", err)
		}
	}
	return c, nil
}

func overridePath(o string, topLevelKeys []string) (roundtrip.Path, error) {
	p, err := roundtrip.ParsePath(o)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, fmt.Errorf("path is empty")
	}
	if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
		return nil, fmt.Errorf("%q: root is not in topLevelKeys", o)
	}
	return p, nil
}

func callsFullname(t *template.Template) bool {
	for _, n := range t.Root.Nodes {
		if a, ok := n.(*parse.ActionNode); ok && fullnameField(a) != nil {
			return true
		}
	}
	return false
}

// bypassProbeProxy is a proxy URL that only lets the matcher tell a bypassed host from a proxied one.
const bypassProbeProxy = "http://proxy.invalid:3128"

// NoProxyBypasses reports whether a Go client (golang.org/x/net/http/httpproxy, which net/http's
// ProxyFromEnvironment wraps) with noProxy as NO_PROXY reaches https://host without the proxy.
func NoProxyBypasses(noProxy, host string) bool {
	proxy := (&httpproxy.Config{HTTPSProxy: bypassProbeProxy, NoProxy: noProxy}).ProxyFunc()
	u, err := proxy(&url.URL{Scheme: "https", Host: host})
	return err == nil && u == nil
}
