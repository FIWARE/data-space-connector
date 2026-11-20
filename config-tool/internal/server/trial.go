package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/trial"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
)

var errDirNotAbsolute = errors.New("trial directory must be absolute")

// trialBody answers GET /api/trial: the rendered releases in install order and every production warning.
type trialBody struct {
	Profile     string              `json:"profile"`
	Environment profile.Environment `json:"environment"`
	Releases    []trialRelease      `json:"releases"`
	Warnings    []trialWarning      `json:"warnings"`
}

// trialRelease is one rendered release; Values is the full file text, header and banner included.
type trialRelease struct {
	Role         string `json:"role"`
	ReleaseName  string `json:"releaseName"`
	Namespace    string `json:"namespace"`
	Chart        string `json:"chart"`
	ChartVersion string `json:"chartVersion"`
	Source       string `json:"source"`
	File         string `json:"file"`
	Values       string `json:"values"`
}

// trialWarning is a trial.Warning without its evidence, which only the tests need.
type trialWarning struct {
	ID       string   `json:"id"`
	Releases []string `json:"releases"`
	Message  string   `json:"message"`
	Link     string   `json:"link,omitempty"`
}

type trialWriteRequest struct {
	Dir string `json:"dir"`
}

// trialWriteResponse lists the written files' paths, in install order.
type trialWriteResponse struct {
	Dir   string   `json:"dir"`
	Files []string `json:"files"`
}

// trialOutputs loads and renders the embedded trial once per server; the result never changes during a run.
func (s *Server) trialOutputs() ([]trial.Output, error) {
	s.trialOnce.Do(func() {
		t, err := trial.Load(s.trial)
		if err == nil {
			s.trialOuts, err = trial.Render(t, s.trial, version.Version)
		}
		s.trialErr = err
	})
	return s.trialOuts, s.trialErr
}

func (s *Server) handleTrial(w http.ResponseWriter, _ *http.Request) {
	outs, err := s.trialOutputs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	body := trialBody{
		Profile:     trial.TrialProfileName,
		Environment: profile.EnvDevelopment,
		Releases:    make([]trialRelease, 0, len(outs)),
	}
	for _, o := range outs {
		t := o.Release.Target
		body.Releases = append(body.Releases, trialRelease{
			Role: o.Release.Role, ReleaseName: t.ReleaseName, Namespace: t.Namespace, Chart: t.Chart,
			ChartVersion: t.ChartVersion, Source: o.Release.Source, File: o.File, Values: string(o.Values),
		})
	}
	for _, warn := range trial.Warnings() {
		body.Warnings = append(body.Warnings, trialWarning{ID: warn.ID, Releases: warn.Releases, Message: warn.Message, Link: warn.Link})
	}
	writeJSON(w, body)
}

// handleTrialWrite writes the trial's values files into an existing directory, all or nothing and never overwriting.
func (s *Server) handleTrialWrite(w http.ResponseWriter, r *http.Request) {
	var req trialWriteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid request: "+err.Error(), "")
		return
	}
	dir, err := checkTrialDir(req.Dir)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	outs, err := s.trialOutputs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	paths, err := trial.WriteAll(dir, outs)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrExist) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error(), "")
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(trialWriteResponse{Dir: dir, Files: paths})
}

// checkTrialDir refuses a relative path or one that is not an existing directory and returns it cleaned.
func checkTrialDir(dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%q: %w", dir, errDirNotAbsolute)
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("directory %s does not exist", dir)
	case err != nil:
		return "", err
	case !info.IsDir():
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return dir, nil
}
