package load

import (
	"maps"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

// SeedAnswers returns answers plus every foundations-stage answer the report derived that answers lacks (ADR 0022).
// A given answer always wins; answers is not mutated.
func SeedAnswers(r Report, answers map[string]any) map[string]any {
	out := make(map[string]any, len(answers)+len(r.Answers))
	maps.Copy(out, answers)
	for _, a := range r.Answers {
		if a.Stage != string(model.StageFoundations) {
			continue
		}
		if _, given := out[a.ID]; !given {
			out[a.ID] = a.Value
		}
	}
	return out
}
