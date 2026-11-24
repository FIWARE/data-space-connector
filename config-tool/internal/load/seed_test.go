package load

import (
	"maps"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

func TestSeedAnswers(t *testing.T) {
	const (
		tir      = "dataspace.tirAddress"
		derived  = "https://tir.example.org/"
		given    = "https://tir.given.org/"
		toggleID = "dsp.rainbow.enabled"
	)
	foundation := Answer{ID: tir, Stage: string(model.StageFoundations), Value: derived}
	toggle := Answer{ID: toggleID, Stage: string(model.StageFeatures), Value: true}
	tests := []struct {
		name    string
		derived []Answer
		answers map[string]any
		want    map[string]any
	}{
		{"fills an empty slot", []Answer{foundation}, map[string]any{}, map[string]any{tir: derived}},
		{"nil answers", []Answer{foundation}, nil, map[string]any{tir: derived}},
		{"keeps a given answer", []Answer{foundation}, map[string]any{tir: given}, map[string]any{tir: given}},
		{"ignores a feature toggle", []Answer{toggle}, map[string]any{}, map[string]any{}},
		{"nothing derived from an invalid value", nil, map[string]any{"domain.base": "x"}, map[string]any{"domain.base": "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := maps.Clone(tt.answers)
			got := SeedAnswers(Report{Answers: tt.derived}, tt.answers)
			if !maps.Equal(got, tt.want) {
				t.Errorf("SeedAnswers = %v, want %v", got, tt.want)
			}
			if !maps.Equal(tt.answers, before) {
				t.Errorf("input mutated: %v, was %v", tt.answers, before)
			}
		})
	}
}
