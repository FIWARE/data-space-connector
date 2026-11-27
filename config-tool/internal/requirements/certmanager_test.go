package requirements

import (
	"reflect"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

// shippedIngresses is the number of ingresses rows in the shipped bundle.
const shippedIngresses = 10

// TestCertManagerSecrets pins that only external lists a TLS Secret per ingress of the shipped bundle.
func TestCertManagerSecrets(t *testing.T) {
	b := shippedBundle(t)
	if len(b.Ingresses) != shippedIngresses {
		t.Fatalf("%d ingresses, want %d", len(b.Ingresses), shippedIngresses)
	}
	tests := []struct {
		mode    string
		answers map[string]any
		want    int
	}{
		{model.ModeBundled, map[string]any{"type": "selfsigned"}, 0},
		{model.ModeCluster, map[string]any{"issuer": "ca-issuer"}, 0},
		{model.ModeExternal, nil, shippedIngresses},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			p := profile.Profile{
				Target:         profile.Target{Namespace: namespace},
				Answers:        map[string]any{domainBase: exampleDomain},
				Infrastructure: map[string]profile.InfraChoice{certManager: {Mode: tc.mode, Answers: tc.answers}},
			}
			got, err := Secrets(b, p)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Fatalf("%d Secrets %+v, want %d", len(got), got, tc.want)
			}
			for _, s := range got {
				if !reflect.DeepEqual(s.Keys, tlsKeys) || s.Namespace != namespace {
					t.Errorf("%s holds %v in %q, want %v in %q", s.Name, s.Keys, s.Namespace, tlsKeys, namespace)
				}
			}
		})
	}
}
