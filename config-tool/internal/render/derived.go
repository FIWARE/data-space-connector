package render

import (
	"fmt"
	"reflect"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

// AnswerDiff is what an answer change does to the answers whose default derives from other answers.
type AnswerDiff struct {
	Rederived []DerivedChange `json:"rederived"`
	// Pinned lists the given answers whose derived value moved: they are pinned, not updated.
	Pinned []PinnedAnswer `json:"pinned"`
}

// DerivedChange is an unanswered derived answer whose resolved value changed; nil stands for unresolved.
type DerivedChange struct {
	ID     string `json:"id"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// PinnedAnswer is a given answer kept while its derived value moved from Before to After; "" when nothing derives.
type PinnedAnswer struct {
	ID     string `json:"id"`
	Kept   any    `json:"kept"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// DerivedDiff compares the derived answers of prev and next in FoundationOrder. A prev of another chart or
// version than b is no baseline, so the diff is empty; next must be on b.
func DerivedDiff(b model.Bundle, prev, next profile.Profile) (AnswerDiff, error) {
	diff := AnswerDiff{Rederived: []DerivedChange{}, Pinned: []PinnedAnswer{}}
	if prev.Model.Chart != b.Chart || prev.Model.Version != b.Version {
		return diff, nil
	}
	before, beforeNS, err := resolve(b, prev.Answers)
	if err != nil {
		return AnswerDiff{}, err
	}
	after, afterNS, err := resolve(b, next.Answers)
	if err != nil {
		return AnswerDiff{}, err
	}
	for _, id := range b.FoundationOrder() {
		f, _ := b.Foundation(id)
		if f.DefaultTemplate() == nil || len(f.DefaultRefs()) == 0 {
			continue
		}
		kept, given := next.Answers[id]
		if !given {
			if !reflect.DeepEqual(before[id], after[id]) {
				diff.Rederived = append(diff.Rederived, DerivedChange{ID: id, Before: before[id], After: after[id]})
			}
			continue
		}
		was, err := derived(f, before, beforeNS)
		if err != nil {
			return AnswerDiff{}, err
		}
		now, err := derived(f, after, afterNS)
		if err != nil {
			return AnswerDiff{}, err
		}
		if was != now {
			diff.Pinned = append(diff.Pinned, PinnedAnswer{ID: id, Kept: kept, Before: was, After: now})
		}
	}
	return diff, nil
}

// derived evaluates f's default over resolved answers; "" when the default does not apply or f's type refuses it.
func derived(f model.CompiledFoundation, answers, ns map[string]any) (string, error) {
	if !defaults(f, answers) {
		return "", nil
	}
	s, err := execute(f.DefaultTemplate(), ns)
	if err != nil {
		return "", fmt.Errorf("render: default of %s: %w", f.ID, err)
	}
	if f.Check(s) != nil {
		return "", nil
	}
	return s, nil
}
