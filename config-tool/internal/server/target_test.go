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

const kubeconfigEnv = "KUBECONFIG"

func TestModel(t *testing.T) {
	bundle := func(version string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("chart: data-space-connector\nversion: " + version + "\n")}
	}
	tests := []struct {
		name     string
		models   fstest.MapFS
		want     int
		wantBody modelBody
	}{
		{
			name:   "versions highest first",
			models: fstest.MapFS{"10.9.0/bundle.yaml": bundle("10.9.0"), "10.16.0/bundle.yaml": bundle("10.16.0")},
			want:   http.StatusOK,
			wantBody: modelBody{
				Charts:   []string{"data-space-connector"},
				Versions: []modelVersion{{Version: "10.16.0", Latest: true}, {Version: "10.9.0"}},
			},
		},
		{name: "no bundles", models: fstest.MapFS{}, want: http.StatusOK, wantBody: modelBody{Charts: []string{}, Versions: []modelVersion{}}},
		{name: "broken bundle", models: fstest.MapFS{"10.16.0/bundle.yaml": bundle("10.15.0")}, want: http.StatusInternalServerError},
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
	if len(got.Versions) == 0 || !got.Versions[0].Latest {
		t.Errorf("body = %+v, want the embedded versions with the first marked latest", got)
	}
}

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
