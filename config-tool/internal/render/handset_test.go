package render

import (
	"reflect"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

func TestHandSet(t *testing.T) {
	const (
		otherDomain = "other.org"
		handDID     = "did:web:id.acme.com"
		handHost    = "sso.acme.com"
	)
	did := func(t *testing.T) model.Bundle { return modelFixture(t, didFixture) }
	hosts := func(t *testing.T) model.Bundle { return fixtureBundle(t) }
	tests := []struct {
		name    string
		bundle  func(*testing.T) model.Bundle
		answers map[string]any
		want    []HandSetAnswer
	}{
		{name: "nothing given", bundle: did, answers: map[string]any{}, want: []HandSetAnswer{}},
		{name: "a derived DID is not listed", bundle: did, answers: map[string]any{"domain.base": domain}, want: []HandSetAnswer{}},
		{name: "a given method is a choice, not a pin", bundle: did,
			answers: map[string]any{"domain.base": domain, "did.method": "did:web"}, want: []HandSetAnswer{}},
		{name: "a hand-set DID names what it would derive to", bundle: did,
			answers: map[string]any{"domain.base": domain, "did.value": handDID},
			want:    []HandSetAnswer{{ID: "did.value", Value: handDID, Derived: didWeb}}},
		{name: "a domain change moves the derived value, not the DID", bundle: did,
			answers: map[string]any{"domain.base": otherDomain, "did.value": handDID},
			want:    []HandSetAnswer{{ID: "did.value", Value: handDID, Derived: "did:web:" + otherDomain}}},
		{name: "without its refs nothing would derive", bundle: did,
			answers: map[string]any{"did.value": handDID},
			want:    []HandSetAnswer{{ID: "did.value", Value: handDID}}},
		{name: "a failing defaultWhen derives nothing", bundle: did,
			answers: map[string]any{"domain.base": domain, "did.method": "did:key", "did.value": didKey},
			want:    []HandSetAnswer{{ID: "did.value", Value: didKey}}},
		{name: "a hand-set host", bundle: hosts,
			answers: map[string]any{"domain.base": otherDomain, "host.keycloak": handHost},
			want:    []HandSetAnswer{{ID: "host.keycloak", Value: handHost, Derived: "keycloak." + otherDomain}}},
		{name: "a host without a default is not listed", bundle: hosts,
			answers: map[string]any{"domain.base": domain, "host.scorpioAlt": scorpioAlt}, want: []HandSetAnswer{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := HandSet(tc.bundle(t), profile.Profile{Answers: tc.answers})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("HandSet = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestHandSetErrors(t *testing.T) {
	_, err := HandSet(modelFixture(t, didFixture), profile.Profile{Answers: map[string]any{"nope": "x"}})
	if err == nil {
		t.Error("HandSet accepted an answer the bundle does not declare")
	}
}

// TestHandSetDIDSurvivesDomainChange renders the same hand-set DID under two domains: the DID stays.
func TestHandSetDIDSurvivesDomainChange(t *testing.T) {
	const handDID = "did:web:id.acme.com"
	for _, d := range []string{domain, "other.org"} {
		tree, err := renderDID(t, map[string]any{"domain.base": d, "did.value": handDID})
		if err != nil {
			t.Fatal(err)
		}
		if got := leaf(t, tree, didPath).Value; got != handDID {
			t.Errorf("domain %s: %s = %v, want %s", d, didPath, got, handDID)
		}
	}
}

func TestHandSetResourceProfile(t *testing.T) {
	b := providerBundle(t)
	tests := []struct {
		name    string
		answers map[string]any
		want    []HandSetAnswer
	}{
		{"a given profile names the environment's", map[string]any{resourceProfileID: customProfile},
			[]HandSetAnswer{{ID: resourceProfileID, Value: customProfile, Derived: prodProfile}}},
		{"an un-given profile is not listed", map[string]any{}, []HandSetAnswer{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := profile.Profile{Target: profile.Target{Environment: profile.EnvProduction}, Answers: tc.answers}
			got, err := HandSet(b, p)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("HandSet = %+v, want %+v", got, tc.want)
			}
		})
	}
}
