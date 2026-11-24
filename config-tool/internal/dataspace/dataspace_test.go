package dataspace

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	shippedChart       = "data-space-connector"
	shippedVersion     = "10.16.0"
	trustAnchorChart   = "trust-anchor"
	trustAnchorVersion = "1.3.6"
	fixtureDir         = "testdata"

	tirAddress     = "https://tir.example.org/"
	otherTIR       = "https://tir.other.example.org/"
	userCredential = "UserCredential"
	operatorCred   = "OperatorCredential"
	userDesc       = "A natural person"
	unusedKey      = "x_contact"
)

func bundle(t *testing.T, chart, version string) model.Bundle {
	t.Helper()
	all, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(b model.Bundle) bool { return b.Chart == chart && b.Version == version })
	if i < 0 {
		t.Fatalf("no bundle %s %s", chart, version)
	}
	return all[i]
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parse(t *testing.T, name string) Descriptor {
	t.Helper()
	d, err := ParseDescriptor(read(t, name))
	if err != nil {
		t.Fatalf("ParseDescriptor(%s): %v", name, err)
	}
	return d
}

func cred(typ, desc string) map[string]any {
	m := map[string]any{model.CredentialTypeKey: typ}
	if desc != "" {
		m[model.CredentialDescriptionKey] = desc
	}
	return m
}

func creds(ms ...map[string]any) []any {
	out := make([]any, len(ms))
	for i, m := range ms {
		out[i] = m
	}
	return out
}

func TestParseDescriptorRefuses(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", read(t, "empty.json"), "is empty"},
		{"blank", read(t, "blank.json"), "is empty"},
		{"not JSON", read(t, "not-json.txt"), "not valid JSON"},
		{"JSON array", read(t, "array.json"), "must be a JSON object, not an array"},
		{"JSON string", []byte(`"tir_address"`), "not a string"},
		{"JSON null", []byte(`null`), "not null"},
		{"trailing value", read(t, "trailing.json"), "continues after its first value"},
		{"truncated", []byte(`{"tir_address": `), "not valid JSON"},
		{"oversized", oversized(), "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDescriptor(tt.data)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "tir_address") {
				t.Errorf("err %q echoes the document", err)
			}
		})
	}
}

// oversized is a valid object one byte over the cap, so only the size refuses it.
func oversized() []byte {
	const wrapper = `{"x":""}`
	return []byte(`{"x":"` + strings.Repeat("a", MaxDescriptorBytes+1-len(wrapper)) + `"}`)
}

func TestParseDescriptorAtCap(t *testing.T) {
	data := oversized()
	data = append(data[:len(data)-3], `"}`...)
	if len(data) != MaxDescriptorBytes {
		t.Fatalf("fixture is %d bytes, want %d", len(data), MaxDescriptorBytes)
	}
	if _, err := ParseDescriptor(data); err != nil {
		t.Fatalf("a document of exactly MaxDescriptorBytes: %v", err)
	}
}

func TestParseDescriptorSplitsKeys(t *testing.T) {
	d := parse(t, "full.json")
	if got := slices.Sorted(maps.Keys(d.Imported)); !reflect.DeepEqual(got, []string{KeyCredentialTypes, KeyParticipantRegistration, KeyTIRAddress}) {
		t.Errorf("Imported keys = %v", got)
	}
	if got := slices.Sorted(maps.Keys(d.Known)); !reflect.DeepEqual(got, []string{keyAuthenticationProtocols, keySupportedModels, keySupportedProtocols}) {
		t.Errorf("Known keys = %v", got)
	}
	if !reflect.DeepEqual(d.Unused, []string{unusedKey}) {
		t.Errorf("Unused = %v", d.Unused)
	}
}

func TestPreview(t *testing.T) {
	b := bundle(t, shippedChart, shippedVersion)
	full := creds(cred(userCredential, userDesc), cred(operatorCred, ""))
	tests := []struct {
		name     string
		fixture  string
		answers  map[string]any
		changes  []Change
		problems []string // "id: message substring"
		message  string
	}{
		{
			name:    "full document",
			fixture: "full.json",
			changes: []Change{
				{ID: TIRAddressID, Imported: tirAddress},
				{ID: CredentialTypesID, Imported: full},
				{ID: AutoRegistrationID, Imported: true},
			},
		},
		{
			name:    "full document over other answers",
			fixture: "full.json",
			answers: map[string]any{TIRAddressID: otherTIR, AutoRegistrationID: false},
			changes: []Change{
				{ID: TIRAddressID, Current: otherTIR, Imported: tirAddress},
				{ID: CredentialTypesID, Imported: full},
				{ID: AutoRegistrationID, Current: false, Imported: true},
			},
		},
		{
			name:    "same values give no changes",
			fixture: "full.json",
			answers: map[string]any{TIRAddressID: tirAddress, CredentialTypesID: full, AutoRegistrationID: true},
		},
		{
			name:    "same values held as typed slices give no changes",
			fixture: "full.json",
			answers: map[string]any{
				TIRAddressID:       tirAddress,
				CredentialTypesID:  []map[string]any{cred(userCredential, userDesc), cred(operatorCred, "")},
				AutoRegistrationID: true,
			},
		},
		{name: "chart document only", fixture: "chart.json", message: NothingToImport},
		{
			name:    "credential types as strings",
			fixture: "credential-strings.json",
			changes: []Change{{ID: CredentialTypesID, Imported: creds(cred(userCredential, ""), cred(operatorCred, ""))}},
		},
		{
			name:    "credential types as objects",
			fixture: "credential-objects.json",
			changes: []Change{{ID: CredentialTypesID, Imported: full}},
		},
		{name: "duplicate type", fixture: "duplicate-type.json", problems: []string{CredentialTypesID + ": repeats an earlier type"}},
		{name: "element of another kind", fixture: "bad-credential-element.json", problems: []string{CredentialTypesID + ": [1] must be a type name"}},
		{name: "invalid URL", fixture: "invalid-url.json", problems: []string{TIRAddressID + ": absolute http or https URL"}},
		{name: "bad registration value", fixture: "bad-registration.json", problems: []string{AutoRegistrationID + `: must be "automatic" or "manual"`}},
		{
			name:    "manual registration",
			fixture: "manual-registration.json",
			answers: map[string]any{AutoRegistrationID: true},
			changes: []Change{{ID: AutoRegistrationID, Current: true, Imported: false}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Preview(b, profile.Profile{Answers: tt.answers}, parse(t, tt.fixture))
			if !sameJSON(r.Changes, orEmpty(tt.changes)) {
				t.Errorf("Changes = %+v, want %+v", r.Changes, tt.changes)
			}
			assertProblems(t, r.Problems, tt.problems)
			if r.Message != tt.message {
				t.Errorf("Message = %q, want %q", r.Message, tt.message)
			}
		})
	}
}

func orEmpty(cs []Change) []Change {
	if cs == nil {
		return []Change{}
	}
	return cs
}

func assertProblems(t *testing.T, got []Problem, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Problems = %+v, want %v", got, want)
	}
	for i, w := range want {
		id, msg, _ := strings.Cut(w, ": ")
		if got[i].ID != id || !strings.Contains(got[i].Message, msg) {
			t.Errorf("Problems[%d] = %+v, want %s", i, got[i], w)
		}
	}
}

func TestPreviewKeepsKnownAndUnused(t *testing.T) {
	b := bundle(t, shippedChart, shippedVersion)
	tests := []struct {
		fixture string
		known   int
		unused  []string
	}{
		{"full.json", len(chartKeys), []string{unusedKey}},
		{"chart.json", len(chartKeys), []string{}},
		{"invalid-url.json", 0, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			r := Preview(b, profile.Profile{}, parse(t, tt.fixture))
			if len(r.Known) != tt.known || !reflect.DeepEqual(r.Unused, tt.unused) {
				t.Errorf("Known = %v, Unused = %v; want %d known, unused %v", r.Known, r.Unused, tt.known, tt.unused)
			}
		})
	}
}

func TestPreviewChartWithoutDataspaceAnswers(t *testing.T) {
	b := bundle(t, trustAnchorChart, trustAnchorVersion)
	r := Preview(b, profile.Profile{}, parse(t, "full.json"))
	if len(r.Changes) != 0 || len(r.Problems) != len(imports) {
		t.Fatalf("Changes = %+v, Problems = %+v; want no changes and one problem per imported key", r.Changes, r.Problems)
	}
}

func TestCredentialTypes(t *testing.T) {
	b := bundle(t, shippedChart, shippedVersion)
	tests := []struct {
		name    string
		answers map[string]any
		want    []CredentialType
	}{
		{"unanswered", nil, nil},
		{
			name:    "keeps order and descriptions",
			answers: map[string]any{CredentialTypesID: creds(cred(operatorCred, ""), cred(userCredential, userDesc))},
			want:    []CredentialType{{Type: operatorCred}, {Type: userCredential, Description: userDesc}},
		},
		{"invalid answer", map[string]any{CredentialTypesID: "UserCredential"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CredentialTypes(b, profile.Profile{Answers: tt.answers}); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CredentialTypes = %+v, want %+v", got, tt.want)
			}
		})
	}
	if got := CredentialTypes(bundle(t, trustAnchorChart, trustAnchorVersion), profile.Profile{Answers: tests[1].answers}); got != nil {
		t.Errorf("a bundle without the foundation gave %+v", got)
	}
}

func TestAutoRegistration(t *testing.T) {
	b := bundle(t, shippedChart, shippedVersion)
	tests := []struct {
		name      string
		answers   map[string]any
		want, set bool
	}{
		{"unanswered", nil, false, false},
		{"on", map[string]any{AutoRegistrationID: true}, true, true},
		{"off", map[string]any{AutoRegistrationID: false}, false, true},
		{"invalid", map[string]any{AutoRegistrationID: registrationAutomatic}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, set := AutoRegistration(b, profile.Profile{Answers: tt.answers})
			if got != tt.want || set != tt.set {
				t.Errorf("AutoRegistration = %v, %v; want %v, %v", got, set, tt.want, tt.set)
			}
		})
	}
}

// FuzzParseDescriptor pins the acceptance criterion: malformed input gives an error, never a panic.
func FuzzParseDescriptor(f *testing.F) {
	for _, name := range []string{"full.json", "chart.json", "duplicate-type.json", "bad-credential-element.json", "not-json.txt", "array.json"} {
		data, err := os.ReadFile(filepath.Join(fixtureDir, name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	all, err := model.Versions(bundles.FS)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := ParseDescriptor(data)
		if err != nil {
			return
		}
		// Answers keyed by answer id reach the compare-against-a-given-answer path; raw values cover a mismatch.
		given := map[string]any{}
		for _, i := range imports {
			raw, ok := d.Imported[i.key]
			if !ok {
				continue
			}
			if v, err := answerOf(i.key, raw); err == nil {
				given[i.id] = v
			} else {
				given[i.id] = raw
			}
		}
		for _, b := range all {
			Preview(b, profile.Profile{}, d)
			Preview(b, profile.Profile{Answers: given}, d)
		}
	})
}
