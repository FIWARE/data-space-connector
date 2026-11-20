package trial

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	// requireHelmEnv turns the missing-helm and missing-dependency skips into failures (set in CI).
	requireHelmEnv   = "DSC_CONFIG_REQUIRE_HELM"
	requireHelmValue = "1"
	helmBinary       = "helm"
	chartsDir        = "charts"
	// helmTemplateTimeout bounds one `helm template` run; the umbrella chart renders in seconds.
	helmTemplateTimeout = 2 * time.Minute
)

// skipOrFail skips the test, or fails it when requireHelmEnv demands helm.
func skipOrFail(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv(requireHelmEnv) == requireHelmValue {
		t.Fatalf("%s (%s=%s)", reason, requireHelmEnv, requireHelmValue)
	}
	t.Skip(reason)
}

// TestRenderedValuesTemplate proves every trial output renders with `helm template` against the repo's chart.
func TestRenderedValuesTemplate(t *testing.T) {
	helm, err := exec.LookPath(helmBinary)
	if err != nil {
		skipOrFail(t, "helm is not on PATH")
	}
	for _, o := range render(t) {
		target := o.Release.Target
		t.Run(o.Release.Role, func(t *testing.T) {
			chart := filepath.Join(repoRoot, chartsDir, target.Chart)
			if _, err := os.Stat(filepath.Join(chart, chartsDir)); errors.Is(err, os.ErrNotExist) {
				skipOrFail(t, "dependencies of "+target.Chart+" are not fetched; run `helm dependency update "+chart+"`")
			}
			file := filepath.Join(t.TempDir(), o.File)
			if err := os.WriteFile(file, o.Values, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), helmTemplateTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, helm, "template", target.ReleaseName, chart,
				"--namespace", target.Namespace, "-f", file)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("helm template %s: %v\n%s", target.ReleaseName, err, stderr.String())
			}
		})
	}
}
