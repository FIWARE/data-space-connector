package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	queryChart   = "chart"
	queryVersion = "version"
)

// rolesResponse is the role catalogue of GET /api/roles, in bundle order.
type rolesResponse struct {
	Roles    []roleBody    `json:"roles"`
	Features []featureBody `json:"features"`
}

type roleBody struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Chart   string `json:"chart"`
	// Floor is null when the footprint depends on the selection.
	Floor    *floorBody             `json:"floor"`
	Docs     string                 `json:"docs"`
	Features map[string]model.Offer `json:"features"`
}

type floorBody struct {
	Memory       string `json:"memory"`
	CPU          string `json:"cpu"`
	MeasuredWith string `json:"measuredWith"`
}

type featureBody struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Summary string      `json:"summary"`
	Group   model.Group `json:"group"`
}

// presetRequest is the body of POST /api/preset; SourceSHA256 names the loaded slot as on save.
type presetRequest struct {
	Profile      profile.Profile `json:"profile"`
	Role         string          `json:"role"`
	SourceSHA256 string          `json:"sourceSha256"`
}

// presetResponse carries the profile with the preset applied; ChartSwitch is null without a switch.
type presetResponse struct {
	Profile     profile.Profile      `json:"profile"`
	ChartSwitch *profile.ChartSwitch `json:"chartSwitch"`
}

// handleRoles lists the roles and features offered for chart@version; an empty version means the latest.
func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	chart, version := r.URL.Query().Get(queryChart), r.URL.Query().Get(queryVersion)
	if !slices.ContainsFunc(bundles, func(b model.Bundle) bool {
		return b.Chart == chart && (b.Version == version || version == "" && b.Latest)
	}) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no bundle for chart %q version %q", chart, version), "")
		return
	}
	cat, err := model.RoleCatalogue(bundles, chart, version)
	switch {
	case errors.Is(err, model.ErrNoRoles):
		writeError(w, http.StatusNotFound, err.Error(), "")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	writeJSON(w, catalogueBody(cat))
}

func catalogueBody(b model.Bundle) rolesResponse {
	body := rolesResponse{Roles: make([]roleBody, 0, len(b.Roles)), Features: make([]featureBody, 0, len(b.Features))}
	for _, r := range b.Roles {
		rb := roleBody{ID: r.ID, Title: r.Title, Summary: r.Summary, Chart: r.Chart, Docs: r.Docs, Features: r.Features}
		if r.Floor != nil {
			rb.Floor = &floorBody{Memory: r.Floor.Memory, CPU: r.Floor.CPU, MeasuredWith: r.Floor.MeasuredWith}
		}
		body.Roles = append(body.Roles, rb)
	}
	for _, f := range b.Features {
		body.Features = append(body.Features, featureBody{ID: f.ID, Title: f.Title, Summary: f.Summary, Group: f.Group})
	}
	return body
}

// handlePreset applies a role preset to the profile after the same preparation a save does (ADR 0015 §7).
func (s *Server) handlePreset(w http.ResponseWriter, r *http.Request) {
	var req presetRequest
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
	res, ok := s.prepareProfile(w, bundles, &p, req.SourceSHA256)
	if !ok {
		return
	}
	out, sw, err := p.ApplyPreset(bundles, req.Role, loadedToggles(res))
	switch {
	case errors.Is(err, profile.ErrUnknownRole), errors.Is(err, profile.ErrChartSwitchLoaded),
		errors.Is(err, profile.ErrPresetConflictsLoaded):
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error(), "")
	default:
		writeJSON(w, presetResponse{Profile: out, ChartSwitch: sw})
	}
}

// loadedToggles maps feature id to the toggle the loaded values file back-derived; nil without a slot.
func loadedToggles(res *load.Result) map[string]bool {
	if res == nil {
		return nil
	}
	toggles := map[string]bool{}
	for _, a := range res.Report.Answers {
		id, isToggle := strings.CutSuffix(a.ID, model.ToggleSuffix)
		if v, isBool := a.Value.(bool); a.Stage == string(model.StageFeatures) && isToggle && isBool {
			toggles[id] = v
		}
	}
	return toggles
}
