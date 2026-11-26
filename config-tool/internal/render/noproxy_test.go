package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	wildcard   = "*"
	publicHost = "example.org"
)

// noProxyClients are the NO_PROXY matchers of the five proxied components; each reports whether host bypasses the proxy.
var noProxyClients = map[string]func(noProxy, host string) bool{
	"go":             model.NoProxyBypasses, // verifier, vc-operator
	"keycloak":       keycloakBypass,        // Keycloak
	"proxy-from-env": proxyFromEnvBypass,    // marketplace logic proxy (axios)
	"lua-resty-http": luaBypass,             // APISIX proxy_opts
}

// keycloakBypass ports ProxyMappings.withFixedProxyMapping (keycloak 26.7.0, services/.../ProxyMappings.java
// L110-125): each untrimmed entry becomes the full-match pattern (?:.+\.)?<quoted entry>.
func keycloakBypass(noProxy, host string) bool {
	for _, e := range strings.Split(noProxy, ",") {
		if regexp.MustCompile(`^(?:.+\.)?` + regexp.QuoteMeta(e) + `$`).MatchString(host) {
			return true
		}
	}
	return false
}

// proxyFromEnvBypass ports shouldProxy (proxy-from-env 1.1.0, index.js L62-95) for a URL without an explicit port:
// an entry without a leading '.' or '*' matches exactly, otherwise as a suffix after dropping a leading '*'.
func proxyFromEnvBypass(noProxy, host string) bool {
	noProxy = strings.ToLower(noProxy)
	if noProxy == wildcard {
		return true
	}
	for _, e := range regexp.MustCompile(`[,\s]`).Split(noProxy, -1) {
		switch {
		case e == "":
		case !strings.HasPrefix(e, ".") && !strings.HasPrefix(e, wildcard):
			if host == e {
				return true
			}
		case strings.HasSuffix(host, strings.TrimPrefix(e, wildcard)):
			return true
		}
	}
	return false
}

// luaBypass ports the no_proxy check of api7-lua-resty-http 0.2.3 (lib/resty/http_connect.lua L114-150): entries
// lose one leading '.', then the host and each parent domain match exactly, stopping before the last label.
func luaBypass(noProxy, host string) bool {
	if noProxy == wildcard {
		return true
	}
	set := map[string]bool{}
	for _, m := range regexp.MustCompile(`\.?([^,]+)`).FindAllStringSubmatch(noProxy, -1) {
		set[m[1]] = true
	}
	for {
		if set[host] {
			return true
		}
		_, host, _ = strings.Cut(host, ".")
		if !strings.Contains(host, ".") {
			return false
		}
	}
}

// TestNoProxyDefaultBypasses checks that every client bypasses every in-cluster host in each form the callers dial,
// and still proxies a public host.
func TestNoProxyDefaultBypasses(t *testing.T) {
	b := providerBundle(t)
	p := profile.Profile{Answers: map[string]any{proxyHTTPSID: httpsProxy}, Target: profile.Target{ReleaseName: release, Namespace: namespace}}
	answers, err := Answers(b, p)
	if err != nil {
		t.Fatal(err)
	}
	noProxy, _ := answers[proxyNoID].(string)
	hosts := []string{"localhost"}
	for _, s := range b.NoProxyServices() {
		name, err := s.Render(release, namespace)
		if err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, name, name+"."+namespace+".svc", name+"."+namespace+".svc.cluster.local")
	}
	for client, bypass := range noProxyClients {
		for _, h := range hosts {
			if !bypass(noProxy, h) {
				t.Errorf("%s proxies %s", client, h)
			}
		}
		if bypass(noProxy, publicHost) {
			t.Errorf("%s bypasses the public host %s", client, publicHost)
		}
	}
}

// TestNoProxyMatchers pins each port to the behaviour the plan's NO_PROXY table records.
func TestNoProxyMatchers(t *testing.T) {
	const host = "a.ns.svc"
	tests := []struct {
		entry string
		want  map[string]bool
	}{
		{entry: "svc", want: map[string]bool{"go": true, "keycloak": true, "proxy-from-env": false, "lua-resty-http": false}},
		{entry: ".svc", want: map[string]bool{"go": true, "keycloak": false, "proxy-from-env": true, "lua-resty-http": false}},
		{entry: "*.svc", want: map[string]bool{"go": true, "keycloak": false, "proxy-from-env": true, "lua-resty-http": false}},
		{entry: "ns.svc", want: map[string]bool{"go": true, "keycloak": true, "proxy-from-env": false, "lua-resty-http": true}},
	}
	for _, tc := range tests {
		for client, bypass := range noProxyClients {
			if got := bypass(tc.entry, host); got != tc.want[client] {
				t.Errorf("%s with NO_PROXY=%s bypasses %s: %v, want %v", client, tc.entry, host, got, tc.want[client])
			}
		}
	}
}
