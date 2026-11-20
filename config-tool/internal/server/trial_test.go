package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/trial"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
	trialdata "github.com/FIWARE/data-space-connector/config-tool/trial"
)

const (
	pathTrial      = "/api/trial"
	pathTrialWrite = "/api/trial/write"
	// keptValues stands in for a user's file the second write must leave alone.
	keptValues = "kept: true\n"
)

// wantRoles is the install order of the embedded descriptor.
var wantRoles = []string{"trust-anchor", "provider", "consumer"}

// trialRequest sends body with the session cookie and the own origin.
func trialRequest(t *testing.T, srv *Server, cookie *http.Cookie, method, path, body string) (int, string) {
	t.Helper()
	resp, raw := do(t, srv, request{method: method, path: path, origin: ownOrigin(srv), cookie: cookie, body: body})
	return resp.StatusCode, raw
}

func writeBody(t *testing.T, dir string) string {
	t.Helper()
	data, err := json.Marshal(trialWriteRequest{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// wantOutputs renders the embedded trial the way the server does.
func wantOutputs(t *testing.T) []trial.Output {
	t.Helper()
	tr, err := trial.Load(trialdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	outs, err := trial.Render(tr, trialdata.FS, version.Version)
	if err != nil {
		t.Fatal(err)
	}
	return outs
}

func TestTrialGet(t *testing.T) {
	srv := started(t, builtUI)
	status, raw := trialRequest(t, srv, login(t, srv), http.MethodGet, pathTrial, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, raw)
	}
	var got trialBody
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("body %q is not a trial body: %v", raw, err)
	}
	if got.Profile != trial.TrialProfileName || got.Environment != profile.EnvDevelopment {
		t.Errorf("profile, environment = %q, %q; want %q, %q", got.Profile, got.Environment, trial.TrialProfileName, profile.EnvDevelopment)
	}
	outs := wantOutputs(t)
	if len(got.Releases) != len(wantRoles) || len(outs) != len(wantRoles) {
		t.Fatalf("releases = %d, rendered = %d, want %d", len(got.Releases), len(outs), len(wantRoles))
	}
	for i, r := range got.Releases {
		o := outs[i]
		target := o.Release.Target
		want := trialRelease{Role: wantRoles[i], ReleaseName: target.ReleaseName, Namespace: target.Namespace,
			Chart: target.Chart, ChartVersion: target.ChartVersion, Source: o.Release.Source, File: o.File, Values: string(o.Values)}
		if r.Values == "" || r != want {
			t.Errorf("release %d = %+v, want %+v", i, r, want)
		}
	}
	warnings := trial.Warnings()
	if len(got.Warnings) != len(warnings) {
		t.Fatalf("warnings = %d, want %d", len(got.Warnings), len(warnings))
	}
	for i, w := range warnings {
		want := trialWarning{ID: w.ID, Releases: w.Releases, Message: w.Message, Link: w.Link}
		if g := got.Warnings[i]; g.ID != want.ID || g.Message != want.Message || g.Link != want.Link || !slices.Equal(g.Releases, want.Releases) {
			t.Errorf("warning %d = %+v, want %+v", i, g, want)
		}
	}
}

func TestTrialWrite(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	dir := t.TempDir()
	outs := wantOutputs(t)

	status, raw := trialRequest(t, srv, cookie, http.MethodPost, pathTrialWrite, writeBody(t, dir))
	if status != http.StatusCreated {
		t.Fatalf("first write status = %d: %s", status, raw)
	}
	var got trialWriteResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("body %q is not a write response: %v", raw, err)
	}
	wantFiles := make([]string, len(outs))
	for i, o := range outs {
		wantFiles[i] = filepath.Join(dir, o.File)
	}
	if got.Dir != dir || !slices.Equal(got.Files, wantFiles) {
		t.Errorf("response = %+v, want dir %s and files %v", got, dir, wantFiles)
	}
	for i, o := range outs {
		if b := readBytes(t, wantFiles[i]); !bytes.Equal(b, o.Values) {
			t.Errorf("%s differs from the rendered output", wantFiles[i])
		}
	}

	for _, p := range wantFiles {
		writeBytes(t, p, keptValues)
	}
	status, raw = trialRequest(t, srv, cookie, http.MethodPost, pathTrialWrite, writeBody(t, dir))
	if status != http.StatusConflict {
		t.Fatalf("second write status = %d, want %d: %s", status, http.StatusConflict, raw)
	}
	var e errorBody
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("body %q is not a JSON error: %v", raw, err)
	}
	for _, p := range wantFiles {
		if !strings.Contains(e.Error, p) {
			t.Errorf("error %q does not name %s", e.Error, p)
		}
		if b := readBytes(t, p); string(b) != keptValues {
			t.Errorf("%s was overwritten", p)
		}
	}
}

func TestTrialWriteRefused(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	writeBytes(t, file, "")
	tests := []struct {
		name string
		body string
		want int
	}{
		{"relative dir", writeBody(t, "out"), http.StatusUnprocessableEntity},
		{"empty dir", writeBody(t, ""), http.StatusUnprocessableEntity},
		{"no body", "", http.StatusUnprocessableEntity},
		{"missing dir", writeBody(t, filepath.Join(dir, "missing")), http.StatusUnprocessableEntity},
		{"a file", writeBody(t, file), http.StatusUnprocessableEntity},
		{"malformed body", "{", http.StatusBadRequest},
		{"oversized body", oversized, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := started(t, builtUI)
			status, raw := trialRequest(t, srv, login(t, srv), http.MethodPost, pathTrialWrite, tc.body)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			var e errorBody
			if err := json.Unmarshal([]byte(raw), &e); err != nil || e.Error == "" {
				t.Errorf("body %q is not a JSON error: %v", raw, err)
			}
		})
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("%s holds %v, %v; want only the file", dir, entries, err)
	}
	if _, err := os.Stat(filepath.Join("out", wantRoles[0]+".yaml")); err == nil {
		t.Error("a relative write created files in the working directory")
	}
}

func TestTrialBrokenDescriptor(t *testing.T) {
	srv := newServer(t, builtUI)
	srv.trial = fstest.MapFS{}
	serve(t, srv)
	cookie := login(t, srv)
	dir := t.TempDir()
	tests := []struct {
		method, path, body string
	}{
		{http.MethodGet, pathTrial, ""},
		{http.MethodPost, pathTrialWrite, writeBody(t, dir)},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			status, raw := trialRequest(t, srv, cookie, tc.method, tc.path, tc.body)
			if status != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d: %s", status, http.StatusInternalServerError, raw)
			}
			var e errorBody
			if err := json.Unmarshal([]byte(raw), &e); err != nil || e.Error == "" {
				t.Errorf("body %q is not a JSON error: %v", raw, err)
			}
		})
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("%s holds %v, %v; want nothing", dir, entries, err)
	}
}
