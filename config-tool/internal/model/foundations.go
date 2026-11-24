package model

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// FoundationType is the value type of a foundation answer; each has one pattern.
type FoundationType string

// Foundation types.
const (
	// TypeString is a non-empty single line without leading or trailing whitespace, control characters or
	// any of " \ < >, because chart templates interpolate it unescaped into JSON and HTML (_realm.tpl).
	TypeString FoundationType = "string"
	// TypeHostname is a DNS-1123 subdomain, as an ingress host must be.
	TypeHostname FoundationType = "hostname"
	// TypeDomain is a hostname of at least two labels, the base other hosts are derived from.
	TypeDomain FoundationType = "domain"
	// TypeChoice is one of the foundation's declared option values; CompiledFoundation.Check checks it.
	TypeChoice FoundationType = "choice"
	// TypeDID is a decentralized identifier, did:<method>:<id>, without characters a chart template would
	// have to escape.
	TypeDID FoundationType = "did"
	// TypeURL is an absolute http or https URL with a host, without userinfo, fragment, whitespace or any
	// character TypeString refuses.
	TypeURL FoundationType = "url"
	// TypeBool is a yes/no answer; only a copy target may write it.
	TypeBool FoundationType = "bool"
	// TypeCredentialTypes is a list of {type, description?} mappings with unique types; only a copy target
	// may write it.
	TypeCredentialTypes FoundationType = "credentialTypes"
)

// FoundationGroup is the Stage 3 panel a foundation is asked in.
type FoundationGroup string

// Foundation groups; an empty group in the bundle means GroupNetwork.
const (
	GroupOrganization FoundationGroup = "organization"
	GroupDataspace    FoundationGroup = "dataspace"
	GroupNetwork      FoundationGroup = "network"
)

// Fn is a target function: how a target's value is made from answers (ADR 0020).
type Fn string

// Target functions; the bundle header documents each.
const (
	// FnTpl renders its one template arg.
	FnTpl Fn = "tpl"
	// FnJoin renders every template arg and joins the results with Sep.
	FnJoin Fn = "join"
	// FnCopy takes the one answer its template arg names, keeping the answer's type.
	FnCopy Fn = "copy"
	// FnConst writes its one scalar arg verbatim; only inside an owned list.
	FnConst Fn = "const"
)

// maxHostnameLength is the DNS limit on a full host name.
const maxHostnameLength = 253

const (
	// maxURLLength caps a url answer at the length browsers and proxies reliably accept.
	maxURLLength = 2048
	// maxCredentialTypes caps a credentialTypes answer; a data space announces a handful.
	maxCredentialTypes = 64
	// CredentialTypeKey is the required key of one credentialTypes element.
	CredentialTypeKey = "type"
	// CredentialDescriptionKey is the optional key of one credentialTypes element.
	CredentialDescriptionKey = "description"
)

// urlSchemes are the schemes a url answer may use.
var urlSchemes = []string{"http", "https"}

var (
	knownTypes = []FoundationType{TypeString, TypeHostname, TypeDomain, TypeChoice, TypeDID, TypeURL, TypeBool, TypeCredentialTypes}
	knownFns   = []Fn{FnTpl, FnJoin, FnCopy, FnConst}
	// knownFoundationGroups are the Stage 3 panels, in screen order.
	knownFoundationGroups = []FoundationGroup{GroupOrganization, GroupDataspace, GroupNetwork}
	// idSegment keeps every segment of an answer id usable as a template field name.
	idSegment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	plainLine = regexp.MustCompile(`^[^\s\x00-\x1f\x7f"\\<>]([^\x00-\x1f\x7f"\\<>]*[^\s\x00-\x1f\x7f"\\<>])?$`)
	hostname  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	// did follows DID Core's ABNF: method-specific-id = *( *idchar ":" ) 1*idchar.
	did = regexp.MustCompile(`^did:[a-z0-9]+:(?:` + didIDChar + `*:)*` + didIDChar + `+$`)
	// credentialType is the name of a verifiable credential type, such as UserCredential.
	credentialType = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]*$`)
)

// didIDChar is DID Core's idchar: ALPHA / DIGIT / "." / "-" / "_" / pct-encoded.
const didIDChar = `(?:[A-Za-z0-9._-]|%[0-9A-Fa-f]{2})`

// Foundation is a cross-cutting answer as declared in a bundle's foundations list.
type Foundation struct {
	// ID is dotted (domain.base); it keys profile.Answers.
	ID    string         `yaml:"id"`
	Stage Stage          `yaml:"stage"`
	Type  FoundationType `yaml:"type"`
	// Group is the Stage 3 panel; empty means GroupNetwork.
	Group FoundationGroup `yaml:"group"`
	// Default is a template over other answers; a defaulted answer exists only when all of them do.
	// Only a choice may have a constant default, which must be one of its options.
	Default string `yaml:"default"`
	// DefaultWhen gates Default on another answer; nil derives it whenever its refs are answered.
	DefaultWhen *Condition `yaml:"defaultWhen"`
	// Prefix is a template over other answers that the resolved value, given or derived, must start with.
	Prefix string `yaml:"prefix"`
	// Options are the values a choice offers; only a choice has them.
	Options []Option `yaml:"options"`
	Label   string   `yaml:"label"`
	Help    string   `yaml:"help"`
	// Doc is a repo-relative documentation path, optionally with an anchor.
	Doc     string   `yaml:"doc"`
	Targets []Target `yaml:"targets"`
	// Features are feature ids; Stage 3 hides the foundation while any is off. Display only: neither the
	// render nor the rules read them (US-5.3).
	Features []string `yaml:"features"`
}

// Condition holds while the resolved answer ID equals Equals.
type Condition struct {
	ID     string `yaml:"id" json:"id"`
	Equals string `yaml:"equals" json:"equals"`
}

// Option is one value of a choice; Note is optional text shown at the point of choice.
type Option struct {
	Value string `yaml:"value" json:"value"`
	Label string `yaml:"label" json:"label"`
	Note  string `yaml:"note" json:"note"`
}

// Target is one chart key a foundation writes.
type Target struct {
	// Path is in roundtrip.ParsePath syntax; an index is allowed only into a list the bundle owns whole.
	Path string `yaml:"path"`
	Fn   Fn     `yaml:"fn"`
	// Args are templates for tpl, join and copy, and the one scalar of const.
	Args []any  `yaml:"args"`
	Sep  string `yaml:"sep"`
}

// CompiledFoundation is a validated Foundation with its templates parsed.
type CompiledFoundation struct {
	ID    string
	Stage Stage
	Type  FoundationType
	Group FoundationGroup
	// Refs are the answer ids Default, DefaultWhen and Prefix reference, sorted; nil without any. They
	// order the foundations; DefaultRefs alone gates the default.
	Refs        []string
	DefaultWhen *Condition
	Options     []Option
	Targets     []CompiledTarget
	// Features gate the foundation's display only; nil when it is always shown.
	Features []string
	deflt    *template.Template
	prefix   *template.Template
	// defaultRefs and prefixRefs split Refs by source, so a bad reference is reported where it is.
	defaultRefs, prefixRefs []string
	// defaultGate is defaultRefs plus the DefaultWhen id: the answers whose values decide the default.
	defaultGate []string
}

// DefaultTemplate is the parsed Default; nil when the foundation has none.
func (f CompiledFoundation) DefaultTemplate() *template.Template {
	return f.deflt
}

// PrefixTemplate is the parsed Prefix; nil when the foundation has none.
func (f CompiledFoundation) PrefixTemplate() *template.Template {
	return f.prefix
}

// DefaultRefs are the answer ids Default and DefaultWhen reference, sorted; nil without any.
// Unlike Refs they leave out the prefix refs, which check a value but never gate its default.
func (f CompiledFoundation) DefaultRefs() []string {
	return f.defaultGate
}

// PrefixRefs are the answer ids Prefix references, sorted; nil without a prefix.
func (f CompiledFoundation) PrefixRefs() []string {
	return f.prefixRefs
}

// Check returns why v is not a valid answer of f, without echoing v; nil when it is valid.
func (f CompiledFoundation) Check(v any) error {
	if f.Type != TypeChoice {
		return f.Type.Check(v)
	}
	if s, ok := v.(string); ok && slices.Contains(f.optionValues(), s) {
		return nil
	}
	return fmt.Errorf("must be one of %s", strings.Join(f.optionValues(), ", "))
}

func (f CompiledFoundation) optionValues() []string {
	vs := make([]string, len(f.Options))
	for i, o := range f.Options {
		vs[i] = o.Value
	}
	return vs
}

// CompiledTarget is a validated Target.
type CompiledTarget struct {
	Path roundtrip.Path
	Fn   Fn
	// Value is the scalar of a const target; nil otherwise.
	Value any
	Sep   string
	// Refs are the answer ids the target references, sorted; empty for const.
	Refs []string
	tpls []*template.Template
}

// Templates are the parsed args of a tpl, join or copy target, in order; nil for const.
func (t CompiledTarget) Templates() []*template.Template {
	return t.tpls
}

// Foundation returns the compiled foundation id; false when the bundle does not declare it.
func (b Bundle) Foundation(id string) (CompiledFoundation, bool) {
	i := slices.IndexFunc(b.foundations, func(f CompiledFoundation) bool { return f.ID == id })
	if i < 0 {
		return CompiledFoundation{}, false
	}
	return b.foundations[i], true
}

// FoundationOrder lists the foundation ids so that each comes after the answers its Refs name; ties
// keep declaration order.
func (b Bundle) FoundationOrder() []string {
	return slices.Clone(b.order)
}

// Check returns why v is not a valid answer of pattern type t, without echoing v; nil when it is valid.
// A choice has no pattern and always fails here: CompiledFoundation.Check checks it against its options.
func (t FoundationType) Check(v any) error {
	switch t {
	case TypeBool:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("must be true or false")
		}
		return nil
	case TypeCredentialTypes:
		return checkCredentialTypes(v)
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}
	switch t {
	case TypeString:
		if !plainLine.MatchString(s) {
			return fmt.Errorf(`must be a single non-empty line without leading or trailing spaces, control characters, '"', '\', '<' or '>'`)
		}
	case TypeHostname, TypeDomain:
		if len(s) > maxHostnameLength || !hostname.MatchString(s) {
			return fmt.Errorf("must be a host name of lowercase letters, digits, '-' and '.', at most %d characters", maxHostnameLength)
		}
		if t == TypeDomain && !strings.Contains(s, ".") {
			return fmt.Errorf("must be a domain of at least two labels")
		}
	case TypeDID:
		if !did.MatchString(s) {
			return fmt.Errorf("must be a DID: did:<method>:<identifier>, the method of lowercase letters and digits, the identifier of letters, digits, '.', '_', '-', %%XX escapes and ':', not ending in ':'")
		}
	case TypeURL:
		return checkURL(s)
	case TypeChoice:
		return fmt.Errorf("a %s is checked against its foundation's options", t)
	default:
		return fmt.Errorf("type %q is unknown", t)
	}
	return nil
}

// IsString reports whether answers of t are strings, which a template may interpolate.
func (t FoundationType) IsString() bool {
	return t != TypeBool && t != TypeCredentialTypes
}

func checkURL(s string) error {
	const want = "must be an absolute http or https URL with a host, without user info, fragment, spaces, control characters, '\"', '\\', '<' or '>'"
	if len(s) > maxURLLength {
		return fmt.Errorf("must be a URL of at most %d bytes", maxURLLength)
	}
	if !plainLine.MatchString(s) || strings.ContainsFunc(s, unicode.IsSpace) || strings.Contains(s, "#") {
		return errors.New(want)
	}
	u, err := url.Parse(s)
	if err != nil || !slices.Contains(urlSchemes, u.Scheme) || u.Hostname() == "" || u.User != nil {
		return errors.New(want)
	}
	return nil
}

// checkCredentialTypes accepts what yaml.v3 and encoding/json decode a list of mappings to: []any of map[string]any.
func checkCredentialTypes(v any) error {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return fmt.Errorf("must be a non-empty list of credential types")
	}
	if len(list) > maxCredentialTypes {
		return fmt.Errorf("must list at most %d credential types", maxCredentialTypes)
	}
	seen := map[string]bool{}
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return fmt.Errorf("[%d] must be a mapping with %s and an optional %s", i, CredentialTypeKey, CredentialDescriptionKey)
		}
		for k := range m {
			if k != CredentialTypeKey && k != CredentialDescriptionKey {
				return fmt.Errorf("[%d] has a key other than %s and %s", i, CredentialTypeKey, CredentialDescriptionKey)
			}
		}
		typ, ok := m[CredentialTypeKey].(string)
		if !ok || !credentialType.MatchString(typ) {
			return fmt.Errorf("[%d].%s must start with a letter, followed by letters, digits, '.', '_' and '-'", i, CredentialTypeKey)
		}
		if seen[typ] {
			return fmt.Errorf("[%d].%s repeats an earlier type", i, CredentialTypeKey)
		}
		seen[typ] = true
		if d, ok := m[CredentialDescriptionKey]; ok {
			if err := TypeString.Check(d); err != nil {
				return fmt.Errorf("[%d].%s %w", i, CredentialDescriptionKey, err)
			}
		}
	}
	return nil
}

// compileFoundations validates the foundations list and returns it compiled with its topological order.
func compileFoundations(fs []Foundation, topLevelKeys []string) ([]CompiledFoundation, []string, error) {
	if len(fs) == 0 {
		return nil, nil, nil
	}
	compiled := make([]CompiledFoundation, len(fs))
	for i, f := range fs {
		c, err := compileFoundation(f, topLevelKeys)
		if err != nil {
			return nil, nil, fmt.Errorf("foundations[%d] (%s): %w", i, f.ID, err)
		}
		if j := slices.IndexFunc(compiled[:i], func(o CompiledFoundation) bool { return o.ID == f.ID }); j >= 0 {
			return nil, nil, fmt.Errorf("foundations[%d]: id %q is already declared by foundations[%d]", i, f.ID, j)
		}
		compiled[i] = c
	}
	if err := checkIDNesting(compiled); err != nil {
		return nil, nil, err
	}
	if err := checkRefs(compiled); err != nil {
		return nil, nil, err
	}
	if err := checkTargetPaths(compiled); err != nil {
		return nil, nil, err
	}
	order, err := topoOrder(compiled)
	if err != nil {
		return nil, nil, err
	}
	return compiled, order, nil
}

func compileFoundation(f Foundation, topLevelKeys []string) (CompiledFoundation, error) {
	if err := checkID(f.ID); err != nil {
		return CompiledFoundation{}, err
	}
	if !slices.Contains(knownStages, f.Stage) {
		return CompiledFoundation{}, fmt.Errorf("stage %q is unknown", f.Stage)
	}
	if !slices.Contains(knownTypes, f.Type) {
		return CompiledFoundation{}, fmt.Errorf("type %q is unknown", f.Type)
	}
	if f.Label == "" {
		return CompiledFoundation{}, fmt.Errorf("label is missing")
	}
	group := f.Group
	if group == "" {
		group = GroupNetwork
	}
	if !slices.Contains(knownFoundationGroups, group) {
		return CompiledFoundation{}, fmt.Errorf("group %q is unknown", f.Group)
	}
	if err := checkOptions(f); err != nil {
		return CompiledFoundation{}, err
	}
	// Default, DefaultWhen and Prefix are string templates and comparisons; they cannot make or check a bool or a list.
	if !f.Type.IsString() && (f.Default != "" || f.DefaultWhen != nil || f.Prefix != "") {
		return CompiledFoundation{}, fmt.Errorf("a %s takes no default, defaultWhen or prefix", f.Type)
	}
	c := CompiledFoundation{ID: f.ID, Stage: f.Stage, Type: f.Type, Group: group, Options: f.Options, Features: f.Features}
	if f.Default != "" {
		t, refs, err := parseTemplate(f.ID, f.Default)
		if err != nil {
			return CompiledFoundation{}, fmt.Errorf("default: %w", err)
		}
		// A constant default would hide a typo in a derived one; only a choice's pick may be constant.
		if len(refs) == 0 && f.Type != TypeChoice {
			return CompiledFoundation{}, fmt.Errorf("default references no answer")
		}
		if len(refs) == 0 && !slices.Contains(c.optionValues(), f.Default) {
			return CompiledFoundation{}, fmt.Errorf("default %q is not one of the options", f.Default)
		}
		c.deflt, c.defaultRefs = t, refs
	}
	if w := f.DefaultWhen; w != nil {
		if f.Default == "" {
			return CompiledFoundation{}, fmt.Errorf("defaultWhen needs a default")
		}
		if w.ID == "" {
			return CompiledFoundation{}, fmt.Errorf("defaultWhen: id is missing")
		}
		c.DefaultWhen = &Condition{ID: w.ID, Equals: w.Equals}
	}
	if f.Prefix != "" {
		t, refs, err := parseTemplate(f.ID+":prefix", f.Prefix)
		if err != nil {
			return CompiledFoundation{}, fmt.Errorf("prefix: %w", err)
		}
		if len(refs) == 0 {
			return CompiledFoundation{}, fmt.Errorf("prefix references no answer")
		}
		c.prefix, c.prefixRefs = t, refs
	}
	gate := map[string]bool{}
	for _, r := range c.defaultRefs {
		gate[r] = true
	}
	if c.DefaultWhen != nil {
		gate[c.DefaultWhen.ID] = true
	}
	c.defaultGate = sortedSet(gate)
	for _, r := range c.prefixRefs {
		gate[r] = true
	}
	c.Refs = sortedSet(gate)
	for i, t := range f.Targets {
		ct, err := compileTarget(f.ID, t, topLevelKeys)
		if err != nil {
			return CompiledFoundation{}, fmt.Errorf("targets[%d]: %w", i, err)
		}
		c.Targets = append(c.Targets, ct)
	}
	return c, nil
}

// checkFoundationFeatures refuses a feature id that is repeated or names no catalogue feature of chart.
func checkFoundationFeatures(fs []CompiledFoundation, features []Feature, chart string) error {
	for i, f := range fs {
		for j, id := range f.Features {
			k := slices.IndexFunc(features, func(c Feature) bool { return c.ID == id })
			switch {
			case k < 0:
				return fmt.Errorf("foundations[%d] (%s): features[%d]: %q names no feature of the catalogue", i, f.ID, j, id)
			case features[k].Chart != chart:
				return fmt.Errorf("foundations[%d] (%s): features[%d]: %s is a feature of chart %s, not %s", i, f.ID, j, id, features[k].Chart, chart)
			case slices.Index(f.Features, id) < j:
				return fmt.Errorf("foundations[%d] (%s): features[%d]: %s is already listed", i, f.ID, j, id)
			}
		}
	}
	return nil
}

// checkOptions requires options with distinct values and labels on a choice and refuses them elsewhere.
func checkOptions(f Foundation) error {
	if f.Type != TypeChoice {
		if len(f.Options) > 0 {
			return fmt.Errorf("options are only allowed on a %s", TypeChoice)
		}
		return nil
	}
	if len(f.Options) == 0 {
		return fmt.Errorf("a %s needs options", TypeChoice)
	}
	seen := map[string]bool{}
	for i, o := range f.Options {
		switch {
		case o.Value == "":
			return fmt.Errorf("options[%d]: value is missing", i)
		case o.Label == "":
			return fmt.Errorf("options[%d]: label is missing", i)
		case seen[o.Value]:
			return fmt.Errorf("options[%d]: value %q is already declared", i, o.Value)
		}
		seen[o.Value] = true
	}
	return nil
}

func checkID(id string) error {
	if id == "" {
		return fmt.Errorf("id is missing")
	}
	for _, s := range strings.Split(id, ".") {
		if !idSegment.MatchString(s) {
			return fmt.Errorf("id %q: segment %q is not a letter or '_' followed by letters, digits and '_'", id, s)
		}
	}
	return nil
}

func compileTarget(id string, t Target, topLevelKeys []string) (CompiledTarget, error) {
	p, err := roundtrip.ParsePath(t.Path)
	if err != nil {
		return CompiledTarget{}, err
	}
	if len(p) == 0 {
		return CompiledTarget{}, fmt.Errorf("path is missing")
	}
	if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
		return CompiledTarget{}, fmt.Errorf("path %q: root is not in topLevelKeys", t.Path)
	}
	c := CompiledTarget{Path: p, Fn: t.Fn, Sep: t.Sep}
	if t.Sep != "" && t.Fn != FnJoin {
		return CompiledTarget{}, fmt.Errorf("sep is only allowed on %s targets", FnJoin)
	}
	switch t.Fn {
	case FnConst:
		if !slices.ContainsFunc(p, func(s roundtrip.Segment) bool { _, ok := s.Index(); return ok }) {
			return CompiledTarget{}, fmt.Errorf("%s is only allowed under a list index, which completes an owned list", FnConst)
		}
		if len(t.Args) != 1 || !isScalar(t.Args[0]) {
			return CompiledTarget{}, fmt.Errorf("%s takes exactly one scalar arg", FnConst)
		}
		if s, ok := t.Args[0].(string); ok && strings.Contains(s, "{{") {
			return CompiledTarget{}, fmt.Errorf("%s is written verbatim and cannot reference an answer", FnConst)
		}
		c.Value = t.Args[0]
		return c, nil
	case FnTpl, FnCopy:
		if len(t.Args) != 1 {
			return CompiledTarget{}, fmt.Errorf("%s takes exactly one template arg", t.Fn)
		}
	case FnJoin:
		if len(t.Args) == 0 {
			return CompiledTarget{}, fmt.Errorf("%s takes at least one template arg", t.Fn)
		}
	default:
		return CompiledTarget{}, fmt.Errorf("fn %q is unknown", t.Fn)
	}
	refs := map[string]bool{}
	for i, a := range t.Args {
		s, ok := a.(string)
		if !ok {
			return CompiledTarget{}, fmt.Errorf("args[%d] is not a template string", i)
		}
		tpl, r, err := parseTemplate(fmt.Sprintf("%s:%s", id, t.Path), s)
		if err != nil {
			return CompiledTarget{}, fmt.Errorf("args[%d]: %w", i, err)
		}
		if len(r) == 0 {
			return CompiledTarget{}, fmt.Errorf("args[%d] references no answer", i)
		}
		if t.Fn == FnCopy && !isSingleField(tpl) {
			return CompiledTarget{}, fmt.Errorf("%s takes one {{ .id }} action and no text", FnCopy)
		}
		c.tpls = append(c.tpls, tpl)
		for _, id := range r {
			refs[id] = true
		}
	}
	c.Refs = sortedSet(refs)
	return c, nil
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, bool, int, float64:
		return true
	}
	return false
}

// parseTemplate parses s and returns the answer ids it references. Only text and {{ .id }} actions
// are allowed, so the references read from the parse tree are complete.
func parseTemplate(name, s string) (*template.Template, []string, error) {
	t, err := template.New(name).Option("missingkey=error").Funcs(template.FuncMap{}).Parse(s)
	if err != nil {
		return nil, nil, err
	}
	refs := map[string]bool{}
	for _, n := range t.Root.Nodes {
		switch n := n.(type) {
		case *parse.TextNode:
		case *parse.ActionNode:
			f := fieldOf(n)
			if f == nil {
				return nil, nil, fmt.Errorf("action %s: only {{ .id }} is allowed", n)
			}
			refs[strings.Join(f.Ident, ".")] = true
		default:
			return nil, nil, fmt.Errorf("%s: only text and {{ .id }} actions are allowed", n)
		}
	}
	return t, sortedSet(refs), nil
}

// fieldOf returns the field of an action that is exactly {{ .a.b }}; nil otherwise.
func fieldOf(n *parse.ActionNode) *parse.FieldNode {
	if len(n.Pipe.Decl) > 0 || len(n.Pipe.Cmds) != 1 || len(n.Pipe.Cmds[0].Args) != 1 {
		return nil
	}
	f, _ := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
	return f
}

func isSingleField(t *template.Template) bool {
	nodes := t.Root.Nodes
	if len(nodes) != 1 {
		return false
	}
	a, ok := nodes[0].(*parse.ActionNode)
	return ok && fieldOf(a) != nil
}

// checkIDNesting refuses an id that is a segment prefix of another: the template namespace nests ids,
// so domain could not be both a value and the parent of domain.base.
func checkIDNesting(fs []CompiledFoundation) error {
	for i, f := range fs {
		for j, o := range fs {
			if strings.HasPrefix(o.ID, f.ID+".") {
				return fmt.Errorf("foundations[%d]: id %q is a prefix of foundations[%d] %q", i, f.ID, j, o.ID)
			}
		}
	}
	return nil
}

// checkRefs refuses a reference to an undeclared id, a bool or list referenced other than by copy, and a
// defaultWhen comparing a choice with a value it does not offer; topoOrder refuses a foundation that depends
// on its own answer.
func checkRefs(fs []CompiledFoundation) error {
	find := func(id string) (CompiledFoundation, bool) {
		i := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == id })
		if i < 0 {
			return CompiledFoundation{}, false
		}
		return fs[i], true
	}
	// check refuses ref r from where unless it names a declared answer, a string one unless anyType.
	check := func(i int, where, r string, anyType bool) error {
		o, ok := find(r)
		if !ok {
			return fmt.Errorf("foundations[%d] (%s): %s references unknown answer %q", i, fs[i].ID, where, r)
		}
		if !anyType && !o.Type.IsString() {
			return fmt.Errorf("foundations[%d] (%s): %s references %s, a %s, which only a %s target may take", i, fs[i].ID, where, r, o.Type, FnCopy)
		}
		return nil
	}
	for i, f := range fs {
		for _, r := range f.defaultRefs {
			if err := check(i, "default", r, false); err != nil {
				return err
			}
		}
		for _, r := range f.prefixRefs {
			if err := check(i, "prefix", r, false); err != nil {
				return err
			}
		}
		if w := f.DefaultWhen; w != nil {
			if err := check(i, "defaultWhen", w.ID, false); err != nil {
				return err
			}
			if o, _ := find(w.ID); o.Type == TypeChoice && !slices.Contains(o.optionValues(), w.Equals) {
				return fmt.Errorf("foundations[%d] (%s): defaultWhen: %q is not an option of %s", i, f.ID, w.Equals, w.ID)
			}
		}
		for j, t := range f.Targets {
			for _, r := range t.Refs {
				if err := check(i, fmt.Sprintf("targets[%d]", j), r, t.Fn == FnCopy); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// checkTargetPaths refuses a path written twice, a path that is a prefix of another (every leaf is a
// scalar), a prefix used both as a list and as a mapping, and a list whose indices are not 0..n-1.
func checkTargetPaths(fs []CompiledFoundation) error {
	type where struct{ f, t int }
	leaves := map[string]where{}
	// interior maps every proper prefix to whether its child segment is an index.
	interior := map[string]bool{}
	indices := map[string]map[int]bool{}
	var lists []string
	for i, f := range fs {
		for j, t := range f.Targets {
			key := t.Path.String()
			if w, dup := leaves[key]; dup {
				return fmt.Errorf("foundations[%d] (%s): targets[%d]: path %q is already written by foundations[%d] targets[%d]", i, f.ID, j, key, w.f, w.t)
			}
			leaves[key] = where{i, j}
			for k := 1; k < len(t.Path); k++ {
				prefix := t.Path[:k].String()
				idx, isIndex := t.Path[k].Index()
				if was, seen := interior[prefix]; seen && was != isIndex {
					return fmt.Errorf("foundations[%d] (%s): targets[%d]: %q is used both as a list and as a mapping", i, f.ID, j, prefix)
				}
				interior[prefix] = isIndex
				if isIndex {
					if indices[prefix] == nil {
						indices[prefix] = map[int]bool{}
						lists = append(lists, prefix)
					}
					indices[prefix][idx] = true
				}
			}
		}
	}
	for key, w := range leaves {
		if _, ok := interior[key]; ok {
			return fmt.Errorf("foundations[%d] (%s): targets[%d]: path %q is a prefix of another target's path", w.f, fs[w.f].ID, w.t, key)
		}
	}
	for _, l := range lists {
		for i := range len(indices[l]) {
			if !indices[l][i] {
				return fmt.Errorf("list %q: index %d is missing; an owned list is written whole, from index 0 without gaps", l, i)
			}
		}
	}
	return nil
}

// topoOrder orders ids after the ids their Refs name, in declaration order otherwise.
func topoOrder(fs []CompiledFoundation) ([]string, error) {
	placed := map[string]bool{}
	order := make([]string, 0, len(fs))
	for len(order) < len(fs) {
		progress := false
		for _, f := range fs {
			ready := !slices.ContainsFunc(f.Refs, func(r string) bool { return !placed[r] })
			if placed[f.ID] || !ready {
				continue
			}
			placed[f.ID] = true
			order = append(order, f.ID)
			progress = true
			break
		}
		if !progress {
			var cycle []string
			for _, f := range fs {
				if !placed[f.ID] {
					cycle = append(cycle, f.ID)
				}
			}
			return nil, fmt.Errorf("foundations: %s depend on each other in a cycle of default, defaultWhen or prefix references", strings.Join(cycle, ", "))
		}
	}
	return order, nil
}

// sortedSet returns the keys of m sorted; nil when m is empty.
func sortedSet(m map[string]bool) []string {
	return slices.Sorted(maps.Keys(m))
}
