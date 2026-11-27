// Package check evaluates a bundle's L2 rules over a profile (ADR 0021); only error findings block generate.
package check

import (
	"cmp"
	"net/url"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

// Finding is one rule that holds for a profile, addressed by the open-field id it points at.
type Finding struct {
	Rule     string         `json:"rule"`
	Severity model.Severity `json:"severity"`
	Stage    model.Stage    `json:"stage"`
	Field    string         `json:"field"`
	Message  string         `json:"message"`
	Doc      string         `json:"doc"`
	// Related is the other field a distinct or didWebHost finding concerns; empty for a when rule.
	Related string `json:"related,omitempty"`
}

const (
	// didWebPrefix starts a did:web DID; its method-specific id follows.
	didWebPrefix = "did:web:"
	// didWebSeparator ends the host of a did:web method-specific id; a path follows it, and decoded, a port.
	didWebSeparator = ":"
	// rootDot is the trailing dot of a fully qualified host name.
	rootDot = "."
)

// Evaluate returns the findings of b's rules over p, sorted by severity, stage and rule id; a rule's own
// findings keep foundation order. Rules read resolved answers, so a derived default counts, and never read
// features, since every host renders whatever the role. The precondition kinds read the tree rendered with opts
// over the loaded file. It errors when the answers do not resolve or the render fails.
func Evaluate(b model.Bundle, p profile.Profile, opts render.Options) ([]Finding, error) {
	answers, err := render.Answers(b, p)
	if err != nil {
		return nil, err
	}
	tree, err := render.Render(b, p, opts)
	if err != nil {
		return nil, err
	}
	values := newChartValues(tree, opts.Overrides)
	findings := []Finding{}
	for _, r := range b.CheckRules() {
		var fs []Finding
		switch {
		case r.Distinct != nil:
			fs, err = distinct(b, r, p, answers)
		case r.DIDWebHost != nil:
			fs = didWebHost(r, answers)
		// Before the plain when case: a memoryLimit rule's when is only its precondition.
		case r.MemoryLimit != nil:
			if allHold(b, r.When, p, answers) {
				fs, err = memoryLimit(b, r, values)
			}
		case r.CACoverage != nil:
			if allHold(b, r.When, p, answers) {
				fs, err = caCoverage(b, r, tree, values)
			}
		case r.NoProxyCoverage != nil:
			if allHold(b, r.When, p, answers) {
				fs, err = noProxyCoverage(b, r, p, answers, values)
			}
		case r.ValueSet != nil:
			if allHold(b, r.When, p, answers) && valueSet(r, values) {
				fs = []Finding{finding(r, r.Field, "", r.Message)}
			}
		case allHold(b, r.When, p, answers):
			fs = []Finding{finding(r, r.Field, "", r.Message)}
		}
		if err != nil {
			return nil, err
		}
		findings = append(findings, fs...)
	}
	slices.SortStableFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.Severity.Rank(), b.Severity.Rank()),
			cmp.Compare(a.Stage.Rank(), b.Stage.Rank()),
			strings.Compare(a.Rule, b.Rule),
		)
	})
	return findings, nil
}

// Blocking reports whether any finding is an error, which stops generate.
func Blocking(findings []Finding) bool {
	return slices.ContainsFunc(findings, func(f Finding) bool { return f.Severity == model.SeverityError })
}

func finding(r model.CheckRule, field, related, message string) Finding {
	return Finding{
		Rule: r.ID, Severity: r.Severity, Stage: r.Stage, Field: field, Related: related, Message: message, Doc: r.Doc,
	}
}

// distinct returns one finding per pair of r's foundations resolving to the same host. It points at the pinned
// one of the pair when exactly one is given, else at the later one.
func distinct(b model.Bundle, r model.CheckRule, p profile.Profile, answers map[string]any) ([]Finding, error) {
	ids := b.DistinctIDs(r)
	var out []Finding
	for i, first := range ids {
		h, ok := host(answers, first)
		if !ok {
			continue
		}
		for _, second := range ids[i+1:] {
			if h2, ok := host(answers, second); !ok || h2 != h {
				continue
			}
			msg, err := r.RenderMessage(map[string]string{
				model.MessageFirst: label(b, first), model.MessageSecond: label(b, second), model.MessageHost: h,
			})
			if err != nil {
				return nil, err
			}
			field, related := second, first
			if _, firstGiven := p.Answers[first]; firstGiven {
				if _, secondGiven := p.Answers[second]; !secondGiven {
					field, related = first, second
				}
			}
			out = append(out, finding(r, model.AnswerFieldPrefix+field, model.AnswerFieldPrefix+related, msg))
		}
	}
	return out, nil
}

// didWebHost returns a finding when the resolved DID is a did:web whose host is not the resolved document host.
func didWebHost(r model.CheckRule, answers map[string]any) []Finding {
	did, ok := answers[r.DIDWebHost.DID].(string)
	msid, isWeb := strings.CutPrefix(did, didWebPrefix)
	docHost, hostOK := host(answers, r.DIDWebHost.Host)
	if !ok || !isWeb || !hostOK {
		return nil
	}
	enc, _, _ := strings.Cut(msid, didWebSeparator)
	// The did type admits only well-formed percent-encoding, so decoding fails only on a hand-built profile.
	dec, err := url.PathUnescape(enc)
	if err != nil {
		dec = enc
	}
	didHost, _, _ := strings.Cut(dec, didWebSeparator)
	if normalize(didHost) == docHost {
		return nil
	}
	return []Finding{finding(r, model.AnswerFieldPrefix+r.DIDWebHost.DID, model.AnswerFieldPrefix+r.DIDWebHost.Host, r.Message)}
}

// host returns the normalized resolved answer id; false when it is unanswered.
func host(answers map[string]any, id string) (string, bool) {
	s, ok := answers[id].(string)
	return normalize(s), ok
}

// normalize makes host names compare equal regardless of case and a trailing root dot.
func normalize(h string) string {
	return strings.ToLower(strings.TrimSuffix(h, rootDot))
}

// label is the label of foundation id as the bundle declares it.
func label(b model.Bundle, id string) string {
	i := slices.IndexFunc(b.Foundations, func(f model.Foundation) bool { return f.ID == id })
	return b.Foundations[i].Label
}

// allHold reports whether every condition of ws holds.
func allHold(b model.Bundle, ws []model.RuleWhen, p profile.Profile, answers map[string]any) bool {
	return !slices.ContainsFunc(ws, func(w model.RuleWhen) bool { return !holds(b, w, p, answers) })
}

// holds reports whether w's field resolves to w.Equals, or for Unset or Set whether the answer is absent or
// resolves; an unanswered or non-string field never equals anything.
func holds(b model.Bundle, w model.RuleWhen, p profile.Profile, answers map[string]any) bool {
	if w.Unset || w.Set {
		_, ok := answers[answerID(b, w.Field)]
		return ok == w.Set
	}
	v, ok := value(b, w.Field, p, answers)
	return ok && v == w.Equals
}

// value reads a field the bundle validated as addressable.
func value(b model.Bundle, field string, p profile.Profile, answers map[string]any) (string, bool) {
	if field == model.FieldTargetEnvironment {
		return string(p.Target.Environment), p.Target.Environment != ""
	}
	if id, ok := strings.CutPrefix(field, model.FeatureFieldPrefix); ok {
		if p.Features[id].Enabled {
			return model.FeatureOn, true
		}
		return model.FeatureOff, true
	}
	s, ok := answers[answerID(b, field)].(string)
	return s, ok
}

// answerID is the resolved-answer id of an answers.<id> or infrastructure.<dep>[.<q>] field.
func answerID(b model.Bundle, field string) string {
	if id, ok := b.InfraFieldAnswer(field); ok {
		return id
	}
	return strings.TrimPrefix(field, model.AnswerFieldPrefix)
}
