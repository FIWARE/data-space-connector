package trial

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/model"
	trialdata "github.com/FIWARE/data-space-connector/config-tool/trial"
)

const (
	repoRoot        = "../../.."
	testToolVersion = "v0.0.0-test"
	syncHint        = "run `make trial-sync`"
)

// wantRoles pins the Quick Start releases in install order, so a shrunken descriptor cannot pass vacuously.
var wantRoles = []string{releaseTrustAnchor, releaseProvider, releaseConsumer}

func embedded(t *testing.T) Trial {
	t.Helper()
	tr, err := Load(trialdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func readRepoFile(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEmbeddedReleases(t *testing.T) {
	var roles []string
	for _, r := range embedded(t).Releases {
		roles = append(roles, r.Role)
		if r.Target.ReleaseName != r.Role {
			t.Errorf("release %s: name %q, want the role", r.Role, r.Target.ReleaseName)
		}
	}
	if !slices.Equal(roles, wantRoles) {
		t.Errorf("roles = %v, want %v", roles, wantRoles)
	}
}

func TestBundledValuesMatchSource(t *testing.T) {
	for _, r := range embedded(t).Releases {
		t.Run(r.Role, func(t *testing.T) {
			got, err := fs.ReadFile(trialdata.FS, r.Values)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, readRepoFile(t, r.Source)) {
				t.Errorf("%s differs from %s: %s", r.Values, r.Source, syncHint)
			}
		})
	}
}

func TestChartVersionMatchesChart(t *testing.T) {
	for _, r := range embedded(t).Releases {
		t.Run(r.Role, func(t *testing.T) {
			var chart struct {
				Version string `yaml:"version"`
			}
			if err := yaml.Unmarshal(readRepoFile(t, "charts/"+r.Target.Chart+"/Chart.yaml"), &chart); err != nil {
				t.Fatal(err)
			}
			if r.Target.ChartVersion != chart.Version {
				t.Errorf("chartVersion = %q, Chart.yaml has %q", r.Target.ChartVersion, chart.Version)
			}
		})
	}
}

// descriptor returns a one-release descriptor; extra is appended to the target mapping.
func descriptor(release, environment, extra string) string {
	return fmt.Sprintf(`releases:
  - role: %[1]s
    source: k3s/%[1]s.yaml
    values: values/%[1]s.yaml
    target:
      type: file
      namespace: %[1]s
      releaseName: %[1]s
      chart: data-space-connector
      chartVersion: 1.0.0
      environment: %[2]s
%[3]s`, release, environment, extra)
}

func TestLoadRefuses(t *testing.T) {
	values := &fstest.MapFile{Data: []byte("a: 1\n")}
	valid := descriptor("provider", "development", "")
	tests := []struct {
		name       string
		descriptor string
		values     bool
		want       string
	}{
		{"unknown key", descriptor("provider", "development", "      extra: 1\n"), true, "not found"},
		{"duplicate release", valid + strings.TrimPrefix(valid, "releases:\n"), true, "duplicate release"},
		{"missing values file", valid, false, "values"},
		{"production environment", descriptor("provider", "production", ""), true, "environment"},
		{"empty releases", "releases: []\n", true, "no releases"},
		{"empty descriptor", "", true, "no releases"},
		{"second document", valid + "---\nreleases: []\n", true, "more than one"},
		{"invalid target", descriptor("Provider", "development", ""), true, "target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{DescriptorFile: {Data: []byte(tt.descriptor)}}
			if tt.values {
				fsys["values/provider.yaml"] = values
				fsys["values/Provider.yaml"] = values
			}
			_, err := Load(fsys)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadMissingDescriptor(t *testing.T) {
	if _, err := Load(fstest.MapFS{}); err == nil {
		t.Error("Load of an empty file system succeeded")
	}
}

func render(t *testing.T) []Output {
	t.Helper()
	out, err := Render(embedded(t), trialdata.FS, testToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRender(t *testing.T) {
	tr := embedded(t)
	out := render(t)
	if len(out) != len(tr.Releases) {
		t.Fatalf("%d outputs, want %d", len(out), len(tr.Releases))
	}
	for i, o := range out {
		r := tr.Releases[i]
		t.Run(r.Role, func(t *testing.T) {
			if o.Release != r || o.File != r.Target.ReleaseName+".yaml" {
				t.Errorf("output %d is %s / %s, want release %s in descriptor order", i, o.Release.Role, o.File, r.Role)
			}
			wantChartVersion := ""
			if r.Target.Chart == load.DefaultChart {
				wantChartVersion = r.Target.ChartVersion
			}
			header := roundtrip.Header{ToolVersion: testToolVersion, ChartVersion: wantChartVersion, Profile: TrialProfileName}
			banner := fmt.Sprintf(bannerFormat, r.Target.Chart, r.Target.ChartVersion, r.Source)
			prefix := append(header.Render(), banner...)
			if !bytes.HasPrefix(o.Values, prefix) {
				t.Fatalf("output does not start with header and banner:\n%s", o.Values[:min(len(o.Values), len(prefix))])
			}
			if !strings.Contains(banner, r.Target.Chart+"@"+r.Target.ChartVersion) {
				t.Errorf("banner does not name %s@%s", r.Target.Chart, r.Target.ChartVersion)
			}
			src, _ := fs.ReadFile(trialdata.FS, r.Values)
			if !bytes.Equal(o.Values[len(prefix):], src) {
				t.Errorf("output body differs from %s", r.Values)
			}
			doc, err := roundtrip.Parse(o.Values)
			if err != nil {
				t.Fatal(err)
			}
			if got := doc.Header(); !doc.Generated() || got != header {
				t.Errorf("parsed header = %+v (generated %v), want %+v", got, doc.Generated(), header)
			}
		})
	}
}

func TestRenderReopensConnectors(t *testing.T) {
	versions, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	reopened := 0
	for _, o := range render(t) {
		// No trust-anchor model bundle exists, so that output is not meant to be reopened (ADR 0016 §6).
		if o.Release.Target.Chart != load.DefaultChart {
			continue
		}
		t.Run(o.Release.Role, func(t *testing.T) {
			res, err := load.Load(o.Values, o.File, versions, load.Options{})
			if err != nil {
				t.Fatal(err)
			}
			r := res.Report
			if r.ChartVersion != o.Release.Target.ChartVersion || r.ChartVersionFrom != load.FromHeader || r.ChartVersionNote != "" {
				t.Errorf("bundle %s from %s (note %q), want %s from the header", r.ChartVersion, r.ChartVersionFrom,
					r.ChartVersionNote, o.Release.Target.ChartVersion)
			}
		})
		reopened++
	}
	if reopened != len(connectors) {
		t.Errorf("reopened %d outputs, want %d", reopened, len(connectors))
	}
}

func TestRenderRefusesLineBreak(t *testing.T) {
	if _, err := Render(embedded(t), trialdata.FS, "v1\nkey: value"); err == nil {
		t.Error("Render accepted a tool version with a line break")
	}
}

func TestRenderMissingValues(t *testing.T) {
	if _, err := Render(embedded(t), fstest.MapFS{}, testToolVersion); err == nil {
		t.Error("Render succeeded without the values files")
	}
}
