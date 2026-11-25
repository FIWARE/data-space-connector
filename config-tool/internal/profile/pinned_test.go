package profile_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	pinnedChart   = "data-space-connector"
	pinnedVersion = "10.16.0"
	pinnedBase    = "provider.example.org"
	domainBaseID  = "domain.base"
	keycloakID    = "host.keycloak"
	verifierID    = "host.verifier"
	handKeycloak  = "sso.acme.com"
	handVerifier  = "verify.acme.com"
)

// TestPinnedHostsSurviveSaveAndOpen saves two pinned hosts and reads them back as pinned, with their derived values.
func TestPinnedHostsSurviveSaveAndOpen(t *testing.T) {
	bs, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	var b model.Bundle
	for _, c := range bs {
		if c.Chart == pinnedChart && c.Version == pinnedVersion {
			b = c
		}
	}
	p := profile.Profile{
		Format: profile.FormatVersion,
		Model:  profile.ModelRef{Chart: b.Chart, Version: b.Version, Revision: b.Revision},
		Answers: map[string]any{
			domainBaseID: pinnedBase,
			keycloakID:   handKeycloak,
			verifierID:   handVerifier,
		},
	}
	path := filepath.Join(t.TempDir(), "pinned"+profile.FileSuffix)
	if err := profile.WriteFile(path, p, false); err != nil {
		t.Fatal(err)
	}
	opened, err := profile.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.HandSet(b, opened)
	if err != nil {
		t.Fatal(err)
	}
	want := []render.HandSetAnswer{
		{ID: keycloakID, Value: handKeycloak, Derived: "keycloak." + pinnedBase},
		{ID: verifierID, Value: handVerifier, Derived: "verifier." + pinnedBase},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("HandSet after save and open = %+v, want %+v", got, want)
	}
}
