package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/cluster"
)

const (
	kubeconfigEnv    = "KUBECONFIG"
	dscChart         = "data-space-connector"
	trustAnchorChart = "trust-anchor"
)

func TestModel(t *testing.T) {
	bundle := func(chart, version string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("chart: " + chart + "\nversion: " + version + "\n")}
	}
	tests := []struct {
		name     string
		models   fstest.MapFS
		want     int
		wantBody modelBody
	}{
		{
			name:   "versions highest first",
			models: fstest.MapFS{"10.9.0/bundle.yaml": bundle(dscChart, "10.9.0"), "10.16.0/bundle.yaml": bundle(dscChart, "10.16.0")},
			want:   http.StatusOK,
			wantBody: modelBody{Charts: []modelChart{
				{Name: dscChart, Versions: []modelVersion{{Version: "10.16.0", Latest: true}, {Version: "10.9.0"}}},
			}},
		},
		{
			name:   "versions keyed by chart",
			models: fstest.MapFS{"10.16.0/bundle.yaml": bundle(dscChart, "10.16.0"), "0.2.0/bundle.yaml": bundle(trustAnchorChart, "0.2.0")},
			want:   http.StatusOK,
			wantBody: modelBody{Charts: []modelChart{
				{Name: dscChart, Versions: []modelVersion{{Version: "10.16.0", Latest: true}}},
				{Name: trustAnchorChart, Versions: []modelVersion{{Version: "0.2.0", Latest: true}}},
			}},
		},
		{name: "no bundles", models: fstest.MapFS{}, want: http.StatusOK, wantBody: modelBody{Charts: []modelChart{}}},
		{name: "broken bundle", models: fstest.MapFS{"10.16.0/bundle.yaml": bundle(dscChart, "10.15.0")}, want: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, builtUI)
			srv.models = tc.models
			serve(t, srv)
			resp, body := do(t, srv, request{path: "/api/model", bearer: srv.token})
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
			if tc.want != http.StatusOK {
				return
			}
			var got modelBody
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.wantBody) {
				t.Errorf("body = %+v, want %+v", got, tc.wantBody)
			}
		})
	}
}

func TestModelServesTheEmbeddedBundles(t *testing.T) {
	srv := started(t, builtUI)
	_, body := do(t, srv, request{path: "/api/model", bearer: srv.token})
	var got modelBody
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Charts) == 0 || got.Charts[0].Name != dscChart || len(got.Charts[0].Versions) == 0 || !got.Charts[0].Versions[0].Latest {
		t.Errorf("body = %+v, want the embedded %s versions with the first marked latest", got, dscChart)
	}
}

// oversizedKubeconfigBytes is above the 8 MiB read cap of internal/cluster.
const oversizedKubeconfigBytes = 9 << 20

func TestClusterContexts(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "config")
	const contents = `current-context: dev
clusters:
- name: dev
  cluster:
    server: https://secret-server.example.invalid
users:
- name: dev
  user:
    token: secret-token
contexts:
- name: dev
  context: {cluster: dev, user: dev}
- name: prod
  context: {cluster: dev, user: dev}
`
	if err := os.WriteFile(kubeconfig, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	malformed := filepath.Join(dir, "malformed")
	if err := os.WriteFile(malformed, []byte("contexts: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Sparse, so the test writes nothing; the size alone exceeds the cluster package's cap.
	oversized := filepath.Join(dir, "oversized")
	if err := os.WriteFile(oversized, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(oversized, oversizedKubeconfigBytes); err != nil {
		t.Fatal(err)
	}
	found := cluster.Kubeconfig{Paths: []string{kubeconfig}, Contexts: []string{"dev", "prod"}, Current: "dev"}
	tests := []struct {
		name     string
		env      string
		explicit string
		want     int
		wantBody cluster.Kubeconfig
	}{
		{name: "default rules read KUBECONFIG", env: kubeconfig, want: http.StatusOK, wantBody: found},
		{name: "default rules without a file", env: filepath.Join(dir, "missing"), want: http.StatusOK, wantBody: cluster.Kubeconfig{Paths: []string{}, Contexts: []string{}}},
		{name: "explicit path", env: filepath.Join(dir, "missing"), explicit: kubeconfig, want: http.StatusOK, wantBody: found},
		{name: "missing explicit path", explicit: filepath.Join(dir, "missing"), want: http.StatusNotFound},
		{name: "malformed explicit kubeconfig", explicit: malformed, want: http.StatusUnprocessableEntity},
		{name: "oversized explicit kubeconfig", explicit: oversized, want: http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(kubeconfigEnv, tc.env)
			srv := started(t, builtUI)
			path := "/api/cluster/contexts"
			if tc.explicit != "" {
				path += "?" + url.Values{queryKubeconfig: {tc.explicit}}.Encode()
			}
			resp, body := do(t, srv, request{path: path, bearer: srv.token})
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
			if strings.Contains(body, "secret-") {
				t.Errorf("body leaks credentials or server URLs: %s", body)
			}
			if tc.want != http.StatusOK {
				var e errorBody
				if err := json.Unmarshal([]byte(body), &e); err != nil || e.Error == "" {
					t.Errorf("body %q is not a JSON error", body)
				}
				return
			}
			var got cluster.Kubeconfig
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.wantBody) {
				t.Errorf("body = %+v, want %+v", got, tc.wantBody)
			}
		})
	}
}
