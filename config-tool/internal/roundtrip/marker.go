package roundtrip

import (
	"bytes"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// MarkerPrefix starts an owned marker line: `# dsc-config:owned <id>[,<id>...]`.
	MarkerPrefix = "# dsc-config:owned "

	markerIDSep = ","
)

// OwnedMarker is an owned marker and the keys in its scope (ADR 0014).
// Path is the marked key; Scope is Path followed by the sibling keys it also owns, each with its
// subtree. Line is the marker's file line.
type OwnedMarker struct {
	Path      Path
	Line      int
	AnswerIDs []string
	Scope     []Path
}

// OwnedMarkers returns the file's owned markers in document order, nil when the file has no
// generated header. Markers count only in the head comment of a block mapping key that is
// reached through mappings alone; markers on sequence items, in flow collections or in line and
// foot comments are ignored.
func (d *Document) OwnedMarkers() []OwnedMarker {
	root := d.src
	if root == nil {
		root = d.root
	}
	if !d.hasHeader() || len(root.Content) == 0 {
		return nil
	}
	var markers []OwnedMarker
	d.collectMarkers(nil, root.Content[0], 0, &markers)
	return markers
}

// OwnedBy returns the marker whose scope covers p; the innermost one when markers nest.
func OwnedBy(markers []OwnedMarker, p Path) (OwnedMarker, bool) {
	var best OwnedMarker
	depth := -1
	for _, m := range markers {
		for _, s := range m.Scope {
			if len(s) > depth && len(s) <= len(p) && slices.Equal(s, p[:len(s)]) {
				best, depth = m, len(s)
			}
		}
	}
	return best, depth >= 0
}

// collectMarkers appends the markers of the block mapping m at p and of the block mappings below
// it. start is the offset of the first line that may hold the first key's head comment.
func (d *Document) collectMarkers(p Path, m *yaml.Node, start int, markers *[]OwnedMarker) {
	if m.Kind != yaml.MappingNode || m.Style&yaml.FlowStyle != 0 {
		return
	}
	live := livePairs(m)
	// gapStart is where the lines between the previous pair and this key begin, -1 when the
	// previous value's end could not be placed: then no marker or scope crosses the gap.
	gapStart := start
	open := -1
	for i := range live {
		key, value := m.Content[2*i], m.Content[2*i+1]
		// A shadowed or non-scalar key is a sibling for the scope rule but is never owned itself.
		var path Path
		if key.Kind == yaml.ScalarNode {
			path = append(p[:len(p):len(p)], Key(key.Value))
		}
		owned := live[i] && path != nil
		keyStart := d.lineStarts[key.Line-1]
		head, blank := d.gapLines(gapStart, keyStart)
		switch {
		case gapStart < 0 || blank || len(head) > 0:
			open = -1
		case open >= 0 && owned:
			(*markers)[open].Scope = append((*markers)[open].Scope, path)
		}
		if ids, line, ok := d.findMarker(head); ok {
			mk := OwnedMarker{Path: path, Line: d.FileLine(line), AnswerIDs: ids}
			if owned {
				mk.Scope = []Path{path}
			}
			*markers = append(*markers, mk)
			open = len(*markers) - 1
		}
		if owned && key.Tag != mergeTag {
			d.collectMarkers(path, value, lineAfter(d.body, keyStart), markers)
		}
		if i < len(live)-1 {
			end, err := d.subtreeEnd(value, leafCtx{owner: key.Column - 1})
			gapStart = end
			if err != nil {
				gapStart = -1
			}
		}
	}
}

// gapLines returns the offsets of the comment lines directly above the key line at keyStart
// (its head comment, top first) and whether a blank line lies in [start, keyStart).
func (d *Document) gapLines(start, keyStart int) ([]int, bool) {
	if start < 0 {
		return nil, false
	}
	var lines []int
	blank := false
	for off := start; off < keyStart; off = lineAfter(d.body, off) {
		t := bytes.TrimLeft(lineText(d.body, off), " \t")
		switch {
		case len(t) == 0:
			blank, lines = true, nil
		case t[0] == commentStart:
			lines = append(lines, off)
		default:
			lines = nil
		}
	}
	return lines, blank
}

// findMarker parses the marker nearest the key among the head comment lines at offsets head.
// It returns the answer ids and the marker's body line; ok is false when there is none.
func (d *Document) findMarker(head []int) ([]string, int, bool) {
	for _, off := range slices.Backward(head) {
		text := strings.TrimSpace(string(lineText(d.body, off)))
		rest, ok := strings.CutPrefix(text, MarkerPrefix)
		if !ok {
			continue
		}
		var ids []string
		for _, id := range strings.Split(rest, markerIDSep) {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			return ids, d.lineAt(off), true
		}
	}
	return nil, 0, false
}
