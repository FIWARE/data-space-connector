package model

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	resourcesFoundations = "testdata/foundations/resources.yaml"
	chartSchema          = repoRoot + "/charts/data-space-connector/values.schema.json"
	profileAnswer        = "resources.profile"
	devProfile           = "development"
)

func TestFoundationsCompilePlatform(t *testing.T) {
	b := loadFixture(t, resourcesFoundations)
	f, ok := b.Foundation(profileAnswer)
	if !ok {
		t.Fatalf("Foundation(%s) not found", profileAnswer)
	}
	if f.Group != GroupPlatform {
		t.Errorf("group = %q, want %q", f.Group, GroupPlatform)
	}
	// The reserved ref gates the default but orders nothing: no foundation declares it.
	if f.Refs != nil {
		t.Errorf("refs = %v, want none", f.Refs)
	}
	if want := []string{FieldTargetEnvironment}; !reflect.DeepEqual(f.DefaultRefs(), want) {
		t.Errorf("default refs = %v, want %v", f.DefaultRefs(), want)
	}
	if b.Resources == nil || len(b.Resources.Components) != 2 || len(b.Resources.Unsized) != 1 {
		t.Errorf("resources = %+v, want 2 components and 1 unsized", b.Resources)
	}
}

func TestResourcesValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ResourceTable)
		err    string
	}{
		{"answer missing", func(r *ResourceTable) { r.Answer = "" }, "resources: answer is missing"},
		{"answer unknown", func(r *ResourceTable) { r.Answer = "nope" }, `answer "nope" names no foundation`},
		{"answer not a choice", func(r *ResourceTable) { r.Answer = "org.name" }, "answer org.name is a string, not a choice"},
		{"measuredOn not semver", func(r *ResourceTable) { r.MeasuredOn = "10.13" }, `measuredOn "10.13" is not semver`},
		{"doc missing", func(r *ResourceTable) { r.Doc = "" }, `doc "" is not a repo-relative path`},
		{"doc a URL", func(r *ResourceTable) { r.Doc = "https://x" }, `doc "https://x" is not a repo-relative path`},
		{"doc outside the repo", func(r *ResourceTable) { r.Doc = "../sizing.md" }, `doc "../sizing.md" is not a repo-relative path`},
		{"id duplicate across lists", func(r *ResourceTable) { r.Unsized[0].ID = "scorpio" },
			`unsized[0]: id "scorpio" is already declared in components[1]`},
		{"id not a field name", func(r *ResourceTable) { r.Components[0].ID = "1x" }, `components[0] (1x)`},
		{"label missing", func(r *ResourceTable) { r.Components[1].Label = "" }, "components[1] (scorpio): label is missing"},
		{"path missing", func(r *ResourceTable) { r.Components[1].Path = "" }, "components[1] (scorpio): path: "},
		{"path root unknown", func(r *ResourceTable) { r.Components[1].Path = "nope.resources" },
			`path: "nope.resources": root is not in topLevelKeys`},
		{"path with an index", func(r *ResourceTable) { r.Components[1].Path = "scorpio.list[0].resources" },
			"an index cannot address a resources block"},
		{"wildcard in a sized path", func(r *ResourceTable) { r.Components[1].Path = "scorpio.*.resources" },
			"* is only allowed in an unsized path"},
		{"wildcard first", func(r *ResourceTable) { r.Unsized[0].Path = "*.resources" }, "* cannot be the first segment"},
		{"wildcard twice", func(r *ResourceTable) { r.Unsized[0].Path = "fdsc-edc.*.a.*.resources" }, "* may appear only once"},
		{"wildcard in a fallback path", func(r *ResourceTable) { r.Unsized[0].FallbackPaths = []string{"fdsc-edc.*.resources"} },
			"unsized[0] (edc): fallbackPaths[0]: "},
		{"fallback root unknown", func(r *ResourceTable) { r.Unsized[0].FallbackPaths = []string{"nope.resources"} },
			`fallbackPaths[0]: "nope.resources": root is not in topLevelKeys`},
		{"enabled key root unknown", func(r *ResourceTable) { r.Components[0].EnabledKeys = map[string]bool{"nope.enabled": true} },
			`enabledKeys "nope.enabled": "nope.enabled": root is not in topLevelKeys`},
		{"chartMemoryLimit not a quantity", func(r *ResourceTable) { r.Components[0].ChartMemoryLimit = "lots" },
			`chartMemoryLimit "lots" is not a quantity`},
		{"profile not an option", func(r *ResourceTable) { r.Profiles["huge"] = r.Profiles[devProfile] },
			`profiles: "huge" is not an option of resources.profile`},
		{"profile misses a component", func(r *ResourceTable) { delete(r.Profiles[devProfile], "scorpio") },
			"profiles.development: component scorpio is missing"},
		{"profile sizes an unsized component", func(r *ResourceTable) { r.Profiles[devProfile]["edc"] = r.Profiles[devProfile]["scorpio"] },
			`profiles.development: "edc" names no entry of components`},
		{"quantity missing", func(r *ResourceTable) { setSpec(r, func(s *ResourceSpec) { s.Requests.CPU = "" }) },
			"profiles.development: keycloak: requests.cpu is missing"},
		{"quantity not parsed", func(r *ResourceTable) { setSpec(r, func(s *ResourceSpec) { s.Limits.Memory = "2 GB" }) },
			`profiles.development: keycloak: limits.memory "2 GB" is not a quantity`},
		{"request above limit", func(r *ResourceTable) { setSpec(r, func(s *ResourceSpec) { s.Requests.Memory = "3Gi" }) },
			"profiles.development: keycloak: requests.memory 3Gi exceeds limits.memory 2Gi"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Versions(fstest.MapFS{fixtureFile: {Data: mutateResources(t, tc.mutate)}})
			if err == nil || !strings.Contains(err.Error(), fixtureFile) || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("err = %v, want it to name %s and contain %q", err, fixtureFile, tc.err)
			}
		})
	}
}

// TestResourcesWithoutFoundations refuses a table whose answer cannot exist.
func TestResourcesWithoutFoundations(t *testing.T) {
	body := foundationsHead + "resources: {answer: " + profileAnswer + ", measuredOn: 1.0.0}\n"
	_, err := Versions(fstest.MapFS{fixtureFile: {Data: []byte(body)}})
	if want := `answer "` + profileAnswer + `" names no foundation`; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to contain %q", err, want)
	}
}

// setSpec edits the development row's Keycloak spec.
func setSpec(r *ResourceTable, edit func(*ResourceSpec)) {
	s := r.Profiles[devProfile]["keycloak"]
	edit(&s)
	r.Profiles[devProfile]["keycloak"] = s
}

// mutateResources returns the resources fixture with its table decoded, edited and encoded back.
func mutateResources(t *testing.T, mutate func(*ResourceTable)) []byte {
	t.Helper()
	data, err := os.ReadFile(resourcesFoundations)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value != "resources" {
			continue
		}
		var table ResourceTable
		if err := root.Content[i+1].Decode(&table); err != nil {
			t.Fatal(err)
		}
		mutate(&table)
		if err := root.Content[i+1].Encode(&table); err != nil {
			t.Fatal(err)
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// shippedResources is the latest connector bundle's table.
func shippedResources(t *testing.T) *ResourceTable {
	t.Helper()
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range got {
		if b.Chart == chart && b.Latest {
			if b.Resources == nil {
				t.Fatalf("%s %s has no resources table", b.Chart, b.Version)
			}
			return b.Resources
		}
	}
	t.Fatalf("no latest %s bundle", chart)
	return nil
}

func TestEmbeddedResourcesDoc(t *testing.T) {
	r := shippedResources(t)
	file, _, _ := strings.Cut(r.Doc, docAnchor)
	if _, err := os.Stat(repoRoot + "/" + file); err != nil {
		t.Errorf("doc %s: %v", r.Doc, err)
	}
}

// TestEmbeddedResourcePathsInSchema walks every table path through the chart's schema down to the first node that
// takes arbitrary keys, which every subchart alias is (Decision 7 of IMPLEMENTATION_PLAN.md).
func TestEmbeddedResourcePathsInSchema(t *testing.T) {
	data, err := os.ReadFile(chartSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	r := shippedResources(t)
	for _, c := range append(r.Components, r.Unsized...) {
		for _, p := range append([]string{c.Path}, c.FallbackPaths...) {
			if err := walkSchema(schema, p); err != nil {
				t.Errorf("%s: %s: %v", c.ID, p, err)
			}
		}
	}
}

func walkSchema(node map[string]any, path string) error {
	p, err := roundtrip.ParsePath(path)
	if err != nil {
		return err
	}
	for _, seg := range p {
		props, _ := node["properties"].(map[string]any)
		if node["additionalProperties"] == true && len(props) == 0 {
			return nil
		}
		k, _ := seg.Key()
		next, ok := props[k].(map[string]any)
		if !ok {
			return fmt.Errorf("segment %q is not in the schema", k)
		}
		node = next
	}
	return nil
}

// subchartDefaults are enabledKeys the umbrella values.yaml leaves unset, with what supplies their true default.
var subchartDefaults = map[string]string{
	"decentralizedIam.vcAuthentication.enabled":         "decentralized-iam chart default",
	"decentralizedIam.odrlAuthorization.enabled":        "decentralized-iam chart default",
	"decentralizedIam.odrlAuthorization.apisix.enabled": "odrl-authorization chart default",
	"fdsc-edc.enabled": "Helm treats a missing dependency condition as true",
}

// checkEnabledDefaults fails for every enabled key of id whose recorded default differs from the chart's.
func checkEnabledDefaults(t *testing.T, values *yaml.Node, id string, keys map[string]bool) {
	t.Helper()
	for _, k := range sortedKeys(keys) {
		got, ok := scalarAt(t, values, k)
		switch {
		case !ok && subchartDefaults[k] == "":
			t.Errorf("%s: %s is not in values.yaml and not listed in subchartDefaults", id, k)
		case !ok && !keys[k]:
			t.Errorf("%s: %s defaults to true (%s), table has false", id, k, subchartDefaults[k])
		case ok && got != strconv.FormatBool(keys[k]):
			t.Errorf("%s: %s = %s in values.yaml, table has %t", id, k, got, keys[k])
		}
	}
}

// TestEmbeddedResourceDefaults keeps the defaults the table records equal to the chart's values.yaml.
func TestEmbeddedResourceDefaults(t *testing.T) {
	values := chartValuesRoot(t)
	r := shippedResources(t)
	for _, c := range append(r.Components, r.Unsized...) {
		checkEnabledDefaults(t, values, c.ID, c.EnabledKeys)
		if c.ChartMemoryLimit == "" {
			continue
		}
		key := c.Path + "." + ResourceLimits + "." + ResourceMemory
		if got, _ := scalarAt(t, values, key); got != c.ChartMemoryLimit {
			t.Errorf("%s: chartMemoryLimit %s, values.yaml %s = %q", c.ID, c.ChartMemoryLimit, key, got)
		}
	}
}

func chartValuesRoot(t *testing.T) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(chartValues)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

// scalarAt returns the scalar at a dotted key path; false when a segment is missing.
func scalarAt(t *testing.T, node *yaml.Node, path string) (string, bool) {
	t.Helper()
	p, err := roundtrip.ParsePath(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, seg := range p {
		k, _ := seg.Key()
		var next *yaml.Node
		for i := 0; node.Kind == yaml.MappingNode && i < len(node.Content); i += 2 {
			if node.Content[i].Value == k {
				next = node.Content[i+1]
			}
		}
		if next == nil {
			return "", false
		}
		node = next
	}
	return node.Value, node.Kind == yaml.ScalarNode
}

func TestWalkSchema(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{
		"sub":   map[string]any{"additionalProperties": true},
		"local": map[string]any{"properties": map[string]any{"resources": map[string]any{"additionalProperties": true}}},
	}}
	tests := []struct {
		path string
		ok   bool
	}{
		{"sub.anything.resources", true},
		{"local.resources.limits", true},
		{"local.nope.resources", false},
		{"nope.resources", false},
	}
	for _, tc := range tests {
		if err := walkSchema(schema, tc.path); (err == nil) != tc.ok {
			t.Errorf("walkSchema(%s) = %v, want ok %v", tc.path, err, tc.ok)
		}
	}
}

// TestMemoryLimitRules pins how a memoryLimit rule decodes and renders, and what the model refuses in one.
func TestMemoryLimitRules(t *testing.T) {
	const (
		rulePrefix = "rules:\n  - {id: r, severity: warning, stage: foundations, field: answers.resources.profile, " +
			"when: [{field: target.environment, equals: production}], "
		message = "Without: {{ .components }}."
	)
	head, err := os.ReadFile(resourcesFoundations)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		components string
		message    string
		// err is a fragment of the error; "" wants the rule accepted.
		err string
	}{
		{"accepted across components and unsized", "[keycloak, edc]", message, ""},
		{"components empty", "[]", message, "memoryLimit: components is empty"},
		{"component unknown", "[keycloak, nope]", message, `memoryLimit: components[1]: "nope" names no entry of the resources table`},
		{"component twice", "[scorpio, scorpio]", message, `memoryLimit: components[1]: "scorpio" is listed twice`},
		{"message key unknown", "[keycloak]", "{{ .first }}", "message: {{ .first }} is not one of components"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rules := rulePrefix + "memoryLimit: {components: " + tc.components + "}, message: \"" + tc.message + "\", doc: doc/a.md}\n"
			got, err := Versions(fstest.MapFS{fixtureFile: {Data: append(slices.Clone(head), rules...)}})
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Errorf("err = %v, want it to contain %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			r := got[0].CheckRules()[0]
			if want := []string{"keycloak", "edc"}; r.MemoryLimit == nil || !slices.Equal(r.MemoryLimit.Components, want) {
				t.Fatalf("MemoryLimit = %+v, want components %v", r.MemoryLimit, want)
			}
			msg, err := r.RenderMessage(map[string]string{MessageComponents: "Keycloak, fdsc-edc (dcp)"})
			if want := "Without: Keycloak, fdsc-edc (dcp)."; err != nil || msg != want {
				t.Errorf("RenderMessage = %q, %v, want %q", msg, err, want)
			}
		})
	}
}
