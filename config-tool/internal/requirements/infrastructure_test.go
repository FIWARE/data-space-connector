package requirements

import (
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	infraFixture  = "../model/testdata/infrastructure/valid.yaml"
	domainBase    = "domain.base"
	hostKeycloak  = "host.keycloak"
	tlsPurpose    = "The certificate and key of the host."
	dbPurpose     = "The database password."
	certManager   = "certManager"
	database      = "database"
	exampleDomain = "example.org"
)

var tlsKeys = []string{"tls.crt", "tls.key"}

func TestInfrastructureSecrets(t *testing.T) {
	data, err := os.ReadFile(infraFixture)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := model.Versions(fstest.MapFS{"1.0.0/bundle.yaml": {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	both := map[string]any{domainBase: exampleDomain}
	tests := []struct {
		name    string
		answers map[string]any
		infra   map[string]profile.InfraChoice
		want    []Secret
	}{
		{"no mode chosen", both, nil, nil},
		{"cluster", both, map[string]profile.InfraChoice{certManager: {Mode: model.ModeCluster, Answers: map[string]any{"issuer": "ca-issuer"}}}, nil},
		{"an inactive mode's answers", both, map[string]profile.InfraChoice{certManager: {Mode: model.ModeBundled, Answers: map[string]any{"secret.keycloak": "kc-tls"}}}, nil},
		{"external, derived names", both, map[string]profile.InfraChoice{certManager: {Mode: model.ModeExternal}}, []Secret{
			{Namespace: namespace, Name: "apisix-provider-tls", Keys: tlsKeys, Purpose: tlsPurpose},
			{Namespace: namespace, Name: "keycloak." + exampleDomain + "-tls", Keys: tlsKeys, Purpose: tlsPurpose},
		}},
		{"external, one Secret for both hosts", both, map[string]profile.InfraChoice{certManager: {Mode: model.ModeExternal,
			Answers: map[string]any{"secret.apisix": "shared-tls", "secret.keycloak": "shared-tls"}}}, []Secret{
			{Namespace: namespace, Name: "shared-tls", Keys: tlsKeys, Purpose: tlsPurpose},
		}},
		{"external, an unanswered owner", map[string]any{hostKeycloak: "kc." + exampleDomain},
			map[string]profile.InfraChoice{certManager: {Mode: model.ModeExternal}}, []Secret{
				{Namespace: namespace, Name: "kc." + exampleDomain + "-tls", Keys: tlsKeys, Purpose: tlsPurpose},
			}},
		{"a mode's own requirement", nil, map[string]profile.InfraChoice{database: {Mode: model.ModeExternal,
			Answers: map[string]any{"credentials": "db-auth"}}}, []Secret{
			{Namespace: namespace, Name: "db-auth", Keys: []string{"password"}, Purpose: dbPurpose},
		}},
		{"a mode's requirement, mode not chosen", nil, map[string]profile.InfraChoice{database: {Mode: model.ModeBundled,
			Answers: map[string]any{"credentials": "db-auth"}}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := profile.Profile{Target: profile.Target{Namespace: namespace}, Answers: tc.answers, Infrastructure: tc.infra}
			got, err := Secrets(bs[0], p)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Secrets = %+v, want %+v", got, tc.want)
			}
		})
	}
}
