package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
)

const (
	providerFile = "../../../k3s/provider.yaml"
	latestChart  = "10.16.0"
	utf8BOM      = "\uFEFF"
	smallValues  = "a: 1\n"
)

// loadPath is the load endpoint with name as its query, unless name is empty.
func loadPath(name string) string {
	if name == "" {
		return "/api/load"
	}
	return "/api/load?" + url.Values{queryName: {name}}.Encode()
}

// postLoad uploads body as name and returns the status, the raw body and, on 200, the report.
func postLoad(t *testing.T, srv *Server, cookie *http.Cookie, name, body string) (int, string, load.Report) {
	t.Helper()
	resp, raw := do(t, srv, request{method: http.MethodPost, path: loadPath(name), origin: ownOrigin(srv), cookie: cookie, body: body})
	var report load.Report
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &report); err != nil {
			t.Fatalf("body %q is not a report: %v", raw, err)
		}
	}
	return resp.StatusCode, raw, report
}

// loadedSlot reads the server's loaded slot under its lock.
func loadedSlot(srv *Server) *load.Result {
	srv.loadMu.Lock()
	defer srv.loadMu.Unlock()
	return srv.loaded
}

func TestLoadEndpoint(t *testing.T) {
	provider, err := os.ReadFile(providerFile)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		file       string
		body       string
		unauth     bool
		models     fstest.MapFS
		want       int
		wantSource string
	}{
		{name: "k3s provider", file: "provider.yaml", body: string(provider), want: http.StatusOK, wantSource: "provider.yaml"},
		{name: "posix path reduced", file: "/home/me/k3s/provider.yaml", body: smallValues, want: http.StatusOK, wantSource: "provider.yaml"},
		{name: "windows path reduced", file: `C:\Users\me\my values.yaml`, body: smallValues, want: http.StatusOK, wantSource: "my values.yaml"},
		{name: "no name", body: smallValues, want: http.StatusOK, wantSource: defaultLoadName},
		{name: "directory only", file: "k3s/", body: smallValues, want: http.StatusOK, wantSource: defaultLoadName},
		{name: "parent directory", file: "k3s/..", body: smallValues, want: http.StatusOK, wantSource: defaultLoadName},
		{name: "invalid yaml", body: "a: [1\n", want: http.StatusUnprocessableEntity},
		{name: "multi-document", body: "a: 1\n---\nb: 2\n", want: http.StatusUnprocessableEntity},
		{name: "above the cap", body: oversized, want: http.StatusRequestEntityTooLarge},
		{name: "no token", body: smallValues, unauth: true, want: http.StatusUnauthorized},
		{name: "no bundle", body: smallValues, models: fstest.MapFS{}, want: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, builtUI)
			if tc.models != nil {
				srv.models = tc.models
			}
			serve(t, srv)
			var cookie *http.Cookie
			if !tc.unauth {
				cookie = login(t, srv)
			}
			status, raw, report := postLoad(t, srv, cookie, tc.file, tc.body)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, raw)
			}
			if tc.want != http.StatusOK {
				if got := loadedSlot(srv); got != nil {
					t.Errorf("failed load filled the slot with %q", got.Report.Source)
				}
				return
			}
			if report.Source != tc.wantSource || report.Chart != load.DefaultChart || report.ChartVersion != latestChart {
				t.Errorf("report = %s %s %s, want %s %s %s", report.Source, report.Chart, report.ChartVersion,
					tc.wantSource, load.DefaultChart, latestChart)
			}
			if report.Counts.Total == 0 {
				t.Error("report counts no leaves")
			}
			if got := loadedSlot(srv); got == nil || got.Report.Source != tc.wantSource {
				t.Errorf("slot = %+v, want the load of %s", got, tc.wantSource)
			}
		})
	}
}

func TestLoadReplacesSlot(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	steps := []struct {
		file       string
		body       string
		want       int
		wantSource string
	}{
		{file: "first.yaml", body: smallValues, want: http.StatusOK, wantSource: "first.yaml"},
		{file: "second.yaml", body: "b: 2\n", want: http.StatusOK, wantSource: "second.yaml"},
		{file: "broken.yaml", body: "a: [1\n", want: http.StatusUnprocessableEntity, wantSource: "second.yaml"},
	}
	for _, step := range steps {
		if status, raw, _ := postLoad(t, srv, cookie, step.file, step.body); status != step.want {
			t.Fatalf("%s: status = %d, want %d: %s", step.file, status, step.want, raw)
		}
		if got := loadedSlot(srv); got == nil || got.Report.Source != step.wantSource {
			t.Errorf("after %s: slot = %+v, want the load of %s", step.file, got, step.wantSource)
		}
	}
}

func TestLoadKeepsUploadedBytes(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	src := utf8BOM + "# kept\r\nb: 2 # line\r\na:\r\n  c: x\r\n"
	if status, raw, _ := postLoad(t, srv, cookie, "bom.yaml", src); status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, raw)
	}
	got, err := loadedSlot(srv).Doc.Write()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("Write() = %q, want the uploaded %q", got, src)
	}
}
