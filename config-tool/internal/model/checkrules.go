package model

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// Severity says whether an L2 finding blocks generate (ADR 0021).
type Severity string

// Severities, most severe first.
const (
	// SeverityError blocks generate.
	SeverityError Severity = "error"
	// SeverityWarning is shown but never blocks.
	SeverityWarning Severity = "warning"
)

// Field ids a check rule may address; they are the open-field ids the UI links to.
const (
	// FieldTargetEnvironment is the Target stage's environment.
	FieldTargetEnvironment = "target.environment"
	// AnswerFieldPrefix starts the field id of a foundation answer: answers.<foundation id>.
	AnswerFieldPrefix = "answers."
	// FeatureFieldPrefix starts the field id of a catalogue feature, features.<feature id>; only a when addresses it.
	FeatureFieldPrefix = "features."
)

// Values a features.<id> field resolves to; a feature the profile does not record is off.
const (
	FeatureOn  = "on"
	FeatureOff = "off"
)

const (
	// docAnchor separates a doc path from its anchor.
	docAnchor = "#"
	// idSeparator separates the segments of a foundation id.
	idSeparator = "."
)

// The keys a distinct rule's message template may use.
const (
	// MessageFirst is the label of the earlier foundation of a colliding pair, in foundation order.
	MessageFirst = "first"
	// MessageSecond is the label of the later foundation of the pair.
	MessageSecond = "second"
	// MessageHost is the host both resolve to.
	MessageHost = "host"
)

// MessageComponents is the key a memoryLimit message template may use: the labels of the unlimited components.
const MessageComponents = "components"

var (
	// distinctKeys are the keys a distinct message may use.
	distinctKeys = []string{MessageFirst, MessageSecond, MessageHost}
	// memoryLimitKeys are the keys a memoryLimit message may use.
	memoryLimitKeys = []string{MessageComponents}
)

var (
	knownSeverities = []Severity{SeverityError, SeverityWarning}
	// targetFields are the Target fields a check rule may address, with the values each takes. They mirror
	// profile.Target and profile.Environments, which import this package; internal/check's test keeps them equal.
	targetFields = map[string]struct {
		stage  Stage
		values []string
	}{
		FieldTargetEnvironment: {StageTarget, []string{"development", "production"}},
	}
)

// Rank orders severities most severe first.
func (s Severity) Rank() int {
	return slices.Index(knownSeverities, s)
}

// Rank orders stages in wizard order.
func (s Stage) Rank() int {
	return slices.Index(knownStages, s)
}

// CheckRule is an L2 rule as written in a bundle's rules list. It has exactly one kind: When (a finding on
// Field while every condition holds), Distinct, DIDWebHost, or MemoryLimit, which needs When as its precondition
// and Field; Distinct and DIDWebHost name their field in the finding.
type CheckRule struct {
	ID          string           `yaml:"id"`
	Severity    Severity         `yaml:"severity"`
	Stage       Stage            `yaml:"stage"`
	Field       string           `yaml:"field"`
	When        []RuleWhen       `yaml:"when"`
	Distinct    *RuleDistinct    `yaml:"distinct"`
	DIDWebHost  *RuleDIDWebHost  `yaml:"didWebHost"`
	MemoryLimit *RuleMemoryLimit `yaml:"memoryLimit"`
	// Message is fixed text, except for a distinct rule (a template over MessageFirst, MessageSecond and
	// MessageHost) and a memoryLimit rule (a template over MessageComponents).
	Message string `yaml:"message"`
	// Doc is a repo-relative documentation path, optionally with an anchor.
	Doc string `yaml:"doc"`
}

// RuleDistinct finds every pair of hostname foundations under Prefix that resolve to the same host.
type RuleDistinct struct {
	// Prefix is a foundation id prefix ending in a dot, such as host.; it matches at least two foundations.
	Prefix string `yaml:"prefix"`
}

// RuleDIDWebHost finds a did:web DID whose host is not the host serving its DID document.
type RuleDIDWebHost struct {
	// DID is the id of a did foundation, Host the id of the hostname foundation serving its document.
	DID  string `yaml:"did"`
	Host string `yaml:"host"`
}

// RuleMemoryLimit finds the deployed components of the resources table that run without a memory limit.
type RuleMemoryLimit struct {
	// Components are ids of the table's components or unsized entries.
	Components []string `yaml:"components"`
}

// RuleWhen holds while the resolved Field equals Equals, or with Unset while the answer Field is absent.
// It has exactly one of the two; an absent field never equals anything.
type RuleWhen struct {
	Field  string `yaml:"field"`
	Equals string `yaml:"equals"`
	Unset  bool   `yaml:"unset"`
}

// CheckRules returns the validated check rules in bundle order.
func (b Bundle) CheckRules() []CheckRule {
	return slices.Clone(b.Checks)
}

// DistinctIDs lists the foundation ids r's prefix matches, in foundation order; nil for another kind of rule.
func (b Bundle) DistinctIDs(r CheckRule) []string {
	if r.Distinct == nil {
		return nil
	}
	var ids []string
	for _, id := range b.order {
		if strings.HasPrefix(id, r.Distinct.Prefix) {
			ids = append(ids, id)
		}
	}
	return ids
}

// RenderMessage returns r's message, with data filled in for a distinct or memoryLimit rule.
func (r CheckRule) RenderMessage(data map[string]string) (string, error) {
	if r.Distinct == nil && r.MemoryLimit == nil {
		return r.Message, nil
	}
	t, _, err := parseTemplate(r.ID, r.Message)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// TargetFieldIDs lists the Target fields a check rule may address, sorted.
func TargetFieldIDs() []string {
	return slices.Sorted(maps.Keys(targetFields))
}

// TargetFieldValues lists the values a rule may compare Target field id with; nil for an unknown id.
func TargetFieldValues(id string) []string {
	return slices.Clone(targetFields[id].values)
}

// checkCheckRules validates rules against the compiled foundations fs, the feature catalogue and the resources table.
func checkCheckRules(rules []CheckRule, fs []CompiledFoundation, features []Feature, order []string, table *ResourceTable) error {
	seen := map[string]int{}
	for i, r := range rules {
		if j, dup := seen[r.ID]; dup {
			return fmt.Errorf("rules[%d]: id %q is already used by rules[%d]", i, r.ID, j)
		}
		seen[r.ID] = i
		if err := checkCheckRule(r, fs, features, order, table); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

func checkCheckRule(r CheckRule, fs []CompiledFoundation, features []Feature, order []string, table *ResourceTable) error {
	switch {
	case r.ID == "":
		return errors.New("id is missing")
	case !slices.Contains(knownSeverities, r.Severity):
		return fmt.Errorf("severity %q is unknown", r.Severity)
	case !slices.Contains(knownStages, r.Stage):
		return fmt.Errorf("stage %q is unknown", r.Stage)
	case r.Message == "":
		return errors.New("message is missing")
	}
	kinds := 0
	// A memoryLimit rule's when is its precondition, not a kind of its own.
	for _, has := range []bool{len(r.When) > 0 && r.MemoryLimit == nil, r.Distinct != nil, r.DIDWebHost != nil, r.MemoryLimit != nil} {
		if has {
			kinds++
		}
	}
	switch {
	case kinds == 0:
		return errors.New("when, distinct, didWebHost or memoryLimit is missing")
	case kinds > 1 && r.MemoryLimit != nil:
		return errors.New("memoryLimit combines with when only, not with distinct or didWebHost")
	case kinds > 1:
		return errors.New("a rule has exactly one of when, distinct and didWebHost")
	case r.MemoryLimit != nil && (len(r.When) == 0 || r.Field == ""):
		return errors.New("memoryLimit needs when, its precondition, and field, where the finding points")
	case len(r.When) == 0 && r.Field != "":
		return errors.New("field is for when only: the finding of distinct and didWebHost names its field")
	}
	var err error
	switch {
	case r.Distinct != nil:
		err = checkDistinct(r, fs, order)
	case r.DIDWebHost != nil:
		err = checkDIDWebHost(r, fs)
	case r.MemoryLimit != nil:
		if err = checkWhenRule(r, fs, features); err == nil {
			err = checkMemoryLimit(r, table)
		}
	default:
		err = checkWhenRule(r, fs, features)
	}
	if err != nil {
		return err
	}
	return checkDoc(r.Doc)
}

func checkWhenRule(r CheckRule, fs []CompiledFoundation, features []Feature) error {
	stage, _, err := addressable(r.Field, fs)
	if err != nil {
		return fmt.Errorf("field: %w", err)
	}
	// The stage labels and links the finding, so it has to be the stage that shows the field.
	if stage != r.Stage {
		return fmt.Errorf("stage %s is not %s, the stage of field %s", r.Stage, stage, r.Field)
	}
	for i, w := range r.When {
		if err := checkWhen(w, fs, features); err != nil {
			return fmt.Errorf("when[%d]: %w", i, err)
		}
	}
	return nil
}

// checkDistinct refuses a prefix that is not a dotted segment or matches fewer than two foundations, a matched
// foundation that is not a hostname shown in the rule's stage, and a message using keys other than distinctKeys.
func checkDistinct(r CheckRule, fs []CompiledFoundation, order []string) error {
	p := r.Distinct.Prefix
	if p == "" || !strings.HasSuffix(p, idSeparator) {
		return fmt.Errorf("distinct: prefix %q does not end in %q", p, idSeparator)
	}
	n := 0
	for _, id := range order {
		if !strings.HasPrefix(id, p) {
			continue
		}
		n++
		if err := checkRuleFoundation(r.Stage, id, TypeHostname, fs); err != nil {
			return fmt.Errorf("distinct: %w", err)
		}
	}
	if n < 2 {
		return fmt.Errorf("distinct: prefix %q matches %d foundations, want at least 2", p, n)
	}
	return checkMessage(r, distinctKeys)
}

// checkMemoryLimit refuses a rule without a resources table, without components, or naming a component the table
// does not declare or one twice, and a message using keys other than memoryLimitKeys.
func checkMemoryLimit(r CheckRule, table *ResourceTable) error {
	ids := r.MemoryLimit.Components
	switch {
	case table == nil:
		return errors.New("memoryLimit: the bundle has no resources table")
	case len(ids) == 0:
		return errors.New("memoryLimit: components is empty")
	}
	for i, id := range ids {
		if _, ok := table.Component(id); !ok {
			return fmt.Errorf("memoryLimit: components[%d]: %q names no entry of the resources table", i, id)
		}
		if slices.Index(ids, id) < i {
			return fmt.Errorf("memoryLimit: components[%d]: %q is listed twice", i, id)
		}
	}
	return checkMessage(r, memoryLimitKeys)
}

// checkMessage refuses a message template that does not parse or uses keys other than keys.
func checkMessage(r CheckRule, keys []string) error {
	_, refs, err := parseTemplate(r.ID, r.Message)
	if err != nil {
		return fmt.Errorf("message: %w", err)
	}
	for _, ref := range refs {
		if !slices.Contains(keys, ref) {
			return fmt.Errorf("message: {{ .%s }} is not one of %s", ref, strings.Join(keys, ", "))
		}
	}
	return nil
}

// checkDIDWebHost refuses a did that is not a did foundation shown in the rule's stage, or a host that is not a
// hostname foundation.
func checkDIDWebHost(r CheckRule, fs []CompiledFoundation) error {
	if err := checkRuleFoundation(r.Stage, r.DIDWebHost.DID, TypeDID, fs); err != nil {
		return fmt.Errorf("didWebHost: did: %w", err)
	}
	if err := checkRuleFoundation("", r.DIDWebHost.Host, TypeHostname, fs); err != nil {
		return fmt.Errorf("didWebHost: host: %w", err)
	}
	return nil
}

// checkRuleFoundation refuses an id that names no foundation of type t, or, when stage is set, one another stage shows.
func checkRuleFoundation(stage Stage, id string, t FoundationType, fs []CompiledFoundation) error {
	i := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == id })
	switch {
	case i < 0:
		return fmt.Errorf("%q names no foundation", id)
	case fs[i].Type != t:
		return fmt.Errorf("%s is of type %s, not %s", id, fs[i].Type, t)
	case stage != "" && fs[i].Stage != stage:
		// The stage labels and links the finding, so it has to be the stage that shows the field.
		return fmt.Errorf("stage %s is not %s, the stage of %s", stage, fs[i].Stage, id)
	}
	return nil
}

func checkWhen(w RuleWhen, fs []CompiledFoundation, features []Feature) error {
	var values []string
	if id, ok := strings.CutPrefix(w.Field, FeatureFieldPrefix); ok {
		if !slices.ContainsFunc(features, func(f Feature) bool { return f.ID == id }) {
			return fmt.Errorf("%q names no feature of the catalogue", w.Field)
		}
		values = []string{FeatureOn, FeatureOff}
	} else {
		var err error
		if _, values, err = addressable(w.Field, fs); err != nil {
			return err
		}
	}
	switch {
	case w.Unset && w.Equals != "":
		return fmt.Errorf("%s has both equals and unset", w.Field)
	case w.Unset && !strings.HasPrefix(w.Field, AnswerFieldPrefix):
		return fmt.Errorf("unset applies to %s<foundation id> only, not %s", AnswerFieldPrefix, w.Field)
	case w.Unset:
		return nil
	case w.Equals == "":
		return fmt.Errorf("%s has neither equals nor unset", w.Field)
	case values != nil && !slices.Contains(values, w.Equals):
		return fmt.Errorf("%q is not a value of %s (%s)", w.Equals, w.Field, strings.Join(values, ", "))
	}
	return nil
}

// addressable returns the stage of field and, when it takes a fixed set of values, those values.
func addressable(field string, fs []CompiledFoundation) (Stage, []string, error) {
	if t, ok := targetFields[field]; ok {
		return t.stage, t.values, nil
	}
	id, ok := strings.CutPrefix(field, AnswerFieldPrefix)
	i := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == id })
	if !ok || i < 0 {
		return "", nil, fmt.Errorf("%q is neither one of %s nor %s<foundation id>", field, strings.Join(TargetFieldIDs(), ", "), AnswerFieldPrefix)
	}
	if fs[i].Type == TypeChoice {
		return fs[i].Stage, fs[i].optionValues(), nil
	}
	return fs[i].Stage, nil, nil
}

// checkDoc refuses a doc that is not a clean repo-relative path, such as a URL or one leaving the repo.
func checkDoc(doc string) error {
	p, _, _ := strings.Cut(doc, docAnchor)
	if p == "" || path.IsAbs(p) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, ":") {
		return fmt.Errorf("doc %q is not a repo-relative path", doc)
	}
	return nil
}
