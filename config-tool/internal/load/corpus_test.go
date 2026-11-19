package load

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	repoRoot          = "../../.."
	k3sDir            = repoRoot + "/k3s"
	chartValues       = repoRoot + "/charts/data-space-connector/values.yaml"
	trustAnchorValues = repoRoot + "/charts/trust-anchor/values.yaml"
	// duplicateKeysFile is the corpus file known to carry shadowed duplicate keys.
	duplicateKeysFile = "consumer.yaml"
)

// requiredCorpus guards against a moved k3s directory letting the corpus test pass vacuously.
var requiredCorpus = []string{
	"provider.yaml", "consumer.yaml", "trust-anchor.yaml",
	"provider-eidas.yaml", "consumer-gaia-x.yaml", "consent-provider.yaml",
}

// corpusFiles returns the chart values files a user may load: k3s/*.yaml and both charts' values.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(k3sDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range requiredCorpus {
		if _, err := os.Stat(filepath.Join(k3sDir, name)); err != nil {
			t.Fatalf("corpus file missing: %v", err)
		}
	}
	return append(files, chartValues, trustAnchorValues)
}

func TestCorpus(t *testing.T) {
	embedded, err := model.Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			src := readFile(t, path)
			res, err := Load(src, filepath.Base(path), embedded, Options{})
			if err != nil {
				t.Fatal(err)
			}
			// assertInvariants also checks that Write still returns exactly src.
			assertInvariants(t, src, res)
			r := res.Report
			if r.Counts.Total == 0 {
				t.Errorf("no leaves reported")
			}
			// None of these files wraps the chart's values, so detection must not find a wrapper.
			if r.Root != "" || r.RootFrom != RootDetected {
				t.Errorf("root = %q from %s, want the document root, detected", r.Root, r.RootFrom)
			}
			if path == filepath.Join(k3sDir, duplicateKeysFile) && len(r.Skipped) == 0 {
				t.Errorf("%s reports no skipped pairs", duplicateKeysFile)
			}
		})
	}
}
