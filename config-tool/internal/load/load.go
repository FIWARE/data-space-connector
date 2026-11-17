// Package load opens an existing values.yaml: it classifies every leaf as owned, recognized or
// unknown, back-derives answers and builds the load report (ADR 0014). It never edits the document.
package load

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// ErrNoBundle means no bundle of DefaultChart is available to fall back to.
var ErrNoBundle = errors.New("load: no bundle for chart " + DefaultChart)

const (
	// DefaultChart is the chart whose bundles a loaded file is read against.
	DefaultChart = "data-space-connector"

	// maxValuePreview caps an entry's value, in runes, so the report stays small.
	maxValuePreview = 80
	ellipsis        = "…"
	aliasPrefix     = "*"
	// complexKeyLabel stands in for a mapping or sequence key, which has no path syntax.
	complexKeyLabel = "(complex key)"

	noteShadowed      = "shadowed by line %d"
	noteNonScalarKey  = "key is not a scalar"
	noteNoHeader      = "the file has no dsc-config header"
	noteHeaderNoChart = "the header names no chart version"
	noteNoBundle      = "chart version %s from the header has no bundle"
)

// VersionSource says where Report.ChartVersion comes from.
type VersionSource string

// The sources of the chart version.
const (
	FromHeader VersionSource = "header"
	FromLatest VersionSource = "latest"
)

// Origin says which class of leaf set an answer.
type Origin string

// The origins of an answer; an owned leaf always wins over a recognized one.
const (
	FromOwned      Origin = "owned"
	FromRecognized Origin = "recognized"
)

// Result is a loaded file. Doc is the untouched source and serves as the overrides layer.
type Result struct {
	Doc    *roundtrip.Document
	Report Report
}

// Report is what the UI shows after a load. Lines are file lines, header included.
type Report struct {
	Source           string        `json:"source"`
	Chart            string        `json:"chart"`
	ChartVersion     string        `json:"chartVersion"`
	ChartVersionFrom VersionSource `json:"chartVersionFrom"`
	// ChartVersionNote says why the latest bundle was used; empty when the header chose it.
	ChartVersionNote string   `json:"chartVersionNote,omitempty"`
	Generated        bool     `json:"generated"`
	Counts           Counts   `json:"counts"`
	Owned            []Entry  `json:"owned"`
	Recognized       []Entry  `json:"recognized"`
	Unknown          []Entry  `json:"unknown"`
	Skipped          []Entry  `json:"skipped"`
	Answers          []Answer `json:"answers"`
	Markers          []Marker `json:"markers"`
}

// Counts are the list lengths; Total is their sum.
type Counts struct {
	Owned      int `json:"owned"`
	Recognized int `json:"recognized"`
	Unknown    int `json:"unknown"`
	Skipped    int `json:"skipped"`
	Total      int `json:"total"`
}

// Entry is one leaf, or one skipped pair. Value is a display preview, not the scalar itself.
type Entry struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Value  string `json:"value"`
	Answer string `json:"answer,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Answer is a back-derived answer. Value is a bool for toggles, a string or number for values.
type Answer struct {
	ID    string   `json:"id"`
	Stage string   `json:"stage"`
	Value any      `json:"value"`
	Paths []string `json:"paths"`
	From  Origin   `json:"from"`
}

// Marker is an owned marker; Underived lists its answer ids that no owned leaf derived.
type Marker struct {
	Path      string   `json:"path"`
	Line      int      `json:"line"`
	AnswerIDs []string `json:"answerIds"`
	Underived []string `json:"underived"`
}

// Load parses src and classifies it against the matching bundle. Parse errors are wrapped, so
// roundtrip.ErrMultiDocument stays matchable; ErrNoBundle when no fallback bundle exists.
func Load(src []byte, name string, bundles []model.Bundle) (*Result, error) {
	doc, err := roundtrip.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", name, err)
	}
	bundle, from, note, err := chooseBundle(doc, bundles)
	if err != nil {
		return nil, err
	}
	markers := doc.OwnedMarkers()
	c := newClassifier(markers, bundle.Rules())
	roundtrip.Classify(doc, c.ownedPass)
	leaves := roundtrip.Classify(doc, c.classify)

	r := Report{
		Source: name, Chart: bundle.Chart, ChartVersion: bundle.Version,
		ChartVersionFrom: from, ChartVersionNote: note, Generated: doc.Generated(),
		Owned: []Entry{}, Recognized: []Entry{}, Unknown: []Entry{}, Skipped: []Entry{},
		Answers: c.answerList(), Markers: c.markerList(),
	}
	for _, l := range leaves {
		key := l.Path.String()
		e := Entry{Path: key, Line: doc.FileLine(l.Line), Value: preview(l.Node),
			Answer: c.leafAnswer[key], Note: c.notes[key]}
		switch l.Class {
		case roundtrip.ClassOwned:
			r.Owned = append(r.Owned, e)
		case roundtrip.ClassRecognized:
			r.Recognized = append(r.Recognized, e)
		default:
			r.Unknown = append(r.Unknown, e)
		}
	}
	for _, s := range roundtrip.Skipped(doc) {
		r.Skipped = append(r.Skipped, skippedEntry(doc, s))
	}
	r.Counts = Counts{Owned: len(r.Owned), Recognized: len(r.Recognized), Unknown: len(r.Unknown),
		Skipped: len(r.Skipped)}
	r.Counts.Total = r.Counts.Owned + r.Counts.Recognized + r.Counts.Unknown + r.Counts.Skipped
	return &Result{Doc: doc, Report: r}, nil
}

// chooseBundle picks the bundle of the header's chart version, else the latest DefaultChart
// bundle together with a note on why.
func chooseBundle(doc *roundtrip.Document, bundles []model.Bundle) (model.Bundle, VersionSource, string, error) {
	version := doc.Header().ChartVersion
	var latest *model.Bundle
	for i, b := range bundles {
		if b.Chart != DefaultChart {
			continue
		}
		if version != "" && b.Version == version {
			return b, FromHeader, "", nil
		}
		if b.Latest {
			latest = &bundles[i]
		}
	}
	if latest == nil {
		return model.Bundle{}, "", "", ErrNoBundle
	}
	note := fmt.Sprintf(noteNoBundle, version)
	switch {
	case !doc.Generated():
		note = noteNoHeader
	case version == "":
		note = noteHeaderNoChart
	}
	return *latest, FromLatest, note, nil
}

// skippedEntry reports a pair Classify skipped; its path ends in the key as it reads.
func skippedEntry(doc *roundtrip.Document, s roundtrip.SkippedPair) Entry {
	key := complexKeyLabel
	switch s.Key.Kind {
	case yaml.ScalarNode:
		key = s.Key.Value
	case yaml.AliasNode:
		key = aliasPrefix + s.Key.Value
	}
	path := append(s.Parent[:len(s.Parent):len(s.Parent)], roundtrip.Key(key))
	note := noteNonScalarKey
	if s.Reason == roundtrip.SkipShadowed {
		note = fmt.Sprintf(noteShadowed, doc.FileLine(s.ShadowedBy))
	}
	return Entry{Path: path.String(), Line: doc.FileLine(s.Line), Value: preview(s.Value), Note: note}
}

// preview is the display string of n: the scalar's first line capped at maxValuePreview runes,
// `*name` for an alias and "" for a collection.
func preview(n *yaml.Node) string {
	switch n.Kind {
	case yaml.AliasNode:
		return aliasPrefix + n.Value
	case yaml.ScalarNode:
	default:
		return ""
	}
	v, rest, cut := strings.Cut(n.Value, "\n")
	cut = cut && rest != ""
	if utf8.RuneCountInString(v) > maxValuePreview {
		v, cut = string([]rune(v)[:maxValuePreview]), true
	}
	if cut {
		v += ellipsis
	}
	return v
}
