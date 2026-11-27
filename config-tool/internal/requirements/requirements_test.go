package requirements

import (
	"reflect"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	chart     = "data-space-connector"
	version   = "10.16.0"
	caSecret  = "ca.privateSecret"
	namespace = "provider"
)

func shippedBundle(t *testing.T) model.Bundle {
	t.Helper()
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bs {
		if b.Chart == chart && b.Version == version {
			return b
		}
	}
	t.Fatalf("no embedded %s %s bundle", chart, version)
	return model.Bundle{}
}

func TestSecrets(t *testing.T) {
	b := shippedBundle(t)
	purpose := b.Requirements()[0].Purpose
	tests := []struct {
		name    string
		answers map[string]any
		target  profile.Target
		want    []Secret
	}{
		{"the private CA", map[string]any{caSecret: "corp-ca"}, profile.Target{Namespace: namespace},
			[]Secret{{Namespace: namespace, Name: "corp-ca", Key: "ca.crt", Purpose: purpose}}},
		{"no namespace yet", map[string]any{caSecret: "corp-ca"}, profile.Target{},
			[]Secret{{Name: "corp-ca", Key: "ca.crt", Purpose: purpose}}},
		{"unanswered", map[string]any{"proxy.https": "http://proxy:3128"}, profile.Target{Namespace: namespace}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Secrets(b, profile.Profile{Answers: tc.answers, Target: tc.target})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Secrets = %+v, want %+v", got, tc.want)
			}
		})
	}
}
