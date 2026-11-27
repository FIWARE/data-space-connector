package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	// InfraIDPrefix starts the id of every synthetic foundation the infrastructure list compiles into (ADR 0026).
	InfraIDPrefix = "infra."
	// ModeQuestion is the reserved local id of a dependency's mode: its answer is infra.<dependency>.mode.
	ModeQuestion = "mode"
)

// Mode ids, the only three a dependency may offer.
const (
	ModeBundled  = "bundled"
	ModeCluster  = "cluster"
	ModeExternal = "external"
)

// tlsSecretNameKey is the key below an ingress block that holds its TLS Secret name.
const tlsSecretNameKey = "tls[0].secretName"

var knownModes = []string{ModeBundled, ModeCluster, ModeExternal}

// Dependency is one Stage 4 dependency as declared in a bundle's infrastructure list.
type Dependency struct {
	// ID is one id segment (certManager); it keys the profile's infrastructure map.
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	Help  string `yaml:"help"`
	Doc   string `yaml:"doc"`
	// ClusterWide marks a dependency a cluster runs once; its bundled mode must then carry Warning and Doc.
	ClusterWide bool   `yaml:"clusterWide"`
	Modes       []Mode `yaml:"modes"`
}

// Mode is one way to meet a dependency; its targets render only while it is the chosen mode.
type Mode struct {
	ID      string `yaml:"id"`
	Label   string `yaml:"label"`
	Note    string `yaml:"note"`
	Warning string `yaml:"warning"`
	Doc     string `yaml:"doc"`
	// Development marks a mode suitable for development only; ProductionDoc then links the production setup.
	Development   bool       `yaml:"development"`
	ProductionDoc string     `yaml:"productionDoc"`
	Targets       []Target   `yaml:"targets"`
	Questions     []Question `yaml:"questions"`
	// PerIngress targets are written below every ingresses row, their keys relative to the row's path.
	PerIngress []IngressTarget `yaml:"perIngress"`
	// PerIngressQuestion is asked once per ingresses row, as <id>.<ingress id>.
	PerIngressQuestion *IngressQuestion `yaml:"perIngressQuestion"`
	// Requirements apply only while the mode is chosen.
	Requirements []Requirement `yaml:"requirements"`
}

// Question is one question of a mode, compiled into the foundation infra.<dependency>.<id>.
type Question struct {
	// ID is one id segment, unique across the dependency's modes; mode is reserved.
	ID      string         `yaml:"id"`
	Type    FoundationType `yaml:"type"`
	Label   string         `yaml:"label"`
	Help    string         `yaml:"help"`
	Doc     string         `yaml:"doc"`
	Options []Option       `yaml:"options"`
	// Default is a template as on a foundation; only a choice may have a constant one.
	Default string `yaml:"default"`
	// When asks the question, and renders its targets, only while another question of the mode has a value.
	When *QuestionWhen `yaml:"when"`
	// ReadOnly shows the derived value without letting the user change it.
	ReadOnly bool     `yaml:"readOnly"`
	Targets  []Target `yaml:"targets"`
}

// QuestionWhen holds while the mode's question Question resolves to Equals.
type QuestionWhen struct {
	Question string `yaml:"question"`
	Equals   string `yaml:"equals"`
}

// IngressTarget is a target below every ingress block; Key is relative to the row's path.
type IngressTarget struct {
	Key  string `yaml:"key"`
	Fn   Fn     `yaml:"fn"`
	Args []any  `yaml:"args"`
	Sep  string `yaml:"sep"`
}

// IngressQuestion is a question asked per ingresses row, whose answer is copied to Key below the row's path.
type IngressQuestion struct {
	ID    string         `yaml:"id"`
	Type  FoundationType `yaml:"type"`
	Label string         `yaml:"label"`
	Help  string         `yaml:"help"`
	Doc   string         `yaml:"doc"`
	Key   string         `yaml:"key"`
	// Replaces writes over the unconditional foundation target at Key, whose value becomes the default.
	Replaces bool `yaml:"replaces"`
	// Requirement is listed once per row while the mode is chosen and the row's owner resolves; it has no name of
	// its own, the answer is the name.
	Requirement *Requirement `yaml:"requirement"`
}

// Ingress is one ingress whose TLS the tool owns: Path is its block, Owner the foundation that writes
// <Path>.tls[0].secretName.
type Ingress struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Path  string `yaml:"path"`
	Owner string `yaml:"owner"`
}

// Dependencies returns the infrastructure dependencies in bundle order.
func (b Bundle) Dependencies() []Dependency {
	return slices.Clone(b.Infrastructure)
}

// Dependency returns the dependency id; false when the bundle does not declare it.
func (b Bundle) Dependency(id string) (Dependency, bool) {
	i := slices.IndexFunc(b.Infrastructure, func(d Dependency) bool { return d.ID == id })
	if i < 0 {
		return Dependency{}, false
	}
	return b.Infrastructure[i], true
}

// Mode returns the mode id of d; false when d does not offer it.
func (d Dependency) Mode(id string) (Mode, bool) {
	i := slices.IndexFunc(d.Modes, func(m Mode) bool { return m.ID == id })
	if i < 0 {
		return Mode{}, false
	}
	return d.Modes[i], true
}

// InfraModeID is the synthetic answer id of dependency dep's mode.
func InfraModeID(dep string) string {
	return InfraIDPrefix + dep + idSeparator + ModeQuestion
}

// InfraQuestionID is the synthetic answer id of the local question id of dependency dep.
func InfraQuestionID(dep, question string) string {
	return InfraIDPrefix + dep + idSeparator + question
}

// isInfraID reports whether id is, or lies under, the reserved infra namespace.
func isInfraID(id string) bool {
	return id == strings.TrimSuffix(InfraIDPrefix, idSeparator) || strings.HasPrefix(id, InfraIDPrefix)
}

// infraRequirement is a requirement of a mode, compiled once every synthetic foundation it may name exists.
type infraRequirement struct {
	where    string
	r        Requirement
	requires []string
}

// ingressRow is a validated ingresses row.
type ingressRow struct {
	Ingress
	// requires gates every target below the row: the owner target's refs and requires, plus the owner itself,
	// since a ref-less const owner target would otherwise leave them ungated.
	requires []string
}

// compileInfrastructure compiles the infrastructure list and the ingresses table into synthetic foundations and
// re-checks them together with the declared ones, which must stay out of the infra namespace.
func compileInfrastructure(b *Bundle) error {
	for i, f := range b.Foundations {
		if isInfraID(f.ID) {
			return fmt.Errorf("foundations[%d]: id %q lies under %s, which only the infrastructure list compiles", i, f.ID, InfraIDPrefix)
		}
	}
	if len(b.Infrastructure) == 0 && len(b.Ingresses) == 0 {
		return nil
	}
	rows, err := compileIngresses(b.Ingresses, b.foundations, b.TopLevelKeys)
	if err != nil {
		return err
	}
	var synthetic []CompiledFoundation
	var pending []infraRequirement
	for i, d := range b.Infrastructure {
		if j := slices.IndexFunc(b.Infrastructure[:i], func(o Dependency) bool { return o.ID == d.ID }); j >= 0 {
			return fmt.Errorf("infrastructure[%d]: id %q is already declared by infrastructure[%d]", i, d.ID, j)
		}
		fs, rs, err := compileDependency(d, rows, b.foundations, b.TopLevelKeys)
		if err != nil {
			return fmt.Errorf("infrastructure[%d] (%s): %w", i, d.ID, err)
		}
		synthetic = append(synthetic, fs...)
		for j := range rs {
			rs[j].where = fmt.Sprintf("infrastructure[%d] (%s): %s", i, d.ID, rs[j].where)
		}
		pending = append(pending, rs...)
	}
	all := slices.Concat(b.foundations, synthetic)
	for _, check := range []func([]CompiledFoundation) error{checkIDNesting, checkRefs, checkTargetPaths} {
		if err := check(all); err != nil {
			return err
		}
	}
	order, err := topoOrder(all)
	if err != nil {
		return err
	}
	for i, p := range pending {
		c, err := compileRequirement(i, p.r, all)
		if err != nil {
			return fmt.Errorf("%s: %w", p.where, err)
		}
		c.Requires = p.requires
		b.requirements = append(b.requirements, c)
	}
	b.foundations, b.order = all, order
	return nil
}

// compileIngresses validates the ingresses table: each owner writes <path>.tls[0].secretName unconditionally and,
// once the table is declared, every such target of a declared foundation has its row.
func compileIngresses(ingresses []Ingress, fs []CompiledFoundation, topLevelKeys []string) ([]ingressRow, error) {
	suffix, err := roundtrip.ParsePath(tlsSecretNameKey)
	if err != nil {
		return nil, err
	}
	rows := make([]ingressRow, len(ingresses))
	paths := make([]roundtrip.Path, len(ingresses))
	for i, in := range ingresses {
		p, err := roundtrip.ParsePath(in.Path)
		switch {
		case !idSegment.MatchString(in.ID):
			return nil, fmt.Errorf("ingresses[%d]: id %q is not a letter or '_' followed by letters, digits and '_'", i, in.ID)
		case slices.ContainsFunc(ingresses[:i], func(o Ingress) bool { return o.ID == in.ID }):
			return nil, fmt.Errorf("ingresses[%d]: id %q is already declared", i, in.ID)
		case in.Label == "":
			return nil, fmt.Errorf("ingresses[%d] (%s): label is missing", i, in.ID)
		case err != nil:
			return nil, fmt.Errorf("ingresses[%d] (%s): path: %w", i, in.ID, err)
		case len(p) == 0:
			return nil, fmt.Errorf("ingresses[%d] (%s): path is missing", i, in.ID)
		}
		if k, ok := p[0].Key(); !ok || !slices.Contains(topLevelKeys, k) {
			return nil, fmt.Errorf("ingresses[%d] (%s): path %q: root is not in topLevelKeys", i, in.ID, in.Path)
		}
		if slices.ContainsFunc(paths[:i], func(o roundtrip.Path) bool { return slices.Equal(o, p) }) {
			return nil, fmt.Errorf("ingresses[%d] (%s): path %q is already listed", i, in.ID, in.Path)
		}
		paths[i] = p
		secret := slices.Concat(p, suffix)
		o := slices.IndexFunc(fs, func(f CompiledFoundation) bool { return f.ID == in.Owner })
		if o < 0 {
			return nil, fmt.Errorf("ingresses[%d] (%s): owner %q is no declared foundation", i, in.ID, in.Owner)
		}
		t := slices.IndexFunc(fs[o].Targets, func(t CompiledTarget) bool { return t.unconditional() && slices.Equal(t.Path, secret) })
		if t < 0 {
			return nil, fmt.Errorf("ingresses[%d] (%s): owner %s has no unconditional target %s", i, in.ID, in.Owner, secret)
		}
		gate := map[string]bool{in.Owner: true}
		for _, id := range slices.Concat(fs[o].Targets[t].Refs, fs[o].Targets[t].Requires) {
			gate[id] = true
		}
		rows[i] = ingressRow{Ingress: in, requires: sortedSet(gate)}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	for i, f := range fs {
		for j, t := range f.Targets {
			if len(t.Path) < len(suffix) || !slices.Equal(t.Path[len(t.Path)-len(suffix):], suffix) {
				continue
			}
			block := t.Path[:len(t.Path)-len(suffix)]
			listed := false
			for k, r := range rows {
				listed = listed || (r.Owner == f.ID && slices.Equal(paths[k], block))
			}
			if !listed {
				return nil, fmt.Errorf("foundations[%d] (%s): targets[%d]: %s is a TLS Secret name no ingresses row lists", i, f.ID, j, t.Path)
			}
		}
	}
	return rows, nil
}

// compileDependency compiles d into its mode foundation and one foundation per question, expanding the per-ingress
// targets and question over rows, and returns its requirements, gated on their mode, to compile once all ids exist;
// fs are the declared foundations, compiled.
func compileDependency(d Dependency, rows []ingressRow, fs []CompiledFoundation, topLevelKeys []string) ([]CompiledFoundation, []infraRequirement, error) {
	switch {
	case !idSegment.MatchString(d.ID):
		return nil, nil, fmt.Errorf("id %q is not a letter or '_' followed by letters, digits and '_'", d.ID)
	case d.Title == "":
		return nil, nil, fmt.Errorf("title is missing")
	case len(d.Modes) == 0:
		return nil, nil, fmt.Errorf("modes is empty")
	}
	if err := checkOptionalDoc("doc", d.Doc); err != nil {
		return nil, nil, err
	}
	options := make([]Option, len(d.Modes))
	for i, m := range d.Modes {
		if err := checkMode(d, i); err != nil {
			return nil, nil, fmt.Errorf("modes[%d] (%s): %w", i, m.ID, err)
		}
		options[i] = Option{Value: m.ID, Label: m.Label, Note: m.Note}
	}
	modeID := InfraModeID(d.ID)
	mode, err := compileFoundation(Foundation{ID: modeID, Stage: StageInfrastructure, Type: TypeChoice, Label: d.Title, Options: options}, topLevelKeys)
	if err != nil {
		return nil, nil, err
	}
	mode.Group = ""
	// seen maps every local question id, per-ingress ones expanded, to where it is declared.
	seen := map[string]string{}
	var questions []CompiledFoundation
	var requirements []infraRequirement
	for i, m := range d.Modes {
		where := fmt.Sprintf("modes[%d] (%s)", i, m.ID)
		when := []TargetCondition{{ID: modeID, Equals: m.ID}}
		for j, r := range m.Requirements {
			r.When = slices.Concat(when, r.When)
			requirements = append(requirements, infraRequirement{where: fmt.Sprintf("%s: requirements[%d]", where, j), r: r})
		}
		for j, t := range m.Targets {
			ct, err := compileInfraTarget(modeID, t, when, topLevelKeys)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: targets[%d]: %w", where, j, err)
			}
			mode.Targets = append(mode.Targets, ct)
		}
		if (len(m.PerIngress) > 0 || m.PerIngressQuestion != nil) && len(rows) == 0 {
			return nil, nil, fmt.Errorf("%s: perIngress and perIngressQuestion need the ingresses table", where)
		}
		for j, pt := range m.PerIngress {
			for _, r := range rows {
				t := Target{Path: r.Path + idSeparator + pt.Key, Fn: pt.Fn, Args: pt.Args, Sep: pt.Sep, Requires: r.requires}
				ct, err := compileInfraTarget(modeID, t, when, topLevelKeys)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: perIngress[%d] (%s): %w", where, j, r.ID, err)
				}
				mode.Targets = append(mode.Targets, ct)
			}
		}
		for j, q := range m.Questions {
			qw := fmt.Sprintf("%s: questions[%d] (%s)", where, j, q.ID)
			if err := checkQuestionID(q.ID, seen); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", qw, err)
			}
			seen[q.ID] = qw
			c, err := compileQuestion(d.ID, m, q, when, topLevelKeys)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", qw, err)
			}
			questions = append(questions, c)
		}
	}
	// Per-ingress questions come last, so a plain question of a later mode is already in seen.
	for i, m := range d.Modes {
		q := m.PerIngressQuestion
		if q == nil {
			continue
		}
		qw := fmt.Sprintf("modes[%d] (%s): perIngressQuestion (%s)", i, m.ID, q.ID)
		if other, ok := seen[q.ID]; ok {
			return nil, nil, fmt.Errorf("%s shares its id with %s; it expands to %s.<ingress id>, which nests under it", qw, other, q.ID)
		}
		if err := checkQuestionID(q.ID, seen); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", qw, err)
		}
		for _, r := range rows {
			local := q.ID + idSeparator + r.ID
			if other, ok := seen[local]; ok {
				return nil, nil, fmt.Errorf("%s: id %q is already declared by %s", qw, local, other)
			}
			seen[local] = qw
			when := []TargetCondition{{ID: modeID, Equals: m.ID}}
			c, err := compileIngressQuestion(d.ID, *q, r, fs, when, topLevelKeys)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %s: %w", qw, r.ID, err)
			}
			questions = append(questions, c)
			if q.Requirement != nil {
				if q.Requirement.Name != "" {
					return nil, nil, fmt.Errorf("%s: requirement: name is the answer, so it is not declared", qw)
				}
				req := *q.Requirement
				req.Name, req.When = "{{ ."+c.ID+" }}", slices.Concat(when, req.When)
				requirements = append(requirements, infraRequirement{where: fmt.Sprintf("%s: %s: requirement", qw, r.ID), r: req, requires: r.requires})
			}
		}
	}
	return append([]CompiledFoundation{mode}, questions...), requirements, nil
}

// checkMode validates mode i of d apart from its targets and questions.
func checkMode(d Dependency, i int) error {
	m := d.Modes[i]
	switch {
	case !slices.Contains(knownModes, m.ID):
		return fmt.Errorf("id %q is not one of %s", m.ID, strings.Join(knownModes, ", "))
	case slices.ContainsFunc(d.Modes[:i], func(o Mode) bool { return o.ID == m.ID }):
		return fmt.Errorf("id %q is already declared", m.ID)
	case m.Label == "":
		return fmt.Errorf("label is missing")
	case d.ClusterWide && m.ID == ModeBundled && (m.Warning == "" || m.Doc == ""):
		return fmt.Errorf("the dependency is cluster-wide, so its %s mode needs a warning and a doc", ModeBundled)
	case m.Development && m.ProductionDoc == "":
		return fmt.Errorf("a development mode needs a productionDoc")
	case !m.Development && m.ProductionDoc != "":
		return fmt.Errorf("productionDoc is only allowed on a development mode")
	}
	if err := checkOptionalDoc("doc", m.Doc); err != nil {
		return err
	}
	return checkOptionalDoc("productionDoc", m.ProductionDoc)
}

// checkQuestionID refuses a local question id that is not one id segment, is the reserved mode, or is in seen.
func checkQuestionID(id string, seen map[string]string) error {
	switch {
	case !idSegment.MatchString(id):
		return fmt.Errorf("id %q is not one segment: a letter or '_' followed by letters, digits and '_'", id)
	case id == ModeQuestion:
		return fmt.Errorf("id %q is reserved for the dependency's mode", id)
	}
	if other, ok := seen[id]; ok {
		return fmt.Errorf("id %q is already declared by %s", id, other)
	}
	return nil
}

// compileQuestion compiles q of mode m of dependency dep; its targets render while when and q's own when hold.
func compileQuestion(dep string, m Mode, q Question, when []TargetCondition, topLevelKeys []string) (CompiledFoundation, error) {
	if err := checkOptionalDoc("doc", q.Doc); err != nil {
		return CompiledFoundation{}, err
	}
	id := InfraQuestionID(dep, q.ID)
	c, err := compileFoundation(Foundation{ID: id, Stage: StageInfrastructure, Type: q.Type, Label: q.Label, Options: q.Options, Default: q.Default}, topLevelKeys)
	if err != nil {
		return CompiledFoundation{}, err
	}
	c.Group = ""
	if q.ReadOnly && q.Default == "" {
		return CompiledFoundation{}, fmt.Errorf("a readOnly question needs a default")
	}
	if w := q.When; w != nil {
		k := slices.IndexFunc(m.Questions, func(o Question) bool { return o.ID == w.Question })
		switch {
		case k < 0:
			return CompiledFoundation{}, fmt.Errorf("when: %q is no question of mode %s", w.Question, m.ID)
		case w.Question == q.ID:
			return CompiledFoundation{}, fmt.Errorf("when: a question cannot depend on itself")
		case w.Equals == "":
			return CompiledFoundation{}, fmt.Errorf("when: equals is missing")
		case m.Questions[k].When != nil:
			return CompiledFoundation{}, fmt.Errorf("when: %s has a when of its own; conditions do not chain", w.Question)
		case !m.Questions[k].Type.IsString():
			return CompiledFoundation{}, fmt.Errorf("when: %s is a %s; a condition compares a string answer", w.Question, m.Questions[k].Type)
		case m.Questions[k].Type == TypeChoice && !slices.ContainsFunc(m.Questions[k].Options, func(o Option) bool { return o.Value == w.Equals }):
			return CompiledFoundation{}, fmt.Errorf("when: %q is not an option of %s", w.Equals, w.Question)
		}
		when = append(slices.Clone(when), TargetCondition{ID: InfraQuestionID(dep, w.Question), Equals: w.Equals})
	}
	for i, t := range q.Targets {
		ct, err := compileInfraTarget(id, t, when, topLevelKeys)
		if err != nil {
			return CompiledFoundation{}, fmt.Errorf("targets[%d]: %w", i, err)
		}
		c.Targets = append(c.Targets, ct)
	}
	return c, nil
}

// compileIngressQuestion compiles q for row r into infra.<dep>.<q.ID>.<r.ID>, copied to q.Key below the row's path.
// A replacing question takes the replaced target's template, or its constant, as its default.
func compileIngressQuestion(dep string, q IngressQuestion, r ingressRow, fs []CompiledFoundation, when []TargetCondition, topLevelKeys []string) (CompiledFoundation, error) {
	if q.Key == "" {
		return CompiledFoundation{}, fmt.Errorf("key is missing")
	}
	if err := checkOptionalDoc("doc", q.Doc); err != nil {
		return CompiledFoundation{}, err
	}
	id := InfraQuestionID(dep, q.ID+idSeparator+r.ID)
	path := r.Path + idSeparator + q.Key
	f := Foundation{ID: id, Stage: StageInfrastructure, Type: q.Type, Label: q.Label}
	var constant *string
	if q.Replaces {
		def, isConst, err := replacedDefault(fs, path)
		if err != nil {
			return CompiledFoundation{}, err
		}
		if isConst {
			constant = &def
		} else {
			f.Default = def
		}
	}
	c, err := compileFoundation(f, topLevelKeys)
	if err != nil {
		return CompiledFoundation{}, err
	}
	c.Group = ""
	if constant != nil {
		if err := c.Check(*constant); err != nil {
			return CompiledFoundation{}, fmt.Errorf("replaces: the replaced constant %w", err)
		}
		if c.deflt, _, err = parseTemplate(id, *constant); err != nil {
			return CompiledFoundation{}, fmt.Errorf("replaces: %w", err)
		}
	}
	t := Target{Path: path, Fn: FnCopy, Args: []any{"{{ ." + id + " }}"}, Requires: r.requires}
	ct, err := compileInfraTarget(id, t, when, topLevelKeys)
	if err != nil {
		return CompiledFoundation{}, err
	}
	ct.Replaces = q.Replaces
	c.Targets = []CompiledTarget{ct}
	return c, nil
}

// replacedDefault returns the source of the unconditional target at path, a template or, with isConst, a constant.
func replacedDefault(fs []CompiledFoundation, path string) (def string, isConst bool, err error) {
	p, err := roundtrip.ParsePath(path)
	if err != nil {
		return "", false, err
	}
	for _, f := range fs {
		for _, t := range f.Targets {
			if !t.unconditional() || !slices.Equal(t.Path, p) {
				continue
			}
			switch {
			case t.Fn == FnConst:
				s, ok := t.Value.(string)
				if !ok {
					return "", false, fmt.Errorf("replaces: %s writes a constant that is no string", path)
				}
				return s, true, nil
			case t.Fn == FnTpl || t.Fn == FnCopy:
				return t.Templates()[0].Root.String(), false, nil
			}
			return "", false, fmt.Errorf("replaces: %s is written by a %s target; only tpl, copy and const give a default", path, t.Fn)
		}
	}
	return "", false, fmt.Errorf("replaces: no unconditional foundation target writes %s", path)
}

// compileInfraTarget compiles a target of a synthetic foundation; list element and each targets stay Stage 3's.
func compileInfraTarget(id string, t Target, when []TargetCondition, topLevelKeys []string) (CompiledTarget, error) {
	if t.Element != nil || t.Each != "" {
		return CompiledTarget{}, fmt.Errorf("element and each targets are not allowed on an infrastructure answer")
	}
	return compileTarget(id, t, when, topLevelKeys)
}

// checkOptionalDoc refuses a non-empty doc that is not a repo-relative path.
func checkOptionalDoc(name, doc string) error {
	if doc == "" {
		return nil
	}
	if err := checkDoc(doc); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
