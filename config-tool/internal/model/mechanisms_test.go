package model

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const elementsFixture = "testdata/foundations/elements.yaml"

func readMechanisms(t *testing.T, table string) (Bundle, error) {
	t.Helper()
	data, err := os.ReadFile(elementsFixture)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := Versions(fstest.MapFS{fixtureFile: {Data: append(data, "mechanisms:\n"+table...)}})
	if err != nil {
		return Bundle{}, err
	}
	return bs[0], nil
}

func TestMechanismsValidation(t *testing.T) {
	tests := []struct{ name, table, err string }{
		{"no id", "  - {conflicts: [{whenSet: [keycloak.tls.enabled]}]}\n", "id is missing"},
		{"unused id", "  - {id: other, conflicts: [{whenSet: [keycloak.tls.enabled]}]}\n", "no target uses the mechanism"},
		{"no conflicts", "  - {id: keycloak-ca}\n", "conflicts is empty"},
		{"empty whenSet", "  - {id: keycloak-ca, conflicts: [{whenSet: []}]}\n", "conflicts[0]: whenSet is empty"},
		{"unknown root", "  - {id: keycloak-ca, conflicts: [{whenSet: [scorpio.tls]}]}\n", "root is not in topLevelKeys"},
		{"a target's path", "  - {id: keycloak-ca, conflicts: [{whenSet: [keycloak.extraEnvVars]}]}\n", "ca.secret writes"},
		{"under a target", "  - {id: keycloak-ca, conflicts: [{whenSet: [keycloak.proxy.enabled.x]}]}\n", "proxy.https writes"},
		{"twice", "  - {id: keycloak-ca, conflicts: [{whenSet: [keycloak.a]}]}\n  - {id: keycloak-ca, conflicts: [{whenSet: [keycloak.b]}]}\n",
			"id is already declared"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readMechanisms(t, tc.table)
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.err)
			}
		})
	}
}

func TestMechanismConflict(t *testing.T) {
	b, err := readMechanisms(t, `  - id: keycloak-ca
    conflicts:
      - whenSet: [keycloak.tls.enabled]
      - whenSet: [keycloak.client.secret, keycloak.client.path]
`)
	if err != nil {
		t.Fatal(err)
	}
	m := b.Mechanisms()[0]
	tests := []struct {
		name, src string
		want      []string
	}{
		{"absent", "keycloak: {}\n", nil},
		{"the first set", "keycloak: {tls: {enabled: true}}\n", []string{"keycloak.tls.enabled"}},
		{"false is unset", "keycloak: {tls: {enabled: false}}\n", nil},
		{"both of the second", "keycloak: {client: {secret: s, path: /p}}\n", []string{"keycloak.client.secret", "keycloak.client.path"}},
		{"one of the second", "keycloak: {client: {secret: s, path: \"\"}}\n", nil},
		{"null is unset", "keycloak: {tls: {enabled: ~}, client: {secret: s}}\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := roundtrip.Parse([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := m.Conflict(doc, nil)
			if !reflect.DeepEqual(got, tc.want) || ok != (tc.want != nil) {
				t.Errorf("Conflict = %v, %v; want %v", got, ok, tc.want)
			}
		})
	}
}
