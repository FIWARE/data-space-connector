package server

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// renderResponse is the tree of POST /api/render and its change against the previous render.
type renderResponse struct {
	Tree render.Tree     `json:"tree"`
	Diff render.TreeDiff `json:"diff"`
}

// foundationsResponse lists a bundle's foundations in declaration order with their fan-out.
type foundationsResponse struct {
	Foundations []foundationBody `json:"foundations"`
}

type foundationBody struct {
	ID    string               `json:"id"`
	Stage model.Stage          `json:"stage"`
	Type  model.FoundationType `json:"type"`
	// Default is the template a derived answer comes from; empty when there is none.
	Default string             `json:"default"`
	Label   string             `json:"label"`
	Help    string             `json:"help"`
	Doc     string             `json:"doc"`
	Targets []render.TargetRef `json:"targets"`
	Count   int                `json:"count"`
	// Components are the distinct top-level keys of Targets, sorted.
	Components []string `json:"components"`
}

// handleRender renders the profile's answers against the bundle it names, pinning the loaded file's
// hand edits, and diffs the tree against the previous render, which it then replaces.
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	var p profile.Profile
	if err := decodeJSON(w, r, &p); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid profile: "+err.Error(), "")
		return
	}
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	if _, ok := s.prepareProfile(w, bundles, &p, ""); !ok {
		return
	}
	// prepareProfile validated p, so its bundle is shipped.
	b := findBundle(bundles, p.Model.Chart, p.Model.Version)
	// Held across the render, so a concurrent load cannot reset lastRender to a tree it did not pin.
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	tree, err := render.Render(*b, p, render.Options{Overrides: overridesFor(s.loaded, b.Chart)})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	diff := render.Diff(*b, s.lastRender, tree)
	s.lastRender = tree
	writeJSON(w, renderResponse{Tree: tree, Diff: diff})
}

// overridesFor is the loaded slot when it holds values of chart; paths of another chart would pin nonsense.
func overridesFor(res *load.Result, chart string) *load.Result {
	if res == nil || res.Report.Chart != chart {
		return nil
	}
	return res
}

// resetRender makes the next render diff against an empty tree; the caller holds loadMu.
func (s *Server) resetRender() {
	s.lastRender = render.Tree{}
}

// handleFoundations lists the foundations of chart@version; an empty version means the latest.
func (s *Server) handleFoundations(w http.ResponseWriter, r *http.Request) {
	bundles, err := model.Versions(s.models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	chart, version := r.URL.Query().Get(queryChart), r.URL.Query().Get(queryVersion)
	i := slices.IndexFunc(bundles, func(b model.Bundle) bool {
		return b.Chart == chart && (b.Version == version || version == "" && b.Latest)
	})
	if i < 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no bundle for chart %q version %q", chart, version), "")
		return
	}
	b := bundles[i]
	body := foundationsResponse{Foundations: make([]foundationBody, 0, len(b.Foundations))}
	for _, f := range b.Foundations {
		targets := render.Targets(b, f.ID)
		components := []string{}
		for _, t := range targets {
			if !slices.Contains(components, t.Component) {
				components = append(components, t.Component)
			}
		}
		slices.Sort(components)
		body.Foundations = append(body.Foundations, foundationBody{
			ID: f.ID, Stage: f.Stage, Type: f.Type, Default: f.Default, Label: f.Label, Help: f.Help, Doc: f.Doc,
			Targets: targets, Count: len(targets), Components: components,
		})
	}
	writeJSON(w, body)
}
