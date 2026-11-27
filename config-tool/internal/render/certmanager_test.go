package render

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	issuerTemplate  = "../../../charts/data-space-connector/templates/cert-manager-issuer.yaml"
	operatorsValues = "../../../k3s/operators.yaml"
	operatorsNS     = "operators"
	issuerKey       = `annotations."cert-manager.io/cluster-issuer"`
	secretNameKey   = "tls[0].secretName"
	cmCRDs          = "cert-manager.crds.enabled"
	cmrIngressClass = "certManagerResources.ingressClass"
	infraClass      = "traefik"
	typeSelfsigned  = "selfsigned"
	typeProd        = "prod"
	// externalSecretSuffix names the answered Secret of every row in external mode.
	externalSecretSuffix = "-cert"
	// infraBlockStart and foundationsStart bound the shipped bundle's ingresses and infrastructure sections,
	// acmeRulesStart and resourcesStart its rules on them.
	infraBlockStart  = "\n# The verifier and DID document ingresses"
	foundationsStart = "\nfoundations:"
	acmeRulesStart   = "\n  # An unanswered question writes no key"
	resourcesStart   = "\nresources:"
	// trustAnchorBundle is shipped beside the provider's; its roles reference it.
	trustAnchorBundle = "1.3.6/bundle.yaml"
)

// certManagerID is the prefix of every synthetic foundation of cert-manager.
var certManagerID = model.InfraIDPrefix + certManager + "."

// rowPaths returns key below every ingresses row of b.
func rowPaths(b model.Bundle, key string) []string {
	var out []string
	for _, r := range b.Ingresses {
		out = append(out, r.Path+"."+key)
	}
	return out
}

// infraLeaves returns the leaves cert-manager's foundations render, by path.
func infraLeaves(tree Tree) map[string]any {
	out := map[string]any{}
	for _, l := range tree.Leaves {
		if strings.HasPrefix(l.Provenance.Foundation, certManagerID) {
			out[l.Path.String()] = l.Value
		}
	}
	return out
}

func pathStrings(refs []TargetRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.Path.String())
	}
	return out
}

// declaredPaths are the paths the Stage 3 foundations target.
func declaredPaths(b model.Bundle) map[string]bool {
	out := map[string]bool{}
	for _, f := range b.CompiledFoundations() {
		if model.IsInfraID(f.ID) {
			continue
		}
		for _, t := range f.Targets {
			out[t.Path.String()] = true
		}
	}
	return out
}

// externalAnswers answers external's Secret question for every row.
func externalAnswers(b model.Bundle) map[string]any {
	out := map[string]any{}
	for _, r := range b.Ingresses {
		out["secret."+r.ID] = r.ID + externalSecretSuffix
	}
	return out
}

func withRows(b model.Bundle, leaves map[string]any, key string, value func(model.Ingress) any) map[string]any {
	out := maps.Clone(leaves)
	for _, r := range b.Ingresses {
		out[r.Path+"."+key] = value(r)
	}
	return out
}

func TestCertManagerModeKeySets(t *testing.T) {
	b := providerBundle(t)
	issuer := func(v string) func(model.Ingress) any { return func(model.Ingress) any { return v } }
	tests := []struct {
		name    string
		mode    string
		answers map[string]any
		want    map[string]any
	}{
		{"bundled, self-signed", model.ModeBundled, map[string]any{"type": typeSelfsigned},
			withRows(b, map[string]any{cmEnabled: true, cmCRDs: true, cmrEnabled: true, cmrType: typeSelfsigned, cmrNamespace: infraNS},
				issuerKey, issuer("selfsigned-issuer"))},
		{"bundled, Let's Encrypt", model.ModeBundled, map[string]any{"type": typeProd, "acmeEmail": infraEmail, "ingressClass": infraClass},
			withRows(b, map[string]any{cmEnabled: true, cmCRDs: true, cmrEnabled: true, cmrType: typeProd, cmrEmail: infraEmail, cmrIngressClass: infraClass},
				issuerKey, issuer("prod-issuer"))},
		{"cluster", model.ModeCluster, map[string]any{"issuer": infraIssuer},
			withRows(b, map[string]any{cmEnabled: false, cmrEnabled: false}, issuerKey, issuer(infraIssuer))},
		{"external", model.ModeExternal, externalAnswers(b),
			withRows(b, map[string]any{cmEnabled: false, cmrEnabled: false}, secretNameKey,
				func(r model.Ingress) any { return r.ID + externalSecretSuffix })},
	}
	declared := declaredPaths(b)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := Render(b, infraProfile(tc.mode, tc.answers), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got := infraLeaves(tree); !maps.Equal(got, tc.want) {
				t.Errorf("cert-manager leaves = %v, want %v", got, tc.want)
			}
			own := pathStrings(ModeTargets(b, certManager, tc.mode))
			for p := range tc.want {
				if !slices.Contains(own, p) {
					t.Errorf("%s is rendered but not declared by mode %s", p, tc.mode)
				}
			}
			rendered := values(tree)
			for _, other := range []string{model.ModeBundled, model.ModeCluster, model.ModeExternal} {
				for _, p := range pathStrings(ModeTargets(b, certManager, other)) {
					if _, ok := rendered[p]; ok && !slices.Contains(own, p) && !declared[p] {
						t.Errorf("%s, declared only by mode %s, is rendered", p, other)
					}
				}
			}
		})
	}
}

func TestCertManagerClusterAsksOneQuestion(t *testing.T) {
	d, ok := providerBundle(t).Dependency(certManager)
	if !ok {
		t.Fatalf("no %s dependency", certManager)
	}
	m, _ := d.Mode(model.ModeCluster)
	if len(m.Questions) != 1 || m.Questions[0].ID != "issuer" || m.PerIngressQuestion != nil {
		t.Errorf("cluster asks %+v and %+v, want issuer alone", m.Questions, m.PerIngressQuestion)
	}
}

// TestCertManagerModeSwitch pins that external drops every issuer annotation and that cluster brings <host>-tls back.
func TestCertManagerModeSwitch(t *testing.T) {
	b := providerBundle(t)
	base, err := Render(b, infraProfile("", nil), Options{})
	if err != nil {
		t.Fatal(err)
	}
	answers := externalAnswers(b)
	answers["issuer"] = infraIssuer
	for _, mode := range []string{model.ModeCluster, model.ModeExternal, model.ModeCluster} {
		tree, err := Render(b, infraProfile(mode, answers), Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range tree.Leaves {
			if mode == model.ModeExternal && strings.HasSuffix(l.Path.String(), issuerKey) {
				t.Errorf("external renders %s", l.Path)
			}
		}
		for _, p := range rowPaths(b, secretNameKey) {
			got, want := leaf(t, tree, p), leaf(t, base, p)
			if mode == model.ModeCluster && !reflect.DeepEqual(got, want) {
				t.Errorf("cluster: %s = %v from %s, want %v from %s", p, got.Value, got.Provenance.Foundation, want.Value, want.Provenance.Foundation)
			}
			if mode == model.ModeExternal && !strings.HasSuffix(fmt.Sprint(got.Value), externalSecretSuffix) {
				t.Errorf("external: %s = %v, want the answered Secret", p, got.Value)
			}
		}
	}
}

// TestCertManagerUnansweredRendersAsBefore compares a profile without infrastructure against the bundle without
// its ingresses, infrastructure and ACME rules, which is revision 18's render.
func TestCertManagerUnansweredRendersAsBefore(t *testing.T) {
	data, err := fs.ReadFile(bundles.FS, providerVersion+"/bundle.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start, end := strings.Index(s, infraBlockStart), strings.Index(s, foundationsStart)
	rulesStart, rulesEnd := strings.Index(s, acmeRulesStart), strings.Index(s, resourcesStart)
	if start < 0 || end < start || rulesStart < 0 || rulesEnd < rulesStart || start < rulesEnd {
		t.Fatal("shipped bundle changed shape: want the ACME rules right before resources, and ingresses and infrastructure right before foundations")
	}
	stripped := s[:rulesStart] + s[rulesEnd:start] + s[end:]
	shipped := fstest.MapFS{providerVersion + "/bundle.yaml": {Data: []byte(stripped)}}
	ta, err := fs.ReadFile(bundles.FS, trustAnchorBundle)
	if err != nil {
		t.Fatal(err)
	}
	shipped[trustAnchorBundle] = &fstest.MapFile{Data: ta}
	bs, err := model.Versions(shipped)
	if err != nil {
		t.Fatal(err)
	}
	p, err := profile.ReadFile(providerProfile)
	if err != nil {
		t.Fatal(err)
	}
	p.Infrastructure = nil
	before, err := Render(bs[0], p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := Render(providerBundle(t), p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("a profile without infrastructure renders differently:\n%v\nwant\n%v", values(after), values(before))
	}
}

// TestCertManagerIssuerNames pins <type>-issuer to the ClusterIssuer the chart creates for that type.
func TestCertManagerIssuerNames(t *testing.T) {
	data, err := os.ReadFile(issuerTemplate)
	if err != nil {
		t.Fatal(err)
	}
	branches := regexp.MustCompile(`eq \.Values\.certManagerResources\.type "([a-z]+)" \}\}`).FindAllStringSubmatchIndex(string(data), -1)
	d, _ := providerBundle(t).Dependency(certManager)
	m, _ := d.Mode(model.ModeBundled)
	q := m.Questions[slices.IndexFunc(m.Questions, func(q model.Question) bool { return q.ID == "type" })]
	for _, o := range q.Options {
		i := slices.IndexFunc(branches, func(br []int) bool { return string(data[br[2]:br[3]]) == o.Value })
		if i < 0 {
			t.Errorf("%s has no branch for type %s", issuerTemplate, o.Value)
			continue
		}
		body := string(data[branches[i][1]:])
		if i+1 < len(branches) {
			body = string(data[branches[i][1]:branches[i+1][0]])
		}
		issuer := regexp.MustCompile(`(?m)^kind: ClusterIssuer\nmetadata:\n  name: ` + regexp.QuoteMeta(o.Value) + `-issuer$`)
		if !issuer.MatchString(body) {
			t.Errorf("type %s creates no ClusterIssuer %s-issuer", o.Value, o.Value)
		}
	}
}

// TestCertManagerOperatorsReference renders bundled, self-signed, into the operators namespace and compares it with
// k3s/operators.yaml's cert-manager keys.
func TestCertManagerOperatorsReference(t *testing.T) {
	// startupapicheck.enabled is the subchart default; the tool does not own it.
	excluded := map[string]bool{"cert-manager.startupapicheck.enabled": true}
	b := providerBundle(t)
	p := infraProfile(model.ModeBundled, map[string]any{"type": typeSelfsigned})
	p.Target.Namespace = operatorsNS
	tree, err := Render(b, p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rendered := infraLeaves(tree)
	for _, l := range roundtrip.Classify(parseValues(t, operatorsValues), roundtrip.DefaultClassifier) {
		key := l.Path.String()
		if !strings.HasPrefix(key, "cert-manager.") && !strings.HasPrefix(key, "certManagerResources.") || excluded[key] {
			continue
		}
		v, ok := rendered[key]
		if !ok {
			t.Errorf("%s is in %s but not rendered", key, operatorsValues)
			continue
		}
		if fmt.Sprint(v) != l.Node.Value {
			t.Errorf("%s = %v, %s has %q", key, v, operatorsValues, l.Node.Value)
		}
		delete(rendered, key)
	}
	for _, key := range slices.Sorted(maps.Keys(rendered)) {
		if !strings.HasSuffix(key, issuerKey) {
			t.Errorf("%s is rendered but not in %s", key, operatorsValues)
		}
	}
}

// TestCertManagerNeedsTheOwner pins that no per-ingress leaf renders while the hosts are unanswered, even APISIX's,
// whose owner target is a constant.
func TestCertManagerNeedsTheOwner(t *testing.T) {
	b := providerBundle(t)
	tests := []struct {
		mode    string
		answers map[string]any
	}{
		{model.ModeCluster, map[string]any{"issuer": infraIssuer}},
		{model.ModeExternal, nil},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			p := infraProfile(tc.mode, tc.answers)
			p.Answers = nil
			tree, err := Render(b, p, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for p := range infraLeaves(tree) {
				if slices.ContainsFunc(b.Ingresses, func(r model.Ingress) bool { return strings.HasPrefix(p, r.Path+".") }) {
					t.Errorf("%s renders without its owner", p)
				}
			}
		})
	}
}
