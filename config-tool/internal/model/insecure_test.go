package model

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
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
// skip...verify is the exception for prose: a label or help text that offers to skip verification.
var insecurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)verify\w*["']?\s*[:=]\s*["']?(false|no|off)\b`),
	regexp.MustCompile(`(?i)skip\w*verify\w*["']?\s*[:=]\s*["']?true`),
	regexp.MustCompile(`(?i)rejectUnauthorized["']?\s*[:=]\s*false`),
	regexp.MustCompile(`(?i)--insecure\b`),
	regexp.MustCompile(`(?i)NODE_TLS_REJECT_UNAUTHORIZED`),
	regexp.MustCompile(`(?i)skip\w*[\s_-]*(tls|ssl|cert\w*)[\s_-]*verif`),
}

// insecureName matches a foundation id or target key that would offer an option about verification.
var insecureName = regexp.MustCompile(`(?i)insecure|skip\w*verif|no_?verif|reject_?unauthorized|(ssl|tls)_?verif`)

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
		{"ssl_verify: no", true},
		{"tls_verify: off", true},
		{`label: "Skip certificate verification"`, true},
		{"skipTlsVerify", true},
		{"skip_ssl_verify", true},
		{"ssl_verify: none of the routes", false},
		{"Turn certificate verification on.", false},
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

// TestInsecureNames pins which foundation ids and target keys the structural scan refuses.
func TestInsecureNames(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"tls.skipVerify", true},
		{"insecureSkipVerify", true},
		{"insecure", true},
		{"ssl_verify", true},
		{"tlsVerify", true},
		{"noVerify", true},
		{"NODE_TLS_REJECT_UNAUTHORIZED", true},
		{"ca.privateSecret", false},
		{"ssl", false},
		{"verify", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := insecureName.MatchString(tc.name); got != tc.want {
				t.Errorf("insecureName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestNoInsecureTarget fails when a shipped bundle offers a foundation about verification, or a target that can
// write a verification key with anything but a const true; the text scan misses an option whose value is an answer.
func TestNoInsecureTarget(t *testing.T) {
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no shipped bundle")
	}
	for _, b := range got {
		for _, msg := range insecureTargets(b) {
			t.Errorf("%s %s: %s", b.Chart, b.Version, msg)
		}
	}
}

// TestInsecureTargets pins what the structural scan refuses, on fixture bundles.
func TestInsecureTargets(t *testing.T) {
	const head = elementsHead + "foundations:\n"
	tests := []struct {
		name, foundations string
		want              int
	}{
		{"const true verify key", `  - {id: a, stage: foundations, type: string, label: A, targets: [{path: keycloak.ssl_verify, fn: const, args: [true], requires: [a]}]}`, 0},
		{"foundation about verification", `  - {id: tls.skipVerify, stage: foundations, type: string, label: A, targets: [{path: keycloak.x, fn: copy, args: ["{{ .tls.skipVerify }}"]}]}`, 1},
		{"verify key copied from an answer", `  - {id: a, stage: foundations, type: string, label: A, targets: [{path: keycloak.insecureSkipVerify, fn: copy, args: ["{{ .a }}"]}]}`, 1},
		{"each verify key copied from an answer", `  - {id: a, stage: foundations, type: string, label: A, targets: [{path: keycloak.routes, each: plugins.oidc.ssl_verify, fn: copy, args: ["{{ .a }}"]}]}`, 1},
		{"const false verify key", `  - {id: a, stage: foundations, type: string, label: A, targets: [{path: keycloak.ssl_verify, fn: const, args: [false], requires: [a]}]}`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Versions(fstest.MapFS{fixtureFile: {Data: []byte(head + tc.foundations + "\n")}})
			if err != nil {
				t.Fatal(err)
			}
			if msgs := insecureTargets(got[0]); len(msgs) != tc.want {
				t.Errorf("insecureTargets = %q, want %d", msgs, tc.want)
			}
		})
	}
}

// insecureTargets describes every foundation of b about verification and every target that writes a verification
// key with anything but a const true.
func insecureTargets(b Bundle) []string {
	var out []string
	for _, decl := range b.Foundations {
		f, _ := b.Foundation(decl.ID)
		if insecureName.MatchString(f.ID) {
			out = append(out, fmt.Sprintf("foundation %s offers an option about verification", f.ID))
		}
		for i, tg := range f.Targets {
			if tg.Fn == FnConst && tg.Value == true {
				continue
			}
			for _, key := range targetKeys(tg) {
				if insecureName.MatchString(key) {
					out = append(out, fmt.Sprintf("%s target %d writes %s with something other than a const true", f.ID, i, key))
				}
			}
		}
	}
	return out
}

// targetKeys are the map keys of t's path and each path, and its element key.
func targetKeys(t CompiledTarget) []string {
	var keys []string
	for _, seg := range slices.Concat(t.Path, t.Each) {
		if k, ok := seg.Key(); ok {
			keys = append(keys, k)
		}
	}
	if t.Element != nil {
		keys = append(keys, t.Element.Key)
	}
	return keys
}

// isTest reports whether file name is a Go or UI test rather than shipped source.
func isTest(name string) bool {
	return strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.")
}
