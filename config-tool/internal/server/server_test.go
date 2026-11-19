package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
)

const (
	indexHTML   = "<!doctype html><title>spa</title>"
	appJS       = "console.log('app')"
	metaRefresh = `<meta http-equiv="refresh" content="0;url=/">`
	testTimeout = 5 * time.Second
	// maxNameBody keeps the oversized body out of subtest names.
	maxNameBody = 20
)

// oversized is one byte over the body cap, so the server rejects it without a reset.
var oversized = `"` + strings.Repeat("a", maxBodyBytes)

var builtUI = fstest.MapFS{
	indexFile:       {Data: []byte(indexHTML)},
	"assets/app.js": {Data: []byte(appJS)},
}

var client = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Timeout:       testTimeout,
}

// newServer creates a server that is closed at the end of the test but not yet serving.
func newServer(t *testing.T, ui fs.FS) *Server {
	t.Helper()
	srv, err := New(ui)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

// serve runs srv until the end of the test and returns a func that stops it and its result.
func serve(t *testing.T, srv *Server) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ctx) }()
	var result error
	var stopped atomic.Bool
	stop := func() error {
		if stopped.CompareAndSwap(false, true) {
			cancel()
			select {
			case result = <-errc:
			case <-time.After(testTimeout):
				result = errors.New("Serve did not return after cancel")
			}
		}
		return result
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

func started(t *testing.T, ui fs.FS) *Server {
	t.Helper()
	srv := newServer(t, ui)
	serve(t, srv)
	return srv
}

type request struct {
	method string
	path   string
	host   string
	origin string
	cookie *http.Cookie
	bearer string
	body   string
	site   string
}

func do(t *testing.T, srv *Server, r request) (*http.Response, string) {
	t.Helper()
	method := r.method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, "http://"+srv.Addr().String()+r.path, strings.NewReader(r.body))
	if err != nil {
		t.Fatal(err)
	}
	if r.host != "" {
		req.Host = r.host
	}
	if r.origin != "" {
		req.Header.Set(headerOrigin, r.origin)
	}
	if r.cookie != nil {
		req.AddCookie(r.cookie)
	}
	if r.bearer != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+r.bearer)
	}
	if r.site != "" {
		req.Header.Set(headerSecFetchSite, r.site)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func pathAndQuery(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.RequestURI()
}

func sessionCookie(t *testing.T, srv *Server, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == srv.cookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", srv.cookieName, resp.Header.Values("Set-Cookie"))
	return nil
}

// login exchanges the run token for a session cookie.
func login(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	resp, _ := do(t, srv, request{path: pathAndQuery(t, srv.URL())})
	return sessionCookie(t, srv, resp)
}

func ownOrigin(srv *Server) string {
	return "http://" + srv.Addr().String()
}

func otherPort(srv *Server) string {
	return strconv.Itoa(srv.port + 1)
}

func assertExchange(t *testing.T, srv *Server, resp *http.Response, body string) *http.Cookie {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	for header, want := range map[string]string{headerCacheControl: "no-store", headerReferrerPolicy: "no-referrer"} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if !strings.Contains(body, metaRefresh) {
		t.Errorf("body has no meta refresh to /:\n%s", body)
	}
	c := sessionCookie(t, srv, resp)
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != pathRoot {
		t.Errorf("cookie = %+v, want HttpOnly, SameSite=Strict, Path=/", c)
	}
	if c.Value == srv.token {
		t.Error("cookie holds the run token, want a separate session secret")
	}
	return c
}

func TestListensOnLoopback(t *testing.T) {
	srv := newServer(t, builtUI)
	addr := srv.Addr().(*net.TCPAddr)
	if !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("listener IP = %s, want 127.0.0.1", addr.IP)
	}
	if !strings.HasPrefix(srv.URL(), "http://127.0.0.1:"+strconv.Itoa(addr.Port)+"/?token=") {
		t.Errorf("URL = %q", srv.URL())
	}
}

func TestRequiresCredentials(t *testing.T) {
	srv := started(t, builtUI)
	tests := []request{
		{path: "/"},
		{path: "/assets/app.js"},
		{path: "/some/route"},
		{path: "/api/model"},
		{path: "/api/cluster/contexts"},
		{path: "/api/profile"},
		{method: http.MethodPut, path: "/api/profile"},
		{method: http.MethodPost, path: "/api/render"},
		{method: http.MethodPost, path: "/api/validate"},
		{method: http.MethodPost, path: "/api/load"},
		{path: pathWS},
		{path: "/?token=wrong"},
		{path: "/?token="},
		{path: "/?launch=wrong"},
		{path: "/?token=" + srv.token, method: http.MethodPost},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp, _ := do(t, srv, tc)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
			if len(resp.Cookies()) != 0 {
				t.Errorf("cookies = %v, want none", resp.Cookies())
			}
		})
	}
}

func TestUnauthorizedRootExplainsTheLink(t *testing.T) {
	srv := started(t, builtUI)
	resp, body := do(t, srv, request{path: "/"})
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "link printed by") {
		t.Errorf("status = %d, body = %q", resp.StatusCode, body)
	}
}

func TestTokenExchangeIsRepeatable(t *testing.T) {
	srv := started(t, builtUI)
	seen := map[string]bool{}
	for i := range 3 {
		resp, body := do(t, srv, request{path: pathAndQuery(t, srv.URL())})
		c := assertExchange(t, srv, resp, body)
		if seen[c.Value] {
			t.Errorf("exchange %d reused a session secret", i)
		}
		seen[c.Value] = true
		if resp, _ := do(t, srv, request{path: "/api/model", cookie: c}); resp.StatusCode != http.StatusOK {
			t.Errorf("exchange %d: cookie gives status %d, want %d", i, resp.StatusCode, http.StatusOK)
		}
	}
}

func TestCookiesArePerInstance(t *testing.T) {
	a, b := started(t, builtUI), started(t, builtUI)
	if a.cookieName == b.cookieName {
		t.Fatalf("both servers use cookie %q", a.cookieName)
	}
	cookieA := login(t, a)
	tests := []struct {
		name   string
		cookie *http.Cookie
	}{
		{"own name", cookieA},
		{"renamed to the other port", &http.Cookie{Name: b.cookieName, Value: cookieA.Value}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if resp, _ := do(t, b, request{path: "/api/model", cookie: tc.cookie}); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestBearer(t *testing.T) {
	srv := started(t, builtUI)
	session := login(t, srv)
	launch, err := srv.LaunchURL()
	if err != nil {
		t.Fatal(err)
	}
	launchCode, _ := url.Parse(launch)
	tests := []struct {
		name   string
		bearer string
		cookie *http.Cookie
		want   int
	}{
		{"run token", srv.token, nil, http.StatusOK},
		{"session secret", session.Value, nil, http.StatusUnauthorized},
		{"launch code", launchCode.Query().Get(queryLaunch), nil, http.StatusUnauthorized},
		{"wrong value", "wrong", nil, http.StatusUnauthorized},
		{"wrong value beats a valid cookie", "wrong", session, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := do(t, srv, request{path: "/api/model", bearer: tc.bearer, cookie: tc.cookie})
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestEndpoints(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	origin := ownOrigin(srv)
	tests := []struct {
		req        request
		want       int
		wantTicket string
	}{
		{request{path: "/api/profile"}, http.StatusNotImplemented, ticketProfile},
		{request{method: http.MethodPut, path: "/api/profile", origin: origin}, http.StatusNotImplemented, ticketProfile},
		{request{method: http.MethodPost, path: "/api/render", origin: origin}, http.StatusNotImplemented, ticketRender},
		{request{method: http.MethodPost, path: "/api/render", origin: origin, body: `{"target":{"namespace":"x"}}`}, http.StatusNotImplemented, ticketRender},
		{request{method: http.MethodPost, path: "/api/render", origin: origin, body: "{"}, http.StatusBadRequest, ""},
		{request{method: http.MethodPost, path: "/api/render", origin: origin, body: "{}xyz"}, http.StatusBadRequest, ""},
		{request{method: http.MethodPost, path: "/api/render", origin: origin, body: "{} {}"}, http.StatusBadRequest, ""},
		{request{method: http.MethodPost, path: "/api/render", origin: origin, body: "{}\n"}, http.StatusNotImplemented, ticketRender},
		{request{method: http.MethodPost, path: "/api/render", origin: origin, body: oversized}, http.StatusRequestEntityTooLarge, ""},
		{request{method: http.MethodPost, path: "/api/validate", origin: origin, body: "a: 1\n"}, http.StatusNotImplemented, ""},
		{request{method: http.MethodPost, path: "/api/validate", origin: origin, body: oversized}, http.StatusRequestEntityTooLarge, ""},
		{request{path: "/api/unknown"}, http.StatusNotFound, ""},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s %s %.*s", tc.req.method, tc.req.path, maxNameBody, tc.req.body), func(t *testing.T) {
			tc.req.cookie = cookie
			resp, body := do(t, srv, tc.req)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
			var got errorBody
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("body %q is not a JSON error: %v", body, err)
			}
			if got.Error == "" || got.Ticket != tc.wantTicket {
				t.Errorf("error body = %+v, want a message and ticket %q", got, tc.wantTicket)
			}
		})
	}
}

func TestHostGuard(t *testing.T) {
	srv := started(t, builtUI)
	port := strconv.Itoa(srv.port)
	tests := []struct {
		host string
		want int
	}{
		{"127.0.0.1:" + port, http.StatusOK},
		{"localhost:" + port, http.StatusOK},
		{"LOCALHOST:" + port, http.StatusOK},
		{"evil.example:" + port, http.StatusForbidden},
		{"evil.example", http.StatusForbidden},
		{"127.0.0.1:" + otherPort(srv), http.StatusForbidden},
		{"127.0.0.1", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			resp, _ := do(t, srv, request{path: "/api/model", host: tc.host, bearer: srv.token})
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
	t.Run("token exchange", func(t *testing.T) {
		resp, _ := do(t, srv, request{path: pathAndQuery(t, srv.URL()), host: "evil.example:" + port})
		if resp.StatusCode != http.StatusForbidden || len(resp.Cookies()) != 0 {
			t.Errorf("status = %d, cookies = %v, want 403 and no cookie", resp.StatusCode, resp.Cookies())
		}
	})
}

func TestOriginGuard(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	port := strconv.Itoa(srv.port)
	tests := []struct {
		name   string
		origin string
		cookie *http.Cookie
		bearer string
		want   int
	}{
		{"own origin", "http://127.0.0.1:" + port, cookie, "", http.StatusNotImplemented},
		{"own localhost origin", "http://localhost:" + port, cookie, "", http.StatusNotImplemented},
		{"foreign port", "http://127.0.0.1:" + otherPort(srv), cookie, "", http.StatusForbidden},
		{"remote site", "https://evil.example", cookie, "", http.StatusForbidden},
		{"opaque origin", "null", cookie, "", http.StatusForbidden},
		{"no origin", "", cookie, "", http.StatusForbidden},
		{"bearer without origin", "", nil, srv.token, http.StatusNotImplemented},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := do(t, srv, request{method: http.MethodPost, path: "/api/render", origin: tc.origin, cookie: tc.cookie, bearer: tc.bearer})
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestFetchSiteGuard(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	tests := []struct {
		name   string
		path   string
		site   string
		cookie *http.Cookie
		bearer string
		want   int
	}{
		{"api same-origin", "/api/model", fetchSiteSameOrigin, cookie, "", http.StatusOK},
		{"api without header", "/api/model", "", cookie, "", http.StatusOK},
		{"api same-site", "/api/model", "same-site", cookie, "", http.StatusForbidden},
		{"api cross-site", "/api/model", "cross-site", cookie, "", http.StatusForbidden},
		{"api typed address", "/api/model", "none", cookie, "", http.StatusForbidden},
		{"contexts same-site", "/api/cluster/contexts?kubeconfig=x", "same-site", cookie, "", http.StatusForbidden},
		{"api bearer same-site", "/api/model", "same-site", nil, srv.token, http.StatusOK},
		{"static typed address", "/", "none", cookie, "", http.StatusOK},
		{"static cross-site", "/", "cross-site", cookie, "", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := do(t, srv, request{path: tc.path, site: tc.site, cookie: tc.cookie, bearer: tc.bearer})
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func dialWS(srv *Server, cookie *http.Cookie, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	if cookie != nil {
		header.Set("Cookie", cookie.String())
	}
	if origin != "" {
		header.Set(headerOrigin, origin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	return websocket.Dial(ctx, "ws://"+srv.Addr().String()+pathWS, &websocket.DialOptions{HTTPHeader: header})
}

func TestWebSocketGuards(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	tests := []struct {
		name   string
		cookie *http.Cookie
		origin string
		want   int
	}{
		{"no cookie", nil, ownOrigin(srv), http.StatusUnauthorized},
		{"foreign origin", cookie, "http://127.0.0.1:" + otherPort(srv), http.StatusForbidden},
		{"no origin", cookie, "", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, resp, err := dialWS(srv, tc.cookie, tc.origin)
			if err == nil {
				conn.CloseNow()
				t.Fatal("upgrade succeeded, want it refused")
			}
			if resp == nil || resp.StatusCode != tc.want {
				t.Errorf("response = %v, want status %d", resp, tc.want)
			}
		})
	}
}

func TestWebSocketEvents(t *testing.T) {
	srv := newServer(t, builtUI)
	stop := serve(t, srv)
	conn, _, err := dialWS(srv, login(t, srv), ownOrigin(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var ev Event
	if err := wsjson.Read(ctx, conn, &ev); err != nil {
		t.Fatal(err)
	}
	if ev != (Event{Type: eventReady, Version: version.Version}) {
		t.Errorf("first event = %+v, want ready with version %q", ev, version.Version)
	}
	srv.Progress().Publish(Event{Type: "progress", Payload: "step"})
	ev = Event{}
	if err := wsjson.Read(ctx, conn, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != "progress" || ev.Payload != "step" {
		t.Errorf("event = %+v, want the published progress event", ev)
	}

	if err := stop(); err != nil {
		t.Fatalf("Serve = %v, want a clean shutdown with an open WebSocket", err)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Errorf("read after shutdown = %v, want close status %d", err, websocket.StatusGoingAway)
	}
}

func TestStatic(t *testing.T) {
	tests := []struct {
		name     string
		ui       fs.FS
		req      request
		want     int
		wantBody string
	}{
		{"root serves index", builtUI, request{path: "/"}, http.StatusOK, indexHTML},
		{"asset", builtUI, request{path: "/assets/app.js"}, http.StatusOK, appJS},
		{"client route falls back to index", builtUI, request{path: "/target/details"}, http.StatusOK, indexHTML},
		{"index by name", builtUI, request{path: "/index.html"}, http.StatusOK, indexHTML},
		{"directory falls back to index", builtUI, request{path: "/assets/"}, http.StatusOK, indexHTML},
		{"missing asset is not found", builtUI, request{path: "/assets/app-oldhash.js"}, http.StatusNotFound, ""},
		{"missing file with extension is not found", builtUI, request{path: "/favicon.ico"}, http.StatusNotFound, ""},
		{"unsafe method", builtUI, request{method: http.MethodPost, path: "/"}, http.StatusMethodNotAllowed, ""},
		{"missing UI", fstest.MapFS{".gitkeep": {}}, request{path: "/"}, http.StatusServiceUnavailable, "make ui"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := started(t, tc.ui)
			tc.req.cookie = login(t, srv)
			tc.req.origin = ownOrigin(srv)
			resp, body := do(t, srv, tc.req)
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			if !strings.Contains(body, tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", body, tc.wantBody)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := started(t, builtUI)
	cookie := login(t, srv)
	tests := []struct {
		name string
		req  request
	}{
		{"unauthorized page", request{path: "/"}},
		{"token exchange", request{path: pathAndQuery(t, srv.URL())}},
		{"foreign host", request{path: "/", host: "evil.example:80"}},
		{"api", request{path: "/api/model", cookie: cookie}},
		{"ui", request{path: "/", cookie: cookie}},
		{"asset", request{path: "/assets/app.js", cookie: cookie}},
	}
	want := map[string]string{
		"Content-Security-Policy": "frame-ancestors 'none'",
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := do(t, srv, tc.req)
			for header, value := range want {
				if got := resp.Header.Get(header); got != value {
					t.Errorf("%s = %q, want %q", header, got, value)
				}
			}
		})
	}
}

func TestCloseEndsWebSockets(t *testing.T) {
	srv := newServer(t, builtUI)
	serve(t, srv)
	conn, _, err := dialWS(srv, login(t, srv), ownOrigin(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	var ev Event
	if err := wsjson.Read(ctx, conn, &ev); err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Errorf("read after Close = %v, want close status %d", err, websocket.StatusGoingAway)
	}
}

func TestLaunchCode(t *testing.T) {
	srv := newServer(t, builtUI)
	var now atomic.Int64
	now.Store(time.Now().UnixNano())
	srv.now = func() time.Time { return time.Unix(0, now.Load()) }
	serve(t, srv)

	launch := func(t *testing.T) string {
		t.Helper()
		u, err := srv.LaunchURL()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(u, srv.token) {
			t.Fatalf("launch URL %q contains the run token", u)
		}
		return pathAndQuery(t, u)
	}

	t.Run("single use", func(t *testing.T) {
		path := launch(t)
		resp, body := do(t, srv, request{path: path})
		cookie := assertExchange(t, srv, resp, body)
		if resp, _ := do(t, srv, request{path: "/api/model", cookie: cookie}); resp.StatusCode != http.StatusOK {
			t.Errorf("cookie gives status %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if resp, _ := do(t, srv, request{path: path}); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("second use: status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
	t.Run("expired", func(t *testing.T) {
		path := launch(t)
		now.Add(int64(launchCodeTTL))
		if resp, _ := do(t, srv, request{path: path}); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
	t.Run("not a bearer token", func(t *testing.T) {
		u, _ := url.Parse(launch(t))
		resp, _ := do(t, srv, request{path: "/api/model", bearer: u.Query().Get(queryLaunch)})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
}
