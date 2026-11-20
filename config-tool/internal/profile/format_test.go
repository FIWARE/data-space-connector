package profile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/atomicfile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	goldenFile  = "testdata/full.dscprofile.yaml"
	dscChart    = "data-space-connector"
	taChart     = "trust-anchor"
	latest      = "10.16.0"
	older       = "10.15.0"
	unshipped   = "10.17.0"
	taVersion   = "0.2.0"
	shippedRev  = 2
	sourceHash  = "9f2c1a0b3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8"
	profileName = "provider" + FileSuffix
)

// Features and roles of testBundles: one feature per chart, one role per chart.
const (
	dscFeature = "f.dsc"
	taFeature  = "f.ta"
)

// testBundles stand in for the shipped ones, so revision and version cases do not depend on them.
var testBundles = []model.Bundle{
	{Chart: dscChart, Version: latest, Revision: shippedRev, Latest: true,
		Features: []model.Feature{{ID: dscFeature, Chart: dscChart}, {ID: taFeature, Chart: taChart}},
		Roles: []model.Role{
			{ID: model.RoleProvider, Chart: dscChart, Features: map[string]model.Offer{dscFeature: model.OfferAvailable, taFeature: model.OfferNotOffered}},
			{ID: model.RoleOperator, Chart: taChart, Features: map[string]model.Offer{dscFeature: model.OfferNotOffered, taFeature: model.OfferAvailable}},
		}},
	{Chart: dscChart, Version: older, Revision: 1},
	{Chart: taChart, Version: taVersion, Revision: 1, Latest: true},
}

// fullProfile is the content of goldenFile, with `open` out of order to exercise the sort.
func fullProfile() Profile {
	return Profile{
		Format: FormatVersion,
		Model:  ModelRef{Chart: dscChart, Version: latest, Revision: 1},
		Target: Target{
			Type:            TargetCluster,
			Context:         "kind-dsc",
			Namespace:       "provider",
			CreateNamespace: true,
			ReleaseName:     "provider",
			Chart:           dscChart,
			ChartVersion:    latest,
			Environment:     EnvProduction,
		},
		Open:   []OpenQuestion{{TargetFieldPrefix + FieldNamespace}, {TargetFieldPrefix + FieldContext}},
		Source: &Source{File: "values.yaml", Root: "dsc", SHA256: sourceHash},
	}
}

func readGolden(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMarshalGolden(t *testing.T) {
	got, err := Marshal(fullProfile())
	if err != nil {
		t.Fatal(err)
	}
	if want := readGolden(t); !bytes.Equal(got, want) {
		t.Errorf("Marshal =\n%s\nwant\n%s", got, want)
	}
}

// TestSaveLoadSave pins determinism: decoding and encoding again gives identical bytes.
func TestSaveLoadSave(t *testing.T) {
	empty, err := Marshal(Profile{})
	if err != nil {
		t.Fatal(err)
	}
	role, err := os.ReadFile(roleGoldenFile)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{"full": readGolden(t), "role": role, "empty": empty}
	for name, first := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := Unmarshal(first)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Errorf("second save =\n%s\nwant\n%s", second, first)
			}
		})
	}
}

func TestUnmarshalRefuses(t *testing.T) {
	const head = "format: 1\n"
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"comment only", Header, "empty"},
		{"not YAML", "format: [", "did not find"},
		{"unknown top-level key", head + "kubeconfig: /home/me/.kube/config\n", "field kubeconfig not found"},
		{"unknown target key", head + "target:\n  kubeconfig: x\n", "field kubeconfig not found"},
		{"unknown source key", head + "source:\n  content: x\n", "field content not found"},
		{"format missing", "open: []\n", "format 0 is not supported"},
		{"format 2", "format: 2\n", "format 2 is not supported"},
		{"format not a number", "format: one\n", "cannot unmarshal"},
		{"second document", head + "---\nformat: 1\n", "more than one YAML document"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Unmarshal([]byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	targetField := func(name string) string { return TargetFieldPrefix + name }
	feature := func(id string) string { return featuresFieldPrefix + id }
	provider := func(p *Profile) {
		p.Role = model.RoleProvider
		p.Features = map[string]FeatureState{dscFeature: {Enabled: true, Source: SourcePreset}}
	}
	operator := func(p *Profile) {
		p.Target.Chart, p.Target.ChartVersion = taChart, ""
		p.Model = ModelRef{Chart: taChart, Version: taVersion, Revision: 1}
		p.Catalogue = &ModelRef{Chart: dscChart, Version: latest, Revision: shippedRev}
		p.Role = model.RoleOperator
		p.Features = map[string]FeatureState{taFeature: {Enabled: true, Source: SourcePreset}}
	}
	tests := []struct {
		name   string
		mutate func(p *Profile)
		want   []string
	}{
		{name: "full profile", mutate: func(*Profile) {}},
		{name: "no source", mutate: func(p *Profile) { p.Source = nil }},
		{name: "older revision opens", mutate: func(p *Profile) { p.Model.Revision = 1 }},
		{name: "shipped revision", mutate: func(p *Profile) { p.Model.Revision = shippedRev }},
		{name: "newer revision", mutate: func(p *Profile) { p.Model.Revision = shippedRev + 1 }, want: []string{FieldModel}},
		{name: "revision missing", mutate: func(p *Profile) { p.Model.Revision = 0 }, want: []string{FieldModel}},
		{name: "format 0", mutate: func(p *Profile) { p.Format = 0 }, want: []string{FieldFormat}},
		{
			name:   "empty chart version keeps an older model",
			mutate: func(p *Profile) { p.Target.ChartVersion = ""; p.Model.Version = older },
		},
		{
			name:   "empty chart version, model not shipped",
			mutate: func(p *Profile) { p.Target.ChartVersion = ""; p.Model.Version = unshipped },
			want:   []string{FieldModel},
		},
		{
			name:   "model version differs from chart version",
			mutate: func(p *Profile) { p.Target.ChartVersion = older },
			want:   []string{FieldModel},
		},
		{
			name:   "chart version not shipped",
			mutate: func(p *Profile) { p.Target.ChartVersion = unshipped },
			want:   []string{targetField(FieldChartVersion), FieldModel},
		},
		{
			name: "model chart differs from target chart",
			mutate: func(p *Profile) {
				p.Model = ModelRef{Chart: taChart, Version: taVersion, Revision: 1}
				p.Target.ChartVersion = ""
			},
			want: []string{FieldModel},
		},
		{
			name:   "target chart not shipped",
			mutate: func(p *Profile) { p.Target.Chart = "other"; p.Target.ChartVersion = "" },
			want:   []string{targetField(FieldChart), FieldModel},
		},
		{name: "namespace format error is kept as typed", mutate: func(p *Profile) { p.Target.Namespace = "Prod_NS" }},
		{name: "long release name is kept as typed", mutate: func(p *Profile) { p.Target.ReleaseName = strings.Repeat("a", 60) }},
		{
			name: "unanswered fields are open questions, not errors",
			mutate: func(p *Profile) {
				p.Target = Target{Type: TargetCluster, Chart: dscChart}
			},
		},
		{name: "unknown environment", mutate: func(p *Profile) { p.Target.Environment = "staging" }, want: []string{targetField(FieldEnvironment)}},
		{name: "unknown type", mutate: func(p *Profile) { p.Target.Type = "helm" }, want: []string{targetField(FieldType)}},
		{name: "every target field can be open", mutate: func(p *Profile) {
			p.Open = nil
			for _, f := range targetFields {
				p.Open = append(p.Open, OpenQuestion{targetField(f)})
			}
		}},
		{name: "open id unknown", mutate: func(p *Profile) { p.Open = append(p.Open, OpenQuestion{"target.kubeconfig"}) }, want: []string{FieldOpen}},
		{name: "open id without section", mutate: func(p *Profile) { p.Open = append(p.Open, OpenQuestion{FieldNamespace}) }, want: []string{FieldOpen}},
		{name: "open id empty", mutate: func(p *Profile) { p.Open = append(p.Open, OpenQuestion{}) }, want: []string{FieldOpen}},
		{name: "open id twice", mutate: func(p *Profile) { p.Open = append(p.Open, p.Open[0]) }, want: []string{FieldOpen}},
		{name: "source sha256 uppercase", mutate: func(p *Profile) { p.Source.SHA256 = strings.ToUpper(sourceHash) }, want: []string{FieldSourceSHA256}},
		{name: "source sha256 short", mutate: func(p *Profile) { p.Source.SHA256 = sourceHash[:12] }, want: []string{FieldSourceSHA256}},
		{name: "source root empty is the whole file", mutate: func(p *Profile) { p.Source.Root = "" }},
		{
			name:   "answer that looks like a secret",
			mutate: func(p *Profile) { p.Answers = map[string]any{"keycloak.adminPassword": "s3cr3t"} },
			want:   []string{"answers.keycloak.adminPassword"},
		},
		{
			name:   "field id is not an answer id",
			mutate: func(p *Profile) { p.Answers = map[string]any{targetField(FieldNamespace): "x"} },
			want:   []string{"answers.target.namespace"},
		},
		{name: "empty answers", mutate: func(p *Profile) { p.Answers = map[string]any{} }},
		{
			name: "secret with a plain value as fingerprint",
			mutate: func(p *Profile) {
				p.Secrets = map[string]SecretRef{"keycloak-admin": {Mode: "generate", Keys: []string{"password"}, Fingerprints: map[string]string{"password": "s3cr3t"}}}
			},
			want: []string{"secrets.keycloak-admin"},
		},
		{name: "provider role", mutate: provider},
		{name: "loaded and user sources", mutate: func(p *Profile) {
			provider(p)
			p.Features[dscFeature] = FeatureState{Enabled: false, Source: SourceLoaded}
			p.Features[taFeature] = FeatureState{Enabled: false, Source: SourceUser}
		}},
		{name: "role without features", mutate: func(p *Profile) { p.Role = model.RoleProvider }},
		{name: "unknown role", mutate: func(p *Profile) { provider(p); p.Role = "admin" }, want: []string{FieldRole}},
		{name: "features without a role", mutate: func(p *Profile) { provider(p); p.Role = "" }, want: []string{FieldRole}},
		{name: "unknown feature", mutate: func(p *Profile) { provider(p); p.Features["f.x"] = FeatureState{Source: SourcePreset} }, want: []string{feature("f.x")}},
		{name: "unknown source", mutate: func(p *Profile) { provider(p); p.Features[dscFeature] = FeatureState{Source: "guess"} }, want: []string{feature(dscFeature)}},
		{name: "enabled not-offered feature", mutate: func(p *Profile) { provider(p); p.Features[taFeature] = FeatureState{Enabled: true, Source: SourceUser} }, want: []string{feature(taFeature)}},
		{name: "role of another chart", mutate: func(p *Profile) { p.Role = model.RoleOperator }, want: []string{FieldRole}},
		{name: "operator with its catalogue", mutate: operator},
		{name: "operator without catalogue", mutate: func(p *Profile) { operator(p); p.Catalogue = nil }, want: []string{FieldRole}},
		{name: "older catalogue revision opens", mutate: func(p *Profile) { operator(p); p.Catalogue.Revision = 1 }},
		{name: "catalogue newer than shipped", mutate: func(p *Profile) { operator(p); p.Catalogue.Revision = shippedRev + 1 }, want: []string{FieldCatalogue}},
		{name: "catalogue revision missing", mutate: func(p *Profile) { operator(p); p.Catalogue.Revision = 0 }, want: []string{FieldCatalogue}},
		{name: "catalogue not shipped", mutate: func(p *Profile) { operator(p); p.Catalogue.Version = unshipped }, want: []string{FieldCatalogue}},
		{name: "catalogue without roles", mutate: func(p *Profile) { operator(p); p.Catalogue.Version = older; p.Catalogue.Revision = 1 }, want: []string{FieldCatalogue}},
		{name: "catalogue without a role", mutate: func(p *Profile) { operator(p); p.Role, p.Features = "", nil }, want: []string{FieldCatalogue}},
	}
	for _, f := range []string{"values.yaml", "my values.yaml", "..values.yaml"} {
		tests = append(tests, struct {
			name   string
			mutate func(p *Profile)
			want   []string
		}{name: "source file " + f, mutate: func(p *Profile) { p.Source.File = f }})
	}
	for _, f := range []string{"", "../../etc/passwd", "/abs/values.yaml", "sub/values.yaml", `sub\values.yaml`, `C:values.yaml`, ".", ".."} {
		tests = append(tests, struct {
			name   string
			mutate func(p *Profile)
			want   []string
		}{name: "source file " + f, mutate: func(p *Profile) { p.Source.File = f }, want: []string{FieldSourceFile}})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := fullProfile()
			tc.mutate(&p)
			var got []string
			for _, e := range p.Validate(testBundles) {
				got = append(got, e.Field)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate fields = %v, want %v (%v)", got, tc.want, p.Validate(testBundles))
			}
		})
	}
}

// TestValidateNeverEchoesValues keeps refused values out of error messages, which reach logs and the UI.
func TestValidateNeverEchoesValues(t *testing.T) {
	const secret = "s3cr3t-value"
	p := fullProfile()
	p.Answers = map[string]any{"keycloak.adminPassword": secret}
	p.Secrets = map[string]SecretRef{"admin": {Fingerprints: map[string]string{"password": secret}}}
	for _, e := range p.Validate(testBundles) {
		if strings.Contains(e.Error(), secret) {
			t.Errorf("error %q echoes the value", e)
		}
	}
}

func TestGoldenValidatesAgainstShippedBundles(t *testing.T) {
	shipped, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{goldenFile, roleGoldenFile} {
		t.Run(file, func(t *testing.T) {
			p, err := ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if errs := p.Validate(shipped); len(errs) != 0 {
				t.Errorf("Validate = %v, want no errors", errs)
			}
		})
	}
}

// TestSecretRefHoldsNoValue fails when a field is added to SecretRef, so a value slot needs a deliberate review.
func TestSecretRefHoldsNoValue(t *testing.T) {
	want := []string{"Mode", "Keys", "Fingerprints"}
	typ := reflect.TypeFor[SecretRef]()
	var got []string
	for i := range typ.NumField() {
		got = append(got, typ.Field(i).Name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("SecretRef fields = %v, want exactly %v", got, want)
	}
}

func TestFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), profileName)
	want := fullProfile()
	if err := WriteFile(path, want, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, want, false); !errors.Is(err, atomicfile.ErrExists) {
		t.Errorf("second WriteFile err = %v, want %v", err, atomicfile.ErrExists)
	}
	if err := WriteFile(path, want, true); err != nil {
		t.Fatalf("WriteFile with overwrite: %v", err)
	}
	if data, _ := os.ReadFile(path); !bytes.Equal(data, readGolden(t)) {
		t.Errorf("written file =\n%s\nwant the golden file", data)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(want.Open, func(a, b OpenQuestion) int { return strings.Compare(a.Field, b.Field) })
	want.Answers, want.Secrets = map[string]any{}, map[string]SecretRef{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadFile = %+v, want %+v", got, want)
	}
}

func TestFileRefusesSuffix(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"values.yaml", "provider.dscprofile.yml", "provider" + FileSuffix + ".bak"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			if err := WriteFile(path, fullProfile(), true); !errors.Is(err, ErrSuffix) {
				t.Errorf("WriteFile err = %v, want %v", err, ErrSuffix)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("WriteFile created %s", path)
			}
			if _, err := ReadFile(path); !errors.Is(err, ErrSuffix) {
				t.Errorf("ReadFile err = %v, want %v", err, ErrSuffix)
			}
		})
	}
}
