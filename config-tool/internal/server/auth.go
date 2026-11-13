package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const (
	// cookiePrefix plus the port names the session cookie, so side-by-side instances keep apart.
	cookiePrefix = "dsc_session_"
	queryToken   = "token"
	queryLaunch  = "launch"
	bearerPrefix = "Bearer "
	pathRoot     = "/"
	pathAPI      = "/api/"
	pathWS       = "/api/ws"

	headerAuthorization  = "Authorization"
	headerOrigin         = "Origin"
	headerUpgrade        = "Upgrade"
	headerCacheControl   = "Cache-Control"
	headerReferrerPolicy = "Referrer-Policy"
	headerContentType    = "Content-Type"
	headerCSP            = "Content-Security-Policy"
	headerFrameOptions   = "X-Frame-Options"
	headerContentOptions = "X-Content-Type-Options"
	upgradeWebSocket     = "websocket"
	contentTypeHTML      = "text/html; charset=utf-8"
)

// Every other port on 127.0.0.1 is the same site and gets the Strict cookie inside an iframe,
// so every response forbids framing to stop clickjacking from a local dev server.
var securityHeaders = map[string]string{
	headerCSP:            "frame-ancestors 'none'",
	headerFrameOptions:   "DENY",
	headerContentOptions: "nosniff",
}

// The exchange answers 200 with a meta refresh, not a 302: a redirect keeps a cross-site
// initiator, and the browser would then withhold the fresh SameSite=Strict cookie.
const exchangePage = `<!doctype html>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0;url=/">
<title>dsc-config</title>
<p><a href="/">Continue to dsc-config</a></p>
`

const unauthorizedPage = `<!doctype html>
<meta charset="utf-8">
<title>dsc-config</title>
<p>Not signed in. Open the link printed by <code>dsc-config</code> in its terminal.</p>
`

// guard wraps next with the security headers, the Host check, the token and launch-code
// exchange, authentication and the Origin check for cookie-authenticated unsafe requests and
// WebSocket upgrades.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for name, value := range securityHeaders {
			w.Header().Set(name, value)
		}
		if !s.hosts[strings.ToLower(r.Host)] {
			writeError(w, http.StatusForbidden, "forbidden host", "")
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathRoot && s.exchange(w, r) {
			return
		}
		bearer, ok := s.authenticate(r)
		if !ok {
			unauthorized(w, r)
			return
		}
		if !bearer && needsOrigin(r) && !s.origins[r.Header.Get(headerOrigin)] {
			writeError(w, http.StatusForbidden, "forbidden origin", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// exchange handles ?token= and ?launch= on the root path; it reports whether it answered.
func (s *Server) exchange(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	var valid bool
	switch {
	case q.Has(queryToken):
		valid = s.isToken(q.Get(queryToken))
	case q.Has(queryLaunch):
		valid = s.redeemLaunchCode(q.Get(queryLaunch))
	default:
		return false
	}
	if !valid {
		unauthorized(w, r)
		return true
	}
	secret, err := newSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return true
	}
	s.mu.Lock()
	s.sessions[secret] = struct{}{}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    secret,
		Path:     pathRoot,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	writeHTML(w, http.StatusOK, exchangePage)
	return true
}

// authenticate accepts the run token as a Bearer header or a session cookie; an
// Authorization header, when present, decides alone.
func (s *Server) authenticate(r *http.Request) (bearer, ok bool) {
	if auth := r.Header.Get(headerAuthorization); auth != "" {
		token, found := strings.CutPrefix(auth, bearerPrefix)
		return true, found && s.isToken(token)
	}
	c, err := r.Cookie(s.cookieName)
	if err != nil {
		return false, false
	}
	s.mu.Lock()
	_, ok = s.sessions[c.Value]
	s.mu.Unlock()
	return false, ok
}

func (s *Server) isToken(v string) bool {
	return subtle.ConstantTimeCompare([]byte(v), []byte(s.token)) == 1
}

func (s *Server) redeemLaunchCode(code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.launchCodes[code]
	delete(s.launchCodes, code)
	return ok && s.now().Before(expiry)
}

func needsOrigin(r *http.Request) bool {
	safe := r.Method == http.MethodGet || r.Method == http.MethodHead
	return !safe || r.URL.Path == pathWS || strings.EqualFold(r.Header.Get(headerUpgrade), upgradeWebSocket)
}

func unauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, pathAPI) {
		writeError(w, http.StatusUnauthorized, "missing or invalid credentials", "")
		return
	}
	writeHTML(w, http.StatusUnauthorized, unauthorizedPage)
}

func writeHTML(w http.ResponseWriter, status int, page string) {
	w.Header().Set(headerContentType, contentTypeHTML)
	w.Header().Set(headerCacheControl, "no-store")
	w.Header().Set(headerReferrerPolicy, "no-referrer")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(page))
}
