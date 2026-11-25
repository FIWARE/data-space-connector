package render

import (
	"os"
	"reflect"
	"slices"
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
	hostPrefix = "host."
	// domainRef is how a host's default names domain.base.
	domainRef       = "{{ .domain.base }}"
	pinnedHost      = "pinned.acme.com"
	hostKeycloakID  = "host.keycloak"
	trustAnchor     = "trust-anchor"
	trustAnchorVer  = "1.3.6"
	verifierIngress = "decentralizedIam.vcAuthentication.vcverifier.ingress"
	didIngress      = "did.ingress"
	// The leaves of the certificates US-8.11 owns; a host change must not write them.
	tlsSegment         = ".tls"
	annotationsSegment = ".annotations"
	verifierSecretKey  = verifierIngress + ".tls[0].secretName"
	verifierCNKey      = verifierIngress + `.annotations."cert-manager.io/common-name"`
	verifierHostKey    = verifierIngress + ".hosts[0].host"
	didSecretKey       = didIngress + ".tls[0].secretName"
	didCNKey           = didIngress + `.annotations."cert-manager.io/common-name"`
	didHostKey         = didIngress + ".hosts[0].host"
	siopVerifierKey    = "marketplace.siop.verifier.host"
	tlsSecretSuffix    = "-tls"
	httpsScheme        = "https://"
	// certificateNote ends an owned marker's scope above a certificate key, which US-8.11 owns.
	certificateNote   = "# certificate (US-8.11)"
	markedToolVersion = "test"
)

// markedProvider is provider.yaml as dsc-config generates it: header plus owned markers on the host leaves only,
// so every other leaf, the certificates included, stays a hand edit.
func markedProvider(t *testing.T, src []byte) []byte {
	t.Helper()
	marker := func(indent string, id string) string { return indent + roundtrip.MarkerPrefix + id + "\n" }
	edits := []struct{ anchor, insert string }{
		{"        hosts:\n          - host: verifier.", marker("        ", hostVerifierID)},
		{"    hosts:\n      - host: mp-operations.org", marker("    ", hostDIDID)},
		{"    tls:\n      - secretName: mp-operations.org-tls", "    " + certificateNote + "\n"},
		{"      host: https://verifier.mp-operations.org\n    allowedRoles:", marker("      ", hostVerifierID)},
	}
	out := string(src)
	for _, e := range edits {
		if strings.Count(out, e.anchor) != 1 {
			t.Fatalf("%q does not occur exactly once in %s", e.anchor, providerValues)
		}
		out = strings.Replace(out, e.anchor, e.insert+e.anchor, 1)
	}
	return append(roundtrip.Header{ToolVersion: markedToolVersion, ChartVersion: providerVersion}.Render(), out...)
}

// onModel is a profile of answers on the bundle chart/version.
func onModel(chart, version string, answers map[string]any) profile.Profile {
	return profile.Profile{Model: profile.ModelRef{Chart: chart, Version: version}, Answers: answers}
}

func onProvider(answers map[string]any) profile.Profile {
	return onModel(providerChart, providerVersion, answers)
}

// shippedHosts returns the derived host foundations of b in FoundationOrder.
func shippedHosts(t *testing.T, b model.Bundle) []model.CompiledFoundation {
	t.Helper()
	var hosts []model.CompiledFoundation
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		if strings.HasPrefix(id, hostPrefix) && f.DefaultTemplate() != nil && len(f.DefaultRefs()) > 0 {
			hosts = append(hosts, f)
		}
	}
	for _, id := range []string{hostVerifierID, hostDIDID, hostKeycloakID} {
		if !slices.ContainsFunc(hosts, func(f model.CompiledFoundation) bool { return f.ID == id }) {
			t.Fatalf("no derived %s among the shipped hosts", id)
		}
	}
	return hosts
}

// hostOn is what host f of b derives to on base: <component>.<base>, or base itself for the DID document.
func hostOn(t *testing.T, b model.Bundle, f model.CompiledFoundation, base string) string {
	t.Helper()
	i := slices.IndexFunc(b.Foundations, func(d model.Foundation) bool { return d.ID == f.ID })
	if i < 0 || !strings.Contains(b.Foundations[i].Default, domainRef) || !slices.Equal(f.DefaultRefs(), []string{domainBase}) {
		t.Fatalf("%s does not derive from %s alone", f.ID, domainBase)
	}
	return strings.ReplaceAll(b.Foundations[i].Default, domainRef, base)
}

func foundationLeaves(tree Tree, id string) map[string]any {
	m := map[string]any{}
	for _, l := range tree.Leaves {
		if l.Provenance.Foundation == id {
			m[l.Path.String()] = l.Value
		}
	}
	return m
}

// TestDerivedDiffPinsEachHost pins each shipped host in turn and moves the domain: only that host is kept.
func TestDerivedDiffPinsEachHost(t *testing.T) {
	b := providerBundle(t)
	for _, pin := range shippedHosts(t, b) {
		t.Run(pin.ID, func(t *testing.T) {
			prev := map[string]any{domainBase: referenceDomain, pin.ID: pinnedHost}
			next := map[string]any{domainBase: otherDomain, pin.ID: pinnedHost}
			want := AnswerDiff{Rederived: []DerivedChange{}, Pinned: []PinnedAnswer{{
				ID: pin.ID, Kept: pinnedHost, Before: hostOn(t, b, pin, referenceDomain), After: hostOn(t, b, pin, otherDomain),
			}}}
			for _, id := range b.FoundationOrder() {
				f, _ := b.Foundation(id)
				switch {
				case id == pin.ID:
				case id == didValueID && pin.ID != hostDIDID:
					want.Rederived = append(want.Rederived, DerivedChange{ID: id,
						Before: didWebMethod + ":" + referenceDomain, After: didWebMethod + ":" + otherDomain})
				case strings.HasPrefix(id, hostPrefix) && f.DefaultTemplate() != nil:
					want.Rederived = append(want.Rederived, DerivedChange{ID: id,
						Before: hostOn(t, b, f, referenceDomain), After: hostOn(t, b, f, otherDomain)})
				}
			}
			got, err := DerivedDiff(b, onProvider(prev), onProvider(next))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("DerivedDiff = %+v\nwant %+v", got, want)
			}
			before, after := foundationLeaves(renderProvider(t, b, prev), pin.ID), foundationLeaves(renderProvider(t, b, next), pin.ID)
			if len(after) == 0 || !reflect.DeepEqual(before, after) {
				t.Errorf("leaves of pinned %s moved with the domain: %v, then %v", pin.ID, before, after)
			}
		})
	}
}

func TestDerivedDiff(t *testing.T) {
	const handSSO = "sso.acme.com"
	b := providerBundle(t)
	ta, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	trust := ta[slices.IndexFunc(ta, func(b model.Bundle) bool { return b.Chart == trustAnchor && b.Version == trustAnchorVer })]
	empty := AnswerDiff{Rederived: []DerivedChange{}, Pinned: []PinnedAnswer{}}
	tests := []struct {
		name       string
		bundle     model.Bundle
		prev, next profile.Profile
		want       AnswerDiff
	}{
		{name: "an unchanged domain moves nothing", bundle: b,
			prev: onProvider(map[string]any{domainBase: referenceDomain, hostKeycloakID: handSSO}),
			next: onProvider(map[string]any{domainBase: referenceDomain, hostKeycloakID: handSSO}), want: empty},
		{name: "reset to derived is a re-derivation", bundle: b,
			prev: onProvider(map[string]any{domainBase: otherDomain, hostKeycloakID: handSSO}),
			next: onProvider(map[string]any{domainBase: otherDomain}),
			want: AnswerDiff{Rederived: []DerivedChange{{ID: hostKeycloakID, Before: handSSO, After: "keycloak." + otherDomain}},
				Pinned: []PinnedAnswer{}}},
		{name: "pinning without a domain change is no pin of a moved value", bundle: b,
			prev: onProvider(map[string]any{domainBase: otherDomain}),
			next: onProvider(map[string]any{domainBase: otherDomain, hostKeycloakID: handSSO}),
			want: empty},
		{name: "a prev on another chart is no baseline", bundle: trust,
			prev: onProvider(map[string]any{domainBase: referenceDomain, hostKeycloakID: handSSO, hostDIDID: pinnedHost}),
			next: onModel(trustAnchor, trustAnchorVer, map[string]any{domainBase: otherDomain}), want: empty},
		{name: "a prev on another version is no baseline", bundle: b,
			prev: onModel(providerChart, trustAnchorVer, map[string]any{domainBase: referenceDomain}),
			next: onProvider(map[string]any{domainBase: otherDomain}), want: empty},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DerivedDiff(tc.bundle, tc.prev, tc.next)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DerivedDiff = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestDerivedDiffDID covers the chain domain.base → host.did → did.value under a domain change.
func TestDerivedDiffDID(t *testing.T) {
	const handDID = "did:web:id.acme.com"
	b := providerBundle(t)
	oldDIDWeb, newDIDWeb := didWebMethod+":"+referenceDomain, didWebMethod+":"+otherDomain
	tests := []struct {
		name          string
		pinned        map[string]any
		wantPinned    []PinnedAnswer
		wantRederived []string
		wantNeither   []string
	}{
		{name: "a derived DID follows its host", pinned: map[string]any{}, wantPinned: []PinnedAnswer{},
			wantRederived: []string{hostDIDID, didValueID}},
		{name: "a pinned DID stays while its host moves", pinned: map[string]any{didValueID: handDID},
			wantPinned:    []PinnedAnswer{{ID: didValueID, Kept: handDID, Before: oldDIDWeb, After: newDIDWeb}},
			wantRederived: []string{hostDIDID}, wantNeither: []string{}},
		{name: "a derived DID follows a pinned host", pinned: map[string]any{hostDIDID: pinnedHost},
			wantPinned:  []PinnedAnswer{{ID: hostDIDID, Kept: pinnedHost, Before: referenceDomain, After: otherDomain}},
			wantNeither: []string{didValueID}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := merge(map[string]any{domainBase: referenceDomain}, tc.pinned)
			next := merge(map[string]any{domainBase: otherDomain}, tc.pinned)
			got, err := DerivedDiff(b, onProvider(prev), onProvider(next))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Pinned, tc.wantPinned) {
				t.Errorf("Pinned = %+v, want %+v", got.Pinned, tc.wantPinned)
			}
			rederived := map[string]bool{}
			for _, c := range got.Rederived {
				rederived[c.ID] = true
			}
			for _, id := range tc.wantRederived {
				if !rederived[id] {
					t.Errorf("%s is not re-derived: %+v", id, got.Rederived)
				}
			}
			for _, id := range tc.wantNeither {
				if rederived[id] || slices.ContainsFunc(got.Pinned, func(p PinnedAnswer) bool { return p.ID == id }) {
					t.Errorf("%s is listed, but follows a pinned host: %+v", id, got)
				}
			}
		})
	}
}

func TestDerivedDiffErrors(t *testing.T) {
	b := providerBundle(t)
	undeclared := map[string]any{"nope": "x"}
	for name, pair := range map[string][2]map[string]any{
		"prev": {undeclared, {domainBase: domain}},
		"next": {{domainBase: domain}, undeclared},
	} {
		if _, err := DerivedDiff(b, onProvider(pair[0]), onProvider(pair[1])); err == nil {
			t.Errorf("DerivedDiff accepted an undeclared answer in %s", name)
		}
	}
}

// TestHostChangeKeepsLoadedCertificates renders host changes over a generated provider.yaml: the ingress host follows,
// while the TLS secret and its cert-manager annotations, which other mounts reference, stay as loaded (US-8.11).
func TestHostChangeKeepsLoadedCertificates(t *testing.T) {
	const handVerifier, handDID = "verify.acme.com", "id.acme.com"
	b := providerBundle(t)
	plain, err := os.ReadFile(providerValues)
	if err != nil {
		t.Fatal(err)
	}
	src := markedProvider(t, plain)
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	root := roundtrip.Path{}
	loaded, err := load.Load(src, providerValues, bs, load.Options{Root: &root})
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]string{
		verifierSecretKey: referenceVerifierHost + tlsSecretSuffix,
		verifierCNKey:     referenceVerifierHost,
		didSecretKey:      referenceDIDHost + tlsSecretSuffix,
		didCNKey:          referenceDIDHost,
	}
	tests := []struct {
		name                  string
		answers               map[string]any
		verifierHost, didHost string
	}{
		{"pinned verifier", map[string]any{domainBase: otherDomain, hostVerifierID: handVerifier}, handVerifier, otherDomain},
		{"pinned DID host", map[string]any{domainBase: otherDomain, hostDIDID: handDID}, "verifier." + otherDomain, handDID},
		{"domain change", map[string]any{domainBase: otherDomain}, "verifier." + otherDomain, otherDomain},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := Render(b, profile.Profile{Answers: tc.answers}, Options{Overrides: loaded})
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range tree.Leaves {
				p := l.Path.String()
				for _, ingress := range []string{verifierIngress, didIngress} {
					if strings.HasPrefix(p, ingress+tlsSegment) || strings.HasPrefix(p, ingress+annotationsSegment) {
						t.Errorf("a host change writes certificate leaf %s", p)
					}
				}
			}
			merged := map[string]any{}
			if err := yaml.Unmarshal(src, &merged); err != nil {
				t.Fatal(err)
			}
			for _, l := range nestedLeaves(tree) {
				merged = insert(merged, l.Path, l.Value).(map[string]any)
			}
			want := map[string]string{
				verifierHostKey: tc.verifierHost,
				siopVerifierKey: httpsScheme + tc.verifierHost,
				didHostKey:      tc.didHost,
			}
			for k, v := range kept {
				want[k] = v
			}
			for k, v := range want {
				if got := subtree(merged, mustPath(t, k)); got != v {
					t.Errorf("%s = %v, want %s", k, got, v)
				}
			}
		})
	}
}
