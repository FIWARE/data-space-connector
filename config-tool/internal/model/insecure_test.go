package model

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Roots of the no-insecure scan, relative to this package. k3s/ and trial/ are reference files and are not scanned.
const (
	bundleGlob = "../../model/*/bundle.yaml"
	uiSrcDir   = "../../ui/src"
	cliDir     = "../cli"
	// uiTestDir holds the UI's test fixtures, testDataDir the Go ones.
	uiTestDir   = "test"
	testDataDir = "testdata"
)

// insecurePatterns match a value that disables certificate verification, never a key alone, so the tool may write
// ssl_verify: true and tell the user to turn verification on. NODE_TLS_REJECT_UNAUTHORIZED is the one bare name:
// as an env element its value sits on the next line, and it has no use other than turning verification off.
var insecurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)verify\w*["']?\s*[:=]\s*["']?false`),
	regexp.MustCompile(`(?i)skip\w*verify\w*["']?\s*[:=]\s*["']?true`),
	regexp.MustCompile(`(?i)rejectUnauthorized["']?\s*[:=]\s*false`),
	regexp.MustCompile(`(?i)--insecure\b`),
	regexp.MustCompile(`(?i)NODE_TLS_REJECT_UNAUTHORIZED`),
}

// insecure reports whether line disables verification by any of insecurePatterns.
func insecure(line string) bool {
	for _, re := range insecurePatterns {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// TestInsecurePatterns pins what the scan treats as disabling verification.
func TestInsecurePatterns(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"ssl_verify: false", true},
		{`"tls_verify": "false"`, true},
		{"verify_ssl = false", true},
		{"- name: NODE_TLS_REJECT_UNAUTHORIZED", true},
		{"InsecureSkipVerify: true", true},
		{"insecureSkipVerify=true", true},
		{"rejectUnauthorized: false", true},
		{"curl --insecure https://example.org", true},
		{"ssl_verify: true", false},
		{"Never use an insecure connection.", false},
		{"verify: true", false},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			if got := insecure(tc.line); got != tc.want {
				t.Errorf("insecure(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

// TestNoInsecureOption fails when a bundle, the UI source or the CLI offers or writes an option that disables
// certificate verification (ADR 0025). Reword a legitimate line rather than allow-listing it.
func TestNoInsecureOption(t *testing.T) {
	files, err := filepath.Glob(bundleGlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no bundle matches %s", bundleGlob)
	}
	for _, dir := range []string{uiSrcDir, cliDir} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && path != dir && (d.Name() == uiTestDir || d.Name() == testDataDir):
				return filepath.SkipDir
			case d.IsDir() || isTest(d.Name()):
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if insecure(line) {
				t.Errorf("%s:%d disables certificate verification: %s", file, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// isTest reports whether file name is a Go or UI test rather than shipped source.
func isTest(name string) bool {
	return strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.")
}
