package trial

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	trialdata "github.com/FIWARE/data-space-connector/config-tool/trial"
)

// wantWarningIDs pins the list, so no warning is dropped silently.
var wantWarningIDs = []string{
	"self-signed-certificates",
	"chart-deployed-databases",
	"local-host-names",
	"trust-all-certificates",
	"example-identities",
	"demo-keystore-password",
	"local-proxy",
	"laptop-memory-limits",
}

func TestWarningIDs(t *testing.T) {
	var ids []string
	seen := map[string]bool{}
	for _, w := range Warnings() {
		if seen[w.ID] {
			t.Errorf("duplicate warning ID %q", w.ID)
		}
		seen[w.ID] = true
		ids = append(ids, w.ID)
	}
	if !slices.Equal(ids, wantWarningIDs) {
		t.Errorf("warning IDs = %v, want %v", ids, wantWarningIDs)
	}
}

func TestWarningEvidence(t *testing.T) {
	values := map[string]string{}
	charts := map[string]string{}
	for _, r := range embedded(t).Releases {
		data, err := fs.ReadFile(trialdata.FS, r.Values)
		if err != nil {
			t.Fatal(err)
		}
		values[r.Target.ReleaseName] = string(data)
		charts[r.Target.ReleaseName] = r.Target.Chart
	}
	for _, w := range Warnings() {
		t.Run(w.ID, func(t *testing.T) {
			if w.Message == "" || len(w.Evidence) == 0 || len(w.Releases) == 0 {
				t.Fatalf("warning lacks message, evidence or releases: %+v", w)
			}
			for _, rel := range w.Releases {
				if _, ok := values[rel]; !ok {
					t.Errorf("names unknown release %q", rel)
				}
				if !slices.ContainsFunc(w.Evidence, func(e Evidence) bool { return e.Release == rel }) {
					t.Errorf("no evidence for release %s", rel)
				}
			}
			for _, e := range w.Evidence {
				if !slices.Contains(w.Releases, e.Release) {
					t.Errorf("evidence for %s, which the warning does not name", e.Release)
					continue
				}
				if e.Text == "" {
					t.Errorf("empty evidence for %s", e.Release)
					continue
				}
				if e.DefaultKey == "" {
					if !strings.Contains(values[e.Release], e.Text) {
						t.Errorf("evidence %q not in the values of %s", e.Text, e.Release)
					}
					continue
				}
				checkDefaultEvidence(t, e, values[e.Release], charts[e.Release])
			}
		})
	}
}

// checkDefaultEvidence asserts the release's values leave e.DefaultKey unset and the chart's default is e.Text.
func checkDefaultEvidence(t *testing.T, e Evidence, releaseValues, chart string) {
	t.Helper()
	path := strings.Split(e.DefaultKey, ".")
	for i := range path {
		prefix := path[:i+1]
		if v, ok := lookup(t, []byte(releaseValues), prefix); ok && (i == len(path)-1 || !isMap(v)) {
			t.Errorf("values of %s set %s, so the chart default %s does not apply",
				e.Release, strings.Join(prefix, "."), e.DefaultKey)
			return
		}
	}
	v, ok := lookup(t, readRepoFile(t, "charts/"+chart+"/values.yaml"), path)
	if !ok {
		t.Errorf("chart %s has no default for %s", chart, e.DefaultKey)
		return
	}
	if got := fmt.Sprint(v); got != e.Text {
		t.Errorf("chart %s default %s = %q, want %q", chart, e.DefaultKey, got, e.Text)
	}
}

// lookup returns the value at path in a YAML document.
func lookup(t *testing.T, doc []byte, path []string) (any, bool) {
	t.Helper()
	var cur any
	if err := yaml.Unmarshal(doc, &cur); err != nil {
		t.Fatal(err)
	}
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func isMap(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

func TestWarningsReturnsCopy(t *testing.T) {
	got := Warnings()
	got[0].ID = "changed"
	got[0].Releases[0] = "changed"
	got[0].Evidence[0].Text = "changed"
	again := Warnings()
	if again[0].ID == "changed" || again[0].Releases[0] == "changed" || again[0].Evidence[0].Text == "changed" {
		t.Error("Warnings exposes the package-level list")
	}
}
