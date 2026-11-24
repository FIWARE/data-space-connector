package server

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/atomicfile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/recent"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// Notes returned next to a saved, opened or listed profile (ADR 0015).
const (
	noteNotNext        = "overrides will not be restored on reopen: %s is not next to the profile"
	noteDiffers        = "overrides will not be restored on reopen: %s differs from the loaded one"
	noteNeighbourError = "overrides may not be restored on reopen: %s cannot be read: %v"
	noteSourceMissing  = "overrides not restored: %s is not next to the profile; saving again drops the reference to it"
	noteSourceBroken   = "overrides not restored: %s cannot be loaded (%v); saving again drops the reference to it"
	noteSourceChanged  = "overrides restored from %s, which has changed since the profile was saved"
	noteRecentReset    = "the recent list was corrupt and has been reset"
	noteRecentFailed   = "the recent list was not updated: %v"
	noteRecentRead     = "the recent list cannot be read: %v"
	noteNoRecent       = "the recent list is not available: no user config directory (set " + recent.ConfigHomeEnv + ")"
)

var (
	errPathNotAbsolute = errors.New("profile path must be absolute")
	errSourceMismatch  = errors.New("sourceSha256 does not match the loaded values file; load it again")
)

// saveRequest is the body of PUT /api/profile. Profile.Model is only read while the chart version is empty.
type saveRequest struct {
	Path      string          `json:"path"`
	Overwrite bool            `json:"overwrite"`
	Profile   profile.Profile `json:"profile"`
	// SourceSHA256 names the load the UI holds; empty saves no source, whatever the server holds.
	SourceSHA256 string `json:"sourceSha256"`
}

type openRequest struct {
	Path string `json:"path"`
}

// profileResponse answers save and open; Report is set when open restored the source.
type profileResponse struct {
	Path    string          `json:"path"`
	Profile profile.Profile `json:"profile"`
	Report  *load.Report    `json:"report,omitempty"`
	Notes   []string        `json:"notes"`
}

type recentResponse struct {
	Entries    []recent.Entry `json:"entries"`
	DefaultDir string         `json:"defaultDir"`
	Notes      []string       `json:"notes"`
}

// handleSaveProfile writes the profile to the requested path (ADR 0015 §2, §3, §5).
func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid request: "+err.Error(), "")
		return
	}
	path, err := checkProfilePath(req.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	p := req.Profile
	if _, ok := s.prepareProfile(w, bundles, &p, req.SourceSHA256); !ok {
		return
	}
	err = profile.WriteFile(path, p, req.Overwrite)
	switch {
	case errors.Is(err, atomicfile.ErrExists):
		writeError(w, http.StatusConflict, err.Error(), "")
		return
	case errors.Is(err, atomicfile.ErrSymlink), errors.Is(err, atomicfile.ErrNotRegular):
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("directory %s does not exist", filepath.Dir(path)), "")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	notes := []string{}
	if p.Source != nil {
		if note := neighbourNote(filepath.Dir(path), p.Source); note != "" {
			notes = append(notes, note)
		}
	}
	notes = append(notes, s.addRecent(path, p)...)
	writeJSON(w, profileResponse{Path: path, Profile: p, Notes: notes})
}

// handleOpenProfile reads a profile and restores its source as the loaded slot, or clears the slot.
func (s *Server) handleOpenProfile(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid request: "+err.Error(), "")
		return
	}
	path, err := checkProfilePath(req.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, err.Error(), "")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	p, err := profile.Unmarshal(data)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s: %v", path, err), "")
		return
	}
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	if errs := p.Validate(bundles); len(errs) > 0 {
		writeFieldErrors(w, errs)
		return
	}
	normalize(&p)
	notes := append([]string{}, p.RevisionNotes(bundles)...)
	// Validate guarantees the bundle is shipped and not newer than this build's revision.
	bundle := findBundle(bundles, p.Model.Chart, p.Model.Version)
	res, note := restoreSource(filepath.Dir(path), p.Source, bundles, bundle)
	if note != "" {
		notes = append(notes, note)
	}
	s.loadMu.Lock()
	s.loaded = res
	s.resetRender()
	s.loadMu.Unlock()
	var report *load.Report
	if res != nil {
		report = &res.Report
		p.Answers = load.SeedAnswers(res.Report, p.Answers)
	}
	resp := profileResponse{Path: path, Profile: p, Report: report}
	resp.Notes = append(notes, s.addRecent(path, p)...)
	writeJSON(w, resp)
}

// handleRecentProfiles lists the recent profiles; a failing store is a note, never an error status.
func (s *Server) handleRecentProfiles(w http.ResponseWriter, _ *http.Request) {
	resp := recentResponse{Entries: []recent.Entry{}, Notes: []string{}}
	if s.recent == nil {
		resp.Notes = append(resp.Notes, noteNoRecent)
	} else if entries, err := s.recent.List(); err != nil {
		resp.Notes = append(resp.Notes, fmt.Sprintf(noteRecentRead, err))
	} else if entries != nil {
		resp.Entries = entries
	}
	resp.DefaultDir = defaultDir(resp.Entries)
	writeJSON(w, resp)
}

// checkProfilePath refuses a relative path or one without profile.FileSuffix and returns it cleaned.
func checkProfilePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%q: %w", path, errPathNotAbsolute)
	}
	path = filepath.Clean(path)
	if !strings.HasSuffix(path, profile.FileSuffix) {
		return "", fmt.Errorf("%s: %w", path, profile.ErrSuffix)
	}
	return path, nil
}

// saveModel picks the bundle a save records (ADR 0015 §5): the target's chart version, else the
// held model of the same chart, else the latest bundle of the chart. The revision is the shipped one.
// Without a shipped bundle it returns a reference Validate refuses.
func saveModel(bundles []model.Bundle, t profile.Target, held profile.ModelRef) profile.ModelRef {
	if t.ChartVersion != "" {
		if b := findBundle(bundles, t.Chart, t.ChartVersion); b != nil {
			return modelRef(*b)
		}
		return profile.ModelRef{Chart: t.Chart, Version: t.ChartVersion}
	}
	if held.Chart == t.Chart {
		if b := findBundle(bundles, held.Chart, held.Version); b != nil {
			return modelRef(*b)
		}
	}
	if i := slices.IndexFunc(bundles, func(b model.Bundle) bool { return b.Chart == t.Chart && b.Latest }); i >= 0 {
		return modelRef(bundles[i])
	}
	return profile.ModelRef{Chart: t.Chart}
}

// saveCatalogue picks the role catalogue a save records: nil without a role or when profile.CatalogueBundle
// is the model's own bundle, else that bundle with the shipped revision.
func saveCatalogue(bundles []model.Bundle, m profile.ModelRef, role string, held *profile.ModelRef) *profile.ModelRef {
	if role == "" {
		return nil
	}
	c, err := profile.CatalogueBundle(bundles, m, held)
	if err != nil || c.Chart == m.Chart && c.Version == m.Version {
		return nil
	}
	ref := modelRef(c)
	return &ref
}

func modelRef(b model.Bundle) profile.ModelRef {
	return profile.ModelRef{Chart: b.Chart, Version: b.Version, Revision: b.Revision}
}

func findBundle(bundles []model.Bundle, chart, version string) *model.Bundle {
	i := slices.IndexFunc(bundles, func(b model.Bundle) bool { return b.Chart == chart && b.Version == version })
	if i < 0 {
		return nil
	}
	return &bundles[i]
}

// prepareProfile sets the fields a save owns (ADR 0015 §5) and validates p, as save and preset share it.
// It returns the loaded slot sha names, or writes the refusal and returns false.
func (s *Server) prepareProfile(w http.ResponseWriter, bundles []model.Bundle, p *profile.Profile, sha string) (*load.Result, bool) {
	p.Format = profile.FormatVersion
	p.Model = saveModel(bundles, p.Target, p.Model)
	p.Catalogue = saveCatalogue(bundles, p.Model, p.Role, p.Catalogue)
	p.Secrets = map[string]profile.SecretRef{}
	res, err := s.slot(sha)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return nil, false
	}
	p.Source = nil
	if res != nil {
		p.Source = &profile.Source{File: res.Report.Source, Root: res.Root.String(), SHA256: sha}
	}
	normalize(p)
	if errs := p.Validate(bundles); len(errs) > 0 {
		writeFieldErrors(w, errs)
		return nil, false
	}
	return res, true
}

// slot returns the loaded slot when sha names it; empty sha means none (ADR 0015 §2).
func (s *Server) slot(sha string) (*load.Result, error) {
	if sha == "" {
		return nil, nil
	}
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	if s.loaded == nil || s.loaded.Report.SHA256 != sha {
		return nil, errSourceMismatch
	}
	return s.loaded, nil
}

// normalize gives p the shape Marshal writes, so a response shows the profile as stored.
func normalize(p *profile.Profile) {
	p.Open = slices.SortedFunc(slices.Values(p.Open), func(a, b profile.OpenQuestion) int { return cmp.Compare(a.Field, b.Field) })
	if p.Answers == nil {
		p.Answers = map[string]any{}
	}
	if p.Secrets == nil {
		p.Secrets = map[string]profile.SecretRef{}
	}
}

// neighbourNote says why reopening a profile in dir will not restore src, or "" when it will.
func neighbourNote(dir string, src *profile.Source) string {
	data, err := os.ReadFile(filepath.Join(dir, src.File))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Sprintf(noteNotNext, src.File)
	case err != nil:
		return fmt.Sprintf(noteNeighbourError, src.File, err)
	case digest(data) != src.SHA256:
		return fmt.Sprintf(noteDiffers, src.File)
	}
	return ""
}

// restoreSource re-loads src from dir against the profile's bundle and recorded root. A nil result
// means there is no overrides layer; the note says why when src was set.
func restoreSource(dir string, src *profile.Source, bundles []model.Bundle, bundle *model.Bundle) (*load.Result, string) {
	if src == nil {
		return nil, ""
	}
	data, err := os.ReadFile(filepath.Join(dir, src.File))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Sprintf(noteSourceMissing, src.File)
	}
	if err != nil {
		return nil, fmt.Sprintf(noteSourceBroken, src.File, err)
	}
	root, err := roundtrip.ParsePath(src.Root)
	if err != nil {
		return nil, fmt.Sprintf(noteSourceBroken, src.File, err)
	}
	res, err := load.Load(data, src.File, bundles, load.Options{Root: &root, Bundle: bundle})
	if err != nil {
		return nil, fmt.Sprintf(noteSourceBroken, src.File, err)
	}
	if res.Report.SHA256 != src.SHA256 {
		return res, fmt.Sprintf(noteSourceChanged, src.File)
	}
	return res, ""
}

// addRecent records path in the recent list and returns the notes its failure gives.
func (s *Server) addRecent(path string, p profile.Profile) []string {
	if s.recent == nil {
		return []string{noteNoRecent}
	}
	err := s.recent.Add(recent.Entry{Path: path, ChartVersion: p.Model.Version, Role: p.Role})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, recent.ErrReset):
		return []string{noteRecentReset}
	default:
		return []string{fmt.Sprintf(noteRecentFailed, err)}
	}
}

// defaultDir is where the UI proposes to save: the newest entry's directory, else the home
// directory, else the working directory, which for a GUI launcher is often / (ADR 0015 §3).
func defaultDir(entries []recent.Entry) string {
	if len(entries) > 0 {
		return filepath.Dir(entries[0].Path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	wd, _ := os.Getwd()
	return wd
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
