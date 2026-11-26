package model

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

// shippedBundleFile is the bundle whose coverage rules the cases edit.
const shippedBundleFile = "10.16.0/bundle.yaml"

// TestCoverageRulesValidation refuses caCoverage, noProxyCoverage and valueSet rules the shipped bundle would
// otherwise carry, each case replacing one fragment of it.
func TestCoverageRulesValidation(t *testing.T) {
	src, err := fs.ReadFile(bundles.FS, shippedBundleFile)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, old, new, err string
	}{
		{"set on a feature", "- {field: answers.ca.privateSecret, set: true}",
			"- {field: features.access.verification, set: true}", "unset and set apply to answers.<foundation id> only"},
		{"set with equals", "- {field: answers.ca.privateSecret, set: true}",
			"- {field: answers.ca.privateSecret, set: true, equals: x}", "has more than one of equals, unset and set"},
		{"caCoverage without when", "    when:\n      - {field: answers.ca.privateSecret, set: true}\n", "",
			"caCoverage needs when"},
		{"unknown mechanism", "mechanism: keycloak-ca, enabledKeys", "mechanism: nope, enabledKeys",
			`mechanism "nope" is no target's mechanism`},
		{"mechanism and paths", "mechanism: keycloak-ca, enabledKeys", "mechanism: keycloak-ca, paths: [keycloak.enabled], enabledKeys",
			"has exactly one of mechanism and paths"},
		{"path no target", "decentralizedIam.odrlAuthorization.apisix.apisix.ssl.enabled],",
			"decentralizedIam.odrlAuthorization.apisix.apisix.ssl.foo],", "is no scalar target's path"},
		{"enabledKey outside the chart", "enabledKeys: {keycloak.enabled: true}}", "enabledKeys: {nope.enabled: true}}",
			"enabledKeys"},
		{"duplicate component", "{id: marketplace, label: marketplace", "{id: keycloak, label: marketplace",
			`id "keycloak" is already used`},
		{"routes without each target", "verify: plugins.openid-connect.ssl_verify,", "verify: plugins.ssl_verify,",
			"is no each target"},
		{"routes of an unknown component", "uri: uri, component: apisix}", "uri: uri, component: nope}",
			`component "nope" names no entry`},
		{"withheld key unknown", "{{ .component }} does not get the private CA: the loaded file sets {{ .paths }}",
			"{{ .component }} does not get the private CA: the loaded file sets {{ .list }}", "{{ .list }} is not one of"},
		{"none missing", "      none: >-\n", "      nothing: >-\n", "field nothing not found"},
		{"noProxyCoverage answer unknown", "answer: proxy.no", "answer: proxy.none", `answer "proxy.none" names no foundation`},
		{"noProxyCoverage key unknown", "Add {{ .service }} to No proxy.", "Add {{ .components }} to No proxy.",
			"{{ .components }} is not one of"},
		{"valueSet outside the chart", "valueSet: {path: keycloak.proxy.spiProxyMappings}", "valueSet: {path: nope.x}",
			"valueSet: path"},
		{"valueSet with distinct", "valueSet: {path: keycloak.proxy.spiProxyMappings}",
			"valueSet: {path: keycloak.proxy.spiProxyMappings}\n    distinct: {prefix: host.}", "valueSet combines with when only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if n := strings.Count(string(src), tc.old); n != 1 {
				t.Fatalf("fragment %q occurs %d times, want once", tc.old, n)
			}
			data := strings.Replace(string(src), tc.old, tc.new, 1)
			_, err := Versions(fstest.MapFS{shippedBundleFile: {Data: []byte(data)}})
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("err = %v, want it to contain %q", err, tc.err)
			}
		})
	}
}
