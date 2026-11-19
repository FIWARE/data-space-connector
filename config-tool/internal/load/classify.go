package load

import (
	"fmt"
	"math"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	boolTag  = "!!bool"
	intTag   = "!!int"
	floatTag = "!!float"
	nullTag  = "!!null"

	noteAlias     = "an alias is not followed, so it does not answer %s"
	noteNotScalar = "not a scalar, so it does not answer %s"
	noteNotBool   = "not a boolean, so it does not answer %s"
	noteNull      = "null does not answer %s"
	notePattern   = "does not match the pattern of %s"
	noteDiffers   = "differs from %s = %v"
)

// classifier holds the state of one load: owned leaves derive answers in pass 1 (ownedPass),
// the rest in pass 2 (classify), each in document order, so the first value wins.
type classifier struct {
	markers []roundtrip.OwnedMarker
	// root is where the chart's values sit; rule paths are relative to it.
	root    roundtrip.Path
	rules   map[string]model.CompiledRule
	answers map[string]*Answer
	order   []string
	// leafAnswer and notes are keyed by path string; live leaf paths are unique.
	leafAnswer map[string]string
	notes      map[string]string
}

func newClassifier(markers []roundtrip.OwnedMarker, rules []model.CompiledRule, root roundtrip.Path) *classifier {
	c := &classifier{
		markers:    markers,
		root:       root,
		rules:      make(map[string]model.CompiledRule, len(rules)),
		answers:    map[string]*Answer{},
		leafAnswer: map[string]string{},
		notes:      map[string]string{},
	}
	for _, r := range rules {
		c.rules[r.Path.String()] = r
	}
	return c
}

func (c *classifier) owned(p roundtrip.Path) bool {
	_, ok := roundtrip.OwnedBy(c.markers, p)
	return ok
}

// ownedPass derives answers from owned leaves only; its classes are discarded.
func (c *classifier) ownedPass(p roundtrip.Path, n *yaml.Node) roundtrip.Class {
	if !c.owned(p) {
		return roundtrip.ClassUnknown
	}
	c.derive(p, n, FromOwned)
	return roundtrip.ClassOwned
}

// classify is the final classifier; owned leaves already derived their answers in ownedPass.
func (c *classifier) classify(p roundtrip.Path, n *yaml.Node) roundtrip.Class {
	switch {
	case c.owned(p):
		return roundtrip.ClassOwned
	case c.derive(p, n, FromRecognized):
		return roundtrip.ClassRecognized
	}
	return roundtrip.ClassUnknown
}

// derive applies p's rule to n, setting the answer when it is still unset. It reports whether n
// agrees with the answer; a leaf under a rule that does not gets a note.
func (c *classifier) derive(p roundtrip.Path, n *yaml.Node, from Origin) bool {
	rel, ok := under(p, c.root)
	if !ok {
		return false
	}
	rule, ok := c.rules[rel.String()]
	if !ok {
		return false
	}
	key := p.String()
	v, note := readValue(rule, n)
	if note != "" {
		c.notes[key] = note
		return false
	}
	a, set := c.answers[rule.Answer]
	switch {
	case !set:
		a = &Answer{ID: rule.Answer, Stage: string(rule.Stage), Value: v, From: from}
		c.answers[rule.Answer] = a
		c.order = append(c.order, rule.Answer)
	case a.Value != v:
		c.notes[key] = fmt.Sprintf(noteDiffers, rule.Answer, a.Value)
		return false
	}
	a.Paths = append(a.Paths, key)
	c.leafAnswer[key] = rule.Answer
	return true
}

// answerList returns the answers in the order they were set.
func (c *classifier) answerList() []Answer {
	list := make([]Answer, 0, len(c.order))
	for _, id := range c.order {
		list = append(list, *c.answers[id])
	}
	return list
}

// markerList reports the owned markers; call it after ownedPass.
func (c *classifier) markerList() []Marker {
	list := []Marker{}
	for _, m := range c.markers {
		underived := []string{}
		for _, id := range m.AnswerIDs {
			if a, ok := c.answers[id]; (!ok || a.From != FromOwned) && !slices.Contains(underived, id) {
				underived = append(underived, id)
			}
		}
		list = append(list, Marker{Path: m.Path.String(), Line: m.Line,
			AnswerIDs: slices.Clone(m.AnswerIDs), Underived: underived})
	}
	return list
}

// readValue derives rule's answer from n, or returns the note on why n does not answer it.
func readValue(rule model.CompiledRule, n *yaml.Node) (any, string) {
	switch {
	case n.Kind == yaml.AliasNode:
		return nil, fmt.Sprintf(noteAlias, rule.Answer)
	case n.Kind != yaml.ScalarNode:
		return nil, fmt.Sprintf(noteNotScalar, rule.Answer)
	}
	tag := n.ShortTag()
	if rule.Kind == model.KindToggle {
		var b bool
		if tag != boolTag || n.Decode(&b) != nil {
			return nil, fmt.Sprintf(noteNotBool, rule.Answer)
		}
		return b, ""
	}
	if tag == nullTag {
		return nil, fmt.Sprintf(noteNull, rule.Answer)
	}
	if rule.Pattern != nil {
		m := rule.Pattern.FindStringSubmatch(n.Value)
		if m == nil {
			return nil, fmt.Sprintf(notePattern, rule.Answer)
		}
		return m[1], ""
	}
	return scalarValue(n), ""
}

// scalarValue types a value rule's scalar: bool, int64 or finite float64 when its tag says so and
// it decodes, else its text. Comparable and JSON-safe either way.
func scalarValue(n *yaml.Node) any {
	switch n.ShortTag() {
	case boolTag:
		var b bool
		if n.Decode(&b) == nil {
			return b
		}
	case intTag:
		var i int64
		if n.Decode(&i) == nil {
			return i
		}
	case floatTag:
		var f float64
		if n.Decode(&f) == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
			return f
		}
	}
	return n.Value
}
