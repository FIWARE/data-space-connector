package check

import (
	"slices"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	acmeEmailRule  = "acme-email-for-prod-issuer"
	acmeClassRule  = "acme-ingress-class-for-prod-issuer"
	acmeEmailField = "infrastructure.certManager.acmeEmail"
	acmeClassField = "infrastructure.certManager.ingressClass"
	acmeEmailKey   = "certManagerResources.acmeEmail"
	acmeClassKey   = "certManagerResources.ingressClass"
	// chartACMEEmail is the chart's certManagerResources.acmeEmail default.
	chartACMEEmail   = "admins@example.com"
	typeProd         = "prod"
	typeSelfsigned   = "selfsigned"
	ingressTraefik   = "traefik"
	ingressNginx     = "nginx"
	acmeProfileFile  = "testdata/verification-on.dscprofile.yaml"
	acmeTestDomain   = "fiware.org"
	acmeClusterIssue = "ca-issuer"
)

// acmeProfile chooses mode with the given answers and marks open as Ask later.
func acmeProfile(mode string, answers map[string]any, open ...string) profile.Profile {
	p := profile.Profile{Answers: map[string]any{"domain.base": acmeTestDomain}}
	if mode != "" {
		p.Infrastructure = map[string]profile.InfraChoice{certManager: {Mode: mode, Answers: answers}}
	}
	for _, f := range open {
		p.Open = append(p.Open, profile.OpenQuestion{Field: f})
	}
	return p
}

// TestACMEContact pins that a bundled Let's Encrypt issuer blocks until its contact and ingress class are given, and
// that no render falls back to the chart's placeholders.
func TestACMEContact(t *testing.T) {
	b := shipped(t)
	prod := func(extra map[string]any) map[string]any {
		out := map[string]any{"type": typeProd}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	complete := map[string]any{"acmeEmail": acmeEmail, "ingressClass": ingressTraefik}
	tests := []struct {
		name    string
		p       profile.Profile
		want    []string
		blocked bool
	}{
		{"prod, both unanswered", acmeProfile(model.ModeBundled, prod(nil)), []string{acmeEmailRule, acmeClassRule}, true},
		{"prod, both Ask later", acmeProfile(model.ModeBundled, prod(nil), acmeEmailField, acmeClassField),
			[]string{acmeEmailRule, acmeClassRule}, true},
		{"prod, email only", acmeProfile(model.ModeBundled, prod(map[string]any{"acmeEmail": acmeEmail})), []string{acmeClassRule}, true},
		{"prod, class only", acmeProfile(model.ModeBundled, prod(map[string]any{"ingressClass": ingressNginx})), []string{acmeEmailRule}, true},
		{"prod, traefik", acmeProfile(model.ModeBundled, prod(complete)), nil, false},
		{"prod, nginx", acmeProfile(model.ModeBundled, prod(map[string]any{"acmeEmail": acmeEmail, "ingressClass": ingressNginx})), nil, false},
		{"selfsigned", acmeProfile(model.ModeBundled, map[string]any{"type": typeSelfsigned}), nil, false},
		{"type unanswered", acmeProfile(model.ModeBundled, nil), nil, false},
		{"cluster", acmeProfile(model.ModeCluster, map[string]any{"issuer": acmeClusterIssue, "type": typeProd}), nil, false},
		{"external", acmeProfile(model.ModeExternal, map[string]any{"type": typeProd}), nil, false},
		{"no mode", acmeProfile("", nil), nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings, err := Evaluate(b, tc.p, render.Options{})
			if err != nil {
				t.Fatal(err)
			}
			got := slices.Concat(only(findings, acmeEmailRule), only(findings, acmeClassRule))
			if ids := ids(got); !slices.Equal(ids, tc.want) {
				t.Errorf("findings = %v, want %v", ids, tc.want)
			}
			if Blocking(got) != tc.blocked {
				t.Errorf("Blocking = %v, want %v", Blocking(got), tc.blocked)
			}
			tree, err := render.Render(b, tc.p, render.Options{})
			if err != nil {
				t.Fatal(err)
			}
			answers := tc.p.Infrastructure[certManager].Answers
			for key, question := range map[string]string{acmeEmailKey: "acmeEmail", acmeClassKey: "ingressClass"} {
				i := slices.IndexFunc(tree.Leaves, func(l render.Leaf) bool { return l.Path.String() == key })
				if given, ok := answers[question]; i >= 0 && (!ok || tree.Leaves[i].Value != given) {
					t.Errorf("%s = %v, but %s is %v", key, tree.Leaves[i].Value, question, given)
				}
			}
		})
	}
}

// TestACMEEmailRefusesPlaceholders pins that the chart's placeholder can never be the typed contact.
func TestACMEEmailRefusesPlaceholders(t *testing.T) {
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		email   string
		refused bool
	}{
		{chartACMEEmail, true},
		{"ops@example.net", true},
		{"ops@ci.test", true},
		{acmeEmail, false},
	}
	for _, tc := range tests {
		t.Run(tc.email, func(t *testing.T) {
			p := readProfile(t, acmeProfileFile)
			p.Infrastructure = map[string]profile.InfraChoice{certManager: {Mode: model.ModeBundled,
				Answers: map[string]any{"type": typeProd, "acmeEmail": tc.email, "ingressClass": ingressTraefik}}}
			errs := p.Validate(bs)
			refused := slices.ContainsFunc(errs, func(e profile.FieldError) bool { return e.Field == acmeEmailField })
			if refused != tc.refused {
				t.Errorf("refused = %v, want %v: %v", refused, tc.refused, errs)
			}
			if !tc.refused && len(errs) > 0 {
				t.Errorf("valid profile refused: %v", errs)
			}
		})
	}
}
