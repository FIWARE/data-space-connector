package check

import (
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	memoryRule     = "java-memory-limit"
	resourcesField = "answers.resources.profile"
	profileAnswer  = "resources.profile"
	profileCustom  = "custom"
	memoryPrefix   = "Without a memory limit, the JVM sizes its heap from the node: "
	scorpioFeature = "data.scorpio"
	loadedName     = "values.yaml"
	// defaultUnlimited are the components a production custom profile leaves unlimited by default, in table order.
	defaultUnlimited = "Scorpio, TMForum API, Trusted Issuers List, contract-management"
)

// loadedOptions loads src against the shipped bundles as the overrides of a render.
func loadedOptions(t *testing.T, src string) render.Options {
	t.Helper()
	if src == "" {
		return render.Options{}
	}
	all, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	b := shipped(t)
	res, err := load.Load([]byte(src), loadedName, all, load.Options{Bundle: &b})
	if err != nil {
		t.Fatal(err)
	}
	return render.Options{Overrides: res}
}

// TestJavaMemoryLimit pins java-memory-limit: deployment from enabledKeys, limits from the tree, the loaded file,
// fallback paths and the chart default, which a loaded null drops.
func TestJavaMemoryLimit(t *testing.T) {
	b := shipped(t)
	tests := []struct {
		name     string
		env      profile.Environment
		resource string
		features map[string]profile.FeatureState
		loaded   string
		// want are the named components; "" wants no finding.
		want string
	}{
		{name: "production custom", env: profile.EnvProduction, resource: profileCustom, want: defaultUnlimited},
		{name: "production profile", env: profile.EnvProduction},
		{name: "production development", env: profile.EnvProduction, resource: string(profile.EnvDevelopment)},
		{name: "development custom", env: profile.EnvDevelopment, resource: profileCustom},
		{name: "feature off still deploys", env: profile.EnvProduction, resource: profileCustom,
			features: map[string]profile.FeatureState{scorpioFeature: {Enabled: false, Source: profile.SourceUser}},
			want:     defaultUnlimited},
		{name: "scorpio disabled", env: profile.EnvProduction, resource: profileCustom,
			loaded: "scorpio:\n  enabled: false\n",
			want:   "TMForum API, Trusted Issuers List, contract-management"},
		{name: "one enabledKey off", env: profile.EnvProduction, resource: profileCustom,
			loaded: "decentralizedIam:\n  enabled: false\n",
			want:   "Scorpio, TMForum API, contract-management"},
		{name: "loaded limit", env: profile.EnvProduction, resource: profileCustom,
			loaded: "scorpio:\n  resources:\n    limits:\n      memory: 1Gi\n",
			want:   "TMForum API, Trusted Issuers List, contract-management"},
		{name: "loaded null limit drops the chart's", env: profile.EnvProduction, resource: profileCustom,
			loaded: "keycloak:\n  resources:\n    limits:\n      memory: null\n",
			want:   "Keycloak, " + defaultUnlimited},
		{name: "loaded null resources drop the chart's", env: profile.EnvProduction, resource: profileCustom,
			loaded: "keycloak:\n  resources: null\n",
			want:   "Keycloak, " + defaultUnlimited},
		{name: "a pinned null beats the profile's limit", env: profile.EnvProduction,
			loaded: "keycloak:\n  resources: null\n", want: "Keycloak"},
		{name: "default-off unsized switched on", env: profile.EnvProduction, resource: profileCustom,
			loaded: "decentralizedIam:\n  vcAuthentication:\n    credentials-config-service:\n      enabled: true\n",
			want:   defaultUnlimited + ", credentials-config-service"},
		{name: "identityhub keeps the chart limit", env: profile.EnvProduction, resource: profileCustom,
			loaded: "identityhub:\n  enabled: true\n",
			want:   defaultUnlimited},
		{name: "identityhub without resources", env: profile.EnvProduction, resource: profileCustom,
			loaded: "identityhub:\n  enabled: true\n  resources: null\n",
			want:   defaultUnlimited + ", IdentityHub"},
		{name: "edc instances", env: profile.EnvProduction,
			loaded: "fdsc-edc:\n  deployment:\n    oid4vc:\n      deployment: {}\n    dcp:\n      deployment: {}\n",
			want:   "fdsc-edc (dcp), fdsc-edc (oid4vc)"},
		{name: "edc common limit", env: profile.EnvProduction,
			loaded: "fdsc-edc:\n  common:\n    deployment:\n      resources:\n        limits:\n          memory: 1Gi\n" +
				"  deployment:\n    dcp:\n      deployment: {}\n"},
		{name: "edc disabled", env: profile.EnvProduction,
			loaded: "fdsc-edc:\n  enabled: false\n  deployment:\n    dcp:\n      deployment: {}\n"},
		{name: "one component", env: profile.EnvProduction, resource: profileCustom,
			loaded: "tm-forum-api:\n  enabled: false\ndecentralizedIam:\n  enabled: false\n" +
				"contract-management:\n  enabled: false\n",
			want: "Scorpio"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]any{}
			if tc.resource != "" {
				answers[profileAnswer] = tc.resource
			}
			p := profileOf(tc.env, answers)
			p.Features = tc.features
			findings, err := Evaluate(b, p, loadedOptions(t, tc.loaded))
			if err != nil {
				t.Fatal(err)
			}
			got := only(findings, memoryRule)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("findings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want exactly one", got)
			}
			f := got[0]
			if want := memoryPrefix + tc.want + "."; f.Message != want {
				t.Errorf("message = %q, want %q", f.Message, want)
			}
			if f.Field != resourcesField || f.Severity != model.SeverityWarning || Blocking(got) {
				t.Errorf("finding = %+v, want a non-blocking warning on %s", f, resourcesField)
			}
		})
	}
}
