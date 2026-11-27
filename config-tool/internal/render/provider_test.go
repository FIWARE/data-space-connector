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

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
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
	didMethodID     = "did.method"
	didValueID      = "did.value"
	tirAddressID    = "dataspace.tirAddress"
	hostVerifierID  = "host.verifier"
	hostDIDID       = "host.did"
	// referenceTIR is the data space's TIR address in provider.yaml; it contains the domain without deriving from it.
	referenceTIR = "https://tir." + referenceDomain + "/"
	didWebMethod = "did:web"
	didKeyMethod = "did:key"
	// referenceDIDHost serves the DID document in provider.yaml; it and the verifier are not on referenceDomain.
	referenceDIDHost      = "mp-operations.org"
	referenceVerifierHost = "verifier." + referenceDIDHost
	// referenceDID is the participant DID of provider.yaml and dsp-provider.yaml.
	referenceDID = didWebMethod + ":" + referenceDIDHost
	// dspProviderValues sets the fdsc-edc keys provider.yaml leaves out, because fdsc-edc is off there.
	dspProviderValues = "../../../k3s/dsp-provider.yaml"
	// generatedDID is a tool-generated file on oldDID, to which the user hand-added a bootstrap DID and kid.
	generatedDID = fixtureDir + "/generated-did.yaml"
	oldDID       = "did:web:old.example.org"
	dspFeature   = "dsp.edc"
	// The keys of the chart's dsc.identityhub.assertHolderKid; keyAliasDefault is values.yaml's keyAlias.
	bootstrapDIDKey  = "identityhub.bootstrap.did"
	keyAliasKey      = "identityhub.bootstrap.keyAlias"
	keyAliasDefault  = "key-1"
	commonKidKey     = "fdsc-edc.common.config.oid4vp.holder.kid"
	edcDeploymentKey = "fdsc-edc.deployment"
	deploymentKidKey = "config.oid4vp.holder.kid"
	participantIDKey = "fdsc-edc.common.config.edc.participant.id"
	fdscEdcPrefix    = "fdsc-edc."
	// realmJSON mirrors how _realm.tpl interpolates keycloak.realm.displayNameHtml: unescaped, in a JSON string.
	realmJSON       = `{"displayNameHtml": "%s"}`
	displayNameKey  = "keycloak.realm.displayNameHtml"
	loginHeaderHTML = `<div class='kc-logo-text'><span>`
)

// referenceValues is what each answer searched in provider.yaml is there.
var referenceValues = map[string]string{
	domainBase: referenceDomain, didValueID: referenceDID, tirAddressID: referenceTIR,
	hostVerifierID: referenceVerifierHost, hostDIDID: referenceDIDHost,
}

// exclusion is a provider.yaml leaf carrying one of answers that another story owns, so no foundation targets it.
type exclusion struct {
	pattern *regexp.Regexp
	answers []string
	story   string
	reason  string
}

// The DID document host is a substring of the DID and of the verifier host, so every row carrying them carries it too.
var (
	domainOnly   = []string{domainBase}
	didOnly      = []string{didValueID, hostDIDID}
	didHostOnly  = []string{hostDIDID}
	verifierOnly = []string{hostVerifierID, hostDIDID}
)

var providerExclusions = []exclusion{
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.registration\.services\[\d+\]\.oidcScopes\..+\.credentials\[\d+\]\.trustedParticipantsLists\[\d+\]$`), domainOnly,
		"US-5.6", "the service registration list, which US-5.6 pre-seeds from the data space's answers"},
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.apisix\.routes\[\d+\]\.host$`), domainOnly,
		"US-5.6", "a route is a protected-service row; the list carries content no foundation owns"},
	{regexp.MustCompile(`^keycloak\.realm\.clients\[\d+\]\.`), []string{domainBase, didValueID, hostDIDID},
		"US-5.5", "realm clients belong to the realm; the list carries content no foundation owns"},
	{regexp.MustCompile(`^fdsc-dashboard\.auth\.config\.providers\[\d+\]\.issuer$`), domainOnly,
		"US-5.3", "the dashboard's feature page; the list carries content no foundation owns"},
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.tpp\.transfers\.host$`), domainOnly,
		"US-5.3", "the DSP feature page; TPP's in-cluster back-channel to Rainbow, which local k3s reaches through nip.io"},
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.deployment\.initContainers\[\d+\]\.args\[\d+\]$`), didOnly,
		"US-3.3", "a k3s-local registration at the in-cluster TIR; no chart key, see ADR 0022"},
	{regexp.MustCompile(`^registration\.issuer\[\d+\]\.did$`), didOnly,
		"US-5.5", "local TIL entries, which also name other participants"},
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.odrl-pap\.additionalEnvVars\[\d+\]\.value$`), didOnly,
		"US-5.6", "an env list with other content"},
	{regexp.MustCompile(`^marketplace\.bizEcosystemLogicProxy\.additionalEnvVars\[\d+\]\.value$`), didOnly,
		"US-5.3", "an env list with other content"},
	{regexp.MustCompile(`^fdsc-dashboard\.auth\.config\.providers\[\d+\]\.clientId$`), didOnly,
		"US-5.3", "the dashboard's feature page; the list carries content no foundation owns"},
	{regexp.MustCompile(`^keycloak\.signingKey\.did$`), didOnly,
		"US-5.5", "alone it yields a java-keystore provider without store, password, alias or algorithm"},
	{regexp.MustCompile(`^keycloak\.realm\.verifiableCredentials\..+\.protocolMappers\[\d+\]\.config\.staticValue$`), didOnly,
		"US-5.5", "realm credentials"},
	{regexp.MustCompile(`^keycloak\.realm\.(clientRoles|users)$`), didOnly,
		"US-5.5", "JSON strings of the realm"},
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.ingress\.(tls\[\d+\]\.|annotations\.)`), verifierOnly,
		"US-8.11", "the TLS certificate is the verifier's signing key; deriving its name would orphan the mounted secret"},
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.deployment\.verifier\.clientIdentification\.id$`), verifierOnly,
		"US-8.11", "x509_san_dns identification needs the signing key and certificate paths with it"},
	{regexp.MustCompile(`^decentralizedIam\.vcAuthentication\.vcverifier\.deployment\.additionalVolumes\[\d+\]\.secret\.secretName$`), verifierOnly,
		"US-8.11", "the signing-key mount of the TLS secret; the list carries other content"},
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.apisix\.routes\[\d+\]\.plugins\.openid-connect\.discovery$`), verifierOnly,
		"US-5.6", "a route is a protected-service row; the list carries content no foundation owns"},
	{regexp.MustCompile(`^did\.ingress\.(tls\[\d+\]\.|annotations\.)`), didHostOnly,
		"US-8.11", "the TLS certificate holds the DID key; deriving its name would orphan the mounted secret"},
	{regexp.MustCompile(`^(did\.volumes|keycloak\.extraVolumes|marketplace\.bizEcosystemLogicProxy\.extraContainerVolumes|vc-operator\.extraVolumes)\[\d+\]\.secret\.secretName$`), didHostOnly,
		"US-8.11", "a mount of the DID key's TLS secret; the list carries other content"},
	{regexp.MustCompile(`^statusListServer\.signing\.issuer\.dnsNames\[\d+\]$`), didHostOnly,
		"US-8.11", "the status list's signing certificate"},
}

// chartOnlyTarget is the reference of a target provider.yaml lacks; an empty file means no k3s file sets it.
type chartOnlyTarget struct {
	file, path, reason string
}

var chartOnlyTargets = map[string]chartOnlyTarget{
	displayNameKey:   {"", "", "no k3s file sets a display name; TestProviderDisplayNameKeepsRealmJSONValid checks it"},
	participantIDKey: {dspProviderValues, participantIDKey, "fdsc-edc is off in provider.yaml"},
	"fdsc-edc.common.config.issuerId": {dspProviderValues, "fdsc-edc.common.config.issuerId",
		"fdsc-edc is off in provider.yaml"},
	"fdsc-edc.common.config.oauth.clientId": {dspProviderValues, "fdsc-edc.common.config.oauth.clientId",
		"fdsc-edc is off in provider.yaml"},
	"fdsc-edc.common.config.oauth.secretAlias": {dspProviderValues, "fdsc-edc.common.config.oauth.secretAlias",
		"fdsc-edc is off in provider.yaml"},
	// dsp-provider.yaml names its deployment, which the tool cannot: the target sits under common, which the
	// subchart merges into every deployment.
	"fdsc-edc.common.config.oid4vp.holder.id": {dspProviderValues, "fdsc-edc.deployment.oid4vc.config.oid4vp.holder.id",
		"fdsc-edc is off in provider.yaml"},
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

func parseValues(t *testing.T, file string) *roundtrip.Document {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := roundtrip.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func providerDoc(t *testing.T) *roundtrip.Document {
	t.Helper()
	return parseValues(t, providerValues)
}

// answerLeaves returns the provider.yaml leaves carrying answer's reference value, minus answer's
// exclusions and the targets of another searched answer whose reference value contains it, and fails
// when one of those exclusion rows matches no leaf.
func answerLeaves(t *testing.T, doc *roundtrip.Document, answer string) []string {
	t.Helper()
	needle := referenceValues[answer]
	others := otherTargets(t, answer, needle)
	hits := make([]int, len(providerExclusions))
	var paths []string
	for _, l := range roundtrip.Classify(doc, roundtrip.DefaultClassifier) {
		if l.Node.Kind != yaml.ScalarNode || !strings.Contains(l.Node.Value, needle) {
			continue
		}
		p := l.Path.String()
		if others[p] {
			continue
		}
		i := slices.IndexFunc(providerExclusions, func(e exclusion) bool {
			return slices.Contains(e.answers, answer) && e.pattern.MatchString(p)
		})
		if i >= 0 {
			hits[i]++
			continue
		}
		paths = append(paths, p)
	}
	for i, n := range hits {
		if e := providerExclusions[i]; n == 0 && slices.Contains(e.answers, answer) {
			t.Errorf("exclusion %s (%s) matches no provider.yaml leaf carrying %s; drop the stale row", e.pattern, e.story, answer)
		}
	}
	sort.Strings(paths)
	return paths
}

// otherTargets returns the target paths of the searched answers other than answer whose reference value contains needle.
func otherTargets(t *testing.T, answer, needle string) map[string]bool {
	t.Helper()
	b := providerBundle(t)
	paths := map[string]bool{}
	for id, ref := range referenceValues {
		if id == answer || !strings.Contains(ref, needle) {
			continue
		}
		f, ok := b.Foundation(id)
		if !ok {
			t.Fatalf("no foundation %s", id)
		}
		for _, tg := range f.Targets {
			paths[tg.Path.String()] = true
		}
	}
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

// carries reports whether a leaf with provenance p carries answer: a searched answer other than the domain is
// carried by its own targets only, every other answer-derived target but the display name's carries the domain.
func carries(answer string, p Provenance) bool {
	if p.Fn == string(model.FnConst) {
		return false
	}
	if answer != domainBase {
		return p.Foundation == answer
	}
	_, searched := referenceValues[p.Foundation]
	return p.Foundation == domainBase || !searched && p.Foundation != orgDisplayName
}

// carrierPaths returns the paths of tree's leaves carrying answer, minus the chart-only targets.
func carrierPaths(tree Tree, answer string) []string {
	var paths []string
	for _, l := range tree.Leaves {
		if _, chartOnly := chartOnlyTargets[l.Path.String()]; !chartOnly && carries(answer, l.Provenance) {
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

// checkKeySet fails unless the leaves carrying answer in provider.yaml are exactly its targets there.
func checkKeySet(t *testing.T, answer string) {
	t.Helper()
	want := answerLeaves(t, providerDoc(t), answer)
	answers := map[string]any{orgDisplayName: orgName}
	for id, v := range referenceValues {
		answers[id] = v
	}
	tree := renderProvider(t, providerBundle(t), answers)
	missing, extra := diffSets(want, carrierPaths(tree, answer))
	for _, p := range missing {
		t.Errorf("provider.yaml leaf %s carries %s but is neither a target nor excluded", p, answer)
	}
	for _, p := range extra {
		t.Errorf("target %s is not a provider.yaml leaf carrying %s, nor a chart-only target", p, answer)
	}
}

func TestProviderKeySet(t *testing.T) {
	for _, answer := range slices.Sorted(maps.Keys(referenceValues)) {
		t.Run(answer, func(t *testing.T) { checkKeySet(t, answer) })
	}
}

func TestProviderValues(t *testing.T) {
	refs := map[string]*roundtrip.Document{providerValues: providerDoc(t)}
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
	tree, err := Render(providerBundle(t), profile.Profile{Answers: p.Answers, Infrastructure: p.Infrastructure}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// provider.yaml hand-tunes resources, so the fixture answers custom and the table renders nothing.
	if got := tableLeaves(tree); len(got) > 0 {
		t.Errorf("%d leaves under resources table paths, want none", len(got))
	}
	owned := map[string]roundtrip.Path{}
	rendered := map[string]bool{}
	for _, l := range tree.Leaves {
		// The proxy fan-out writes into lists provider.yaml also fills; TestProviderProxy checks it over the file.
		if slices.Contains(proxyIDs, l.Provenance.Foundation) {
			continue
		}
		key := l.Path.String()
		rendered[key] = true
		for _, lp := range lists(l.Path) {
			owned[lp.String()] = lp
		}
		file, ref := providerValues, l.Path
		if c, ok := chartOnlyTargets[key]; ok {
			if _, err := refs[providerValues].Lookup(l.Path); err == nil {
				t.Errorf("%s is in provider.yaml; drop its chartOnlyTargets row", key)
			}
			if c.file == "" {
				continue
			}
			file, ref = c.file, mustPath(t, c.path)
		}
		if refs[file] == nil {
			refs[file] = parseValues(t, file)
		}
		n, err := refs[file].Lookup(ref)
		if err != nil {
			t.Errorf("%s: %s not in %s: %v", key, ref, file, err)
			continue
		}
		if !scalarEquals(t, l.Value, n) {
			t.Errorf("%s = %#v, %s has %q at %s", key, l.Value, file, n.Value, ref)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(chartOnlyTargets)) {
		if !rendered[key] {
			t.Errorf("chart-only target %s is not rendered; drop the stale row", key)
		}
	}
	nested := tree.Nested()
	for _, key := range slices.Sorted(maps.Keys(owned)) {
		lp := owned[key]
		n, err := refs[providerValues].Lookup(lp)
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

// scalarEquals reports whether n is a scalar holding v: a string as written, anything else as decoded.
func scalarEquals(t *testing.T, v any, n *yaml.Node) bool {
	t.Helper()
	if n.Kind != yaml.ScalarNode {
		return false
	}
	if s, ok := v.(string); ok {
		return s == n.Value
	}
	var decoded any
	if err := n.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(v, decoded)
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

func changedPaths(before, after map[string]any) []string {
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
	return changed
}

// targetPaths returns the paths of every target answer reaches, directly or through a derived answer.
func targetPaths(b model.Bundle, answer string) []string {
	var paths []string
	for _, r := range Targets(b, answer) {
		paths = append(paths, r.Path.String())
	}
	return paths
}

func TestProviderDIDChange(t *testing.T) {
	b := providerBundle(t)
	domainLeaves := answerLeaves(t, providerDoc(t), domainBase)
	// The verifier stays pinned: TestProviderTargetsAndDiff covers it deriving from the domain.
	pinned := map[string]any{orgDisplayName: orgName, hostVerifierID: referenceVerifierHost}
	didTargets := targetPaths(b, didValueID)
	didHostTargets := slices.DeleteFunc(targetPaths(b, hostDIDID), func(p string) bool { return slices.Contains(didTargets, p) })
	tests := []struct {
		name  string
		fixed map[string]any
		want  []string
	}{
		{"did.value and host.did derived", pinned, slices.Concat(domainLeaves, didHostTargets, didTargets)},
		{"did.value given", merge(pinned, map[string]any{didValueID: referenceDID}), slices.Concat(domainLeaves, didHostTargets)},
		{"host.did given", merge(pinned, map[string]any{hostDIDID: referenceDIDHost}), domainLeaves},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answers := func(domain string) map[string]any {
				return merge(tc.fixed, map[string]any{domainBase: domain})
			}
			changed := changedPaths(values(renderProvider(t, b, answers(referenceDomain))),
				values(renderProvider(t, b, answers(otherDomain))))
			missing, extra := diffSets(tc.want, changed)
			if len(missing)+len(extra) > 0 {
				t.Errorf("changing %s changed the wrong leaves: unchanged %v, unexpected %v", domainBase, missing, extra)
			}
		})
	}
}

func TestProviderDIDMethod(t *testing.T) {
	b := providerBundle(t)
	tests := []struct {
		name    string
		answers map[string]any
		// want is every DID target's value; empty when none may be emitted.
		want string
	}{
		{"did:web derives from the domain", map[string]any{didMethodID: didWebMethod}, didWebMethod + ":" + referenceDomain},
		{"did:key without a value emits nothing", map[string]any{didMethodID: didKeyMethod}, ""},
		{"did:key with a value", map[string]any{didMethodID: didKeyMethod, didValueID: didKey}, didKey},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderProvider(t, b, merge(tc.answers, map[string]any{domainBase: referenceDomain}))
			n := 0
			for _, l := range tree.Leaves {
				if l.Provenance.Foundation != didValueID {
					continue
				}
				n++
				if tc.want == "" || !strings.HasPrefix(l.Value.(string), tc.want) {
					t.Errorf("%s = %v, want a value starting with %q", l.Path, l.Value, tc.want)
				}
			}
			if tc.want != "" && n == 0 {
				t.Errorf("no DID target rendered, want %q", tc.want)
			}
		})
	}
}

// TestChartOnlyTargetsUngated pins the accepted state that the fdsc-edc DID targets ignore the dsp.edc
// feature; US-5.3 replaces it with a gated test.
func TestChartOnlyTargetsUngated(t *testing.T) {
	p := profile.Profile{
		Answers:  map[string]any{domainBase: referenceDomain, didValueID: referenceDID},
		Features: map[string]profile.FeatureState{dspFeature: {Enabled: false, Source: profile.SourceUser}},
	}
	tree, err := Render(providerBundle(t), p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var want, got []string
	for key := range chartOnlyTargets {
		if strings.HasPrefix(key, fdscEdcPrefix) {
			want = append(want, key)
		}
	}
	for _, l := range tree.Leaves {
		if key := l.Path.String(); strings.HasPrefix(key, fdscEdcPrefix) {
			got = append(got, key)
		}
	}
	sort.Strings(want)
	if missing, extra := diffSets(want, got); len(missing)+len(extra) > 0 {
		t.Errorf("fdsc-edc leaves with %s off: missing %v, unexpected %v", dspFeature, missing, extra)
	}
}

// TestNoFdscEdcDeploymentTargets keeps every render from adding an fdsc-edc controlplane: the subchart
// creates one per key under fdsc-edc.deployment.
func TestNoFdscEdcDeploymentTargets(t *testing.T) {
	b := providerBundle(t)
	prefix := fdscEdcPrefix + "deployment"
	for _, decl := range b.Foundations {
		f, _ := b.Foundation(decl.ID)
		for i, tg := range f.Targets {
			if p := tg.Path.String(); overlaps(p, prefix) {
				t.Errorf("%s targets[%d] writes %s, which adds an fdsc-edc deployment", f.ID, i, p)
			}
		}
	}
}

// TestDIDKeysOtherStoriesOwn keeps the key-id and keystore keys out of every render: their right form
// needs answers the tool does not own (US-8.11, US-5.5).
func TestDIDKeysOtherStoriesOwn(t *testing.T) {
	p, err := profile.ReadFile(providerProfile)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := regexp.MustCompile(`^(identityhub\.bootstrap\.did|fdsc-edc\.(common|deployment\..+)\.config\.oid4vp\.holder\.kid|keycloak\.signingKey(\..+)?)$`)
	for _, l := range renderProvider(t, providerBundle(t), p.Answers).Leaves {
		if forbidden.MatchString(l.Path.String()) {
			t.Errorf("%s targets %s", l.Provenance.Foundation, l.Path)
		}
	}
}

// holderKidErrors mirrors the chart's dsc.identityhub.assertHolderKid over merged values.
func holderKidErrors(t *testing.T, merged map[string]any) []string {
	did, _ := subtree(merged, mustPath(t, bootstrapDIDKey)).(string)
	if did == "" {
		return nil
	}
	alias, ok := subtree(merged, mustPath(t, keyAliasKey)).(string)
	if !ok {
		alias = keyAliasDefault
	}
	want := did + "#" + alias
	kids := map[string]any{commonKidKey: subtree(merged, mustPath(t, commonKidKey))}
	deployments, _ := subtree(merged, mustPath(t, edcDeploymentKey)).(map[string]any)
	for name, d := range deployments {
		kids[edcDeploymentKey+"."+name+"."+deploymentKidKey] = subtree(d, mustPath(t, deploymentKidKey))
	}
	var errs []string
	for key, kid := range kids {
		if kid != nil && kid != any(want) {
			errs = append(errs, fmt.Sprintf("%s = %v, want %s", key, kid, want))
		}
	}
	sort.Strings(errs)
	return errs
}

// TestHolderKidMatchesBootstrapDID checks the merged output (loaded file plus render) against the chart's
// assertHolderKid, which fails helm template.
func TestHolderKidMatchesBootstrapDID(t *testing.T) {
	newDID := didWebMethod + ":" + otherDomain
	tests := []struct {
		name    string
		loaded  string
		answers map[string]any
		// bootstrapDID and participantID are the merged values; empty means unset.
		bootstrapDID, participantID string
	}{
		{"fresh render", "", map[string]any{domainBase: referenceDomain}, "", didWebMethod + ":" + referenceDomain},
		// The hand-added bootstrap DID and kid stay on the old DID while the owned targets follow the domain.
		// That drift is accepted until US-8.11 owns the pair, which flips bootstrapDID to newDID.
		{"generated file, domain changed", generatedDID, map[string]any{domainBase: otherDomain}, oldDID, newDID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			merged := map[string]any{}
			var opts Options
			if tc.loaded != "" {
				src, err := os.ReadFile(tc.loaded)
				if err != nil {
					t.Fatal(err)
				}
				bs, err := model.Versions(bundles.FS)
				if err != nil {
					t.Fatal(err)
				}
				root := roundtrip.Path{}
				if opts.Overrides, err = load.Load(src, tc.loaded, bs, load.Options{Root: &root}); err != nil {
					t.Fatal(err)
				}
				if err := yaml.Unmarshal(src, &merged); err != nil {
					t.Fatal(err)
				}
			}
			tree, err := Render(providerBundle(t), profile.Profile{Answers: tc.answers}, opts)
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range nestedLeaves(tree) {
				merged = insert(merged, l.Path, l.Value).(map[string]any)
			}
			if errs := holderKidErrors(t, merged); len(errs) > 0 {
				t.Errorf("helm template would fail in assertHolderKid: %v", errs)
			}
			bootstrapDID, _ := subtree(merged, mustPath(t, bootstrapDIDKey)).(string)
			participantID, _ := subtree(merged, mustPath(t, participantIDKey)).(string)
			if bootstrapDID != tc.bootstrapDID || participantID != tc.participantID {
				t.Errorf("%s = %q, %s = %q; want %q, %q", bootstrapDIDKey, bootstrapDID, participantIDKey, participantID,
					tc.bootstrapDID, tc.participantID)
			}
		})
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

// referenceProxy is the forward proxy every proxied component of provider.yaml uses.
const referenceProxy = "http://squid-proxy.infra.svc.cluster.local:8888"

// proxyExclusion is a proxy leaf on which the fan-out deliberately differs from provider.yaml; a nil foundation
// list matches a provider.yaml leaf carrying referenceProxy that no proxy target writes.
type proxyExclusion struct {
	foundations []string
	pattern     *regexp.Regexp
	reason      string
}

const (
	verifierEnvPattern = `^decentralizedIam\.vcAuthentication\.vcverifier\.deployment\.additionalEnvVars\[\d+\]\.`
	routeProxyOpts     = `^decentralizedIam\.odrlAuthorization\.apisix\.routes\[\d+\]\.plugins\.openid-connect\.proxy_opts\.`
)

var (
	httpAndNo               = []string{proxyHTTPID, proxyNoID}
	providerProxyExclusions = []proxyExclusion{
		{[]string{proxyNoID}, regexp.MustCompile(verifierEnvPattern),
			"US-3.6: k3s tailors the verifier's NO_PROXY (w3.org); the default bypasses every in-cluster service instead"},
		{[]string{proxyNoID}, regexp.MustCompile(`^vc-operator\.extraEnvVars\[\d+\]\.`),
			"US-3.6: k3s appends the 10.0.0.0/8 CIDR; the default holds none, the field help says to append it"},
		{[]string{proxyNoID}, regexp.MustCompile(`^keycloak\.proxy\.noProxy$`),
			"US-3.6: k3s writes *. forms Keycloak does not match, and relies on spiProxyMappings instead"},
		{[]string{proxyHTTPID}, regexp.MustCompile(`^keycloak\.proxy\.httpProxy$`),
			"US-3.6: k3s sets no HTTP proxy for Keycloak; the fan-out sets all three"},
		{httpAndNo, regexp.MustCompile(`^marketplace\.bizEcosystemLogicProxy\.additionalEnvVars\[\d+\]\.`),
			"US-3.6: k3s sets only HTTPS_PROXY for the logic proxy; the fan-out sets all three"},
		{httpAndNo, regexp.MustCompile(routeProxyOpts),
			"US-3.6: k3s sets only https_proxy in a route; the fan-out sets all three"},
		{nil, regexp.MustCompile(`^keycloak\.proxy\.spiProxyMappings$`),
			"US-3.6: the tool never writes it; Keycloak ignores the proxy env vars while it is set (a Step 5 warning)"},
	}
)

// matchProxyExclusion returns the index of the row matching a leaf at path from foundation, or -1.
func matchProxyExclusion(foundation, path string) int {
	return slices.IndexFunc(providerProxyExclusions, func(e proxyExclusion) bool {
		if foundation == "" {
			return e.foundations == nil && e.pattern.MatchString(path)
		}
		return slices.Contains(e.foundations, foundation) && e.pattern.MatchString(path)
	})
}

// TestProviderProxy renders the proxy answer over provider.yaml: every proxy leaf reproduces the file unless a
// providerProxyExclusions row says why not, and every leaf of the file carrying the proxy is a proxy target or excluded.
func TestProviderProxy(t *testing.T) {
	p, err := profile.ReadFile(providerProfile)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Answers[proxyHTTPSID]; got != referenceProxy {
		t.Fatalf("%s answers %s = %v, want %s", providerProfile, proxyHTTPSID, got, referenceProxy)
	}
	tree := renderOverProvider(t, p)
	doc := providerDoc(t)
	hits := make([]int, len(providerProxyExclusions))
	written := map[string]bool{}
	components := map[string]bool{}
	for _, l := range tree.Leaves {
		if !slices.Contains(proxyIDs, l.Provenance.Foundation) || l.Provenance.Origin != "" {
			continue
		}
		key := l.Path.String()
		written[key] = true
		k, _ := l.Path[0].Key()
		components[k] = true
		v := l.Value
		if l.Provenance.Pinned {
			v = l.Provenance.Rendered
		}
		n, err := doc.Lookup(l.Path)
		if err == nil && n.Kind == yaml.ScalarNode && fmt.Sprint(v) == n.Value {
			continue
		}
		if i := matchProxyExclusion(l.Provenance.Foundation, key); i >= 0 {
			hits[i]++
			continue
		}
		t.Errorf("%s renders %s = %v, which provider.yaml does not hold", l.Provenance.Foundation, key, v)
	}
	for _, l := range roundtrip.Classify(doc, roundtrip.DefaultClassifier) {
		key := l.Path.String()
		if l.Node.Kind != yaml.ScalarNode || !strings.Contains(l.Node.Value, referenceProxy) || written[key] {
			continue
		}
		if i := matchProxyExclusion("", key); i >= 0 {
			hits[i]++
			continue
		}
		t.Errorf("provider.yaml leaf %s carries the proxy but no proxy target writes it", key)
	}
	for i, n := range hits {
		if n == 0 {
			t.Errorf("proxy exclusion %s (%s) matches nothing; drop the stale row", providerProxyExclusions[i].pattern, providerProxyExclusions[i].reason)
		}
	}
	// decentralizedIam holds both the verifier and APISIX.
	want := []string{"decentralizedIam", "keycloak", "marketplace", "vc-operator"}
	if got := slices.Sorted(maps.Keys(components)); !reflect.DeepEqual(got, want) {
		t.Errorf("the proxy reaches %v in provider.yaml, want %v", got, want)
	}
}

// renderOverProvider renders p over the loaded provider.yaml.
func renderOverProvider(t *testing.T, p profile.Profile) Tree {
	t.Helper()
	src, err := os.ReadFile(providerValues)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	root := roundtrip.Path{}
	loaded, err := load.Load(src, providerValues, bs, load.Options{Root: &root})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := Render(providerBundle(t), p, Options{Overrides: loaded})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// referenceCASecret is the cert-manager Secret whose ca.crt provider.yaml's hand-made CA mechanisms read.
const referenceCASecret = "mp-operations.org-tls"

// caExclusion is a CA leaf on which the fan-out deliberately differs from provider.yaml.
type caExclusion struct {
	pattern *regexp.Regexp
	reason  string
}

// providerCAExclusions hold because provider.yaml trusts its CA by hand-made mechanisms the tool does not own.
var providerCAExclusions = []caExclusion{
	{regexp.MustCompile(`^decentralizedIam\.odrlAuthorization\.apisix\.apisix\.ssl\.`),
		"US-3.6: k3s gives APISIX no trust store, because its routes set ssl_verify false"},
	{regexp.MustCompile(`^keycloak\.extra(Volumes|VolumeMounts|EnvVars)\[\d+\]\.`),
		"US-3.6: k3s gives Keycloak no outgoing trust store; the fan-out adds truststore-paths"},
	{regexp.MustCompile(`^marketplace\.bizEcosystemLogicProxy\.(extraContainerVolumes|extraVolumeMounts|additionalEnvVars)\[\d+\]\.`),
		"US-3.6: k3s turns the logic proxy's verification off instead, which the tool never writes"},
	{regexp.MustCompile(`^vc-operator\.(extraVolumes|extraVolumeMounts|extraEnvVars)\[\d+\]\.`),
		"US-3.6: k3s mounts the CA into /etc/ssl/certs; the fan-out uses its own mount and SSL_CERT_DIR"},
	{regexp.MustCompile(`^contract-management\.(deployment\.(initContainers|additionalVolumes|additionalVolumeMounts)|additionalEnvVars)\[\d+\]\.`),
		"US-3.6: k3s gives Contract Management no trust store, because it calls only in-cluster HTTP hosts"},
}

// TestProviderCA renders the private CA over provider.yaml. The verifier mechanism is withheld, because k3s
// already puts its own bundle (from the signing-key Secret) at /etc/ssl/cert.pem; the routes' loaded ssl_verify
// false stays pinned; every other owned CA leaf reproduces the file or is excluded with a reason.
func TestProviderCA(t *testing.T) {
	p, err := profile.ReadFile(providerProfile)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Answers[caSecretID]; ok {
		t.Fatalf("%s answers %s; provider.yaml trusts its CA by hand", providerProfile, caSecretID)
	}
	p.Answers[caSecretID] = referenceCASecret
	tree := renderOverProvider(t, p)
	want := []Withheld{{Reason: WithheldCollision, Mechanism: "verifier-ca", Element: "private-ca-bundle",
		List: verifierMounts, Field: "mountPath", Value: "/etc/ssl/cert.pem", ForeignKey: "root-ca"}}
	if !reflect.DeepEqual(tree.Withheld, want) {
		t.Errorf("withheld = %+v, want %+v", tree.Withheld, want)
	}
	doc := providerDoc(t)
	hits := make([]int, len(providerCAExclusions))
	routes := 0
	for _, l := range tree.Leaves {
		if l.Provenance.Foundation != caSecretID || l.Provenance.Origin != "" {
			continue
		}
		key := l.Path.String()
		if strings.HasSuffix(key, ".ssl_verify") {
			routes++
			if !l.Provenance.Pinned || l.Value != false {
				t.Errorf("%s = %v (pinned %v), want the loaded false pinned", key, l.Value, l.Provenance.Pinned)
			}
			continue
		}
		if n, err := doc.Lookup(l.Path); err == nil && n.Kind == yaml.ScalarNode && fmt.Sprint(l.Value) == n.Value {
			continue
		}
		i := slices.IndexFunc(providerCAExclusions, func(e caExclusion) bool { return e.pattern.MatchString(key) })
		if i < 0 {
			t.Errorf("%s = %v, which provider.yaml does not hold", key, l.Value)
			continue
		}
		hits[i]++
	}
	if routes == 0 {
		t.Error("no route ssl_verify rendered; provider.yaml has openid-connect routes")
	}
	for i, n := range hits {
		if n == 0 {
			t.Errorf("CA exclusion %s (%s) matches nothing; drop the stale row", providerCAExclusions[i].pattern, providerCAExclusions[i].reason)
		}
	}
}
