package model

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// requirementsFoundation is the one foundation a requirements row may reference in these tests.
const requirementsFoundation = `foundations:
  - id: ca.privateSecret
    stage: foundations
    type: secretName
    label: Private CA Secret
    help: The Secret holding the CA.
    targets:
      - {path: keycloak.ca, fn: copy, args: ["{{ .ca.privateSecret }}"]}
`

func readRequirements(row string) (Bundle, error) {
	body := foundationsHead + requirementsFoundation + "requirements:\n  - " + row + "\n"
	bs, err := Versions(fstest.MapFS{fixtureFile: {Data: []byte(body)}})
	if err != nil {
		return Bundle{}, err
	}
	return bs[0], nil
}

func TestRequirementsValidation(t *testing.T) {
	tests := []struct{ name, row, err string }{
		{"unknown kind", `{kind: configMap, name: "{{ .ca.privateSecret }}", key: ca.crt, purpose: p}`, `kind "configMap" is unknown`},
		{"empty key", `{kind: secret, name: "{{ .ca.privateSecret }}", purpose: p}`, "key must be a non-empty literal"},
		{"templated key", `{kind: secret, name: "{{ .ca.privateSecret }}", key: "{{ .ca.privateSecret }}", purpose: p}`,
			"key must be a non-empty literal"},
		{"no purpose", `{kind: secret, name: "{{ .ca.privateSecret }}", key: ca.crt}`, "purpose is missing"},
		{"literal name", `{kind: secret, name: private-ca, key: ca.crt, purpose: p}`, "references no answer"},
		{"reserved ref", `{kind: secret, name: "{{ .target.namespace }}-ca", key: ca.crt, purpose: p}`, "reserved"},
		{"key and keys", `{kind: secret, name: "{{ .ca.privateSecret }}", key: ca.crt, keys: [tls.crt], purpose: p}`, "key and keys are exclusive"},
		{"empty keys entry", `{kind: secret, name: "{{ .ca.privateSecret }}", keys: [tls.crt, ""], purpose: p}`, "key must be a non-empty literal"},
		{"repeated key", `{kind: secret, name: "{{ .ca.privateSecret }}", keys: [tls.crt, tls.crt], purpose: p}`, "keys lists a key twice"},
		{"when without equals", `{kind: secret, name: "{{ .ca.privateSecret }}", key: ca.crt, purpose: p, when: [{id: ca.privateSecret}]}`,
			"when: ca.privateSecret: equals is missing"},
		{"when on no foundation", `{kind: secret, name: "{{ .ca.privateSecret }}", key: ca.crt, purpose: p, when: [{id: proxy.https, equals: x}]}`,
			"when: proxy.https is no foundation"},
		{"unknown answer", `{kind: secret, name: "{{ .proxy.https }}", key: ca.crt, purpose: p}`, "proxy.https, which is no foundation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readRequirements(tc.row)
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.err)
			}
		})
	}
}

func TestRequirementResolve(t *testing.T) {
	b, err := readRequirements(`{kind: secret, name: "{{ .ca.privateSecret }}-ca", key: ca.crt, purpose: " The CA. "}`)
	if err != nil {
		t.Fatal(err)
	}
	rs := b.Requirements()
	if len(rs) != 1 || rs[0].Purpose != "The CA." || !slices.Equal(rs[0].Keys, []string{"ca.crt"}) {
		t.Fatalf("requirements = %+v, want one trimmed row", rs)
	}
	tests := []struct {
		name    string
		answers map[string]any
		want    string
		ok      bool
	}{
		{"answered", map[string]any{"ca.privateSecret": "corp"}, "corp-ca", true},
		{"unanswered", map[string]any{}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := rs[0].Resolve(tc.answers)
			if err != nil || got != tc.want || ok != tc.ok {
				t.Errorf("Resolve = %q, %v, %v; want %q, %v", got, ok, err, tc.want, tc.ok)
			}
		})
	}
}
