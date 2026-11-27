package profile

import (
	"maps"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

// InfraChoice is the stored state of one Stage 4 dependency (ADR 0026).
// Answers is keyed by local question id and keeps the answers of inactive modes, so switching back restores them.
type InfraChoice struct {
	// Mode is the chosen mode id; "" while none is chosen.
	Mode    string         `json:"mode" yaml:"mode,omitempty"`
	Answers map[string]any `json:"answers,omitempty" yaml:"answers,omitempty"`
}

// InfraAnswers returns the flat infra.<dep>.mode / infra.<dep>.<question> view render and check consume.
// Answers of inactive modes are included; the model's mode conditions keep them from rendering.
func (p Profile) InfraAnswers() map[string]any {
	out := map[string]any{}
	for dep, c := range p.Infrastructure {
		if c.Mode != "" {
			out[model.InfraModeID(dep)] = c.Mode
		}
		for q, v := range c.Answers {
			out[model.InfraQuestionID(dep, q)] = v
		}
	}
	return out
}

// SetInfraMode returns a copy of p with dep's mode set; the answers of every mode are kept.
func (p Profile) SetInfraMode(dep, mode string) Profile {
	return p.withInfra(dep, func(c *InfraChoice) { c.Mode = mode })
}

// SetInfraAnswer returns a copy of p with dep's question answered; a nil v removes the answer.
func (p Profile) SetInfraAnswer(dep, question string, v any) Profile {
	return p.withInfra(dep, func(c *InfraChoice) {
		if v == nil {
			delete(c.Answers, question)
			return
		}
		if c.Answers == nil {
			c.Answers = map[string]any{}
		}
		c.Answers[question] = v
	})
}

// withInfra returns a copy of p whose entry dep is edited; p's maps are never modified.
func (p Profile) withInfra(dep string, edit func(*InfraChoice)) Profile {
	p.Infrastructure = cloneInfrastructure(p.Infrastructure)
	if p.Infrastructure == nil {
		p.Infrastructure = map[string]InfraChoice{}
	}
	c := p.Infrastructure[dep]
	edit(&c)
	p.Infrastructure[dep] = c
	return p
}

// validateInfrastructure refuses an unknown dependency, a mode it does not offer, a question none of its modes
// declares, an answer to a read-only question and a value the question's type refuses; fields are infrastructure.<dep>[.<question>].
func (p Profile) validateInfrastructure(b *model.Bundle) []FieldError {
	var errs []FieldError
	for _, dep := range sortedKeys(p.Infrastructure) {
		field := model.InfrastructureFieldPrefix + dep
		var d model.Dependency
		ok := b != nil
		if ok {
			d, ok = b.Dependency(dep)
		}
		if !ok {
			errs = append(errs, FieldError{field, "is not a known dependency"})
			continue
		}
		c := p.Infrastructure[dep]
		if _, offered := d.Mode(c.Mode); c.Mode != "" && !offered {
			errs = append(errs, FieldError{field, "must be " + quotedAlternatives(modeIDs(d))})
		}
		for _, q := range sortedKeys(c.Answers) {
			qField := field + "." + q
			f, ok := b.Foundation(model.InfraQuestionID(dep, q))
			if !ok || q == model.ModeQuestion {
				errs = append(errs, FieldError{qField, "is not a question of " + dep})
				continue
			}
			if isReadOnly(d, q) {
				errs = append(errs, FieldError{qField, "is read-only"})
				continue
			}
			if err := f.Check(c.Answers[q]); err != nil {
				errs = append(errs, FieldError{qField, err.Error()})
			}
		}
	}
	return errs
}

// isInfraAnswerID reports whether id lies in the infra namespace, whose answers live only under infrastructure.
func isInfraAnswerID(id string) bool {
	return strings.HasPrefix(id, model.InfraIDPrefix)
}

// isReadOnly reports whether question q of d shows only its derived value, so a stored answer would override it.
func isReadOnly(d model.Dependency, q string) bool {
	for _, m := range d.Modes {
		for _, mq := range m.Questions {
			if mq.ID == q {
				return mq.ReadOnly
			}
		}
	}
	return false
}

// modeIDs lists the mode ids d offers, in bundle order.
func modeIDs(d model.Dependency) []string {
	ids := make([]string, len(d.Modes))
	for i, m := range d.Modes {
		ids[i] = m.ID
	}
	return ids
}

// cloneInfrastructure copies m and each entry's answers, so a setter never modifies the caller's maps; nil stays nil.
func cloneInfrastructure(m map[string]InfraChoice) map[string]InfraChoice {
	if m == nil {
		return nil
	}
	out := make(map[string]InfraChoice, len(m))
	for dep, c := range m {
		c.Answers = maps.Clone(c.Answers)
		out[dep] = c
	}
	return out
}
