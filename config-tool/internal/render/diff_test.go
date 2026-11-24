package render

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
)

const (
	otherFixtureDomain = "other.org"
	// handEdit is a loaded rainbow host that differs from the one derived from the recognized domain.
	handEdit       = "rainbow.hand.org"
	handEditedFile = "keycloak:\n  ingress:\n    hostname: keycloak." + domain + "\nrainbow:\n  ingress:\n    host: " + handEdit + "\n"
)

func TestTargetsFollowDefaultRefsOnly(t *testing.T) {
	b := modelFixture(t, prefixFixture)
	tests := []struct {
		answer string
		want   []string
	}{
		{answer: "tag.code", want: []string{didPath}},
		{answer: "tag.scheme"},
	}
	for _, tc := range tests {
		t.Run(tc.answer, func(t *testing.T) {
			var paths []string
			for _, r := range Targets(b, tc.answer) {
				paths = append(paths, r.Path.String())
			}
			if !slices.Equal(paths, tc.want) {
				t.Errorf("targets = %v, want %v", paths, tc.want)
			}
		})
	}
}

func TestTargets(t *testing.T) {
	tests := []struct {
		answer         string
		wantPaths      []string
		wantComponents []string
	}{
		{
			answer: "domain.base",
			wantPaths: []string{
				`keycloak.ingress.annotations."cert-manager.io/alt-names"`, "keycloak.ingress.hostname",
				"keycloak.ingress.tls[0].hosts[0]", "keycloak.ingress.tls[0].secretName",
				"marketplace.url", "rainbow.ingress.host", "scorpio.ingress.hosts[0].host",
			},
			wantComponents: []string{"keycloak", "keycloak", "keycloak", "keycloak", "marketplace", "rainbow", "scorpio"},
		},
		{
			answer:         "host.rainbow",
			wantPaths:      []string{`keycloak.ingress.annotations."cert-manager.io/alt-names"`, "rainbow.ingress.host"},
			wantComponents: []string{"keycloak", "rainbow"},
		},
		{
			answer:         "host.scorpioAlt",
			wantPaths:      []string{"scorpio.ingress.hosts[1].host"},
			wantComponents: []string{"scorpio"},
		},
		{answer: "org.displayName", wantPaths: []string{"marketplace.title"}, wantComponents: []string{"marketplace"}},
		{answer: "host.unknown"},
	}
	b := fixtureBundle(t)
	for _, tc := range tests {
		t.Run(tc.answer, func(t *testing.T) {
			var paths, components []string
			for _, r := range Targets(b, tc.answer) {
				paths = append(paths, r.Path.String())
				components = append(components, r.Component)
			}
			if !slices.Equal(paths, tc.wantPaths) || !slices.Equal(components, tc.wantComponents) {
				t.Errorf("targets = %v in %v, want %v in %v", paths, components, tc.wantPaths, tc.wantComponents)
			}
		})
	}
}

func TestTargetRefJSON(t *testing.T) {
	got, err := json.Marshal(Targets(fixtureBundle(t), "host.scorpioAlt"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"path":"scorpio.ingress.hosts[1].host","component":"scorpio"}]`; string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

// change is the comparable form of a Change in the diff tests.
type change struct {
	foundation, path string
	status           Status
	before, after    any
	kept             any
	hasKept          bool
}

func flatten(d TreeDiff) []change {
	cs := []change{}
	for _, g := range d.Groups {
		for _, c := range g.Changes {
			cs = append(cs, change{g.Foundation, c.Path.String(), c.Status, c.Before, c.After, c.Kept, c.HasKept})
		}
	}
	return cs
}

func TestDiff(t *testing.T) {
	answers := func(base string) map[string]any {
		return map[string]any{"org.displayName": displayName, "domain.base": base, "host.scorpioAlt": scorpioAlt}
	}
	withDomain := answers(domain)
	tests := []struct {
		name       string
		prev, next map[string]any
		want       []change
	}{
		{
			name: "first render adds every leaf",
			prev: nil,
			next: map[string]any{"org.displayName": displayName},
			want: []change{{foundation: "org.displayName", path: "marketplace.title", status: StatusAdded, after: displayName}},
		},
		{
			name: "nothing changed",
			prev: withDomain, next: withDomain,
			want: []change{},
		},
		{
			name: "a domain change changes its fan-out, grouped in bundle order",
			prev: withDomain, next: answers(otherFixtureDomain),
			want: []change{
				{"domain.base", "marketplace.url", StatusChanged, "https://market." + domain, "https://market." + otherFixtureDomain, nil, false},
				{"host.keycloak", `keycloak.ingress.annotations."cert-manager.io/alt-names"`, StatusChanged,
					"keycloak." + domain + ",rainbow." + domain, "keycloak." + otherFixtureDomain + ",rainbow." + otherFixtureDomain, nil, false},
				{"host.keycloak", "keycloak.ingress.hostname", StatusChanged, "keycloak." + domain, "keycloak." + otherFixtureDomain, nil, false},
				{"host.keycloak", "keycloak.ingress.tls[0].hosts[0]", StatusChanged, "keycloak." + domain, "keycloak." + otherFixtureDomain, nil, false},
				{"host.keycloak", "keycloak.ingress.tls[0].secretName", StatusChanged, "keycloak." + domain + "-tls", "keycloak." + otherFixtureDomain + "-tls", nil, false},
				{"host.rainbow", "rainbow.ingress.host", StatusChanged, "rainbow." + domain, "rainbow." + otherFixtureDomain, nil, false},
				{"host.scorpio", "scorpio.ingress.hosts[0].host", StatusChanged, "scorpio." + domain, "scorpio." + otherFixtureDomain, nil, false},
			},
		},
		{
			name: "an unanswered host removes its owned list whole",
			prev: withDomain, next: map[string]any{"org.displayName": displayName, "domain.base": domain},
			want: []change{
				{"host.scorpio", "scorpio.ingress.hosts[0].host", StatusRemoved, "scorpio." + domain, nil, nil, false},
				{"host.scorpio", "scorpio.ingress.hosts[0].paths[0]", StatusRemoved, "/", nil, nil, false},
				{"host.scorpioAlt", "scorpio.ingress.hosts[1].host", StatusRemoved, scorpioAlt, nil, nil, false},
				{"host.scorpioAlt", "scorpio.ingress.hosts[1].paths[0]", StatusRemoved, "/", nil, nil, false},
			},
		},
	}
	b := fixtureBundle(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var prev Tree
			if tc.prev != nil {
				prev = renderFixture(t, tc.prev, Options{})
			}
			got := flatten(Diff(b, prev, renderFixture(t, tc.next, Options{})))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("diff =\n%v\nwant\n%v", got, tc.want)
			}
		})
	}
}

func TestDiffPinned(t *testing.T) {
	b := fixtureBundle(t)
	r, err := load.Load([]byte(handEditedFile), "values.yaml", fixtureBundles(t), load.Options{})
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Overrides: r}
	before := renderFixture(t, map[string]any{"domain.base": domain}, opts)
	after := renderFixture(t, map[string]any{"domain.base": otherFixtureDomain}, opts)

	pinned := func(d TreeDiff) []change {
		return slices.DeleteFunc(flatten(d), func(c change) bool { return c.status != StatusPinned })
	}
	want := []change{{"host.rainbow", "rainbow.ingress.host", StatusPinned, "rainbow." + domain, "rainbow." + otherFixtureDomain, handEdit, true}}
	if got := pinned(Diff(b, before, after)); !reflect.DeepEqual(got, want) {
		t.Errorf("domain change: pinned = %v, want %v", got, want)
	}
	if got := flatten(Diff(b, after, after)); len(got) != 0 {
		t.Errorf("unchanged render: diff = %v, want none", got)
	}
	first := pinned(Diff(b, Tree{}, before))
	if len(first) != 1 || first[0].before != nil || first[0].after != "rainbow."+domain || first[0].kept != handEdit {
		t.Errorf("first render: pinned = %v, want the hand edit with no before", first)
	}
}

func TestChangeJSON(t *testing.T) {
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{"added has no before", Change{Path: mustPath(t, "a.b"), Status: StatusAdded, After: "x", HasAfter: true},
			`{"path":"a.b","status":"added","after":"x"}`},
		{"removed has no after", Change{Path: mustPath(t, "a.b"), Status: StatusRemoved, Before: "x", HasBefore: true},
			`{"path":"a.b","status":"removed","before":"x"}`},
		{"a kept null is written", Change{Path: mustPath(t, "a[0]"), Status: StatusPinned, Before: "x", HasBefore: true, After: "y", HasAfter: true, HasKept: true},
			`{"path":"a[0]","status":"pinned","before":"x","after":"y","kept":null}`},
		{"a pin without a kept value has no kept", Change{Path: mustPath(t, "a"), Status: StatusPinned, After: "y", HasAfter: true},
			`{"path":"a","status":"pinned","after":"y"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.change)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("JSON = %s, want %s", got, tc.want)
			}
		})
	}
	if got, _ := json.Marshal(Diff(fixtureBundle(t), Tree{}, Tree{})); string(got) != `{"groups":[]}` {
		t.Errorf("empty diff JSON = %s", got)
	}
}

func TestProviderTargetsAndDiff(t *testing.T) {
	b := providerBundle(t)
	want := domainLeaves(t, providerDoc(t))
	var targets []string
	for _, r := range Targets(b, domainBase) {
		targets = append(targets, r.Path.String())
	}
	sort.Strings(targets)
	if missing, extra := diffSets(want, targets); len(missing)+len(extra) > 0 {
		t.Errorf("Targets(%s): missing %v, extra %v", domainBase, missing, extra)
	}
	prev := renderProvider(t, b, map[string]any{orgDisplayName: orgName, domainBase: referenceDomain})
	next := renderProvider(t, b, map[string]any{orgDisplayName: orgName, domainBase: otherDomain})
	var changed []string
	for _, c := range flatten(Diff(b, prev, next)) {
		if c.status != StatusChanged {
			t.Errorf("%s: status %s, want %s", c.path, c.status, StatusChanged)
		}
		changed = append(changed, c.path)
	}
	sort.Strings(changed)
	if missing, extra := diffSets(want, changed); len(missing)+len(extra) > 0 {
		t.Errorf("diff of a domain change: missing %v, extra %v", missing, extra)
	}
}
