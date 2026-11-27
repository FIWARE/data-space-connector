package check

import (
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

const (
	caRule         = "ca-coverage"
	caAnswer       = "ca.privateSecret"
	caSecret       = "private-ca"
	noProxyRule    = "no-proxy-coverage"
	mappingsRule   = "keycloak-proxy-mappings"
	httpsAnswer    = "proxy.https"
	noProxyAnswer  = "proxy.no"
	proxyURL       = "http://proxy.example.org:3128"
	release        = "provider"
	namespace      = "provider"
	withheldPrefix = " does not get the private CA"
	notReached     = "The private CA does not reach "

	apisixRoutes = "decentralizedIam:\n  odrlAuthorization:\n    apisix:\n      routes:\n" +
		"        - uri: /a\n          plugins:\n            openid-connect:\n"
	verifierMounts = "decentralizedIam:\n  vcAuthentication:\n    vcverifier:\n      deployment:\n" +
		"        additionalVolumeMounts:\n          - name: root-ca\n            mountPath: /etc/ssl/cert.pem\n" +
		"            subPath: cert.pem\n"
	keycloakEnv  = "keycloak:\n  extraEnvVars:\n    - name: KC_TRUSTSTORE_PATHS\n      value: "
	keycloakIdp  = "keycloak:\n  fullnameOverride: idp\n"
	enableAddOns = "marketplace:\n  enabled: true\nvc-operator:\n  enabled: true\n"
	// contractEnv sets JDK_JAVA_OPTIONS for something else, as a user adding an agent would.
	contractEnv   = "contract-management:\n  additionalEnvVars:\n    - name: JDK_JAVA_OPTIONS\n      value: -Xss1m\n"
	contractMount = "contract-management:\n  deployment:\n    additionalVolumeMounts:\n" +
		"      - name: my-store\n        mountPath: /ca-store\n"
	enableTracing = "opentelemetry-collector:\n  enabled: true\n"
	// proxyMappings is the string form the chart declares, with the value k3s/provider.yaml sets.
	proxyMappings = "keycloak:\n  proxy:\n    spiProxyMappings: \".*\\\\.127\\\\.0\\\\.0\\\\.1\\\\.nip\\\\.io;http://squid-proxy.infra.svc.cluster.local:8888\"\n"
	tilRenamed    = "decentralizedIam:\n  vcAuthentication:\n    trusted-issuers-list:\n      fullnameOverride: til\n"
)

// targetProfile is a profile whose release and namespace resolve the NO_PROXY default.
func targetProfile(answers map[string]any) profile.Profile {
	return profile.Profile{Target: profile.Target{ReleaseName: release, Namespace: namespace}, Answers: answers}
}

// split returns the messages of findings by severity.
func split(findings []Finding) (errs, warns []string) {
	for _, f := range findings {
		if f.Severity == model.SeverityError {
			errs = append(errs, f.Message)
		} else {
			warns = append(warns, f.Message)
		}
	}
	return errs, warns
}

// wantPrefixes fails unless every message starts with its prefix, in order and in equal number.
func wantPrefixes(t *testing.T, kind string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %q, want %d starting with %q", kind, got, len(want), want)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("%s[%d] = %q, want it to start with %q", kind, i, got[i], want[i])
		}
	}
}

// TestCACoverage pins ca-coverage: owned leaves in effect cover, pinned ones do not, withheld mechanisms and
// components without one warn, and a route that does not verify errors.
func TestCACoverage(t *testing.T) {
	b := shipped(t)
	tests := []struct {
		name   string
		noCA   bool
		loaded string
		// errs and warns are the message prefixes the rule reports, errors first.
		errs, warns []string
	}{
		{name: "CA unset", noCA: true, loaded: keycloakEnv + "/other\n" + apisixRoutes + "              ssl_verify: false\n"},
		{name: "all covered"},
		{name: "marketplace and vc-operator covered", loaded: enableAddOns},
		{name: "contract-management pinned away", loaded: contractEnv,
			errs: []string{notReached + "contract-management:"}},
		{name: "contract-management disabled", loaded: contractEnv + "  enabled: false\n"},
		{name: "contract-management withheld by a foreign mount", loaded: contractMount,
			warns: []string{"contract-management" + withheldPrefix + ": its element ca-store would share mountPath"}},
		{name: "scalar deleted by a loaded null",
			loaded: "decentralizedIam:\n  odrlAuthorization:\n    apisix:\n      apisix:\n        ssl:\n          existingCASecret: null\n",
			errs:   []string{notReached + "APISIX:"}},
		{name: "element missing from the loaded list is added again",
			loaded: "keycloak:\n  extraEnvVars:\n    - name: KC_HEAP_SIZE\n      value: 2g\n"},
		{name: "owned element pinned to another value", loaded: keycloakEnv + "/other\n",
			errs: []string{notReached + "Keycloak:"}},
		{name: "owned element with the rendered value",
			loaded: keycloakEnv + "/opt/keycloak/conf/truststores/private-ca.pem\n"},
		{name: "two components pinned away", loaded: keycloakEnv + "/other\n" +
			"vc-operator:\n  enabled: true\n  extraEnvVars:\n    - name: SSL_CERT_DIR\n      value: /etc/ssl/certs\n",
			errs: []string{notReached + "Keycloak, vc-operator:"}},
		{name: "verifier withheld by the k3s root-ca mount", loaded: verifierMounts,
			warns: []string{"verifier" + withheldPrefix + ": its element private-ca-bundle would share mountPath"}},
		{name: "keycloak list edited", loaded: "keycloak:\n  extraEnvVars: {}\n",
			warns: []string{"Keycloak" + withheldPrefix + ": the loaded file changed"}},
		{name: "verifier conflicts with the chart's caCert",
			loaded: "decentralizedIam:\n  vcAuthentication:\n    vcverifier:\n      deployment:\n        serverCertificates:\n" +
				"          caCert:\n            enabled: true\n",
			warns: []string{"verifier" + withheldPrefix + ": the loaded file sets"}},
		{name: "route verify unset", loaded: apisixRoutes + "              client_id: c\n"},
		{name: "route verify pinned false", loaded: apisixRoutes + "              ssl_verify: false\n",
			errs: []string{"APISIX route 0 (/a) does not verify"}},
		{name: "route verify no, false to Helm's YAML 1.1", loaded: apisixRoutes + "              ssl_verify: no\n",
			errs: []string{"APISIX route 0 (/a) does not verify"}},
		{name: "route verify OFF", loaded: apisixRoutes + "              ssl_verify: OFF\n",
			errs: []string{"APISIX route 0 (/a) does not verify"}},
		{name: "route verify quoted no is a string", loaded: apisixRoutes + "              ssl_verify: \"no\"\n"},
		{name: "route verify true", loaded: apisixRoutes + "              ssl_verify: true\n"},
		{name: "APISIX disabled skips its routes", loaded: apisixRoutes + "              ssl_verify: false\n" +
			"      enabled: false\n"},
		{name: "odrl-authorization disabled skips its routes", loaded: apisixRoutes +
			"              ssl_verify: false\n    enabled: false\n"},
		{name: "verifier disabled is not withheld", loaded: verifierMounts + "      enabled: false\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]any{}
			if !tc.noCA {
				answers[caAnswer] = caSecret
			}
			findings, err := Evaluate(b, targetProfile(answers), loadedOptions(t, tc.loaded))
			if err != nil {
				t.Fatal(err)
			}
			got := only(findings, caRule)
			errs, warns := split(got)
			wantPrefixes(t, "errors", errs, tc.errs)
			wantPrefixes(t, "warnings", warns, tc.warns)
			if Blocking(got) != (len(tc.errs) > 0) {
				t.Errorf("Blocking = %v, want %v", Blocking(got), len(tc.errs) > 0)
			}
		})
	}
}

// TestNoProxyCoverage pins no-proxy-coverage: the derived default covers every service, a loaded rename warns until
// a given proxy.no covers it, and nothing is checked without a proxy.
func TestNoProxyCoverage(t *testing.T) {
	b := shipped(t)
	derived, err := render.Answers(b, targetProfile(map[string]any{httpsAnswer: proxyURL}))
	if err != nil {
		t.Fatal(err)
	}
	def, _ := derived[noProxyAnswer].(string)
	noCollector := strings.TrimSuffix(def, ",provider-opentelemetry-collector")
	if noCollector == def {
		t.Fatalf("default %q does not end with the collector", def)
	}
	tests := []struct {
		name      string
		noProxy   string
		noHTTPS   bool
		noRelease bool
		loaded    string
		want      []string
	}{
		{name: "derived default"},
		{name: "no proxy", noHTTPS: true, loaded: keycloakIdp},
		{name: "fullnameOverride", loaded: keycloakIdp,
			want: []string{"The loaded file renames an in-cluster service to idp (keycloak.fullnameOverride)"}},
		{name: "fullnameOverride covered", noProxy: def + ",idp", loaded: keycloakIdp},
		{name: "nameOverride", loaded: "keycloak:\n  nameOverride: sso\n",
			want: []string{"The loaded file renames an in-cluster service to provider-sso (keycloak.nameOverride)"}},
		{name: "fullnameOverride wins over nameOverride", loaded: keycloakIdp + "  nameOverride: sso\n",
			want: []string{"The loaded file renames an in-cluster service to idp (keycloak.fullnameOverride)"}},
		{name: "unrenamed service", noProxy: noCollector, loaded: enableTracing,
			want: []string{"No proxy does not cover the in-cluster service provider-opentelemetry-collector"}},
		{name: "unrenamed service not deployed", noProxy: noCollector},
		{name: "renamed service not deployed", loaded: keycloakIdp + "  enabled: false\n"},
		{name: "no release, fixed name renamed", noRelease: true, noProxy: def, loaded: tilRenamed,
			want: []string{"The loaded file renames an in-cluster service to til (decentralizedIam.vcAuthentication.trusted-issuers-list.fullnameOverride)"}},
		{name: "no release, release-prefixed name renamed", noRelease: true, noProxy: def, loaded: keycloakIdp,
			want: []string{"The loaded file renames an in-cluster service to idp (keycloak.fullnameOverride)"}},
		{name: "no release, release-prefixed name skipped", noRelease: true, noProxy: noCollector, loaded: enableTracing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]any{}
			if !tc.noHTTPS {
				answers[httpsAnswer] = proxyURL
			}
			if tc.noProxy != "" {
				answers[noProxyAnswer] = tc.noProxy
			}
			p := targetProfile(answers)
			if tc.noRelease {
				p.Target.ReleaseName = ""
			}
			findings, err := Evaluate(b, p, loadedOptions(t, tc.loaded))
			if err != nil {
				t.Fatal(err)
			}
			errs, warns := split(only(findings, noProxyRule))
			wantPrefixes(t, "errors", errs, nil)
			wantPrefixes(t, "warnings", warns, tc.want)
		})
	}
}

// TestKeycloakProxyMappings pins keycloak-proxy-mappings: a loaded non-empty spiProxyMappings warns while a proxy is set.
func TestKeycloakProxyMappings(t *testing.T) {
	b := shipped(t)
	tests := []struct {
		name   string
		https  bool
		loaded string
		want   bool
	}{
		{name: "mappings with a proxy", https: true, loaded: proxyMappings, want: true},
		{name: "mappings without a proxy", loaded: proxyMappings},
		{name: "empty mappings", https: true, loaded: "keycloak:\n  proxy:\n    spiProxyMappings: \"\"\n"},
		{name: "nothing loaded", https: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]any{}
			if tc.https {
				answers[httpsAnswer] = proxyURL
			}
			findings, err := Evaluate(b, targetProfile(answers), loadedOptions(t, tc.loaded))
			if err != nil {
				t.Fatal(err)
			}
			got := only(findings, mappingsRule)
			if (len(got) == 1) != tc.want || len(got) > 1 || Blocking(got) {
				t.Errorf("findings = %+v, want one warning: %v", got, tc.want)
			}
		})
	}
}
