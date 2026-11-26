package model

import (
	"strings"
	"testing"
	"testing/fstest"
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
