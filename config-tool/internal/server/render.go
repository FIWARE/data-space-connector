package server

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/check"
	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// renderRequest is the body of POST /api/render; SourceSHA256 names the loaded slot as on save, and
// empty renders without overrides whatever the server holds.
type renderRequest struct {
	Profile      profile.Profile `json:"profile"`
	SourceSHA256 string          `json:"sourceSha256"`
}

// renderResponse is the tree of POST /api/render, its change against the previous render, the
// answers given in place of their derived default and the L2 findings, which never fail the request.
type renderResponse struct {
	Tree render.Tree     `json:"tree"`
	Diff render.TreeDiff `json:"diff"`
	// Derived is what the answer change did to the derived answers since the previous render.
	Derived  render.AnswerDiff      `json:"derived"`
	HandSet  []render.HandSetAnswer `json:"handSet"`
	Findings []check.Finding        `json:"findings"`
}

// foundationsResponse lists a bundle's foundations in declaration order with their fan-out.
type foundationsResponse struct {
	Foundations []foundationBody `json:"foundations"`
}

type foundationBody struct {
	ID    string               `json:"id"`
	Stage model.Stage          `json:"stage"`
	Type  model.FoundationType `json:"type"`
	// Group is the Stage 3 panel, never empty.
	Group model.FoundationGroup `json:"group"`
	// Default is the template a derived answer comes from; empty when there is none.
	Default string `json:"default"`
	// DefaultWhen gates Default on another answer; null when Default always applies.
	DefaultWhen *model.Condition `json:"defaultWhen"`
	// Prefix is the template the answer must start with; empty when there is none.
	Prefix string `json:"prefix"`
	// Options are the values a choice offers; empty for every other type.
	Options []model.Option     `json:"options"`
	Label   string             `json:"label"`
	Help    string             `json:"help"`
	Doc     string             `json:"doc"`
	Targets []render.TargetRef `json:"targets"`
	Count   int                `json:"count"`
	// Components are the distinct top-level keys of Targets, sorted.
	Components []string `json:"components"`
	// Features hide the foundation in Stage 3 while any is off; empty when it is always shown.
	Features []string `json:"features"`
}

// handleRender renders the profile's answers against the bundle it names, pinning the hand edits of the
// load sourceSha256 names, and diffs the tree against the previous render of that same source.
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	var req renderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, bodyErrorStatus(err), "invalid render request: "+err.Error(), "")
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
	// prepareProfile validated p, so its bundle is shipped.
	b := findBundle(bundles, p.Model.Chart, p.Model.Version)
	// Held across the render, so a concurrent load cannot reset lastRender to a tree it did not pin.
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	if res != nil && res != s.loaded {
		writeError(w, http.StatusUnprocessableEntity, errSourceMismatch.Error(), "")
		return
	}
	tree, err := render.Render(*b, p, render.Options{Overrides: overridesFor(res, b.Chart)})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	handSet, err := render.HandSet(*b, p)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	findings, err := check.Evaluate(*b, p)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	// A render of another source (e.g. a new configuration after a load) or of another chart or version (a
	// Stage 1 switch keeps the source) starts a new baseline.
	if s.lastRenderSHA != req.SourceSHA256 || s.lastProfile.Model != p.Model {
		s.resetRender()
	}
	derived, err := render.DerivedDiff(*b, s.lastProfile, p)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "")
		return
	}
	diff := render.Diff(*b, s.lastRender, tree)
	s.lastRender, s.lastRenderSHA, s.lastProfile = tree, req.SourceSHA256, p
	writeJSON(w, renderResponse{Tree: tree, Diff: diff, Derived: derived, HandSet: handSet, Findings: findings})
}

// overridesFor is the loaded slot when it holds values of chart; paths of another chart would pin nonsense.
func overridesFor(res *load.Result, chart string) *load.Result {
	if res == nil || res.Report.Chart != chart {
		return nil
	}
	return res
}

// resetRender makes the next render diff against an empty tree and no previous answers; the caller holds loadMu.
func (s *Server) resetRender() {
	s.lastRender = render.Tree{}
	s.lastProfile = profile.Profile{}
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
		// Declared foundations compile, so the compiled one exists.
		c, _ := b.Foundation(f.ID)
		body.Foundations = append(body.Foundations, foundationBody{
			ID: f.ID, Stage: f.Stage, Type: f.Type, Group: c.Group, Default: f.Default, DefaultWhen: c.DefaultWhen, Prefix: f.Prefix, Options: append([]model.Option{}, f.Options...),
			Label: f.Label, Help: f.Help, Doc: f.Doc,
			Targets: targets, Count: len(targets), Components: components, Features: append([]string{}, f.Features...),
		})
	}
	writeJSON(w, body)
}
