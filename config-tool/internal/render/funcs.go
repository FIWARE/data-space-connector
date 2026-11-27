package render

import (
	"fmt"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

// evaluate computes a target's value. Every answer it references is in answers; ns is the same
// answers nested by id segment, the namespace its templates run against.
func evaluate(t model.CompiledTarget, answers, ns map[string]any) (any, error) {
	switch t.Fn {
	case model.FnConst:
		return t.Value, nil
	case model.FnCopy:
		// A copy target references exactly one answer; its value keeps its type.
		return answers[t.Refs[0]], nil
	case model.FnTpl:
		return model.Execute(t.Templates()[0], ns)
	case model.FnJoin:
		parts := make([]string, len(t.Templates()))
		for i, tpl := range t.Templates() {
			s, err := model.Execute(tpl, ns)
			if err != nil {
				return nil, err
			}
			parts[i] = s
		}
		return strings.Join(parts, t.Sep), nil
	}
	return nil, fmt.Errorf("fn %q is unknown", t.Fn)
}
