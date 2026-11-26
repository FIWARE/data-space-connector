package model

import (
	"strings"
	"testing"
	"testing/fstest"

	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

func TestNoProxyServicesValidation(t *testing.T) {
	for _, tc := range []struct {
		name, row, err string
	}{
		{"valid", `{name: '{{ fullname .target.release "keycloak" }}', overrides: [keycloak.fullnameOverride], nameOverride: keycloak.nameOverride}`, ""},
		{"nameOverride without fullname", `{name: idp, nameOverride: keycloak.nameOverride}`, "no fullname call"},
		{"nameOverride outside topLevelKeys", `{name: '{{ fullname .target.release "keycloak" }}', nameOverride: other.nameOverride}`, "root is not in topLevelKeys"},
		{"override outside topLevelKeys", `{name: idp, overrides: [other.fullnameOverride]}`, "root is not in topLevelKeys"},
		{"name reads an answer", `{name: '{{ .domain.base }}'}`, "only target.release and target.namespace"},
		{"enabled key outside topLevelKeys", `{name: idp, enabledKeys: {other.enabled: true}}`, "enabledKeys"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := elementsHead + "noProxyServices:\n  - " + tc.row + "\n"
			_, err := Versions(fstest.MapFS{fixtureFile: {Data: []byte(body)}})
			if tc.err == "" && err != nil {
				t.Errorf("err = %v, want none", err)
			}
			if tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
				t.Errorf("err = %v, want it to contain %q", err, tc.err)
			}
		})
	}
}

// TestNoProxyServiceDefaults keeps the enabledKeys defaults of the shipped noProxyServices rows equal to the chart's.
func TestNoProxyServiceDefaults(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	values := chartValuesRoot(t)
	rows := 0
	for _, b := range got {
		if b.Chart != chart || !b.Latest {
			continue
		}
		for _, s := range b.NoProxyServicesTable {
			checkEnabledDefaults(t, values, s.Name, s.EnabledKeys)
			rows++
		}
	}
	if rows == 0 {
		t.Fatalf("the latest %s bundle has no noProxyServices rows", chart)
	}
}

// TestCACoverageDefaults keeps the enabledKeys defaults of the shipped caCoverage components equal to the chart's.
func TestCACoverageDefaults(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	values := chartValuesRoot(t)
	components := 0
	for _, b := range got {
		if b.Chart != chart || !b.Latest {
			continue
		}
		for _, r := range b.CheckRules() {
			if r.CACoverage == nil {
				continue
			}
			for _, c := range r.CACoverage.Components {
				checkEnabledDefaults(t, values, c.ID, c.EnabledKeys)
				components++
			}
		}
	}
	if components == 0 {
		t.Fatalf("the latest %s bundle has no caCoverage components", chart)
	}
}

// TestNoProxyServiceReadsRelease pins which names need the release name, so the coverage check skips only those.
func TestNoProxyServiceReadsRelease(t *testing.T) {
	for _, tc := range []struct {
		name, row string
		want      bool
	}{
		{"fixed", `{name: trusted-issuers-list}`, false},
		{"namespace only", `{name: 'svc.{{ .target.namespace }}'}`, false},
		{"release prefixed", `{name: '{{ fullname .target.release "keycloak" }}'}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := elementsHead + "noProxyServices:\n  - " + tc.row + "\n"
			got, err := Versions(fstest.MapFS{fixtureFile: {Data: []byte(body)}})
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range got {
				if s := b.NoProxyServices(); len(s) != 1 || s[0].ReadsRelease != tc.want {
					t.Errorf("services = %+v, want one with ReadsRelease %v", s, tc.want)
				}
			}
		})
	}
}
