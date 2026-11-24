package render

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	providerValues  = "../../../k3s/provider.yaml"
	providerProfile = fixtureDir + "/provider.dscprofile.yaml"
	providerChart   = "data-space-connector"
	providerVersion = "10.16.0"
	// referenceDomain is the base domain every public host in provider.yaml is derived from.
	referenceDomain = "127.0.0.1.nip.io"
	otherDomain     = "provider.example.org"
	orgName         = "MP Operations"
	orgDisplayName  = "org.displayName"
	domainBase      = "domain.base"
	// realmJSON mirrors how _realm.tpl interpolates keycloak.realm.displayNameHtml: unescaped, in a JSON string.
	realmJSON       = `{"displayNameHtml": "%s"}`
	displayNameKey  = "keycloak.realm.displayNameHtml"
	loginHeaderHTML = `<div class='kc-logo-text'><span>`
)

// exclusion is a domain-bearing provider.yaml leaf another story owns, so no foundation targets it.
type exclusion struct {
	pattern *regexp.Regexp
	story   string
	reason  string
}

var providerExclusions = []exclusion{
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.deployment\.verifier\.tirAddress$`),
		"US-3.3", "the data space's TIR, not one of the participant's hosts"},
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.registration\.services\[\d+\]\.oidcScopes\..+\.credentials\[\d+\]\.trustedParticipantsLists\[\d+\]$`),
		"US-3.3", "the data space's TIR, not one of the participant's hosts"},
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.apisix\.routes\[\d+\]\.host$`),
		"US-5.6", "a route is a protected-service row; the list carries content no foundation owns"},
	{regexp.MustCompile(`^keycloak\.realm\.clients\[\d+\]\.`),
		"US-5.5", "realm clients belong to the realm; the list carries content no foundation owns"},
	{regexp.MustCompile(`^fdsc-dashboard\.auth\.config\.providers\[\d+\]\.issuer$`),
		"US-5.3", "the dashboard's feature page; the list carries content no foundation owns"},
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.tpp\.transfers\.host$`),
		"US-5.3", "the DSP feature page; TPP's in-cluster back-channel to Rainbow, which local k3s reaches through nip.io"},
}

func providerBundle(t *testing.T) model.Bundle {
	t.Helper()
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bs {
		if b.Chart == providerChart && b.Version == providerVersion {
			return b
		}
	}
	t.Fatalf("no embedded %s %s bundle", providerChart, providerVersion)
	return model.Bundle{}
}

func providerDoc(t *testing.T) *roundtrip.Document {
	t.Helper()
	src, err := os.ReadFile(providerValues)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := roundtrip.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// domainLeaves returns the provider.yaml leaves containing the reference domain, minus the
// exclusions, and fails when an exclusion row matches no leaf.
func domainLeaves(t *testing.T, doc *roundtrip.Document) []string {
	t.Helper()
	hits := make([]int, len(providerExclusions))
	var paths []string
	for _, l := range roundtrip.Classify(doc, roundtrip.DefaultClassifier) {
		if l.Node.Kind != yaml.ScalarNode || !strings.Contains(l.Node.Value, referenceDomain) {
			continue
		}
		p := l.Path.String()
		i := slices.IndexFunc(providerExclusions, func(e exclusion) bool { return e.pattern.MatchString(p) })
		if i >= 0 {
			hits[i]++
			continue
		}
		paths = append(paths, p)
	}
	for i, n := range hits {
		if n == 0 {
			e := providerExclusions[i]
			t.Errorf("exclusion %s (%s) matches no provider.yaml leaf; drop the stale row", e.pattern, e.story)
		}
	}
	sort.Strings(paths)
	return paths
}

func renderProvider(t *testing.T, b model.Bundle, answers map[string]any) Tree {
	t.Helper()
	tree, err := Render(b, profile.Profile{Answers: answers}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// derivedPaths are the domain-derived leaves: answer-derived (not const), not the display name's.
func derivedPaths(tree Tree) []string {
	var paths []string
	for _, l := range tree.Leaves {
		if l.Provenance.Fn != string(model.FnConst) && l.Provenance.Foundation != orgDisplayName {
			paths = append(paths, l.Path.String())
		}
	}
	sort.Strings(paths)
	return paths
}

func diffSets(want, got []string) (missing, extra []string) {
	for _, p := range want {
		if !slices.Contains(got, p) {
			missing = append(missing, p)
		}
	}
	for _, p := range got {
		if !slices.Contains(want, p) {
			extra = append(extra, p)
		}
	}
	return missing, extra
}

func TestProviderKeySet(t *testing.T) {
	want := domainLeaves(t, providerDoc(t))
	tree := renderProvider(t, providerBundle(t), map[string]any{orgDisplayName: orgName, domainBase: referenceDomain})
	missing, extra := diffSets(want, derivedPaths(tree))
	for _, p := range missing {
		t.Errorf("provider.yaml leaf %s carries the domain but is neither a target nor excluded", p)
	}
	for _, p := range extra {
		t.Errorf("target %s is not a domain-bearing provider.yaml leaf", p)
	}
}

func TestProviderValues(t *testing.T) {
	doc := providerDoc(t)
	p, err := profile.ReadFile(providerProfile)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	if errs := p.Validate(bs); len(errs) > 0 {
		t.Fatalf("%s is invalid: %v", providerProfile, errs)
	}
	tree := renderProvider(t, providerBundle(t), p.Answers)
	owned := map[string]roundtrip.Path{}
	for _, l := range tree.Leaves {
		for _, lp := range lists(l.Path) {
			owned[lp.String()] = lp
		}
		if l.Provenance.Foundation == orgDisplayName {
			// provider.yaml sets no display name; the target is the chart's documented key.
			continue
		}
		n, err := doc.Lookup(l.Path)
		if err != nil {
			t.Errorf("%s: not in provider.yaml: %v", l.Path, err)
			continue
		}
		if got, ok := l.Value.(string); !ok || n.Kind != yaml.ScalarNode || got != n.Value {
			t.Errorf("%s = %#v, provider.yaml has %q", l.Path, l.Value, n.Value)
		}
	}
	nested := tree.Nested()
	for _, key := range slices.Sorted(maps.Keys(owned)) {
		lp := owned[key]
		n, err := doc.Lookup(lp)
		if err != nil {
			t.Errorf("owned list %s: not in provider.yaml: %v", key, err)
			continue
		}
		var want any
		if err := n.Decode(&want); err != nil {
			t.Fatal(err)
		}
		if got := subtree(nested, lp); !reflect.DeepEqual(got, want) {
			t.Errorf("owned list %s = %#v, provider.yaml has %#v", key, got, want)
		}
	}
}

func subtree(node any, p roundtrip.Path) any {
	for _, s := range p {
		if i, ok := s.Index(); ok {
			l, _ := node.([]any)
			if i >= len(l) {
				return nil
			}
			node = l[i]
			continue
		}
		k, _ := s.Key()
		m, _ := node.(map[string]any)
		node = m[k]
	}
	return node
}

func TestProviderDomainChange(t *testing.T) {
	b := providerBundle(t)
	before := values(renderProvider(t, b, map[string]any{orgDisplayName: orgName, domainBase: referenceDomain}))
	after := values(renderProvider(t, b, map[string]any{orgDisplayName: orgName, domainBase: otherDomain}))
	var changed []string
	for p, v := range after {
		if before[p] != v {
			changed = append(changed, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	missing, extra := diffSets(domainLeaves(t, providerDoc(t)), changed)
	if len(missing)+len(extra) > 0 {
		t.Errorf("changing %s changed the wrong leaves: unchanged %v, unexpected %v", domainBase, missing, extra)
	}
}

func TestProviderWithoutDomain(t *testing.T) {
	tree := renderProvider(t, providerBundle(t), map[string]any{orgDisplayName: orgName})
	for _, l := range tree.Leaves {
		if l.Provenance.Foundation != orgDisplayName {
			t.Errorf("%s emitted by %s with %s unanswered", l.Path, l.Provenance.Foundation, domainBase)
		}
	}
}

func TestProviderDisplayNameKeepsRealmJSONValid(t *testing.T) {
	b := providerBundle(t)
	for _, name := range []string{orgName, "R&D Labs", "O'Brien GmbH"} {
		t.Run(name, func(t *testing.T) {
			f, _ := b.Foundation(orgDisplayName)
			if err := f.Type.Check(name); err != nil {
				t.Fatalf("%q refused: %v", name, err)
			}
			v, ok := values(renderProvider(t, b, map[string]any{orgDisplayName: name}))[displayNameKey].(string)
			if !ok {
				t.Fatalf("%s not rendered as a string", displayNameKey)
			}
			if !strings.HasPrefix(v, loginHeaderHTML) || !strings.Contains(v, name) {
				t.Errorf("%s = %q, want the chart's login-header wrapper around %q", displayNameKey, v, name)
			}
			if doc := fmt.Sprintf(realmJSON, v); !json.Valid([]byte(doc)) {
				t.Errorf("realm JSON is invalid with %s = %q", displayNameKey, v)
			}
		})
	}
}
