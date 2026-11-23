package model

import (
	"errors"
	"os"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/model"
)

// trustAnchorFixture is the second chart's fixture bundle; the catalogue fixture's roles and features refer to it.
const trustAnchorFixture = "0.1.0/bundle.yaml"

// catalogueFS returns both fixture bundles, with old replaced by new in the catalogue fixture.
func catalogueFS(t *testing.T, old, new string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	for _, file := range []string{fixtureFile, trustAnchorFixture} {
		data, err := os.ReadFile(path.Join(fixtureDir, file))
		if err != nil {
			t.Fatal(err)
		}
		fsys[file] = &fstest.MapFile{Data: data}
	}
	if old == "" {
		return fsys
	}
	src := string(fsys[fixtureFile].Data)
	if !strings.Contains(src, old) {
		t.Fatalf("fixture does not contain %q", old)
	}
	fsys[fixtureFile] = &fstest.MapFile{Data: []byte(strings.Replace(src, old, new, 1))}
	return fsys
}

func TestCatalogue(t *testing.T) {
	got, err := Versions(catalogueFS(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	b := got[slices.IndexFunc(got, func(b Bundle) bool { return b.Chart == chart })]
	charts := map[string]string{}
	for _, f := range b.Features {
		charts[f.ID] = f.Chart
	}
	if charts["identity.did"] != chart || charts["ta.registry"] != trustAnchor {
		t.Errorf("feature charts = %v, want %s by default and %s where set", charts, chart, trustAnchor)
	}
	ids := make([]string, len(b.Roles))
	for i, r := range b.Roles {
		ids[i] = r.ID
	}
	if !slices.Equal(ids, KnownRoles) {
		t.Errorf("roles = %v, want %v", ids, KnownRoles)
	}
	if b.Roles[4].Floor != nil || *b.Roles[0].Floor != (Floor{Memory: "0.8Gi", CPU: "50m", MeasuredWith: "10.13.2"}) {
		t.Errorf("floors = %+v / %+v, want consumer's and none for custom", b.Roles[0].Floor, b.Roles[4].Floor)
	}
	want := &RoleMatrix{
		Components:     map[string]string{"Keycloak": "identity.issuer", "DID": "identity.did", "Trusted Issuers Registry": "ta.registry"},
		Infrastructure: []string{"PostgreSQL"},
	}
	if !reflect.DeepEqual(b.RoleMatrix, want) {
		t.Errorf("roleMatrix = %+v, want %+v", b.RoleMatrix, want)
	}
}

func TestCatalogueValidation(t *testing.T) {
	const (
		didFeature   = "{id: identity.did, title: DID, summary: Publish a DID, group: identity, keys: [did]}"
		consumerOffs = "{identity.issuer: recommended, identity.did: recommended, data.scorpio: not-offered,"
		customOffs   = "{identity.issuer: available, identity.did: available,"
		taFeature    = "group: operator, chart: trust-anchor, keys: [trusted-issuers-list]}"
		head         = "chart: " + chart + "\nversion: 1.0.0\nrevision: 1\n"
		oneFeature   = "features:\n  - {id: a, title: A, summary: A, group: data}\n"
	)
	tests := []struct {
		name, old, new string
		wantErr        string
	}{
		// features
		{"feature id missing", didFeature, "{title: DID, summary: s, group: identity}", "features[1]: id is missing"},
		{"feature id repeated", "{id: identity.did,", "{id: identity.issuer,", `features[1]: id "identity.issuer" is already used by features[0]`},
		{"feature title missing", "title: DID, ", "", "features[1] (identity.did): title is missing"},
		{"feature summary missing", "summary: Publish a DID, ", "", "features[1] (identity.did): summary is missing"},
		{"feature group missing", "group: identity, keys: [did]", "keys: [did]", "features[1] (identity.did): group is missing"},
		{"feature group unknown", "group: identity, keys: [did]", "group: misc, keys: [did]", `features[1] (identity.did): group "misc" is unknown`},
		{"requires unknown", "requires: [identity.did]", "requires: [identity.nope]", `features[2] (data.scorpio): requires "identity.nope", which is not in the catalogue`},
		{"requires itself", "requires: [identity.did]", "requires: [data.scorpio]", "features[2] (data.scorpio): requires itself"},
		{"key malformed", "keys: [did]", "keys: ['did..x']", "features[1] (identity.did): keys[0]: roundtrip: invalid path"},
		{"key empty", "keys: [did]", "keys: ['']", "features[1] (identity.did): keys[0] is empty"},
		{"key with wildcard", "keys: [did]", "keys: [did.*]", `features[1] (identity.did): keys[0]: "did.*" holds "*"`},
		{"key repeated", "keys: [did]", "keys: [did, did]", `features[1] (identity.did): keys[1]: "did" is already listed at keys[0]`},
		{"key under an earlier key", "keys: [did]", "keys: [did, did.port]", `features[1] (identity.did): keys[1]: "did.port" is under keys[0] "did"`},
		{"key above a later key", "keys: [did]", "keys: [did.port, did]", `features[1] (identity.did): keys[1]: keys[0] "did.port" is under "did"`},
		{"key outside topLevelKeys", "keys: [did]", "keys: [vault]", `1.0.0/bundle.yaml: features[1] (identity.did): keys[0]: "vault" is not in topLevelKeys of data-space-connector 1.0.0`},
		{"key outside the other chart's topLevelKeys", "keys: [trusted-issuers-list]", "keys: [did]", `features[6] (ta.registry): keys[0]: "did" is not in topLevelKeys of trust-anchor 0.1.0`},
		{"feature chart not shipped", "", head + "features:\n  - {id: a, title: A, summary: A, group: data, chart: nowhere}\n", `features[0] (a): chart "nowhere" has no shipped bundle`},
		{"unknown feature field", "group: identity, keys: [did]", "group: identity, key: [did]", "field key not found"},
		// roles
		{"roles out of order", "  - id: consumer\n", "  - id: provider\n", "roles are [provider provider consumer-provider operator custom], want [consumer provider consumer-provider operator custom]"},
		{"role unknown", "  - id: custom\n", "  - id: everything\n", "roles are [consumer provider consumer-provider operator everything]"},
		{"role title missing", "    title: Consumer\n", "", "roles[0] (consumer): title is missing"},
		{"role summary missing", "    summary: Consume data\n", "", "roles[0] (consumer): summary is missing"},
		{"role chart missing", "Consume data\n    chart: data-space-connector\n", "Consume data\n", "roles[0] (consumer): chart is missing"},
		{"role docs missing", "    docs: roles/consumer/README.md\n", "", "roles[0] (consumer): docs is missing"},
		{"floor memory unit", "memory: 0.8Gi", "memory: 0.8G", `roles[0] (consumer): floor.memory "0.8G" does not match`},
		{"floor memory missing", "memory: 0.8Gi, ", "", `roles[0] (consumer): floor.memory "" does not match`},
		{"floor cpu in cores", "cpu: 50m", "cpu: 0.05", `roles[0] (consumer): floor.cpu "0.05" does not match`},
		{"floor measuredWith not semver", "measuredWith: 10.13.2}", "measuredWith: '10.13'}", `roles[0] (consumer): floor.measuredWith "10.13" is not semver`},
		{"preset names an unknown feature", consumerOffs, "{identity.nope: available, identity.issuer: recommended, identity.did: recommended, data.scorpio: not-offered,", `roles[0] (consumer): feature "identity.nope" is not in the catalogue`},
		{"preset offer unknown", consumerOffs, "{identity.issuer: maybe, identity.did: recommended, data.scorpio: not-offered,", `roles[0] (consumer): feature identity.issuer: offer "maybe" is unknown`},
		{"preset misses a feature", consumerOffs, "{identity.issuer: recommended, identity.did: recommended,", "roles[0] (consumer): feature data.scorpio is missing"},
		{"preset offers another chart's feature", "obs.metrics: available, ta.registry: not-offered}", "obs.metrics: available, ta.registry: available}", "roles[0] (consumer): offers ta.registry of chart trust-anchor, want only features of chart data-space-connector"},
		{"operator offers a dsc feature", "{identity.issuer: not-offered,", "{identity.issuer: available,", "roles[3] (operator): offers identity.issuer of chart data-space-connector, want only features of chart trust-anchor"},
		{"custom recommends", customOffs, "{identity.issuer: recommended, identity.did: available,", "roles[4] (custom): recommends identity.issuer; custom must recommend nothing"},
		{"recommended requires available", "identity.did: recommended, data.scorpio: recommended", "identity.did: available, data.scorpio: recommended", "roles[1] (provider): recommends data.scorpio but not its requirement identity.did"},
		{"available requires not offered", customOffs, "{identity.issuer: available, identity.did: not-offered,", "roles[4] (custom): offers data.scorpio but not its requirement identity.did"},
		{"roles without features", "", head + "roles: [{id: consumer}]\n", "roles need features"},
		{"missing role", "", head + oneFeature + "roles: [{id: consumer}]\n", "roles are [consumer], want"},
		// roleMatrix
		{"matrix names an unknown feature", "DID: identity.did", "DID: identity.nope", `roleMatrix.components["DID"]: "identity.nope" is not in the catalogue`},
		{"matrix row is a component and infrastructure", "infrastructure: [PostgreSQL]", "infrastructure: [PostgreSQL, DID]", `roleMatrix.infrastructure[1]: "DID" is also a component`},
		{"matrix without roles", "", head + oneFeature + "roleMatrix: {infrastructure: [PostgreSQL]}\n", "roleMatrix needs roles"},
		// recognize rules at stage features
		{"feature rule names no catalogue id", "answer: dsp.rainbow.enabled", "answer: dsp.edc.enabled", `recognize[2]: answer "dsp.edc.enabled" is not <catalogue id>.enabled`},
		{"feature rule answer without suffix", "answer: dsp.rainbow.enabled", "answer: dsp.rainbow", `recognize[2]: answer "dsp.rainbow" is not <catalogue id>.enabled`},
		{"feature rule is a value", "answer: dsp.rainbow.enabled, stage: features, kind: toggle", "answer: dsp.rainbow.enabled, stage: features, kind: value", "recognize[2]: kind value at stage features, want toggle"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := catalogueFS(t, tc.old, tc.new)
			if tc.old == "" {
				fsys[fixtureFile] = &fstest.MapFile{Data: []byte(tc.new)}
			}
			_, err := Versions(fsys)
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to name %s and mention %q", err, fixtureFile, tc.wantErr)
			}
		})
	}
}

func TestFeatureConsequence(t *testing.T) {
	const old = "summary: Publish a DID, "
	tests := []struct {
		name, consequence string
		want, wantErr     string
	}{
		{"decoded", "consequence: No DID document, ", "No DID document", ""},
		{"empty is accepted", "", "", ""},
		{"blank is refused", "consequence: ' ', ", "", "features[1] (identity.did): consequence is blank"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Versions(catalogueFS(t, old, old+tc.consequence))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b := got[slices.IndexFunc(got, func(b Bundle) bool { return b.Chart == chart })]
			if c := b.Features[1].Consequence; c != tc.want {
				t.Errorf("consequence = %q, want %q", c, tc.want)
			}
		})
	}
}

// TestShippedRecommendedHaveConsequence keeps a consequence on every feature a shipped role recommends.
func TestShippedRecommendedHaveConsequence(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got {
		recommended := map[string]bool{}
		for _, r := range b.Roles {
			for id, offer := range r.Features {
				if offer == OfferRecommended {
					recommended[id] = true
				}
			}
		}
		for _, f := range b.Features {
			if recommended[f.ID] && strings.TrimSpace(f.Consequence) == "" {
				t.Errorf("%s %s: feature %s is recommended but has no consequence", b.Chart, b.Version, f.ID)
			}
		}
	}
}

func TestCatalogueRoleChartNotShipped(t *testing.T) {
	fsys := catalogueFS(t, "", "")
	delete(fsys, trustAnchorFixture)
	const want = fixtureFile + `: roles[3] (operator): chart "trust-anchor" has no shipped bundle`
	if _, err := Versions(fsys); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to mention %q", err, want)
	}
}

func TestCatalogueSharedKeys(t *testing.T) {
	if _, err := Versions(catalogueFS(t, "keys: [did]", "keys: [did, keycloak.realm]")); err != nil {
		t.Errorf("a key shared with another feature must be valid: %v", err)
	}
}

func TestRoleCatalogue(t *testing.T) {
	roles := []Role{{ID: RoleConsumer}}
	dsc := Bundle{Chart: chart, Version: "10.16.0", Latest: true, Roles: roles}
	dscOld := Bundle{Chart: chart, Version: "10.15.0", Roles: roles}
	dscBare := Bundle{Chart: chart, Version: "10.16.0", Latest: true}
	rc := Bundle{Chart: chart, Version: "10.17.0-rc1", Roles: roles}
	ta := Bundle{Chart: trustAnchor, Version: "1.3.6", Latest: true}
	taRoles := Bundle{Chart: trustAnchor, Version: "1.3.6", Latest: true, Roles: roles}
	tests := []struct {
		name           string
		bundles        []Bundle
		chart, version string
		want           Bundle
		wantErr        error
	}{
		{"own bundle", []Bundle{dsc, dscOld, ta}, chart, "10.15.0", dscOld, nil},
		{"empty version is the chart's latest", []Bundle{dsc, dscOld, ta}, chart, "", dsc, nil},
		{"fallback to the latest bundle with roles", []Bundle{dsc, dscOld, ta}, trustAnchor, "1.3.6", dsc, nil},
		{"fallback for an empty version", []Bundle{dsc, dscOld, ta}, trustAnchor, "", dsc, nil},
		{"unknown version falls back", []Bundle{dsc, ta}, chart, "9.0.0", dsc, nil},
		{"a newer rc is not the fallback", []Bundle{rc, dsc, ta}, trustAnchor, "1.3.6", dsc, nil},
		{"the rc is the fallback when no latest has roles", []Bundle{rc, dscBare, ta}, trustAnchor, "1.3.6", rc, nil},
		{"the highest latest with roles wins", []Bundle{taRoles, dsc}, "other", "1.0.0", dsc, nil},
		{"no bundle declares roles", []Bundle{dscBare, ta}, trustAnchor, "1.3.6", Bundle{}, ErrNoRoles},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RoleCatalogue(tc.bundles, tc.chart, tc.version)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RoleCatalogue = %s %s, want %s %s", got.Chart, got.Version, tc.want.Chart, tc.want.Version)
			}
		})
	}
}
