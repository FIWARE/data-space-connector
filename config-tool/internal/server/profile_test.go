package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/recent"
)

const (
	profileName = "provider" + profile.FileSuffix
	valuesName  = "values.yaml"
	// wrappedValues holds the chart's values under the "dsc" key, so a restored root is visible.
	wrappedValues = "dsc:\n  did:\n    enabled: true\n"
	olderVersion  = "1.0.0"
	newerVersion  = "2.0.0"
	// newerRevision is the shipped revision of newerVersion in twoBundles.
	newerRevision = 2
)

// twoBundles ships two versions of the default chart, the newer one at revision newerRevision.
var twoBundles = fstest.MapFS{
	olderVersion + "/bundle.yaml": {Data: []byte("chart: " + load.DefaultChart + "\nversion: " + olderVersion + "\nrevision: 1\n")},
	newerVersion + "/bundle.yaml": {Data: []byte("chart: " + load.DefaultChart + "\nversion: " + newerVersion + "\nrevision: 2\n")},
}

func completeTarget() profile.Target {
	return profile.Target{Type: profile.TargetFile, Namespace: "dsc", ReleaseName: "provider",
		Chart: load.DefaultChart, ChartVersion: latestChart, Environment: profile.EnvDevelopment}
}

// profileSession is a served test server with a logged-in cookie.
type profileSession struct {
	t      *testing.T
	srv    *Server
	cookie *http.Cookie
}

func newProfileSession(t *testing.T) *profileSession {
	t.Helper()
	srv := started(t, builtUI)
	return &profileSession{t: t, srv: srv, cookie: login(t, srv)}
}

// call sends body as JSON and decodes a 200 response into a profileResponse.
func (ps *profileSession) call(method, path string, body any) (int, string, profileResponse) {
	ps.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		ps.t.Fatal(err)
	}
	resp, raw := do(ps.t, ps.srv, request{method: method, path: path, origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(data)})
	var got profileResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			ps.t.Fatalf("body %q is not a profile response: %v", raw, err)
		}
	}
	return resp.StatusCode, raw, got
}

func (ps *profileSession) save(req saveRequest) (int, string, profileResponse) {
	ps.t.Helper()
	return ps.call(http.MethodPut, "/api/profile", req)
}

func (ps *profileSession) open(path string) (int, string, profileResponse) {
	ps.t.Helper()
	return ps.call(http.MethodPost, "/api/profile/open", openRequest{Path: path})
}

// mustSave saves and fails the test on any status but 200.
func (ps *profileSession) mustSave(req saveRequest) profileResponse {
	ps.t.Helper()
	status, raw, got := ps.save(req)
	if status != http.StatusOK {
		ps.t.Fatalf("save status = %d: %s", status, raw)
	}
	return got
}

// load uploads body as name and returns its report.
func (ps *profileSession) load(name, body string) load.Report {
	ps.t.Helper()
	status, raw, report := postLoad(ps.t, ps.srv, ps.cookie, name, body)
	if status != http.StatusOK {
		ps.t.Fatalf("load status = %d: %s", status, raw)
	}
	return report
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeBytes(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSaveOpenSaveIsByteIdentical(t *testing.T) {
	ps := newProfileSession(t)
	dir := t.TempDir()
	report := ps.load(valuesName, wrappedValues)
	writeBytes(t, filepath.Join(dir, valuesName), wrappedValues)
	path := filepath.Join(dir, profileName)
	ps.mustSave(saveRequest{Path: path, SourceSHA256: report.SHA256, Profile: profile.Profile{Target: completeTarget(),
		Open: []profile.OpenQuestion{{Field: "target.namespace"}, {Field: "target.context"}}}})
	first := readBytes(t, path)

	status, raw, opened := ps.open(path)
	if status != http.StatusOK {
		t.Fatalf("open status = %d: %s", status, raw)
	}
	if opened.Report == nil {
		t.Fatal("open restored no report")
	}
	ps.mustSave(saveRequest{Path: path, Overwrite: true, SourceSHA256: opened.Report.SHA256, Profile: opened.Profile})
	if second := readBytes(t, path); !bytes.Equal(first, second) {
		t.Errorf("second save differs:\n%s\nwant\n%s", second, first)
	}
}

func TestProfilePathErrors(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing"+profile.FileSuffix)
	writeBytes(t, existing, "format: 1\n")
	symlink := filepath.Join(dir, "link"+profile.FileSuffix)
	if err := os.Symlink(existing, symlink); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(dir, "invalid"+profile.FileSuffix)
	writeBytes(t, invalid, "format: 1\nunknown: x\n")
	const (
		save = http.MethodPut
		open = http.MethodPost
	)
	tests := []struct {
		name     string
		endpoint string
		path     string
		want     int
	}{
		{"save relative path", save, profileName, http.StatusUnprocessableEntity},
		{"open relative path", open, profileName, http.StatusUnprocessableEntity},
		{"save without suffix", save, filepath.Join(dir, "values.yaml"), http.StatusUnprocessableEntity},
		{"open without suffix", open, filepath.Join(dir, "values.yaml"), http.StatusUnprocessableEntity},
		{"save existing without overwrite", save, existing, http.StatusConflict},
		{"save into a missing directory", save, filepath.Join(dir, "missing", profileName), http.StatusUnprocessableEntity},
		{"save onto a symlink", save, symlink, http.StatusUnprocessableEntity},
		{"open missing file", open, filepath.Join(dir, profileName), http.StatusNotFound},
		{"open invalid profile", open, invalid, http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			ps.load(valuesName, smallValues)
			before := loadedSlot(ps.srv)
			var status int
			var raw string
			if tc.endpoint == save {
				status, raw, _ = ps.save(saveRequest{Path: tc.path, Profile: profile.Profile{Target: completeTarget()}})
			} else {
				status, raw, _ = ps.open(tc.path)
			}
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			if got := loadedSlot(ps.srv); got != before {
				t.Error("a refused request changed the loaded slot")
			}
			if entries, err := ps.srv.recent.List(); err != nil || len(entries) != 0 {
				t.Errorf("recent list = %v, %v; want empty", entries, err)
			}
		})
	}
	if _, err := os.Stat(profileName); err == nil {
		t.Errorf("a relative save wrote %s into the working directory", profileName)
	}
}

func TestHalfFinishedProfile(t *testing.T) {
	tests := []struct {
		name   string
		target profile.Target
		want   int
	}{
		{"format error and unanswered field", profile.Target{Type: profile.TargetCluster, Namespace: "Prod_NS",
			Chart: load.DefaultChart, Environment: profile.EnvProduction}, http.StatusOK},
		{"release name too long", profile.Target{Type: profile.TargetFile, Namespace: "dsc", ReleaseName: strings.Repeat("a", 60),
			Chart: load.DefaultChart, ChartVersion: latestChart, Environment: profile.EnvDevelopment}, http.StatusOK},
		{"unknown environment", profile.Target{Type: profile.TargetFile, Chart: load.DefaultChart, Environment: "staging"},
			http.StatusUnprocessableEntity},
		{"unknown chart version", profile.Target{Type: profile.TargetFile, Chart: load.DefaultChart, ChartVersion: "0.0.1"},
			http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			path := filepath.Join(t.TempDir(), profileName)
			status, raw, _ := ps.save(saveRequest{Path: path, Profile: profile.Profile{Target: tc.target}})
			if status != tc.want {
				t.Fatalf("save status = %d, want %d: %s", status, tc.want, raw)
			}
			if tc.want != http.StatusOK {
				var body errorBody
				if err := json.Unmarshal([]byte(raw), &body); err != nil || len(body.Fields) == 0 {
					t.Errorf("422 body %s carries no field errors", raw)
				}
				if _, err := os.Stat(path); err == nil {
					t.Error("a refused save wrote the file")
				}
				return
			}
			first := readBytes(t, path)
			status, raw, opened := ps.open(path)
			if status != http.StatusOK {
				t.Fatalf("open status = %d: %s", status, raw)
			}
			if opened.Profile.Target != tc.target {
				t.Errorf("reopened target = %+v, want %+v", opened.Profile.Target, tc.target)
			}
			ps.mustSave(saveRequest{Path: path, Overwrite: true, Profile: opened.Profile})
			if second := readBytes(t, path); !bytes.Equal(first, second) {
				t.Errorf("second save differs:\n%s\nwant\n%s", second, first)
			}
		})
	}
}

func TestSaveSourceStatement(t *testing.T) {
	const otherSHA = "0000000000000000000000000000000000000000000000000000000000000000"
	tests := []struct {
		name       string
		load       bool
		sha        func(load.Report) string
		want       int
		wantSource bool
	}{
		{"loaded and named", true, func(r load.Report) string { return r.SHA256 }, http.StatusOK, true},
		{"loaded but not named, as after New configuration", true, func(load.Report) string { return "" }, http.StatusOK, false},
		{"named another load", true, func(load.Report) string { return otherSHA }, http.StatusUnprocessableEntity, false},
		{"named with nothing loaded", false, func(load.Report) string { return otherSHA }, http.StatusUnprocessableEntity, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			var report load.Report
			if tc.load {
				report = ps.load(valuesName, wrappedValues)
			}
			path := filepath.Join(t.TempDir(), profileName)
			status, raw, got := ps.save(saveRequest{Path: path, SourceSHA256: tc.sha(report), Profile: profile.Profile{Target: completeTarget()}})
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			if status != http.StatusOK {
				return
			}
			onDisk, err := profile.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if (onDisk.Source != nil) != tc.wantSource || (got.Profile.Source != nil) != tc.wantSource {
				t.Fatalf("source on disk = %+v, in response = %+v, want present %v", onDisk.Source, got.Profile.Source, tc.wantSource)
			}
			if tc.wantSource {
				want := profile.Source{File: valuesName, Root: "dsc", SHA256: report.SHA256}
				if *onDisk.Source != want {
					t.Errorf("source = %+v, want %+v", *onDisk.Source, want)
				}
			}
		})
	}
}

func TestSaveNeighbourCheck(t *testing.T) {
	tests := []struct {
		name      string
		neighbour string
		wantNote  string
	}{
		{"same bytes", wrappedValues, ""},
		{"missing", "", "overrides will not be restored on reopen: values.yaml is not next to the profile"},
		{"different bytes", wrappedValues + "  extra: 1\n", "overrides will not be restored on reopen: values.yaml differs from the loaded one"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			report := ps.load(valuesName, wrappedValues)
			dir := t.TempDir()
			if tc.neighbour != "" {
				writeBytes(t, filepath.Join(dir, valuesName), tc.neighbour)
			}
			got := ps.mustSave(saveRequest{Path: filepath.Join(dir, profileName), SourceSHA256: report.SHA256,
				Profile: profile.Profile{Target: completeTarget()}})
			var want []string
			if tc.wantNote != "" {
				want = []string{tc.wantNote}
			}
			if !slices.Equal(got.Notes, want) {
				t.Errorf("notes = %q, want %q", got.Notes, want)
			}
		})
	}
}

func TestOpenRestoresSource(t *testing.T) {
	tests := []struct {
		name        string
		replace     func(t *testing.T, values string)
		wantReport  bool
		wantNote    string
		wantChanged bool
	}{
		{name: "unchanged", replace: func(*testing.T, string) {}, wantReport: true},
		{name: "changed", replace: func(t *testing.T, values string) { writeBytes(t, values, wrappedValues+"  extra: 1\n") },
			wantReport: true, wantNote: "overrides restored from values.yaml, which has changed since the profile was saved", wantChanged: true},
		{name: "missing", replace: func(t *testing.T, values string) {
			if err := os.Remove(values); err != nil {
				t.Fatal(err)
			}
		}, wantNote: "overrides not restored: values.yaml is not next to the profile; saving again drops the reference to it"},
		{name: "no longer YAML", replace: func(t *testing.T, values string) { writeBytes(t, values, "a: [1\n") },
			wantNote: "overrides not restored: values.yaml cannot be loaded"},
		{name: "root gone", replace: func(t *testing.T, values string) { writeBytes(t, values, "dsc: 1\n") },
			wantNote: "overrides not restored: values.yaml cannot be loaded"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			dir := t.TempDir()
			values := filepath.Join(dir, valuesName)
			writeBytes(t, values, wrappedValues)
			saved := ps.load(valuesName, wrappedValues)
			path := filepath.Join(dir, profileName)
			ps.mustSave(saveRequest{Path: path, SourceSHA256: saved.SHA256, Profile: profile.Profile{Target: completeTarget()}})
			tc.replace(t, values)
			ps.load("other.yaml", smallValues)

			status, raw, got := ps.open(path)
			if status != http.StatusOK {
				t.Fatalf("status = %d: %s", status, raw)
			}
			if len(got.Notes) > 1 || (tc.wantNote == "") != (len(got.Notes) == 0) ||
				(tc.wantNote != "" && !strings.HasPrefix(got.Notes[0], tc.wantNote)) {
				t.Errorf("notes = %q, want one starting with %q", got.Notes, tc.wantNote)
			}
			slot := loadedSlot(ps.srv)
			if !tc.wantReport {
				if got.Report != nil || slot != nil {
					t.Errorf("report = %v, slot = %v; want both cleared", got.Report, slot)
				}
				return
			}
			if got.Report == nil || slot == nil || slot.Report.SHA256 != got.Report.SHA256 {
				t.Fatalf("report = %v, slot = %v; want the restored load in both", got.Report, slot)
			}
			r := got.Report
			if r.Source != valuesName || r.Root != "dsc" || r.RootFrom != load.RootRequested ||
				r.ChartVersion != latestChart || r.ChartVersionFrom != load.FromRequested {
				t.Errorf("report = %s root %q from %s, chart %s from %s; want %s root dsc requested, chart %s requested",
					r.Source, r.Root, r.RootFrom, r.ChartVersion, r.ChartVersionFrom, valuesName, latestChart)
			}
			if changed := r.SHA256 != saved.SHA256; changed != tc.wantChanged {
				t.Errorf("report hash changed = %v, want %v", changed, tc.wantChanged)
			}
		})
	}
}

func TestOpenRefusesUntrustedProfiles(t *testing.T) {
	const sha = "37b128c59f1f5097f73f82691cb519f1f568667faab5ced1b4ab979d36837eae"
	modelLine := "model: {chart: " + load.DefaultChart + ", version: " + latestChart + ", revision: 1}\n"
	targetLine := "target: {type: file, namespace: dsc, releaseName: p, chart: " + load.DefaultChart + ", chartVersion: " + latestChart + ", environment: development}\n"
	tests := []struct {
		name string
		body string
	}{
		{"source outside the directory", "source: {file: ../" + valuesName + ", root: '', sha256: " + sha + "}\n"},
		{"absolute source", "source: {file: /etc/passwd, root: '', sha256: " + sha + "}\n"},
		{"an answer", "answers: {keycloak.adminPassword: s3cr3t}\n"},
		{"a secret", "secrets: {admin: {mode: inline, keys: [password], fingerprints: {}}}\n"},
		{"an unknown open field", "open: [{field: target.password}]\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			parent := t.TempDir()
			dir := filepath.Join(parent, "profiles")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			writeBytes(t, filepath.Join(parent, valuesName), smallValues)
			path := filepath.Join(dir, profileName)
			writeBytes(t, path, "format: 1\n"+modelLine+targetLine+tc.body)
			ps.load("other.yaml", smallValues)
			before := loadedSlot(ps.srv)

			status, raw, _ := ps.open(path)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", status, raw)
			}
			if loadedSlot(ps.srv) != before {
				t.Error("a refused open changed the loaded slot")
			}
		})
	}
}

func TestOpenBundleRevision(t *testing.T) {
	tests := []struct {
		name     string
		revision int
		want     int
		wantNote string
	}{
		{"older revision", 1, http.StatusOK, "the profile was saved with revision 1 of bundle data-space-connector@2.0.0; this build ships revision 2"},
		{"shipped revision", newerRevision, http.StatusOK, ""},
		{"newer revision", newerRevision + 1, http.StatusUnprocessableEntity, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, builtUI)
			srv.models = twoBundles
			serve(t, srv)
			ps := &profileSession{t: t, srv: srv, cookie: login(t, srv)}
			path := filepath.Join(t.TempDir(), profileName)
			p := profile.Profile{Format: profile.FormatVersion,
				Model:  profile.ModelRef{Chart: load.DefaultChart, Version: newerVersion, Revision: tc.revision},
				Target: profile.Target{Type: profile.TargetFile, Chart: load.DefaultChart, Environment: profile.EnvDevelopment}}
			if err := profile.WriteFile(path, p, false); err != nil {
				t.Fatal(err)
			}
			status, raw, got := ps.open(path)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			if status != http.StatusOK {
				return
			}
			var want []string
			if tc.wantNote != "" {
				want = []string{tc.wantNote}
			}
			if !slices.Equal(got.Notes, want) {
				t.Errorf("notes = %q, want %q", got.Notes, want)
			}
		})
	}
}

func TestSaveModel(t *testing.T) {
	bundles, err := model.Versions(twoBundles)
	if err != nil {
		t.Fatal(err)
	}
	chart := load.DefaultChart
	older := profile.ModelRef{Chart: chart, Version: olderVersion, Revision: 1}
	newer := profile.ModelRef{Chart: chart, Version: newerVersion, Revision: newerRevision}
	tests := []struct {
		name         string
		chart        string
		chartVersion string
		held         profile.ModelRef
		want         profile.ModelRef
	}{
		{name: "chart version wins over the held model", chart: chart, chartVersion: newerVersion, held: older, want: newer},
		{name: "unshipped chart version", chart: chart, chartVersion: "9.9.9", want: profile.ModelRef{Chart: chart, Version: "9.9.9"}},
		{name: "held model keeps its version, with the shipped revision", chart: chart,
			held: profile.ModelRef{Chart: chart, Version: olderVersion, Revision: 7}, want: older},
		{name: "held model of another chart", chart: chart, held: profile.ModelRef{Chart: "other", Version: olderVersion}, want: newer},
		{name: "held model not shipped", chart: chart, held: profile.ModelRef{Chart: chart, Version: "9.9.9"}, want: newer},
		{name: "nothing held", chart: chart, want: newer},
		{name: "unshipped chart", chart: "other", want: profile.ModelRef{Chart: "other"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := saveModel(bundles, profile.Target{Chart: tc.chart, ChartVersion: tc.chartVersion}, tc.held)
			if got != tc.want {
				t.Errorf("saveModel = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSaveAndOpenFillRecentList(t *testing.T) {
	ps := newProfileSession(t)
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"+profile.FileSuffix), filepath.Join(dir, "second"+profile.FileSuffix)
	ps.mustSave(saveRequest{Path: first, Profile: profile.Profile{Target: completeTarget()}})
	ps.mustSave(saveRequest{Path: second, Profile: profile.Profile{Target: completeTarget()}})
	if status, raw, _ := ps.open(first); status != http.StatusOK {
		t.Fatalf("open status = %d: %s", status, raw)
	}
	resp, raw := do(t, ps.srv, request{path: "/api/profile/recent", cookie: ps.cookie})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}
	var got recentResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Path != first || got.Entries[1].Path != second {
		t.Fatalf("entries = %+v, want %s then %s", got.Entries, first, second)
	}
	if e := got.Entries[0]; e.Name != "first" || e.ChartVersion != latestChart {
		t.Errorf("entry = %+v, want name first and chart %s", e, latestChart)
	}
	if got.DefaultDir != dir {
		t.Errorf("defaultDir = %s, want %s", got.DefaultDir, dir)
	}
}

func TestRecentEndpointNeverFails(t *testing.T) {
	tests := []struct {
		name     string
		store    func(t *testing.T) *recent.Store
		wantNote string
	}{
		{"empty store", func(t *testing.T) *recent.Store { return &recent.Store{Dir: t.TempDir()} }, ""},
		{"corrupt store", func(t *testing.T) *recent.Store {
			dir := t.TempDir()
			writeBytes(t, filepath.Join(dir, recent.FileName), "entries: [")
			return &recent.Store{Dir: dir}
		}, "the recent list cannot be read"},
		{"no config directory", func(*testing.T) *recent.Store { return nil }, noteNoRecent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setHome(t, home)
			ps := newProfileSession(t)
			ps.srv.recent = tc.store(t)
			resp, raw := do(t, ps.srv, request{path: "/api/profile/recent", cookie: ps.cookie})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, raw)
			}
			if !strings.Contains(raw, `"entries":[]`) {
				t.Errorf("body %s does not list entries as []", raw)
			}
			var got recentResponse
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatal(err)
			}
			if got.DefaultDir != home {
				t.Errorf("defaultDir = %s, want %s", got.DefaultDir, home)
			}
			if len(got.Notes) > 1 || (tc.wantNote == "") != (len(got.Notes) == 0) ||
				(tc.wantNote != "" && !strings.HasPrefix(got.Notes[0], tc.wantNote)) {
				t.Errorf("notes = %q, want one starting with %q", got.Notes, tc.wantNote)
			}
		})
	}
}

func TestDefaultDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	newest := filepath.Join(home, "newest", profileName)
	tests := []struct {
		name    string
		entries []recent.Entry
		home    string
		want    string
	}{
		{"newest entry", []recent.Entry{{Path: newest}, {Path: filepath.Join(home, "older", profileName)}}, home, filepath.Dir(newest)},
		{"home directory", nil, home, home},
		{"working directory without a home", nil, "", wd},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setHome(t, tc.home)
			if got := defaultDir(tc.entries); got != tc.want {
				t.Errorf("defaultDir = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSavedProfileHoldsNoSecret(t *testing.T) {
	ps := newProfileSession(t)
	token := rand.Text()
	adminPassword, unknownPassword := "s3cr3t-"+token, "hidden-"+token
	values := "keycloak:\n  adminPassword: " + adminPassword + "\nmystery:\n  password: " + unknownPassword + "\n"
	report := ps.load(valuesName, values)
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, valuesName), values)
	path := filepath.Join(dir, profileName)
	got := ps.mustSave(saveRequest{Path: path, SourceSHA256: report.SHA256, Profile: profile.Profile{Target: completeTarget()}})
	if got.Profile.Source == nil {
		t.Fatal("the save references no source, so the test proves nothing")
	}
	data := readBytes(t, path)
	for _, secret := range []string{adminPassword, unknownPassword, token} {
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("profile holds %q:\n%s", secret, data)
		}
	}
}

// setHome points os.UserHomeDir at dir; "" makes it fail.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestSaveCatalogue(t *testing.T) {
	roles := []model.Role{{ID: model.RoleOperator, Chart: trustAnchorChart}}
	const taVersion = "1.0.0"
	bundles := []model.Bundle{
		{Chart: dscChart, Version: newerVersion, Revision: newerRevision, Latest: true, Roles: roles},
		{Chart: dscChart, Version: olderVersion, Revision: 1, Roles: roles},
		{Chart: dscChart, Version: "0.9.0", Revision: 1},
		{Chart: trustAnchorChart, Version: taVersion, Revision: 1, Latest: true},
	}
	ta := profile.ModelRef{Chart: trustAnchorChart, Version: taVersion, Revision: 1}
	newer := &profile.ModelRef{Chart: dscChart, Version: newerVersion, Revision: newerRevision}
	older := &profile.ModelRef{Chart: dscChart, Version: olderVersion, Revision: 1}
	tests := []struct {
		name  string
		model profile.ModelRef
		role  string
		held  *profile.ModelRef
		want  *profile.ModelRef
	}{
		{name: "no role", model: ta, held: newer},
		{name: "model declares the roles", model: *newer, role: model.RoleProvider, held: older},
		{name: "fallback when nothing is held", model: ta, role: model.RoleOperator, want: newer},
		{name: "older held catalogue is kept, with the shipped revision", model: ta, role: model.RoleOperator,
			held: &profile.ModelRef{Chart: dscChart, Version: olderVersion, Revision: 5}, want: older},
		{name: "held catalogue not shipped", model: ta, role: model.RoleOperator, held: &profile.ModelRef{Chart: dscChart, Version: "9.9.9"}, want: newer},
		{name: "held catalogue without roles", model: ta, role: model.RoleOperator, held: &profile.ModelRef{Chart: dscChart, Version: "0.9.0"}, want: newer},
		{name: "held catalogue of another chart", model: ta, role: model.RoleOperator, held: &ta, want: newer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := saveCatalogue(bundles, tc.model, tc.role, tc.held)
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Errorf("saveCatalogue = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSaveRole(t *testing.T) {
	operatorTarget := completeTarget()
	operatorTarget.Chart, operatorTarget.ChartVersion = trustAnchorChart, ""
	enabled := map[string]profile.FeatureState{"ta.registry": {Enabled: true, Source: profile.SourcePreset}}
	dscCatalogue := &profile.ModelRef{Chart: dscChart, Version: latestChart, Revision: 10}
	tests := []struct {
		name          string
		profile       profile.Profile
		want          int
		wantCatalogue *profile.ModelRef
	}{
		{name: "operator without catalogue gets the DSC bundle", want: http.StatusOK, wantCatalogue: dscCatalogue,
			profile: profile.Profile{Target: operatorTarget, Role: model.RoleOperator, Features: enabled}},
		{name: "provider has no catalogue", want: http.StatusOK,
			profile: profile.Profile{Target: completeTarget(), Role: model.RoleProvider, Catalogue: dscCatalogue,
				Features: map[string]profile.FeatureState{"data.scorpio": {Enabled: true, Source: profile.SourcePreset}}}},
		{name: "unknown role", want: http.StatusUnprocessableEntity,
			profile: profile.Profile{Target: completeTarget(), Role: "admin"}},
		{name: "role of another chart", want: http.StatusUnprocessableEntity,
			profile: profile.Profile{Target: completeTarget(), Role: model.RoleOperator}},
		{name: "enabled feature the role does not offer", want: http.StatusUnprocessableEntity,
			profile: profile.Profile{Target: completeTarget(), Role: model.RoleProvider, Features: enabled}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			status, raw, got := ps.save(saveRequest{Path: filepath.Join(t.TempDir(), profileName), Profile: tc.profile})
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			if status != http.StatusOK {
				return
			}
			if c := got.Profile.Catalogue; (c == nil) != (tc.wantCatalogue == nil) || c != nil && *c != *tc.wantCatalogue {
				t.Errorf("catalogue = %+v, want %+v", c, tc.wantCatalogue)
			}
		})
	}
}

func TestSaveRecordsRoleInRecentList(t *testing.T) {
	ps := newProfileSession(t)
	path := filepath.Join(t.TempDir(), profileName)
	ps.mustSave(saveRequest{Path: path, Profile: profile.Profile{Target: completeTarget(), Role: model.RoleProvider}})
	entries, err := ps.srv.recent.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != path || entries[0].Role != model.RoleProvider {
		t.Errorf("entries = %+v, want %s with role %s", entries, path, model.RoleProvider)
	}
}

// TestOpenSeedsLoadedAnswers fills a foundation answer the restored source derives, unless the profile gives one.
func TestOpenSeedsLoadedAnswers(t *testing.T) {
	const (
		answerID = "dataspace.tirAddress"
		loaded   = "https://tir.example.org/"
		given    = "https://tir.given.org/"
		values   = "dsc:\n  decentralizedIam:\n    vcAuthentication:\n      vcverifier:\n        deployment:\n" +
			"          verifier:\n            tirAddress: " + loaded + "\n"
	)
	tests := []struct {
		name    string
		answers map[string]any
		want    string
	}{
		{"unanswered", nil, loaded},
		{"given", map[string]any{answerID: given}, given},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			dir := t.TempDir()
			writeBytes(t, filepath.Join(dir, valuesName), values)
			report := ps.load(valuesName, values)
			path := filepath.Join(dir, profileName)
			ps.mustSave(saveRequest{Path: path, SourceSHA256: report.SHA256,
				Profile: profile.Profile{Target: completeTarget(), Answers: tc.answers}})

			status, raw, got := ps.open(path)
			if status != http.StatusOK {
				t.Fatalf("status = %d: %s", status, raw)
			}
			if got.Profile.Answers[answerID] != tc.want {
				t.Errorf("answers = %v, want %s = %s", got.Profile.Answers, answerID, tc.want)
			}
		})
	}
}

// TestOpenedProfilePassesTIRRule renders an opened profile whose source sets the TIR address and whose answers do not:
// the seeded answer keeps tir-address-for-verification from blocking it.
func TestOpenedProfilePassesTIRRule(t *testing.T) {
	ps := newProfileSession(t)
	dir := t.TempDir()
	writeBytes(t, filepath.Join(dir, valuesName), tirValues)
	report := ps.load(valuesName, tirValues)
	path := filepath.Join(dir, profileName)
	ps.mustSave(saveRequest{Path: path, SourceSHA256: report.SHA256, Profile: verificationProfile(true)})

	status, raw, got := ps.open(path)
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, raw)
	}
	if !got.Profile.Features["access.verification"].Enabled {
		t.Fatalf("features = %v, want credential verification on", got.Profile.Features)
	}
	if rules := findingRules(ps.mustRender(got.Profile, report.SHA256).Findings); len(rules) != 0 {
		t.Errorf("findings = %v, want none", rules)
	}
}
