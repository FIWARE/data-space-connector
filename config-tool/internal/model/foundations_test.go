package model

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

const (
	validFoundations     = "testdata/foundations/valid.yaml"
	invalidFoundations   = "testdata/foundations/invalid.yaml"
	didFoundations       = "testdata/foundations/did.yaml"
	prefixFoundations    = "testdata/foundations/prefix.yaml"
	dataspaceFoundations = "testdata/foundations/dataspace.yaml"
	foundationsHead      = "chart: " + chart + "\nversion: 1.0.0\nrevision: 1\ntopLevelKeys: [keycloak, scorpio]\n"
)

func loadFixture(t *testing.T, file string) Bundle {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Versions(fstest.MapFS{fixtureFile: {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	return got[0]
}

func TestFoundationsCompile(t *testing.T) {
	b := loadFixture(t, validFoundations)
	if got, want := b.FoundationOrder(), []string{"domain.base", "host.keycloak", "org.displayName"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FoundationOrder = %v, want %v", got, want)
	}
	if _, ok := b.Foundation("host.verifier"); ok {
		t.Error("Foundation(host.verifier) found an undeclared id")
	}
	host, ok := b.Foundation("host.keycloak")
	if !ok {
		t.Fatal("Foundation(host.keycloak) not found")
	}
	if want := []string{"domain.base"}; !reflect.DeepEqual(host.Refs, want) || host.DefaultTemplate() == nil {
		t.Errorf("host.keycloak refs = %v (default %v), want %v with a default", host.Refs, host.DefaultTemplate(), want)
	}
	targets := []struct {
		path  string
		fn    Fn
		refs  []string
		value any
		tpls  int
	}{
		{"keycloak.ingress.hostname", FnCopy, []string{"host.keycloak"}, nil, 1},
		{"keycloak.ingress.tls[0].hosts[0]", FnTpl, []string{"host.keycloak"}, nil, 1},
		{"keycloak.ingress.tls[0].secretName", FnTpl, []string{"host.keycloak"}, nil, 1},
		{`keycloak.ingress.annotations."cert-manager.io/alt-names"`, FnJoin, []string{"domain.base", "host.keycloak"}, nil, 2},
	}
	for i, want := range targets {
		got := host.Targets[i]
		if got.Path.String() != want.path || got.Fn != want.fn || !reflect.DeepEqual(got.Refs, want.refs) ||
			got.Value != want.value || len(got.Templates()) != want.tpls {
			t.Errorf("targets[%d] = %s %s %v %v (%d templates), want %+v", i, got.Path, got.Fn, got.Refs, got.Value, len(got.Templates()), want)
		}
	}
	base, _ := b.Foundation("domain.base")
	if c := base.Targets[1]; c.Fn != FnConst || c.Value != "/" || len(c.Refs) != 0 || c.Templates() != nil {
		t.Errorf("const target = %+v, want the scalar / without refs or templates", c)
	}
}

func TestFoundationsCompileChoiceAndDID(t *testing.T) {
	b := loadFixture(t, didFoundations)
	if got, want := b.FoundationOrder(), []string{"did.method", "domain.base", "did.value"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FoundationOrder = %v, want %v", got, want)
	}
	method, _ := b.Foundation("did.method")
	if method.Refs != nil || method.DefaultTemplate() == nil || len(method.Options) != 2 {
		t.Errorf("did.method = refs %v, default %v, %d options; want a constant default and 2 options", method.Refs, method.DefaultTemplate(), len(method.Options))
	}
	value, _ := b.Foundation("did.value")
	if want := []string{"did.method", "domain.base"}; !reflect.DeepEqual(value.Refs, want) {
		t.Errorf("did.value refs = %v, want %v", value.Refs, want)
	}
	if want := []string{"did.method", "domain.base"}; !reflect.DeepEqual(value.DefaultRefs(), want) {
		t.Errorf("did.value default refs = %v, want %v", value.DefaultRefs(), want)
	}
	if want := (&Condition{ID: "did.method", Equals: "did:web"}); !reflect.DeepEqual(value.DefaultWhen, want) {
		t.Errorf("did.value defaultWhen = %+v, want %+v", value.DefaultWhen, want)
	}
	if want := []string{"did.method"}; value.PrefixTemplate() == nil || !reflect.DeepEqual(value.PrefixRefs(), want) {
		t.Errorf("did.value prefix refs = %v, want %v with a template", value.PrefixRefs(), want)
	}
}

func TestFoundationGroup(t *testing.T) {
	b := loadFixture(t, didFoundations)
	for id, want := range map[string]FoundationGroup{"did.method": GroupOrganization, "did.value": GroupNetwork} {
		if f, _ := b.Foundation(id); f.Group != want {
			t.Errorf("%s group = %q, want %q", id, f.Group, want)
		}
	}
}

// TestFoundationsCompileDataspace pins that copy takes a bool and a list and a tpl interpolates a url.
func TestFoundationsCompileDataspace(t *testing.T) {
	b := loadFixture(t, dataspaceFoundations)
	tests := []struct {
		id    string
		typ   FoundationType
		group FoundationGroup
		refs  []string
	}{
		{"dataspace.tirAddress", TypeURL, GroupDataspace, []string{"dataspace.tirAddress"}},
		{"dataspace.autoRegistration", TypeBool, GroupDataspace, []string{"dataspace.autoRegistration"}},
		{"dataspace.credentialTypes", TypeCredentialTypes, GroupDataspace, nil},
		{"host.tir", TypeHostname, GroupNetwork, []string{"dataspace.tirAddress", "host.tir"}},
	}
	for _, tc := range tests {
		f, ok := b.Foundation(tc.id)
		if !ok {
			t.Fatalf("Foundation(%s) not found", tc.id)
		}
		var refs []string
		for _, target := range f.Targets {
			refs = append(refs, target.Refs...)
		}
		if f.Type != tc.typ || f.Group != tc.group || !reflect.DeepEqual(refs, tc.refs) {
			t.Errorf("%s = %s in %s, target refs %v; want %s in %s, %v", tc.id, f.Type, f.Group, refs, tc.typ, tc.group, tc.refs)
		}
	}
}

func TestDefaultRefsLeaveOutPrefixRefs(t *testing.T) {
	f, _ := loadFixture(t, prefixFoundations).Foundation("tag.value")
	if want := []string{"tag.code", "tag.scheme"}; !reflect.DeepEqual(f.Refs, want) {
		t.Errorf("refs = %v, want %v", f.Refs, want)
	}
	if want := []string{"tag.code"}; !reflect.DeepEqual(f.DefaultRefs(), want) {
		t.Errorf("default refs = %v, want %v", f.DefaultRefs(), want)
	}
}

func TestCompiledFoundationCheck(t *testing.T) {
	b := loadFixture(t, didFoundations)
	tests := []struct {
		id    string
		value any
		ok    bool
	}{
		{"did.method", "did:web", true},
		{"did.method", "did:key", true},
		{"did.method", "did:ebsi", false},
		{"did.method", 1, false},
		{"did.value", "did:web:example.org", true},
		{"did.value", "did:web", false},
	}
	for _, tc := range tests {
		f, _ := b.Foundation(tc.id)
		if err := f.Check(tc.value); (err == nil) != tc.ok {
			t.Errorf("%s.Check(%v) = %v, want ok %v", tc.id, tc.value, err, tc.ok)
		}
	}
}

func TestFoundationsValidation(t *testing.T) {
	data, err := os.ReadFile(invalidFoundations)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string    `yaml:"name"`
		Err         string    `yaml:"err"`
		Foundations yaml.Node `yaml:"foundations"`
	}
	if err := yaml.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			body, err := yaml.Marshal(map[string]*yaml.Node{"foundations": &tc.Foundations})
			if err != nil {
				t.Fatal(err)
			}
			_, err = Versions(fstest.MapFS{fixtureFile: {Data: []byte(foundationsHead + string(body))}})
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.Err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.Err)
			}
		})
	}
}

func TestFoundationFeatures(t *testing.T) {
	const hostID = "host.did"
	tests := []struct {
		name     string
		features string
		want     []string
		wantErr  string
	}{
		{"ungated", "", nil, ""},
		{"one feature", "[identity.did]", []string{"identity.did"}, ""},
		{"two features", "[identity.did, data.scorpio]", []string{"identity.did", "data.scorpio"}, ""},
		{"unknown feature", "[identity.nope]", nil, `features[0]: "identity.nope" names no feature of the catalogue`},
		{"another chart's feature", "[ta.registry]", nil, "ta.registry is a feature of chart trust-anchor, not data-space-connector"},
		{"repeated feature", "[identity.did, identity.did]", nil, "features[1]: identity.did is already listed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := catalogueFS(t, "", "")
			foundation := "\nfoundations:\n  - {id: " + hostID + ", stage: foundations, type: hostname, label: DID host"
			if tc.features != "" {
				foundation += ", features: " + tc.features
			}
			fsys[fixtureFile].Data = append(fsys[fixtureFile].Data, foundation+"}\n"...)
			got, err := Versions(fsys)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(got, func(b Bundle) bool { return b.Chart == chart })
			f, _ := got[i].Foundation(hostID)
			if !reflect.DeepEqual(f.Features, tc.want) {
				t.Errorf("features = %v, want %v", f.Features, tc.want)
			}
		})
	}
}

func TestFoundationTypeCheck(t *testing.T) {
	tests := []struct {
		typ   FoundationType
		value any
		ok    bool
	}{
		{TypeString, "ACME Corp", true},
		{TypeString, " ACME", false},
		{TypeString, "", false},
		{TypeString, "two\nlines", false},
		{TypeString, "R&D Labs", true},
		{TypeString, `ACME "Labs"`, false},
		{TypeString, `R&D\Ops`, false},
		{TypeString, "<b>ACME</b>", false},
		{TypeString, "tab\there", false},
		{TypeString, 42, false},
		{TypeHostname, "localhost", true},
		{TypeHostname, "keycloak.127.0.0.1.nip.io", true},
		{TypeHostname, "Keycloak.example.org", false},
		{TypeHostname, "-a.example.org", false},
		{TypeHostname, "a..b", false},
		{TypeHostname, strings.Repeat("a.", maxHostnameLength/2) + "ab", false},
		{TypeDomain, "example.org", true},
		{TypeDomain, "localhost", false},
		{TypeDomain, "https://example.org", false},
		{TypeDID, "did:web:example.org", true},
		{TypeDID, "did:web:example.org%3A8443:user:alice", true},
		{TypeDID, "did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK", true},
		{TypeDID, "did:web:", false},
		{TypeDID, "did:web", false},
		{TypeDID, "DID:web:example.org", false},
		{TypeDID, "did:Web:example.org", false},
		{TypeDID, "did:web:example.org/path", false},
		{TypeDID, `did:web:a"b`, false},
		{TypeDID, "did:web:a b", false},
		{TypeDID, "did:web:example.org:", false},
		{TypeDID, "did:web::", false},
		{TypeDID, "did:web:a%", false},
		{TypeDID, "did:web:a%zz", false},
		{TypeURL, "https://tir.127.0.0.1.nip.io/", true},
		{TypeURL, "http://tir.example.org:8080/v4/issuers?page=1", true},
		{TypeURL, "https://[::1]:8443", true},
		{TypeURL, "https://tir.example.org/" + strings.Repeat("a", maxURLLength), false},
		// maxURLLength counts bytes: 1100 two-byte runes exceed it in 1124 characters.
		{TypeURL, "https://tir.example.org/" + strings.Repeat("ü", 1100), false},
		{TypeURL, "https://tür.example.org/", true},
		{TypeURL, "https://a!$&'()*+,;=b.example.org/", true},
		{TypeURL, "https://a%C3%BCb.example.org/", true},
		{TypeURL, "https://a{b}.example.org/", false},
		{TypeURL, "https://a|b.example.org/", false},
		{TypeURL, "https://a^b.example.org/", false},
		{TypeURL, "https://a`b.example.org/", false},
		{TypeURL, "https://a%41b.example.org/", false},
		{TypeURL, "https://tir.example.org:x/", false},
		{TypeURL, "https://x.org/%zz", false},
		{TypeURL, "https://x.org/a%", false},
		{TypeURL, "https://x.org/?a=%zz", true},
		{TypeURL, "tir.example.org", false},
		{TypeURL, "/v4/issuers", false},
		{TypeURL, "ftp://tir.example.org/", false},
		{TypeURL, "https://", false},
		{TypeURL, "https:tir.example.org", false},
		{TypeURL, "https://user:pass@tir.example.org/", false},
		{TypeURL, "https://tir.example.org/#issuers", false},
		{TypeURL, "https://tir.example.org/a b", false},
		{TypeURL, "https://tir.example.org/\t", false},
		{TypeURL, "https://tir.example.org/<x>", false},
		{TypeURL, `https://tir.example.org/"x"`, false},
		{TypeURL, " https://tir.example.org/", false},
		{TypeURL, true, false},
		{TypeBool, true, true},
		{TypeBool, false, true},
		{TypeBool, "true", false},
		{TypeBool, 1, false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "UserCredential"}), true},
		{TypeCredentialTypes, credentialTypes(
			map[string]any{"type": "UserCredential", "description": "Employees of a participant"},
			map[string]any{"type": "LegalParticipant"},
			map[string]any{"type": "eu.europa.ec.eudi.pid_1"},
		), true},
		{TypeCredentialTypes, credentialTypes(), false},
		{TypeCredentialTypes, nil, false},
		{TypeCredentialTypes, []map[string]any{{"type": "UserCredential"}}, false},
		{TypeCredentialTypes, []any{"UserCredential"}, false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"description": "no type"}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": 42}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "1Credential"}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "User Credential"}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "UserCredential", "format": "jwt_vc"}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "UserCredential", "description": " padded"}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "UserCredential", "description": 1}), false},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "A"}, map[string]any{"type": "A"}), false},
		{TypeCredentialTypes, manyCredentialTypes(maxCredentialTypes), true},
		{TypeCredentialTypes, manyCredentialTypes(maxCredentialTypes + 1), false},
		{TypeChoice, "did:web", false},
		{FoundationType("number"), "1", false},
	}
	for _, tc := range tests {
		if err := tc.typ.Check(tc.value); (err == nil) != tc.ok {
			t.Errorf("%s.Check(%v) = %v, want ok %v", tc.typ, tc.value, err, tc.ok)
		}
	}
}

// credentialTypes builds a credentialTypes answer the way yaml.v3 and encoding/json decode one.
func credentialTypes(elems ...map[string]any) []any {
	list := []any{}
	for _, e := range elems {
		list = append(list, e)
	}
	return list
}

// manyCredentialTypes is a valid answer of n distinct types.
func manyCredentialTypes(n int) []any {
	list := make([]any, n)
	for i := range list {
		list[i] = map[string]any{"type": fmt.Sprintf("Credential%d", i)}
	}
	return list
}

// TestCheckNeverEchoesValues keeps refused values out of error messages, which reach logs and the UI.
func TestCheckNeverEchoesValues(t *testing.T) {
	const secret = "s3cr3t"
	tests := []struct {
		typ   FoundationType
		value any
	}{
		{TypeURL, "https://" + secret + "@tir.example.org/"},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": secret + " x"})},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "A", "description": secret + "\n"})},
		{TypeCredentialTypes, credentialTypes(map[string]any{"type": "A", secret: "x"})},
	}
	for _, tc := range tests {
		if err := tc.typ.Check(tc.value); err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("%s.Check = %v, want an error that does not echo the value", tc.typ, err)
		}
	}
}

func TestRecognizedFoundationValidation(t *testing.T) {
	data, err := os.ReadFile(dataspaceFoundations)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		rule    string
		wantErr string
	}{
		{"url read as a value", "{path: keycloak.tirAddress, answer: dataspace.tirAddress, stage: foundations, kind: value}", ""},
		{"bool read as a toggle", "{path: keycloak.register, answer: dataspace.autoRegistration, stage: foundations, kind: toggle}", ""},
		{"other stage", "{path: keycloak.tirAddress, answer: dataspace.tirAddress, stage: features, kind: value}",
			`answer "dataspace.tirAddress" is a foundation of stage foundations, not features`},
		{"url read as a toggle", "{path: keycloak.tirAddress, answer: dataspace.tirAddress, stage: foundations, kind: toggle}",
			"only a value rule reads"},
		{"bool read as a value", "{path: keycloak.register, answer: dataspace.autoRegistration, stage: foundations, kind: value}",
			"only a toggle rule reads"},
		{"list", "{path: keycloak.types, answer: dataspace.credentialTypes, stage: foundations, kind: value}",
			"which a single leaf cannot answer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Versions(fstest.MapFS{fixtureFile: {Data: append(data, []byte("\nrecognize:\n  - "+tc.rule+"\n")...)}})
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("err = %v, want none", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
