package render

import (
	"fmt"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

// HandSetAnswer is a given answer that overrides a default derived from other answers, so it no longer follows them.
type HandSetAnswer struct {
	ID    string `json:"id"`
	Value any    `json:"value"`
	// Derived is what the default gives now; empty when its refs are unanswered, its defaultWhen fails
	// or its type refuses the result.
	Derived string `json:"derived"`
}

// HandSet lists p's given answers whose foundation derives its default from other answers, in
// FoundationOrder. A constant default (a choice's) is left out: picking it is not pinning. Unlike
// Provenance.Pinned, which marks a hand edit of a loaded file, this is about answers.
func HandSet(b model.Bundle, p profile.Profile) ([]HandSetAnswer, error) {
	answers, ns, err := resolve(b, p.Answers)
	if err != nil {
		return nil, err
	}
	list := []HandSetAnswer{}
	for _, id := range b.FoundationOrder() {
		v, ok := p.Answers[id]
		f, _ := b.Foundation(id)
		if !ok || f.DefaultTemplate() == nil || len(f.DefaultRefs()) == 0 {
			continue
		}
		h := HandSetAnswer{ID: id, Value: v}
		if defaults(f, answers) {
			s, err := execute(f.DefaultTemplate(), ns)
			if err != nil {
				return nil, fmt.Errorf("render: default of %s: %w", id, err)
			}
			if f.Check(s) == nil {
				h.Derived = s
			}
		}
		list = append(list, h)
	}
	return list, nil
}
