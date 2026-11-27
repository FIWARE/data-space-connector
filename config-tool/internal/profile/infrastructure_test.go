package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

const (
	infraBundleFile  = "testdata/infrastructure.bundle.yaml"
	infraProfileFile = "testdata/infrastructure" + FileSuffix
	depCertManager   = "certManager"
	infraField       = model.InfrastructureFieldPrefix + depCertManager
)

// infraBundles holds the fixture bundle declaring the three-mode certManager dependency.
func infraBundles(t *testing.T) []model.Bundle {
	t.Helper()
	data, err := os.ReadFile(infraBundleFile)
	if err != nil {
		t.Fatal(err)
	}
	b, err := model.Versions(fstest.MapFS{latest + "/bundle.yaml": {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// infraProfile reads the fixture profile, which validates against infraBundles.
func infraProfile(t *testing.T) Profile {
	t.Helper()
	p, err := ReadFile(infraProfileFile)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInfrastructureRoundTrip(t *testing.T) {
	shipped := infraBundles(t)
	want, err := os.ReadFile(infraProfileFile)
	if err != nil {
		t.Fatal(err)
	}
	opened := infraProfile(t)
	viaJSON, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}
	var fromAPI Profile
	if err := json.Unmarshal(viaJSON, &fromAPI); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]Profile{"yaml": opened, "json": fromAPI} {
		t.Run(name, func(t *testing.T) {
			if errs := p.Validate(shipped); len(errs) != 0 {
				t.Errorf("Validate = %v, want no errors", errs)
			}
			path := filepath.Join(t.TempDir(), profileName)
			if err := WriteFile(path, p, false); err != nil {
				t.Fatal(err)
			}
			if data, _ := os.ReadFile(path); !bytes.Equal(data, want) {
				t.Errorf("saved =\n%s\nwant\n%s", data, want)
			}
		})
	}
}

func TestInfrastructureUnmarshalRefusesUnknownKeys(t *testing.T) {
	tests := []struct{ name, entry, wantErr string }{
		{"key beside mode", "    mode: cluster\n    issuer: letsencrypt\n", "field issuer not found"},
		{"entry not a mapping", "    - cluster\n", "cannot unmarshal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Unmarshal([]byte("format: 1\ninfrastructure:\n  certManager:\n" + tc.entry))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateInfrastructure(t *testing.T) {
	shipped := infraBundles(t)
	choice := func(mode string, answers map[string]any) map[string]InfraChoice {
		return map[string]InfraChoice{depCertManager: {Mode: mode, Answers: answers}}
	}
	tests := []struct {
		name  string
		infra map[string]InfraChoice
		// wantFields are the FieldError fields; none when the entry is valid.
		wantFields []string
	}{
		{"no mode chosen", choice("", map[string]any{"issuer": "letsencrypt"}), nil},
		{"answers of other modes kept", choice(model.ModeExternal, map[string]any{"issuer": "letsencrypt", "type": "prod"}), nil},
		{"per-ingress answer", choice(model.ModeExternal, map[string]any{"secret.keycloak": "my-tls"}), nil},
		{"unknown dependency", map[string]InfraChoice{"vault": {Mode: model.ModeBundled}}, []string{model.InfrastructureFieldPrefix + "vault"}},
		{"mode not offered", choice("operator", nil), []string{infraField}},
		{"unknown question", choice(model.ModeCluster, map[string]any{"server": "x"}), []string{infraField + ".server"}},
		{"mode as a question", choice(model.ModeCluster, map[string]any{model.ModeQuestion: model.ModeCluster}), []string{infraField + ".mode"}},
		{"per-ingress question unexpanded", choice(model.ModeExternal, map[string]any{"secret": "my-tls"}), []string{infraField + ".secret"}},
		{"per-ingress question of an unknown ingress", choice(model.ModeExternal, map[string]any{"secret.apisix": "my-tls"}), []string{infraField + ".secret.apisix"}},
		{"value of the wrong type", choice(model.ModeCluster, map[string]any{"issuer": "Not A Host"}), []string{infraField + ".issuer"}},
		{"option not offered", choice(model.ModeBundled, map[string]any{"type": "staging"}), []string{infraField + ".type"}},
		{"read-only question answered", choice(model.ModeBundled, map[string]any{"namespace": "other"}), []string{infraField + ".namespace"}},
		{"reserved email", choice(model.ModeBundled, map[string]any{"acmeEmail": "admins@example.com"}), []string{infraField + ".acmeEmail"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := infraProfile(t)
			p.Infrastructure = tc.infra
			var got []string
			for _, e := range p.Validate(shipped) {
				got = append(got, e.Field)
			}
			if !slices.Equal(got, tc.wantFields) {
				t.Errorf("errors = %v, want fields %v", p.Validate(shipped), tc.wantFields)
			}
		})
	}
}

// TestInfrastructureFieldIDs keeps the open-field ids and the FieldError ids of infrastructure answers one scheme.
func TestInfrastructureFieldIDs(t *testing.T) {
	shipped := infraBundles(t)
	b := findBundle(shipped, dscChart, latest)
	tests := []struct {
		field string
		known bool
	}{
		{infraField, true},
		{infraField + ".issuer", true},
		{infraField + ".acmeEmail", true},
		{infraField + ".secret.keycloak", true},
		{infraField + ".mode", false},
		{infraField + ".secret", false},
		{infraField + ".server", false},
		{model.InfrastructureFieldPrefix + "vault", false},
		{answersFieldPrefix + model.InfraModeID(depCertManager), false},
		{answersFieldPrefix + model.InfraQuestionID(depCertManager, "issuer"), false},
	}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			if got := isKnownField(tc.field, b); got != tc.known {
				t.Errorf("isKnownField = %v, want %v", got, tc.known)
			}
			p := infraProfile(t)
			p.Open = []OpenQuestion{{Field: tc.field}}
			if errs := p.Validate(shipped); (len(errs) == 0) != tc.known {
				t.Errorf("Validate = %v, want valid %v", errs, tc.known)
			}
		})
	}
	// A refused value is reported on the very id an open question names.
	p := infraProfile(t).SetInfraAnswer(depCertManager, "issuer", "Not A Host")
	errs := p.Validate(shipped)
	if len(errs) != 1 || !isKnownField(errs[0].Field, b) || errs[0].Field != infraField+".issuer" {
		t.Errorf("Validate = %v, want one error on the known field %s.issuer", errs, infraField)
	}
}

func TestInfraAnswersRefusedUnderAnswers(t *testing.T) {
	shipped := infraBundles(t)
	id := model.InfraQuestionID(depCertManager, "issuer")
	p := infraProfile(t)
	p.Answers[id] = "letsencrypt"
	errs := p.Validate(shipped)
	if want := []FieldError{{answersFieldPrefix + id, "is not a known answer id"}}; !reflect.DeepEqual(errs, want) {
		t.Errorf("Validate = %v, want %v", errs, want)
	}
}

func TestInfraAnswers(t *testing.T) {
	want := map[string]any{
		model.InfraModeID(depCertManager):                        model.ModeCluster,
		model.InfraQuestionID(depCertManager, "acmeEmail"):       "ops@fiware.org",
		model.InfraQuestionID(depCertManager, "issuer"):          "letsencrypt",
		model.InfraQuestionID(depCertManager, "secret.keycloak"): "keycloak-tls",
		model.InfraQuestionID(depCertManager, "type"):            "prod",
	}
	if got := infraProfile(t).InfraAnswers(); !reflect.DeepEqual(got, want) {
		t.Errorf("InfraAnswers = %v, want %v", got, want)
	}
	if got := (Profile{Infrastructure: map[string]InfraChoice{depCertManager: {}}}).InfraAnswers(); len(got) != 0 {
		t.Errorf("InfraAnswers without a mode = %v, want empty", got)
	}
}

func TestInfraSettersKeepOtherModesAnswers(t *testing.T) {
	shipped := infraBundles(t)
	before := infraProfile(t)
	snapshot, err := Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	p := before.SetInfraMode(depCertManager, model.ModeExternal).
		SetInfraAnswer(depCertManager, "secret.keycloak", "own-tls").
		SetInfraAnswer(depCertManager, "acmeEmail", nil).
		SetInfraMode(depCertManager, model.ModeCluster)
	if errs := p.Validate(shipped); len(errs) != 0 {
		t.Errorf("Validate = %v, want no errors", errs)
	}
	want := InfraChoice{Mode: model.ModeCluster, Answers: map[string]any{"issuer": "letsencrypt", "secret.keycloak": "own-tls", "type": "prod"}}
	if got := p.Infrastructure[depCertManager]; !reflect.DeepEqual(got, want) {
		t.Errorf("certManager = %+v, want %+v", got, want)
	}
	if after, _ := Marshal(before); !bytes.Equal(after, snapshot) {
		t.Errorf("the setters modified their receiver:\n%s", after)
	}
	fresh := Profile{}.SetInfraAnswer(depCertManager, "issuer", "letsencrypt")
	if got := fresh.Infrastructure[depCertManager]; got.Mode != "" || got.Answers["issuer"] != "letsencrypt" {
		t.Errorf("on an empty profile = %+v, want the answer and no mode", got)
	}
}

func TestApplyPresetInfrastructure(t *testing.T) {
	shipped := shippedBundles(t)
	infra := map[string]InfraChoice{depCertManager: {Mode: model.ModeCluster, Answers: map[string]any{"issuer": "letsencrypt"}}}
	open := []OpenQuestion{{Field: infraField + ".issuer"}, {Field: TargetFieldPrefix + FieldNamespace}}
	tests := []struct {
		name, role string
		wantInfra  map[string]InfraChoice
		wantOpen   []OpenQuestion
		// wantDropped is the switch's DroppedInfrastructure; nil without a chart switch.
		wantDropped []string
	}{
		{"within the chart", model.RoleConsumer, infra, open, nil},
		{"chart switch", model.RoleOperator, nil, open[1:], []string{infraField}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := preset(t, shipped, model.RoleProvider, nil, nil)
			p.Infrastructure, p.Open = infra, slices.Clone(open)
			got, sw, err := p.ApplyPreset(shipped, tc.role, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Infrastructure, tc.wantInfra) || !slices.Equal(got.Open, tc.wantOpen) {
				t.Errorf("infrastructure = %+v, open = %v; want %+v, %v", got.Infrastructure, got.Open, tc.wantInfra, tc.wantOpen)
			}
			if (sw == nil) != (tc.wantDropped == nil) || sw != nil && !slices.Equal(sw.DroppedInfrastructure, tc.wantDropped) {
				t.Errorf("switch = %+v, want dropped infrastructure %v", sw, tc.wantDropped)
			}
			if p.Infrastructure[depCertManager].Mode != model.ModeCluster {
				t.Errorf("ApplyPreset modified its receiver: %+v", p.Infrastructure)
			}
		})
	}
}
