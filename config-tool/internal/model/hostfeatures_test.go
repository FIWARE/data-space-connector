package model

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	hostPrefix = "host."
	// apisixKey is the shared ingress of every host behind policy enforcement.
	apisixKey = "decentralizedIam.odrlAuthorization.apisix"
)

// ingressHost matches the target that names a host on its ingress.
var ingressHost = regexp.MustCompile(`\.hosts\[\d+\]\.host$`)

// apisixUpstreams is the feature that deploys the upstream of each host routed through APISIX (its nodes in
// k3s/provider.yaml); empty when no feature deploys it.
var apisixUpstreams = map[string]string{
	"host.dataService":           "data.scorpio",
	"host.tppDataService":        "data.scorpio",
	"host.consentDataService":    "data.scorpio",
	"host.tppService":            "access.verification",
	"host.tmfApi":                "contract.tmforum",
	"host.contractManagement":    "contract.management",
	"host.dsp":                   "dsp.edc",
	"host.dcp":                   "dsp.edc",
	"host.identityhub":           "dsp.edc",
	"host.identityhubManagement": "dsp.edc",
	"host.oid4vcManagement":      "dsp.edc",
	"host.dcpManagement":         "dsp.edc",
	"host.vault":                 "",
}

// under reports whether path p is key k or lies below it.
func under(t *testing.T, p, k string) bool {
	t.Helper()
	kp, err := roundtrip.ParsePath(k)
	if err != nil {
		t.Fatal(err)
	}
	ks := kp.String()
	return p == ks || strings.HasPrefix(p, ks+".") || strings.HasPrefix(p, ks+"[")
}

// TestShippedHostFeatures keeps each host's features equal to the features whose keys cover its ingress host,
// plus the upstream's feature of an APISIX host.
func TestShippedHostFeatures(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, b := range got {
		for _, decl := range b.Foundations {
			if !strings.HasPrefix(decl.ID, hostPrefix) {
				continue
			}
			f, _ := b.Foundation(decl.ID)
			var ingress []string
			for _, tg := range f.Targets {
				if p := tg.Path.String(); ingressHost.MatchString(p) {
					ingress = append(ingress, p)
				}
			}
			if len(ingress) != 1 {
				t.Errorf("%s %s: %s has ingress host targets %v, want exactly one", b.Chart, b.Version, f.ID, ingress)
				continue
			}
			var want []string
			for _, feat := range b.Features {
				if feat.Chart == b.Chart && slices.ContainsFunc(feat.Keys, func(k string) bool { return under(t, ingress[0], k) }) {
					want = append(want, feat.ID)
				}
			}
			if under(t, ingress[0], apisixKey) {
				upstream, ok := apisixUpstreams[f.ID]
				if !ok {
					t.Errorf("%s is routed through APISIX but has no apisixUpstreams row", f.ID)
				}
				seen[f.ID] = true
				if upstream != "" && !slices.Contains(want, upstream) {
					want = append(want, upstream)
				}
			}
			slices.Sort(want)
			have := slices.Sorted(slices.Values(f.Features))
			if !slices.Equal(have, want) {
				t.Errorf("%s %s: %s features = %v, want %v", b.Chart, b.Version, f.ID, have, want)
			}
		}
	}
	for id := range apisixUpstreams {
		if !seen[id] {
			t.Errorf("apisixUpstreams row %s names no host routed through APISIX; drop the stale row", id)
		}
	}
}
