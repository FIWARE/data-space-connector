package profile

import (
	"encoding/json"
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
	tests := []struct {
		name   string
		mutate func(*Target)
		want   []string
	}{
		{"valid cluster target", func(*Target) {}, nil},
		{"file target needs no context", func(t *Target) { t.Type = TargetFile; t.Context = "" }, nil},
		{"cluster target needs a context", func(t *Target) { t.Context = "" }, []string{FieldContext}},
		{"unknown target type", func(t *Target) { t.Type = "ftp" }, []string{FieldType}},
		{"uppercase namespace", func(t *Target) { t.Namespace = "Provider" }, []string{FieldNamespace}},
		{"leading dash in namespace", func(t *Target) { t.Namespace = "-provider" }, []string{FieldNamespace}},
		{"trailing dash in release", func(t *Target) { t.ReleaseName = "dsc-" }, []string{FieldReleaseName}},
		{"empty namespace", func(t *Target) { t.Namespace = "" }, []string{FieldNamespace}},
		{"namespace at limit", func(t *Target) { t.Namespace = strings.Repeat("a", MaxNamespaceLength) }, nil},
		{"namespace too long", func(t *Target) { t.Namespace = strings.Repeat("a", MaxNamespaceLength+1) }, []string{FieldNamespace}},
		{"release at limit", func(t *Target) { t.ReleaseName = strings.Repeat("a", MaxReleaseNameLength) }, nil},
		{"release too long", func(t *Target) { t.ReleaseName = strings.Repeat("a", MaxReleaseNameLength+1) }, []string{FieldReleaseName}},
		{"empty chart", func(t *Target) { t.Chart = "" }, []string{FieldChart}},
		{"empty chart version", func(t *Target) { t.ChartVersion = "" }, []string{FieldChartVersion}},
		{"unknown environment", func(t *Target) { t.Environment = "staging" }, []string{FieldEnvironment}},
		{"several errors", func(t *Target) { t.Context = ""; t.ReleaseName = "DSC" }, []string{FieldContext, FieldReleaseName}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := validTarget()
			tc.mutate(&target)
			var got []string
			for _, e := range target.Validate() {
				got = append(got, e.Field)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fields with errors = %v, want %v", got, tc.want)
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
