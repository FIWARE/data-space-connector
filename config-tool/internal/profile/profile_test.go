package profile

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validTarget() Target {
	return Target{
		Type:         TargetCluster,
		Context:      "kind-provider",
		Namespace:    "provider",
		ReleaseName:  "dsc",
		Chart:        "data-space-connector",
		ChartVersion: "10.16.0",
		Environment:  EnvDevelopment,
	}
}

func TestTargetValidate(t *testing.T) {
	const (
		msgType = `must be "file" or "cluster"`
		msgEnv  = `must be "development" or "production"`
	)
	tests := []struct {
		name   string
		mutate func(*Target)
		want   []FieldError
	}{
		{"valid cluster target", func(*Target) {}, nil},
		{"file target needs no context", func(t *Target) { t.Type = TargetFile; t.Context = "" }, nil},
		{"cluster target needs a context", func(t *Target) { t.Context = "" }, []FieldError{{FieldContext, "a kube context is required to deploy to a cluster"}}},
		{"empty target type", func(t *Target) { t.Type = "" }, []FieldError{{FieldType, msgRequired}}},
		{"unknown target type", func(t *Target) { t.Type = "ftp" }, []FieldError{{FieldType, msgType}}},
		{"uppercase namespace", func(t *Target) { t.Namespace = "Provider" }, []FieldError{{FieldNamespace, msgDNS1123}}},
		{"leading dash in namespace", func(t *Target) { t.Namespace = "-provider" }, []FieldError{{FieldNamespace, msgDNS1123}}},
		{"leading digit in release", func(t *Target) { t.ReleaseName = "1dsc" }, []FieldError{{FieldReleaseName, msgDNS1035}}},
		{"leading digit in namespace", func(t *Target) { t.Namespace = "1provider" }, nil},
		{"trailing dash in release", func(t *Target) { t.ReleaseName = "dsc-" }, []FieldError{{FieldReleaseName, msgDNS1035}}},
		{"empty namespace", func(t *Target) { t.Namespace = "" }, []FieldError{{FieldNamespace, msgRequired}}},
		{"empty release", func(t *Target) { t.ReleaseName = "" }, []FieldError{{FieldReleaseName, msgRequired}}},
		{"namespace at limit", func(t *Target) { t.Namespace = strings.Repeat("a", MaxNamespaceLength) }, nil},
		{"namespace too long", func(t *Target) { t.Namespace = strings.Repeat("a", MaxNamespaceLength+1) }, []FieldError{{FieldNamespace, fmt.Sprintf("must be at most %d characters", MaxNamespaceLength)}}},
		{"release at limit", func(t *Target) { t.ReleaseName = strings.Repeat("a", MaxReleaseNameLength) }, nil},
		{"release too long", func(t *Target) { t.ReleaseName = strings.Repeat("a", MaxReleaseNameLength+1) }, []FieldError{{FieldReleaseName, fmt.Sprintf("must be at most %d characters", MaxReleaseNameLength)}}},
		{"empty chart", func(t *Target) { t.Chart = "" }, []FieldError{{FieldChart, msgRequired}}},
		{"empty chart version", func(t *Target) { t.ChartVersion = "" }, []FieldError{{FieldChartVersion, msgRequired}}},
		{"empty environment", func(t *Target) { t.Environment = "" }, []FieldError{{FieldEnvironment, msgRequired}}},
		{"unknown environment", func(t *Target) { t.Environment = "staging" }, []FieldError{{FieldEnvironment, msgEnv}}},
		{"several errors", func(t *Target) { t.Context = ""; t.ReleaseName = "DSC" }, []FieldError{
			{FieldContext, "a kube context is required to deploy to a cluster"},
			{FieldReleaseName, msgDNS1035},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := validTarget()
			tc.mutate(&target)
			if got := target.Validate(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Validate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTargetNeverStoresKubeconfig(t *testing.T) {
	marshallers := map[string]func(any) ([]byte, error){
		"json": json.Marshal,
		"yaml": yaml.Marshal,
	}
	for name, marshal := range marshallers {
		t.Run(name, func(t *testing.T) {
			out, err := marshal(Profile{Target: validTarget()})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.ToLower(string(out)), "kubeconfig") {
				t.Errorf("marshalled target mentions kubeconfig:\n%s", out)
			}
		})
	}
}
