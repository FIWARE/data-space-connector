package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/dataspace"
	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

const (
	dsBase        = "https://ds.example.org"
	dsDescriptor  = dsBase + descriptorPath
	dsMirror      = "https://mirror.example.org" + descriptorPath
	dsPlain       = "http://ds.example.org" + descriptorPath
	tirURL        = "https://tir.example.org/"
	dialError     = "connection refused"
	testProxy     = "http://proxy.example.org:3128"
	proxiedHost   = "https://tir.example.org/"
	excludedHost  = "https://excluded.example.org/"
	excludedProxy = "excluded.example.org"
	// dsDocument is a descriptor with all three imported keys, the chart's keys and one unused key.
	dsDocument = `{"supported_protocols": ["https"], "tir_address": "` + tirURL + `",
		"credential_types": [{"type": "UserCredential", "description": "A natural person"}, "OperatorCredential"],
		"participant_registration": "automatic", "x_contact": "ops@example.org"}`
)

// stubResponse is a canned answer of stubTransport; a set err fails the round trip instead.
type stubResponse struct {
	status   int
	body     string
	location string
	err      error
}

// stubTransport serves fixtures by URL and records every request, so no test reaches the network.
type stubTransport struct {
	fixtures map[string]stubResponse
	mu       sync.Mutex
	calls    []string
}

func (st *stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	st.mu.Lock()
	st.calls = append(st.calls, r.URL.String())
	st.mu.Unlock()
	f, ok := st.fixtures[r.URL.String()]
	if !ok {
		return nil, errors.New("no fixture")
	}
	if f.err != nil {
		return nil, f.err
	}
	h := http.Header{}
	if f.location != "" {
		h.Set("Location", f.location)
	}
	return &http.Response{StatusCode: f.status, Header: h, Body: io.NopCloser(strings.NewReader(f.body)), Request: r}, nil
}

func (st *stubTransport) callCount() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.calls)
}

// roundTripperFunc adapts a func to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// outboundSession is a logged-in session whose server sends outbound calls to rt.
func outboundSession(t *testing.T, rt http.RoundTripper) *profileSession {
	t.Helper()
	srv := newServer(t, builtUI)
	srv.outbound = rt
	serve(t, srv)
	return &profileSession{t: t, srv: srv, cookie: login(t, srv)}
}

// post sends body as JSON to path and returns the status and the raw response.
func (ps *profileSession) post(path string, body any) (int, string) {
	ps.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		ps.t.Fatal(err)
	}
	resp, raw := do(ps.t, ps.srv, request{method: http.MethodPost, path: path, origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(data)})
	return resp.StatusCode, raw
}

func TestDataspaceCheck(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		fixture    stubResponse
		wantCode   int
		want       dataspaceCheckResponse
		wantCalls  int
		wantErrMsg string
	}{
		{"reachable", tirURL, stubResponse{status: http.StatusOK}, http.StatusOK,
			dataspaceCheckResponse{Status: reachStatusOK, HTTPStatus: http.StatusOK}, 1, ""},
		{"not found is reachable", tirURL, stubResponse{status: http.StatusNotFound}, http.StatusOK,
			dataspaceCheckResponse{Status: reachStatusOK, HTTPStatus: http.StatusNotFound}, 1, ""},
		{"transport error", tirURL, stubResponse{err: errors.New(dialError)}, http.StatusOK,
			dataspaceCheckResponse{Status: reachStatusFailed, Error: dialError}, 1, ""},
		{"redirect not followed", tirURL, stubResponse{status: http.StatusFound, location: dsBase}, http.StatusOK,
			dataspaceCheckResponse{Status: reachStatusOK, HTTPStatus: http.StatusFound}, 1, ""},
		{"invalid url", "ftp://tir.example.org/", stubResponse{}, http.StatusUnprocessableEntity,
			dataspaceCheckResponse{}, 0, "must be an absolute http or https URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := &stubTransport{fixtures: map[string]stubResponse{tc.url: tc.fixture}}
			ps := outboundSession(t, st)
			status, raw := ps.post("/api/dataspace/check", dataspaceCheckRequest{URL: tc.url})
			if status != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", status, tc.wantCode, raw)
			}
			if n := st.callCount(); n != tc.wantCalls {
				t.Errorf("outbound calls = %d, want %d", n, tc.wantCalls)
			}
			if tc.wantErrMsg != "" {
				if !strings.Contains(raw, tc.wantErrMsg) {
					t.Errorf("body = %s, want it to contain %q", raw, tc.wantErrMsg)
				}
				return
			}
			var got dataspaceCheckResponse
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("body %q: %v", raw, err)
			}
			if got != tc.want {
				t.Errorf("response = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDescriptorURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{dsBase, dsDescriptor},
		{dsBase + "/", dsDescriptor},
		{dsDescriptor, dsDescriptor},
		{dsBase + "/space", dsBase + "/space" + descriptorPath},
		{dsBase + "/space" + descriptorPath, dsBase + "/space" + descriptorPath},
		{dsBase + "?tenant=a", dsDescriptor + "?tenant=a"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := descriptorURL(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("descriptorURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDataspaceImport(t *testing.T) {
	ok := stubResponse{status: http.StatusOK, body: dsDocument}
	tests := []struct {
		name     string
		url      string
		fixtures map[string]stubResponse
		wantCode int
		// wantMsg is a substring of the error, or of the preview's message on 200.
		wantMsg   string
		wantCalls int
	}{
		{"base url", dsBase, map[string]stubResponse{dsDescriptor: ok}, http.StatusOK, "", 1},
		{"full url", dsDescriptor, map[string]stubResponse{dsDescriptor: ok}, http.StatusOK, "", 1},
		{"same-scheme redirect", dsBase, map[string]stubResponse{
			dsDescriptor: {status: http.StatusMovedPermanently, location: dsMirror}, dsMirror: ok}, http.StatusOK, "", 2},
		{"nothing to import", dsBase, map[string]stubResponse{dsDescriptor: {status: http.StatusOK, body: `{"x_contact": "a"}`}},
			http.StatusOK, dataspace.NothingToImport, 1},
		{"https to http redirect", dsBase, map[string]stubResponse{
			dsDescriptor: {status: http.StatusFound, location: dsPlain}, dsPlain: ok}, http.StatusBadGateway,
			"redirected from https to http", 1},
		{"redirect loop", dsBase, map[string]stubResponse{dsDescriptor: {status: http.StatusFound, location: dsDescriptor}},
			http.StatusBadGateway, errTooManyRedirects.Error(), maxImportRedirects + 1},
		{"not json", dsBase, map[string]stubResponse{dsDescriptor: {status: http.StatusOK, body: "<html>"}},
			http.StatusBadGateway, "not valid JSON", 1},
		{"oversized", dsBase, map[string]stubResponse{dsDescriptor: {status: http.StatusOK,
			body: `"` + strings.Repeat("a", dataspace.MaxDescriptorBytes)}}, http.StatusBadGateway, "larger than", 1},
		{"server error", dsBase, map[string]stubResponse{dsDescriptor: {status: http.StatusInternalServerError, body: dsDocument}},
			http.StatusBadGateway, "HTTP 500", 1},
		{"transport error", dsBase, map[string]stubResponse{dsDescriptor: {err: errors.New(dialError)}},
			http.StatusBadGateway, dialError, 1},
		{"invalid url", "ds.example.org", nil, http.StatusUnprocessableEntity, "must be an absolute", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := &stubTransport{fixtures: tc.fixtures}
			ps := outboundSession(t, st)
			status, raw := ps.post("/api/dataspace/import", dataspaceImportRequest{URL: tc.url, Profile: domainProfile(renderDomain)})
			if status != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", status, tc.wantCode, raw)
			}
			if n := st.callCount(); n != tc.wantCalls {
				t.Errorf("outbound calls = %d (%v), want %d", n, st.calls, tc.wantCalls)
			}
			if status != http.StatusOK {
				if !strings.Contains(raw, tc.wantMsg) || strings.Contains(raw, "ds.example.org") {
					t.Errorf("body = %s, want %q and no URL", raw, tc.wantMsg)
				}
				return
			}
			var got dataspaceImportResponse
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("body %q: %v", raw, err)
			}
			if got.URL != dsDescriptor {
				t.Errorf("url = %q, want %q", got.URL, dsDescriptor)
			}
			if got.Preview.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", got.Preview.Message, tc.wantMsg)
			}
		})
	}
}

// TestDataspaceImportPreview checks the preview of the fixture document against a profile that answers nothing.
func TestDataspaceImportPreview(t *testing.T) {
	ps := outboundSession(t, &stubTransport{fixtures: map[string]stubResponse{dsDescriptor: {status: http.StatusOK, body: dsDocument}}})
	status, raw := ps.post("/api/dataspace/import", dataspaceImportRequest{URL: dsBase, Profile: domainProfile(renderDomain)})
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, raw)
	}
	var got dataspaceImportResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	imported := map[string]string{}
	for _, c := range got.Preview.Changes {
		data, err := json.Marshal(c.Imported)
		if err != nil {
			t.Fatal(err)
		}
		imported[c.ID] = string(data)
	}
	want := map[string]string{
		dataspace.TIRAddressID:       `"` + tirURL + `"`,
		dataspace.CredentialTypesID:  `[{"description":"A natural person","type":"UserCredential"},{"type":"OperatorCredential"}]`,
		dataspace.AutoRegistrationID: "true",
	}
	if len(imported) != len(want) {
		t.Errorf("changes = %v, want %v", imported, want)
	}
	for id, v := range want {
		if imported[id] != v {
			t.Errorf("%s = %s, want %s", id, imported[id], v)
		}
	}
	if len(got.Preview.Problems) != 0 {
		t.Errorf("problems = %v, want none", got.Preview.Problems)
	}
	if strings.Join(got.Preview.Unused, ",") != "x_contact" {
		t.Errorf("unused = %v, want [x_contact]", got.Preview.Unused)
	}
}

func TestOutboundProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", testProxy)
	t.Setenv("HTTP_PROXY", testProxy)
	t.Setenv("NO_PROXY", excludedProxy)
	tr, ok := newOutbound().(*http.Transport)
	if !ok {
		t.Fatal("newOutbound is not an *http.Transport")
	}
	tests := []struct {
		url, want string
	}{
		{proxiedHost, testProxy},
		{excludedHost, ""},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := tr.Proxy(req)
			if err != nil {
				t.Fatal(err)
			}
			gotURL := ""
			if got != nil {
				gotURL = got.String()
			}
			if gotURL != tc.want {
				t.Errorf("proxy = %q, want %q", gotURL, tc.want)
			}
		})
	}
}

// TestNoUnaskedNetwork drives every other API route against a server whose outbound transport fails the test.
func TestNoUnaskedNetwork(t *testing.T) {
	// Errorf, not Fatal: the transport runs on a handler goroutine.
	ps := outboundSession(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unasked outbound call to %s", r.URL.Host)
		return nil, errors.New("no network in this test")
	}))
	dir := t.TempDir()
	profilePath := filepath.Join(dir, profileName)
	featured := verificationProfile(true)
	tests := []struct {
		method, path string
		body         any
		want         int
	}{
		{http.MethodGet, "/api/model", nil, http.StatusOK},
		{http.MethodGet, foundationsPath(load.DefaultChart, latestChart), nil, http.StatusOK},
		{http.MethodGet, rolesPath(load.DefaultChart, latestChart), nil, http.StatusOK},
		{http.MethodPost, "/api/render", renderRequest{Profile: featured}, http.StatusOK},
		{http.MethodPut, "/api/profile", saveRequest{Path: profilePath, Profile: featured}, http.StatusOK},
		{http.MethodPost, "/api/profile/open", openRequest{Path: profilePath}, http.StatusOK},
		{http.MethodGet, "/api/profile/recent", nil, http.StatusOK},
		{http.MethodPost, "/api/preset", presetRequest{Profile: domainProfile(renderDomain), Role: model.RoleProvider}, http.StatusOK},
		{http.MethodPost, "/api/features", featuresRequest{Profile: featured}, http.StatusOK},
		{http.MethodGet, "/api/trial", nil, http.StatusOK},
		{http.MethodPost, "/api/trial/write", trialWriteRequest{Dir: t.TempDir()}, http.StatusCreated},
		{http.MethodGet, "/api/cluster/contexts?" + queryKubeconfig + "=" + filepath.Join(dir, "missing"), nil, http.StatusNotFound},
		{http.MethodPost, "/api/dataspace/check", dataspaceCheckRequest{URL: "not a url"}, http.StatusUnprocessableEntity},
		{http.MethodPost, "/api/dataspace/import", dataspaceImportRequest{URL: dsBase}, http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			body := ""
			if tc.body != nil {
				data, err := json.Marshal(tc.body)
				if err != nil {
					t.Fatal(err)
				}
				body = string(data)
			}
			resp, raw := do(t, ps.srv, request{method: tc.method, path: tc.path, origin: ownOrigin(ps.srv), cookie: ps.cookie, body: body})
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d: %.200s", resp.StatusCode, tc.want, raw)
			}
		})
	}
	// Raw bodies: a values file that sets a TIR address, and the validate stub.
	if status, raw, _ := postLoad(t, ps.srv, ps.cookie, valuesName, tirValues); status != http.StatusOK {
		t.Errorf("load status = %d: %s", status, raw)
	}
	resp, raw := do(t, ps.srv, request{method: http.MethodPost, path: "/api/validate", origin: ownOrigin(ps.srv), cookie: ps.cookie, body: tirValues})
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("validate status = %d: %s", resp.StatusCode, raw)
	}
}
