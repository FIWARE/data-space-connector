package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"

	"github.com/FIWARE/data-space-connector/config-tool/internal/dataspace"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	// outboundTimeout bounds a whole outbound exchange, redirects included, so a button press never hangs.
	outboundTimeout = 10 * time.Second
	// maxImportRedirects is how many same-scheme redirects a descriptor import follows.
	maxImportRedirects = 5
	// descriptorPath is where a data space publishes its configuration (the chart's dsconfig service).
	descriptorPath     = "/.well-known/data-space-configuration"
	reachStatusOK      = "reachable"
	reachStatusFailed  = "unreachable"
	headerAccept       = "Accept"
	successStatusFloor = http.StatusOK
	successStatusCeil  = http.StatusMultipleChoices
)

var errTooManyRedirects = fmt.Errorf("the data space redirected more than %d times", maxImportRedirects)

// dataspaceCheckRequest is the body of POST /api/dataspace/check.
type dataspaceCheckRequest struct {
	URL string `json:"url"`
}

// dataspaceCheckResponse is the result of one GET; it is session state, never a profile field (ADR 0022).
type dataspaceCheckResponse struct {
	Status string `json:"status"`
	// HTTPStatus is the response's status code; absent when no response arrived.
	HTTPStatus int `json:"httpStatus,omitempty"`
	// Error is the transport error, without the URL; absent when a response arrived.
	Error string `json:"error,omitempty"`
}

// dataspaceImportRequest is the body of POST /api/dataspace/import; URL is a base or a full descriptor URL.
type dataspaceImportRequest struct {
	URL     string          `json:"url"`
	Profile profile.Profile `json:"profile"`
}

// dataspaceImportResponse is the preview of a fetched descriptor; the UI applies it, the server writes nothing.
type dataspaceImportResponse struct {
	// URL is the descriptor URL that was requested.
	URL     string                  `json:"url"`
	Preview dataspace.PreviewResult `json:"preview"`
}

// newOutbound is the default transport of user-triggered calls; it reads the proxy environment once, here.
func newOutbound() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	proxy := httpproxy.FromEnvironment().ProxyFunc()
	t.Proxy = func(r *http.Request) (*url.URL, error) { return proxy(r.URL) }
	return t
}

// handleDataspaceCheck sends one GET to the TIR address and reports whether anything answered.
func (s *Server) handleDataspaceCheck(w http.ResponseWriter, r *http.Request) {
	var req dataspaceCheckRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid request: "+err.Error(), "")
		return
	}
	if err := model.TypeURL.Check(req.URL); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "url "+err.Error(), "")
		return
	}
	out, err := http.NewRequestWithContext(r.Context(), http.MethodGet, req.URL, nil)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "url "+withoutURL(err), "")
		return
	}
	client := s.outboundClient(func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
	resp, err := client.Do(out)
	if err != nil {
		writeJSON(w, dataspaceCheckResponse{Status: reachStatusFailed, Error: withoutURL(err)})
		return
	}
	resp.Body.Close()
	writeJSON(w, dataspaceCheckResponse{Status: reachStatusOK, HTTPStatus: resp.StatusCode})
}

// handleDataspaceImport fetches a data space's descriptor and previews it against the profile's answers.
func (s *Server) handleDataspaceImport(w http.ResponseWriter, r *http.Request) {
	var req dataspaceImportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid request: "+err.Error(), "")
		return
	}
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	p := req.Profile
	if _, ok := s.prepareProfile(w, bundles, &p, ""); !ok {
		return
	}
	// prepareProfile validated p, so its bundle is shipped.
	b := findBundle(bundles, p.Model.Chart, p.Model.Version)
	target, err := descriptorURL(req.URL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "url "+err.Error(), "")
		return
	}
	body, err := s.fetchDescriptor(r, target)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}
	d, err := dataspace.ParseDescriptor(body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}
	writeJSON(w, dataspaceImportResponse{URL: target, Preview: dataspace.Preview(*b, p, d)})
}

// descriptorURL appends descriptorPath to a base URL; a URL already ending with it is kept.
func descriptorURL(raw string) (string, error) {
	if err := model.TypeURL.Check(raw); err != nil {
		return "", err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New(withoutURL(err))
	}
	if !strings.HasSuffix(u.Path, descriptorPath) {
		u = u.JoinPath(descriptorPath)
	}
	return u.String(), nil
}

// fetchDescriptor GETs target, following same-scheme redirects only, and returns at most one byte more than
// dataspace.MaxDescriptorBytes so the parser refuses an oversized body. Errors never carry the URL.
func (s *Server) fetchDescriptor(r *http.Request, target string) ([]byte, error) {
	out, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, errors.New(withoutURL(err))
	}
	out.Header.Set(headerAccept, contentTypeJSON)
	client := s.outboundClient(func(next *http.Request, via []*http.Request) error {
		if len(via) > maxImportRedirects {
			return errTooManyRedirects
		}
		if from := via[0].URL.Scheme; next.URL.Scheme != from {
			return fmt.Errorf("the data space redirected from %s to %s, which is refused", from, next.URL.Scheme)
		}
		return nil
	})
	resp, err := client.Do(out)
	if err != nil {
		return nil, errors.New(withoutURL(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < successStatusFloor || resp.StatusCode >= successStatusCeil {
		return nil, fmt.Errorf("the data space answered HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, dataspace.MaxDescriptorBytes+1))
	if err != nil {
		return nil, errors.New("reading the descriptor failed: " + withoutURL(err))
	}
	return body, nil
}

func (s *Server) outboundClient(redirect func(*http.Request, []*http.Request) error) *http.Client {
	return &http.Client{Transport: s.outbound, CheckRedirect: redirect, Timeout: outboundTimeout}
}

// withoutURL is err's text without the URL net/url wraps it with, so no address reaches a message.
func withoutURL(err error) string {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err.Error()
	}
	return err.Error()
}
