package profile

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

// Answer ids of the shipped foundations, and one the trust-anchor bundle does not declare.
const (
	answerDisplayName = "org.displayName"
	answerDomain      = "domain.base"
	answerHost        = "host.wallet"
)

func TestValidateAnswers(t *testing.T) {
	shipped := shippedBundles(t)
	field := func(id string) string { return answersFieldPrefix + id }
	tests := []struct {
		name    string
		answers map[string]any
		open    []string
		want    []string
	}{
		{name: "declared answers", answers: map[string]any{answerDisplayName: "Acme Corp", answerDomain: "acme.example.org"}},
		{name: "undeclared answer", answers: map[string]any{answerHost: "wallet.acme.example.org"}, want: []string{field(answerHost)}},
		{name: "domain of one label", answers: map[string]any{answerDomain: "localhost"}, want: []string{field(answerDomain)}},
		{name: "domain with upper case", answers: map[string]any{answerDomain: "Acme.example.org"}, want: []string{field(answerDomain)}},
		{name: "domain too long", answers: map[string]any{answerDomain: longDomain()}, want: []string{field(answerDomain)}},
		{name: "display name with a trailing space", answers: map[string]any{answerDisplayName: "Acme "}, want: []string{field(answerDisplayName)}},
		{name: "display name not a string", answers: map[string]any{answerDisplayName: 42}, want: []string{field(answerDisplayName)}},
		{name: "open declared answer", open: []string{field(answerDomain)}},
		{name: "open undeclared answer", open: []string{field(answerHost)}, want: []string{FieldOpen}},
		{name: "open answer section only", open: []string{answersFieldPrefix}, want: []string{FieldOpen}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := dscProfile()
			p.Answers = tc.answers
			for _, f := range tc.open {
				p.Open = append(p.Open, OpenQuestion{f})
			}
			var got []string
			for _, e := range p.Validate(shipped) {
				got = append(got, e.Field)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("errors = %v, want fields %v", p.Validate(shipped), tc.want)
			}
		})
	}
}

// dnsMaxLength is the DNS limit on a full host name.
const dnsMaxLength = 253

// longDomain is a well-formed domain one character over the DNS limit.
func longDomain() string {
	const tld = ".org"
	return strings.Repeat("a", dnsMaxLength+1-len(tld)) + tld
}

// hostFoundation declares answerHost as a later step of US-3.1 does; it ends the DSC bundle's foundations list.
const hostFoundation = `  - id: host.wallet
    stage: foundations
    type: hostname
    default: "wallet.{{ .domain.base }}"
    label: Wallet host
    help: Derived from the base domain.
`

// hostBundles are the shipped bundles with answerHost declared by the DSC bundle only.
func hostBundles(t *testing.T) []model.Bundle {
	t.Helper()
	dsc, err := fs.ReadFile(bundles.FS, latest+"/bundle.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ta, err := fs.ReadFile(bundles.FS, shippedTAVersion+"/bundle.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, tail, ok := strings.Cut(string(dsc), foundationsKey)
	if !ok || !strings.HasSuffix(tail, "\n") || strings.Contains(tail, "\n\n") || strings.Contains(string(dsc), answerHost) ||
		slices.ContainsFunc(strings.Split(strings.TrimSuffix(tail, "\n"), "\n"), func(l string) bool { return !strings.HasPrefix(l, " ") }) {
		t.Fatal("shipped DSC bundle changed shape: want foundations last and no " + answerHost)
	}
	b, err := model.Versions(fstest.MapFS{
		latest + "/bundle.yaml":           {Data: append(dsc, hostFoundation...)},
		shippedTAVersion + "/bundle.yaml": {Data: ta},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestApplyPresetChartSwitchCarriesFoundations(t *testing.T) {
	shipped := hostBundles(t)
	p := dscProfile()
	p.Answers = map[string]any{answerDisplayName: "Acme Corp", answerDomain: "acme.example.org", answerHost: "wallet.acme.example.org"}
	p.Open = []OpenQuestion{{answersFieldPrefix + answerDomain}, {answersFieldPrefix + answerHost}}
	if errs := p.Validate(shipped); len(errs) > 0 {
		t.Fatalf("the provider profile does not validate: %v", errs)
	}
	got, sw, err := p.ApplyPreset(shipped, model.RoleOperator, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{answerDomain, answerDisplayName}; sw == nil || !slices.Equal(sw.CarriedAnswers, want) || !slices.Equal(sw.DroppedAnswers, []string{answerHost}) {
		t.Fatalf("switch = %+v, want carried %v and %s dropped", sw, want, answerHost)
	}
	if _, ok := got.Answers[answerHost]; ok || len(got.Answers) != 2 {
		t.Errorf("answers = %v, want only the carried ones", got.Answers)
	}
	if want := []OpenQuestion{{answersFieldPrefix + answerDomain}}; !slices.Equal(got.Open, want) {
		t.Errorf("open = %v, want %v", got.Open, want)
	}
	if errs := got.Validate(shipped); len(errs) > 0 {
		t.Errorf("the switched profile does not validate: %v", errs)
	}
}
