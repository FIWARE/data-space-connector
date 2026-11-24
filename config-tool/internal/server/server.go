// Package server is the loopback HTTP front end of dsc-config: the token-guarded API, the
// WebSocket and the embedded UI. It is the only package that knows HTTP.
package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/recent"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/trial"
	"github.com/FIWARE/data-space-connector/config-tool/model"
	trialdata "github.com/FIWARE/data-space-connector/config-tool/trial"
)

const (
	loopbackHost = "127.0.0.1"
	localhost    = "localhost"
	// listenAddr picks a random free port; the host is never configurable.
	listenAddr = loopbackHost + ":0"
	// secretBytes is the entropy of the run token, session secrets and launch codes.
	secretBytes = 32
	// launchCodeTTL bounds how long a launch code left in a browser's argv stays usable.
	launchCodeTTL     = 60 * time.Second
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Server serves the dsc-config UI and API on a loopback port for the lifetime of one run.
type Server struct {
	listener   net.Listener
	port       int
	token      string
	cookieName string
	origins    map[string]bool
	hosts      map[string]bool
	ui         fs.FS
	// models holds the question-model bundles; tests swap it before serving.
	models fs.FS
	// recent is the recent-profiles list; nil when there is no config directory. Tests swap it.
	recent   *recent.Store
	progress *Progress
	http     *http.Server
	// shutdown is cancelled when the server shuts down, which ends open WebSockets.
	shutdown context.Context
	cancel   context.CancelFunc
	now      func() time.Time

	// loaded is the overrides layer: the latest POST /api/load or the source an opened profile restored.
	loaded *load.Result
	// lastRender is the previous POST /api/render tree; a load or profile open empties it.
	lastRender render.Tree
	// lastRenderSHA is the sourceSha256 lastRender was rendered with; a render of another one diffs against empty.
	lastRenderSHA string
	// loadMu guards loaded, lastRender and lastRenderSHA.
	loadMu sync.Mutex

	// trial holds the local trial descriptor and values; tests swap it before the first trial request.
	trial     fs.FS
	trialOnce sync.Once
	trialOuts []trial.Output
	trialErr  error

	mu          sync.Mutex
	sessions    map[string]struct{}
	launchCodes map[string]time.Time
}

// New listens on 127.0.0.1 with a random port and serves ui (the root of the built SPA).
func New(ui fs.FS) (*Server, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	token, err := newSecret()
	if err != nil {
		ln.Close()
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	hostPort := func(host string) string { return net.JoinHostPort(host, strconv.Itoa(port)) }
	s := &Server{
		listener:    ln,
		port:        port,
		token:       token,
		cookieName:  cookiePrefix + strconv.Itoa(port),
		origins:     map[string]bool{"http://" + hostPort(loopbackHost): true, "http://" + hostPort(localhost): true},
		hosts:       map[string]bool{hostPort(loopbackHost): true, hostPort(localhost): true},
		ui:          ui,
		models:      bundles.FS,
		trial:       trialdata.FS,
		progress:    NewProgress(),
		now:         time.Now,
		sessions:    map[string]struct{}{},
		launchCodes: map[string]time.Time{},
	}
	if dir, err := recent.DefaultDir(); err == nil {
		s.recent = &recent.Store{Dir: dir}
	}
	s.shutdown, s.cancel = context.WithCancel(context.Background())
	s.http = &http.Server{Handler: s.Handler(), ReadHeaderTimeout: readHeaderTimeout}
	s.http.RegisterOnShutdown(s.cancel)
	return s, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

// URL is the token URL to print; it stays valid for the whole run.
func (s *Server) URL() string {
	return fmt.Sprintf("%s/?%s=%s", s.origin(), queryToken, s.token)
}

// LaunchURL issues a single-use launch code that expires after launchCodeTTL.
// Its URL is safe to put on a command line: it never contains the run token.
func (s *Server) LaunchURL() (string, error) {
	code, err := newSecret()
	if err != nil {
		return "", err
	}
	now := s.now()
	s.mu.Lock()
	for c, expiry := range s.launchCodes {
		if !now.Before(expiry) {
			delete(s.launchCodes, c)
		}
	}
	s.launchCodes[code] = now.Add(launchCodeTTL)
	s.mu.Unlock()
	return fmt.Sprintf("%s/?%s=%s", s.origin(), queryLaunch, code), nil
}

// Progress is the broadcaster whose events reach every open WebSocket.
func (s *Server) Progress() *Progress {
	return s.progress
}

// Serve handles requests until ctx is cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() { errc <- s.http.Serve(s.listener) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Close stops the server at once, dropping open connections; for tests and failed starts.
func (s *Server) Close() error {
	// http.Server.Close neither runs the shutdown hooks nor closes hijacked WebSockets.
	s.cancel()
	err := s.http.Close()
	if lerr := s.listener.Close(); lerr != nil && !errors.Is(lerr, net.ErrClosed) && err == nil {
		err = lerr
	}
	return err
}

func (s *Server) origin() string {
	return "http://" + net.JoinHostPort(loopbackHost, strconv.Itoa(s.port))
}

func newSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
