package profile

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
)

// Field ids reported by Profile.Validate for the Role stage.
const (
	FieldRole           = "role"
	FieldCatalogue      = "catalogue"
	featuresFieldPrefix = "features."
)

// FeatureSource says who decided a feature's state (ADR 0015 §7).
type FeatureSource string

// Feature sources; a role change re-applies the preset only to features of SourcePreset (US-2.3).
const (
	SourcePreset FeatureSource = "preset"
	SourceLoaded FeatureSource = "loaded"
	SourceUser   FeatureSource = "user"
)

var knownSources = []FeatureSource{SourcePreset, SourceLoaded, SourceUser}

// FeatureState is the recorded state of one catalogue feature.
type FeatureState struct {
	Enabled bool          `json:"enabled" yaml:"enabled"`
	Source  FeatureSource `json:"source" yaml:"source"`
}

// ChartSwitch reports the target chart and version ApplyPreset replaced.
type ChartSwitch struct {
	FromChart   string `json:"fromChart"`
	FromVersion string `json:"fromVersion"`
	ToChart     string `json:"toChart"`
	// DroppedChoices are user toggles of the old chart, which the new chart cannot carry; never nil.
	DroppedChoices []string `json:"droppedChoices"`
	// CarriedAnswers are the answer ids kept: of a carriedStages stage and declared by the new bundle; sorted, never nil.
	CarriedAnswers []string `json:"carriedAnswers"`
	// DroppedAnswers are the other answer ids, with their open questions; sorted, never nil.
	DroppedAnswers []string `json:"droppedAnswers"`
}

// carriedStages are the stages whose answers survive a chart switch: they hold no chart-specific values.
var carriedStages = []model.Stage{model.StageTarget, model.StageFoundations}

// Errors of ApplyPreset and SetFeature; the API maps them to 422.
var (
	ErrUnknownRole       = errors.New("unknown role")
	ErrChartSwitchLoaded = errors.New("the role deploys another chart than the loaded values file; start a new configuration to switch charts")
	// ErrNoRole means the profile has no role yet.
	ErrNoRole = errors.New("no role is chosen yet")
	// ErrUnknownFeature means the id is not a feature of the role catalogue.
	ErrUnknownFeature = errors.New("unknown feature")
	// ErrFeatureOfOtherChart means the feature sets values of another chart than the role deploys.
	ErrFeatureOfOtherChart = errors.New("the feature sets values of another chart than the role deploys")
)

// msgOlderRevision is the open note for a model or catalogue saved with an older bundle revision.
const msgOlderRevision = "the profile was saved with revision %d of bundle %s@%s; this build ships revision %d"

// ApplyPreset sets role and its preset features; toggles from the loaded values file win over the preset,
// and user toggles win over both (ADR 0018). It expects a normalised Model, never modifies p's maps and returns p unchanged with any error.
// Answers are kept within a chart; a chart switch keeps only those of carriedStages the new bundle declares (ADR 0019, 0020).
func (p Profile) ApplyPreset(bundles []model.Bundle, role string, loaded map[string]bool) (Profile, *ChartSwitch, error) {
	cat, err := CatalogueBundle(bundles, p.Model, p.Catalogue)
	if err != nil {
		return p, nil, err
	}
	r := findRole(cat, role)
	if r == nil {
		return p, nil, fmt.Errorf("%w %q", ErrUnknownRole, role)
	}

	out := p
	out.Answers = maps.Clone(p.Answers)
	var sw *ChartSwitch
	if r.Chart != p.Target.Chart {
		if p.Source != nil {
			return p, nil, ErrChartSwitchLoaded
		}
		i := slices.IndexFunc(bundles, func(b model.Bundle) bool { return b.Chart == r.Chart && b.Latest })
		if i < 0 {
			return p, nil, fmt.Errorf("chart %q of role %s is not shipped", r.Chart, role)
		}
		sw = &ChartSwitch{FromChart: p.Target.Chart, FromVersion: p.Target.ChartVersion, ToChart: r.Chart, DroppedChoices: []string{}}
		out.Model = ModelRef{Chart: r.Chart, Version: bundles[i].Version, Revision: bundles[i].Revision}
		sw.CarriedAnswers, sw.DroppedAnswers = splitAnswers(bundles, p.Model, out.Model, p.Answers)
		maps.DeleteFunc(out.Answers, func(id string, _ any) bool { return slices.Contains(sw.DroppedAnswers, id) })
		out.Open = slices.DeleteFunc(slices.Clone(p.Open), func(q OpenQuestion) bool {
			id, ok := strings.CutPrefix(q.Field, answersFieldPrefix)
			return ok && slices.Contains(sw.DroppedAnswers, id)
		})
		out.Target.Chart, out.Target.ChartVersion = r.Chart, ""
		if cat, err = CatalogueBundle(bundles, out.Model, p.Catalogue); err != nil {
			return p, nil, err
		}
		if r = findRole(cat, role); r == nil {
			return p, nil, fmt.Errorf("%w %q in bundle %s@%s", ErrUnknownRole, role, cat.Chart, cat.Version)
		}
	}

	features := map[string]FeatureState{}
	for id, o := range r.Features {
		switch o {
		case model.OfferRecommended:
			features[id] = FeatureState{Enabled: true, Source: SourcePreset}
		case model.OfferAvailable:
			features[id] = FeatureState{Enabled: false, Source: SourcePreset}
		}
	}
	var foreign []string
	for _, id := range sortedKeys(loaded) {
		o, ok := r.Features[id]
		switch {
		case !ok:
			return p, nil, fmt.Errorf("loaded toggle %q is not a feature of bundle %s@%s", id, cat.Chart, cat.Version)
		case loaded[id] && !inScope(cat, r, id):
			foreign = append(foreign, id)
		case o != model.OfferNotOffered || loaded[id]:
			features[id] = FeatureState{Enabled: loaded[id], Source: SourceLoaded}
		}
	}
	if len(foreign) > 0 {
		return p, nil, fmt.Errorf("%w: %s", ErrFeatureOfOtherChart, strings.Join(foreign, ", "))
	}
	for _, id := range sortedKeys(p.Features) {
		f := p.Features[id]
		switch {
		case f.Source != SourceUser:
		case inScope(cat, r, id):
			features[id] = f
		case sw != nil:
			sw.DroppedChoices = append(sw.DroppedChoices, id)
		}
	}

	out.Role, out.Features, out.Catalogue = role, features, nil
	if cat.Chart != out.Model.Chart || cat.Version != out.Model.Version {
		out.Catalogue = &ModelRef{Chart: cat.Chart, Version: cat.Version, Revision: cat.Revision}
	}
	return out, sw, nil
}

// splitAnswers carries the answer ids whose stage in model from (by its recognize: rules or foundations:)
// is in carriedStages and that model to declares in foundations:, so the result validates; it drops the rest.
func splitAnswers(bundles []model.Bundle, from, to ModelRef, answers map[string]any) (carried, dropped []string) {
	stages := map[string]model.Stage{}
	if b := findBundle(bundles, from.Chart, from.Version); b != nil {
		for _, r := range b.Rules() {
			stages[r.Answer] = r.Stage
		}
		for _, id := range b.FoundationOrder() {
			f, _ := b.Foundation(id)
			stages[id] = f.Stage
		}
	}
	next := findBundle(bundles, to.Chart, to.Version)
	carried, dropped = []string{}, []string{}
	for _, id := range sortedKeys(answers) {
		_, declares := declared(next, id)
		if s, ok := stages[id]; ok && slices.Contains(carriedStages, s) && declares {
			carried = append(carried, id)
		} else {
			dropped = append(dropped, id)
		}
	}
	return carried, dropped
}

// CatalogueBundle is the bundle a profile reads roles from (ADR 0015 §7): the model's bundle when it
// declares roles, else a held catalogue that is shipped, declares roles and is of the fallback's chart,
// else the model.RoleCatalogue fallback. ApplyPreset, Validate and the server's save path share it.
func CatalogueBundle(bundles []model.Bundle, m ModelRef, held *ModelRef) (model.Bundle, error) {
	c, err := model.RoleCatalogue(bundles, m.Chart, m.Version)
	if err != nil || c.Chart == m.Chart && c.Version == m.Version || held == nil {
		return c, err
	}
	if b := findBundle(bundles, held.Chart, held.Version); b != nil && b.Chart == c.Chart && len(b.Roles) > 0 {
		return *b, nil
	}
	return c, nil
}

// inScope reports whether catalogue feature id sets values of the chart role r deploys (ADR 0018).
func inScope(cat model.Bundle, r *model.Role, id string) bool {
	return featureChart(cat, id) == r.Chart
}

func findRole(b model.Bundle, id string) *model.Role {
	i := slices.IndexFunc(b.Roles, func(r model.Role) bool { return r.ID == id })
	if i < 0 {
		return nil
	}
	return &b.Roles[i]
}

// RevisionNotes says which recorded bundle (model, catalogue) is an older revision than the shipped one.
// It expects a profile Validate accepted.
func (p Profile) RevisionNotes(bundles []model.Bundle) []string {
	var notes []string
	for _, ref := range []*ModelRef{&p.Model, p.Catalogue} {
		if ref == nil {
			continue
		}
		if b := findBundle(bundles, ref.Chart, ref.Version); b != nil && ref.Revision < b.Revision {
			notes = append(notes, fmt.Sprintf(msgOlderRevision, ref.Revision, ref.Chart, ref.Version, b.Revision))
		}
	}
	return notes
}

// validateRole checks catalogue, role and features against the recorded role catalogue.
func (p Profile) validateRole(bundles []model.Bundle) []FieldError {
	var errs []FieldError
	if p.Catalogue != nil {
		if p.Role == "" {
			errs = append(errs, FieldError{FieldCatalogue, "is only recorded with a role"})
		}
		errs = append(errs, checkRef(FieldCatalogue, *p.Catalogue, bundles)...)
	}
	if p.Role == "" {
		if len(p.Features) > 0 {
			errs = append(errs, FieldError{FieldRole, "is required while features are recorded"})
		}
		return errs
	}
	if len(errs) > 0 {
		return errs
	}
	if findBundle(bundles, p.Model.Chart, p.Model.Version) == nil {
		// validateModel already reports the unshipped model.
		return errs
	}
	// The same rule as ApplyPreset and save, so a profile that opens cleanly also saves cleanly.
	cat, err := CatalogueBundle(bundles, p.Model, p.Catalogue)
	ownRoles := err == nil && cat.Chart == p.Model.Chart && cat.Version == p.Model.Version
	switch {
	case ownRoles && p.Catalogue != nil:
		return append(errs, FieldError{FieldCatalogue, fmt.Sprintf("is omitted because bundle %s@%s declares roles itself", cat.Chart, cat.Version)})
	case !ownRoles && p.Catalogue == nil:
		return append(errs, FieldError{FieldRole, fmt.Sprintf("bundle %s@%s declares no roles and no catalogue is recorded", p.Model.Chart, p.Model.Version)})
	case !ownRoles && (cat.Chart != p.Catalogue.Chart || cat.Version != p.Catalogue.Version):
		return append(errs, FieldError{FieldCatalogue, fmt.Sprintf("must be a bundle of chart %q, where the role catalogue comes from", cat.Chart)})
	}
	r := findRole(cat, p.Role)
	if r == nil {
		return append(errs, FieldError{FieldRole, fmt.Sprintf("%q is not a role of bundle %s@%s", p.Role, cat.Chart, cat.Version)})
	}
	if p.Target.Chart != "" && r.Chart != p.Target.Chart {
		errs = append(errs, FieldError{FieldRole, fmt.Sprintf("role %s deploys chart %q, not target.chart %q", r.ID, r.Chart, p.Target.Chart)})
	}
	for _, id := range sortedKeys(p.Features) {
		f := p.Features[id]
		o, known := r.Features[id]
		switch {
		case !known:
			errs = append(errs, FieldError{featuresFieldPrefix + id, "is not a known feature"})
		case !slices.Contains(knownSources, f.Source):
			errs = append(errs, FieldError{featuresFieldPrefix + id, fmt.Sprintf("source %q is unknown", f.Source)})
		case f.Enabled && !inScope(cat, r, id):
			errs = append(errs, FieldError{featuresFieldPrefix + id, fmt.Sprintf("sets values of chart %s, not of chart %s that role %s deploys", featureChart(cat, id), r.Chart, r.ID)})
		case f.Enabled && o == model.OfferNotOffered && f.Source == SourcePreset:
			errs = append(errs, FieldError{featuresFieldPrefix + id, fmt.Sprintf("is not offered by role %s, so it cannot come from the preset", r.ID)})
		}
	}
	return errs
}

// featureChart is the chart catalogue feature id sets values of; "" when id is not in the catalogue.
func featureChart(cat model.Bundle, id string) string {
	i := slices.IndexFunc(cat.Features, func(f model.Feature) bool { return f.ID == id })
	if i < 0 {
		return ""
	}
	return cat.Features[i].Chart
}
