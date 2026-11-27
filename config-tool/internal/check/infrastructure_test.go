package check

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// infraBundle is a two-mode dependency with a defaulted and a plain question, and one rule per kind of when below.
const infraBundle = `chart: c
version: 1.0.0
revision: 1
topLevelKeys: [certManagerResources]
infrastructure:
  - id: certManager
    title: cert-manager
    modes:
      - id: bundled
        label: Bundled
        questions:
          - id: type
            type: choice
            label: Issuer
            default: selfsigned
            options: [{value: selfsigned, label: Self-signed}, {value: prod, label: Prod}]
            targets: [{path: certManagerResources.type, fn: copy, args: ["{{ .infra.certManager.type }}"]}]
          - {id: acmeEmail, type: email, label: ACME contact}
      - id: cluster
        label: Cluster
        questions: [{id: issuer, type: hostname, label: ClusterIssuer}]
rules:
  - {id: mode-set, severity: warning, stage: infrastructure, field: infrastructure.certManager, when: [{field: infrastructure.certManager, set: true}], message: M, doc: doc/a.md}
  - {id: mode-unset, severity: warning, stage: infrastructure, field: infrastructure.certManager, when: [{field: infrastructure.certManager, unset: true}], message: M, doc: doc/a.md}
  - {id: mode-cluster, severity: warning, stage: infrastructure, field: infrastructure.certManager, when: [{field: infrastructure.certManager, equals: cluster}], message: M, doc: doc/a.md}
  - {id: type-prod, severity: warning, stage: infrastructure, field: infrastructure.certManager.type, when: [{field: infrastructure.certManager.type, equals: prod}], message: M, doc: doc/a.md}
  - {id: email-set, severity: warning, stage: infrastructure, field: infrastructure.certManager.acmeEmail, when: [{field: infrastructure.certManager.acmeEmail, set: true}], message: M, doc: doc/a.md}
  - {id: email-unset, severity: error, stage: infrastructure, field: infrastructure.certManager.acmeEmail, when: [{field: infrastructure.certManager, equals: bundled}, {field: infrastructure.certManager.acmeEmail, unset: true}], message: M, doc: doc/a.md}
`

const (
	certManager = "certManager"
	acmeEmail   = "ops@fiware.org"
)

func TestInfrastructureFields(t *testing.T) {
	all, err := model.Versions(fstest.MapFS{"1.0.0/bundle.yaml": {Data: []byte(infraBundle)}})
	if err != nil {
		t.Fatal(err)
	}
	b := all[0]
	tests := []struct {
		name   string
		choice *profile.InfraChoice
		want   []string
	}{
		{"no mode", nil, []string{"mode-unset"}},
		// type defaults to selfsigned, so type-prod stays quiet.
		{"bundled, defaults", &profile.InfraChoice{Mode: model.ModeBundled}, []string{"email-unset", "mode-set"}},
		{"bundled, prod with email", &profile.InfraChoice{Mode: model.ModeBundled, Answers: map[string]any{"type": "prod", "acmeEmail": acmeEmail}},
			[]string{"email-set", "mode-set", "type-prod"}},
		{"cluster", &profile.InfraChoice{Mode: model.ModeCluster, Answers: map[string]any{"issuer": "ca-issuer"}},
			[]string{"mode-cluster", "mode-set"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := profile.Profile{}
			if tc.choice != nil {
				p.Infrastructure = map[string]profile.InfraChoice{certManager: *tc.choice}
			}
			got, err := Evaluate(b, p, render.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if ids := slices.Sorted(slices.Values(ids(got))); !slices.Equal(ids, tc.want) {
				t.Errorf("findings = %v, want %v", ids, tc.want)
			}
			for _, f := range got {
				if !strings.HasPrefix(f.Field, model.InfrastructureFieldPrefix) {
					t.Errorf("finding %s field = %q, want an %s field", f.Rule, f.Field, model.InfrastructureFieldPrefix)
				}
			}
		})
	}
}
