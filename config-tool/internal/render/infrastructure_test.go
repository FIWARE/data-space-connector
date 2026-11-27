package render

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	// infraFixture is the model package's bundle with cert-manager's three modes over a const-owned (apisix) and a
	// ref-owned (keycloak) ingress.
	infraFixture = "../model/testdata/infrastructure/valid.yaml"
	certManager  = "certManager"
	infraIssuer  = "ca-issuer"
	infraEmail   = "ops@fiware.org"
	infraNS      = "provider"
	hostKeycloak = "host.keycloak"

	cmEnabled       = "cert-manager.enabled"
	cmrEnabled      = "certManagerResources.enabled"
	cmrType         = "certManagerResources.type"
	cmrEmail        = "certManagerResources.acmeEmail"
	cmrNamespace    = "certManagerResources.namespace"
	apisixIssuer    = `apisix.ingress.annotations."cert-manager.io/cluster-issuer"`
	keycloakIssuer  = `keycloak.ingress.annotations."cert-manager.io/cluster-issuer"`
	apisixSecret    = "apisix.ingress.tls[0].secretName"
	keycloakSecret  = "keycloak.ingress.tls[0].secretName"
	keycloakTLSHost = "keycloak.ingress.tls[0].hosts[0]"
	keycloakHostKey = "keycloak.ingress.hostname"
)

func infraBundle(t *testing.T) model.Bundle {
	t.Helper()
	return modelFixture(t, infraFixture)
}

// infraProfile answers domain.base and chooses mode with the given local answers.
func infraProfile(mode string, answers map[string]any) profile.Profile {
	p := profile.Profile{Target: profile.Target{Namespace: infraNS}, Answers: map[string]any{domainBase: domain}}
	if mode != "" || answers != nil {
		p.Infrastructure = map[string]profile.InfraChoice{certManager: {Mode: mode, Answers: answers}}
	}
	return p
}

// sharedInfraLeaves are the leaves every mode renders alike, from the declared foundations.
var sharedInfraLeaves = map[string]any{
	keycloakHostKey: "keycloak." + domain,
	keycloakTLSHost: "keycloak." + domain,
}

func TestRenderInfrastructureModes(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		answers map[string]any
		want    map[string]any
	}{
		{"no mode chosen", "", nil, map[string]any{apisixSecret: "apisix-provider-tls", keycloakSecret: "keycloak." + domain + "-tls"}},
		{"bundled, self-signed by default", model.ModeBundled, nil, map[string]any{
			cmEnabled: true, cmrEnabled: true, cmrType: "selfsigned", cmrNamespace: infraNS,
			apisixIssuer: "selfsigned-issuer", keycloakIssuer: "selfsigned-issuer",
			apisixSecret: "apisix-provider-tls", keycloakSecret: "keycloak." + domain + "-tls",
		}},
		{"bundled, prod: the when-gated questions switch", model.ModeBundled, map[string]any{"type": "prod", "acmeEmail": infraEmail, "namespace": "ignored"}, map[string]any{
			cmEnabled: true, cmrEnabled: true, cmrType: "prod", cmrEmail: infraEmail,
			apisixIssuer: "prod-issuer", keycloakIssuer: "prod-issuer",
			apisixSecret: "apisix-provider-tls", keycloakSecret: "keycloak." + domain + "-tls",
		}},
		{"cluster", model.ModeCluster, map[string]any{"issuer": infraIssuer}, map[string]any{
			cmEnabled: false, cmrEnabled: false, apisixIssuer: infraIssuer, keycloakIssuer: infraIssuer,
			apisixSecret: "apisix-provider-tls", keycloakSecret: "keycloak." + domain + "-tls",
		}},
		{"cluster keeps the inactive modes' answers out", model.ModeCluster, map[string]any{"issuer": infraIssuer, "type": "prod", "acmeEmail": infraEmail, "secret.keycloak": "kc-tls"}, map[string]any{
			cmEnabled: false, cmrEnabled: false, apisixIssuer: infraIssuer, keycloakIssuer: infraIssuer,
			apisixSecret: "apisix-provider-tls", keycloakSecret: "keycloak." + domain + "-tls",
		}},
		{"external, default Secret names equal the foundation's", model.ModeExternal, nil, map[string]any{
			cmEnabled: false, cmrEnabled: false, apisixSecret: "apisix-provider-tls", keycloakSecret: "keycloak." + domain + "-tls",
		}},
		{"external, answered Secret names replace the foundation's", model.ModeExternal, map[string]any{"secret.apisix": "gw-tls", "secret.keycloak": "kc-tls"}, map[string]any{
			cmEnabled: false, cmrEnabled: false, apisixSecret: "gw-tls", keycloakSecret: "kc-tls",
		}},
	}
	b := infraBundle(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := Render(b, infraProfile(tc.mode, tc.answers), Options{})
			if err != nil {
				t.Fatal(err)
			}
			want := maps.Clone(sharedInfraLeaves)
			maps.Copy(want, tc.want)
			got := values(tree)
			if !maps.Equal(got, want) {
				t.Errorf("leaves = %v, want %v", got, want)
			}
			if len(tree.Leaves) != len(got) {
				t.Errorf("%d leaves for %d paths; a replaced leaf was kept", len(tree.Leaves), len(got))
			}
		})
	}
}

func TestRenderInfrastructureProvenance(t *testing.T) {
	b := infraBundle(t)
	tree, err := Render(b, infraProfile(model.ModeExternal, map[string]any{"secret.keycloak": "kc-tls"}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := leaf(t, tree, keycloakSecret).Provenance.Foundation, model.InfraQuestionID(certManager, "secret.keycloak"); got != want {
		t.Errorf("%s provenance = %s, want %s", keycloakSecret, got, want)
	}
	if got, want := leaf(t, tree, cmEnabled).Provenance.Foundation, model.InfraModeID(certManager); got != want {
		t.Errorf("%s provenance = %s, want %s", cmEnabled, got, want)
	}
}

// TestRenderInfrastructureModeSwitch pins that switching away from external brings the foundation's Secret name back.
func TestRenderInfrastructureModeSwitch(t *testing.T) {
	b := infraBundle(t)
	answers := map[string]any{"secret.keycloak": "kc-tls", "issuer": infraIssuer}
	for _, step := range []struct {
		mode, want, from string
	}{
		{model.ModeExternal, "kc-tls", model.InfraQuestionID(certManager, "secret.keycloak")},
		{model.ModeCluster, "keycloak." + domain + "-tls", hostKeycloak},
		{model.ModeExternal, "kc-tls", model.InfraQuestionID(certManager, "secret.keycloak")},
	} {
		tree, err := Render(b, infraProfile(step.mode, answers), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if l := leaf(t, tree, keycloakSecret); l.Value != step.want || l.Provenance.Foundation != step.from {
			t.Errorf("%s: %s = %v from %s, want %s from %s", step.mode, keycloakSecret, l.Value, l.Provenance.Foundation, step.want, step.from)
		}
	}
}

// TestRenderInactiveModeKeepsLists pins that an inactive mode's unresolved target never drops a list the active mode
// renders: external's per-ingress targets are left unresolved, and every tls[0] block still renders.
func TestRenderInactiveModeKeepsLists(t *testing.T) {
	b := infraBundle(t)
	// host.keycloak given without domain.base: the apisix row's owner is unanswered, so external's apisix Secret
	// question does not resolve while cluster is chosen.
	p := profile.Profile{
		Answers:        map[string]any{hostKeycloak: "keycloak." + domain},
		Infrastructure: map[string]profile.InfraChoice{certManager: {Mode: model.ModeCluster, Answers: map[string]any{"issuer": infraIssuer}}},
	}
	tree, err := Render(b, p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := values(tree)
	for _, path := range []string{keycloakTLSHost, keycloakSecret, keycloakIssuer} {
		if _, ok := got[path]; !ok {
			t.Errorf("%s is missing; leaves = %v", path, got)
		}
	}
}

// inactiveListFixture has an inactive mode whose question, without a default, writes into the list host.keycloak owns.
const inactiveListFixture = `chart: data-space-connector
version: 1.0.0
revision: 1
topLevelKeys: [keycloak, postgres]
foundations:
  - id: host.keycloak
    stage: foundations
    type: hostname
    label: Keycloak host
    targets:
      - {path: 'keycloak.ingress.tls[0].hosts[0]', fn: copy, args: ["{{ .host.keycloak }}"]}
infrastructure:
  - id: db
    title: Database
    modes:
      - id: bundled
        label: Install a database
        targets: [{path: postgres.enabled, fn: const, args: [true]}]
      - id: external
        label: Use an external database
        questions:
          - id: alias
            type: hostname
            label: Alias
            targets: [{path: 'keycloak.ingress.tls[0].hosts[1]', fn: copy, args: ["{{ .infra.db.alias }}"]}]
`

// TestRenderInactiveModeKeepsOwnedList pins that an unresolved target of an unchosen mode leaves a list another
// foundation writes whole.
func TestRenderInactiveModeKeepsOwnedList(t *testing.T) {
	bs, err := model.Versions(fstest.MapFS{"1.0.0/bundle.yaml": {Data: []byte(inactiveListFixture)}})
	if err != nil {
		t.Fatal(err)
	}
	host := "keycloak." + domain
	p := profile.Profile{
		Answers:        map[string]any{hostKeycloak: host},
		Infrastructure: map[string]profile.InfraChoice{"db": {Mode: model.ModeBundled}},
	}
	tree, err := Render(bs[0], p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := values(tree)[keycloakTLSHost]; got != host {
		t.Errorf("%s = %v, want %s", keycloakTLSHost, got, host)
	}
}

// TestPerIngressTargetsNeedTheOwner pins, per ingresses row, that an unanswered owner gates every infra leaf on its
// ingress, whether the row's owner target is a const (apisix) or a template (keycloak).
func TestPerIngressTargetsNeedTheOwner(t *testing.T) {
	b := infraBundle(t)
	rows := []struct {
		ingress, path, owner string
		answers              map[string]any
	}{
		{"apisix", "apisix.ingress", domainBase, map[string]any{hostKeycloak: "keycloak." + domain}},
		{"keycloak", "keycloak.ingress", hostKeycloak, map[string]any{}},
	}
	modes := []struct {
		mode    string
		answers map[string]any
	}{
		{model.ModeCluster, map[string]any{"issuer": infraIssuer}},
		{model.ModeExternal, nil},
	}
	for _, r := range rows {
		for _, m := range modes {
			t.Run(r.ingress+"/"+m.mode, func(t *testing.T) {
				p := profile.Profile{Answers: r.answers, Infrastructure: map[string]profile.InfraChoice{certManager: {Mode: m.mode, Answers: m.answers}}}
				tree, err := Render(b, p, Options{})
				if err != nil {
					t.Fatal(err)
				}
				for _, l := range tree.Leaves {
					if strings.HasPrefix(l.Path.String(), r.path+".") && strings.HasPrefix(l.Provenance.Foundation, model.InfraIDPrefix) {
						t.Errorf("%s = %v from %s, want nothing while %s is unanswered", l.Path, l.Value, l.Provenance.Foundation, r.owner)
					}
				}
				for _, f := range b.CompiledFoundations() {
					for _, ct := range f.Targets {
						if strings.HasPrefix(f.ID, model.InfraIDPrefix) && strings.HasPrefix(ct.Path.String(), r.path+".") && !slices.Contains(ct.Requires, r.owner) {
							t.Errorf("%s target %s requires %v, want %s among them", f.ID, ct.Path, ct.Requires, r.owner)
						}
					}
				}
			})
		}
	}
}

func TestModeTargets(t *testing.T) {
	b := infraBundle(t)
	tests := []struct {
		mode string
		want []string
	}{
		{model.ModeBundled, []string{apisixIssuer, cmEnabled, cmrEmail, cmrEnabled, cmrNamespace, cmrType, keycloakIssuer}},
		{model.ModeCluster, []string{apisixIssuer, cmEnabled, cmrEnabled, keycloakIssuer}},
		{model.ModeExternal, []string{apisixSecret, cmEnabled, cmrEnabled, keycloakSecret}},
		{"unknown", []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			got := []string{}
			for _, r := range ModeTargets(b, certManager, tc.mode) {
				got = append(got, r.Path.String())
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("ModeTargets(%s) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
	if got := ModeTargets(b, "nope", model.ModeBundled); got != nil {
		t.Errorf("ModeTargets of an unknown dependency = %v, want nil", got)
	}
	if got := Targets(b, model.InfraModeID(certManager)); len(got) == 0 {
		t.Errorf("Targets(%s) is empty, want the mode-gated keys", model.InfraModeID(certManager))
	}
}

// TestRenderInfrastructureReload pins that owned infra leaves of the old mode leave the re-render, while a hand-set
// annotation at an owned path stays pinned.
func TestRenderInfrastructureReload(t *testing.T) {
	b := infraBundle(t)
	src := header +
		"keycloak:\n  ingress:\n    annotations:\n      # dsc-config:owned infra.certManager.issuer\n      cert-manager.io/cluster-issuer: " + infraIssuer + "\n" +
		"apisix:\n  ingress:\n    annotations:\n      cert-manager.io/cluster-issuer: hand-issuer\n"
	r, err := load.Load([]byte(src), "values.yaml", []model.Bundle{b}, load.Options{Root: &roundtrip.Path{}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		mode    string
		answers map[string]any
		pinned  bool
	}{
		{model.ModeExternal, nil, false},
		{model.ModeCluster, map[string]any{"issuer": "other-issuer"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			tree, err := Render(b, infraProfile(tc.mode, tc.answers), Options{Overrides: r})
			if err != nil {
				t.Fatal(err)
			}
			got := values(tree)
			if _, ok := got[keycloakIssuer]; ok != tc.pinned {
				t.Errorf("%s in the re-render = %v, want %v", keycloakIssuer, ok, tc.pinned)
			}
			if !tc.pinned {
				return
			}
			if l := leaf(t, tree, keycloakIssuer); l.Provenance.Pinned || l.Value != "other-issuer" {
				t.Errorf("owned %s = %v (pinned %v), want the re-rendered answer", keycloakIssuer, l.Value, l.Provenance.Pinned)
			}
			if l := leaf(t, tree, apisixIssuer); !l.Provenance.Pinned || l.Value != "hand-issuer" {
				t.Errorf("hand-set %s = %v (pinned %v), want it kept", apisixIssuer, l.Value, l.Provenance.Pinned)
			}
		})
	}
}

// TestAnswerListsSkipInfrastructure pins that Stage 3's and Review's answer lists never name a synthetic infra.* id,
// whether its mode is chosen or not: those answers belong to Stage 4.
func TestAnswerListsSkipInfrastructure(t *testing.T) {
	const (
		otherBase   = "other.org"
		staleSecret = "kc"
	)
	b := infraBundle(t)
	stale := map[string]any{"secret.keycloak": staleSecret, "issuer": infraIssuer}
	tests := []struct {
		name string
		mode string
	}{
		{"no mode chosen", ""},
		{"inactive external answers", model.ModeCluster},
		{"active external answers", model.ModeExternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answers := stale
			if tc.mode == "" {
				answers = nil
			}
			prev := infraProfile(tc.mode, answers)
			prev.Model = profile.ModelRef{Chart: b.Chart, Version: b.Version}
			next := prev
			next.Answers = map[string]any{domainBase: otherBase}
			hand, err := HandSet(b, next)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hand {
				if model.IsInfraID(h.ID) {
					t.Errorf("HandSet lists %s", h.ID)
				}
			}
			diff, err := DerivedDiff(b, prev, next)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range diff.Rederived {
				if model.IsInfraID(c.ID) {
					t.Errorf("Rederived lists %s", c.ID)
				}
			}
			for _, p := range diff.Pinned {
				if model.IsInfraID(p.ID) {
					t.Errorf("Pinned lists %s", p.ID)
				}
			}
			if !slices.ContainsFunc(diff.Rederived, func(c DerivedChange) bool { return c.ID == hostKeycloak }) {
				t.Errorf("Rederived = %+v, want %s", diff.Rederived, hostKeycloak)
			}
		})
	}
}
