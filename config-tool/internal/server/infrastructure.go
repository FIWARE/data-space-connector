package server

import (
	"net/http"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// infrastructureResponse lists a bundle's Stage 4 dependencies in bundle order; every string is bundle data.
type infrastructureResponse struct {
	Dependencies []dependencyBody `json:"dependencies"`
}

type dependencyBody struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Help        string     `json:"help"`
	Doc         string     `json:"doc"`
	ClusterWide bool       `json:"clusterWide"`
	Modes       []modeBody `json:"modes"`
}

type modeBody struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Note          string `json:"note"`
	Warning       string `json:"warning"`
	Doc           string `json:"doc"`
	Development   bool   `json:"development"`
	ProductionDoc string `json:"productionDoc"`
	// Keys are the chart keys the mode may write, per-ingress ones included.
	Keys      []render.TargetRef `json:"keys"`
	Questions []questionBody     `json:"questions"`
}

// questionBody is one question of a mode; a per-ingress question appears once per ingress, its id expanded.
type questionBody struct {
	// ID is local to the dependency; the answer sits in profile.infrastructure.<dependency>.answers.<id>.
	ID    string               `json:"id"`
	Type  model.FoundationType `json:"type"`
	Label string               `json:"label"`
	Help  string               `json:"help"`
	Doc   string               `json:"doc"`
	// Options are the values a choice offers; empty for every other type.
	Options []model.Option `json:"options"`
	// Default is the template a derived answer comes from, in Stage 3's syntax; empty when there is none.
	Default string `json:"default"`
	// When names another question of the same mode that must equal a value; null when always asked.
	When     *questionWhenBody `json:"when"`
	ReadOnly bool              `json:"readOnly"`
	// Ingress is the row a per-ingress question was expanded for; null for a plain question.
	Ingress *ingressBody `json:"ingress"`
}

type questionWhenBody struct {
	Question string `json:"question"`
	Equals   string `json:"equals"`
}

type ingressBody struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Owner string `json:"owner"`
}

// handleInfrastructure lists the dependencies of chart@version with their modes and questions; an empty version
// means the latest.
func (s *Server) handleInfrastructure(w http.ResponseWriter, r *http.Request) {
	b, ok := s.queriedBundle(w, r)
	if !ok {
		return
	}
	deps := b.Dependencies()
	body := infrastructureResponse{Dependencies: make([]dependencyBody, 0, len(deps))}
	for _, d := range deps {
		dep := dependencyBody{ID: d.ID, Title: d.Title, Help: d.Help, Doc: d.Doc, ClusterWide: d.ClusterWide, Modes: make([]modeBody, 0, len(d.Modes))}
		for _, m := range d.Modes {
			dep.Modes = append(dep.Modes, newModeBody(b, d.ID, m))
		}
		body.Dependencies = append(body.Dependencies, dep)
	}
	writeJSON(w, body)
}

// newModeBody describes mode m of dependency dep; the bundle compiled, so every question's foundation exists.
func newModeBody(b model.Bundle, dep string, m model.Mode) modeBody {
	body := modeBody{
		ID: m.ID, Label: m.Label, Note: m.Note, Warning: m.Warning, Doc: m.Doc,
		Development: m.Development, ProductionDoc: m.ProductionDoc,
		Keys: render.ModeTargets(b, dep, m.ID), Questions: []questionBody{},
	}
	for _, q := range m.Questions {
		qb := questionBody{
			ID: q.ID, Type: q.Type, Label: q.Label, Help: q.Help, Doc: q.Doc,
			Options: append([]model.Option{}, q.Options...), Default: q.Default, ReadOnly: q.ReadOnly,
		}
		if q.When != nil {
			qb.When = &questionWhenBody{Question: q.When.Question, Equals: q.When.Equals}
		}
		body.Questions = append(body.Questions, qb)
	}
	if q := m.PerIngressQuestion; q != nil {
		for _, row := range b.Ingresses {
			id := model.IngressQuestionID(q.ID, row.ID)
			// A replacing question's default is the replaced target's, which only the compiled foundation knows.
			c, _ := b.Foundation(model.InfraQuestionID(dep, id))
			body.Questions = append(body.Questions, questionBody{
				ID: id, Type: q.Type, Label: q.Label, Help: q.Help, Doc: q.Doc,
				Options: []model.Option{}, Default: c.DefaultSource(), Ingress: &ingressBody{ID: row.ID, Label: row.Label, Owner: row.Owner},
			})
		}
	}
	return body
}
