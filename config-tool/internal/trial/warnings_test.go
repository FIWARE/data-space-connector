package trial

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

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
	for _, r := range embedded(t).Releases {
		data, err := fs.ReadFile(trialdata.FS, r.Values)
		if err != nil {
			t.Fatal(err)
		}
		values[r.Target.ReleaseName] = string(data)
	}
	for _, w := range Warnings() {
		t.Run(w.ID, func(t *testing.T) {
			if w.Message == "" || w.Evidence == "" || len(w.Releases) == 0 {
				t.Fatalf("warning lacks message, evidence or releases: %+v", w)
			}
			for _, rel := range w.Releases {
				src, ok := values[rel]
				if !ok {
					t.Errorf("names unknown release %q", rel)
					continue
				}
				if !strings.Contains(src, w.Evidence) {
					t.Errorf("evidence %q not in the values of %s", w.Evidence, rel)
				}
			}
		})
	}
}

func TestWarningsReturnsCopy(t *testing.T) {
	got := Warnings()
	got[0].ID = "changed"
	got[0].Releases[0] = "changed"
	again := Warnings()
	if again[0].ID == "changed" || again[0].Releases[0] == "changed" {
		t.Error("Warnings exposes the package-level list")
	}
}
