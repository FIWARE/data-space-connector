package roundtrip

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Sentinel errors of the package; callers match them with errors.Is.
var (
	// ErrMultiDocument refuses a stream with more than one YAML document.
	ErrMultiDocument = errors.New("roundtrip: more than one YAML document")
	// ErrInvalidHeader refuses a header passed to SetHeader whose field contains a line break.
	ErrInvalidHeader = errors.New("roundtrip: header field contains a line break")
	// ErrInvalidPath refuses a malformed path string.
	ErrInvalidPath = errors.New("roundtrip: invalid path")
	// ErrNotFound reports a path whose key or index does not exist.
	ErrNotFound = errors.New("roundtrip: path not found")
	// ErrAliasPath refuses a path that runs through an alias, since editing it would change the anchor.
	ErrAliasPath = errors.New("roundtrip: path runs through an alias")
	// ErrNotMapping refuses a Set whose path needs a key below a non-null scalar or a sequence.
	ErrNotMapping = errors.New("roundtrip: path runs into a value that is not a mapping")
	// ErrUnsupportedValue refuses a Set with a Go value of a type the engine does not write.
	ErrUnsupportedValue = errors.New("roundtrip: unsupported value type")
	// ErrUnsupportedScalar refuses an edit whose source text the scanner cannot place exactly.
	ErrUnsupportedScalar = errors.New("roundtrip: scalar text cannot be edited in place")
	// ErrPatchVerification refuses a write whose re-parsed output differs from the edited tree.
	ErrPatchVerification = errors.New("roundtrip: patched output does not match the edits")
	// ErrPatchConflict refuses pending patches whose byte spans overlap.
	ErrPatchConflict = errors.New("roundtrip: overlapping patches")
)

const (
	utf8BOM        = "\xef\xbb\xbf"
	lineFeed       = "\n"
	tab            = "\t"
	crlf           = "\r\n"
	docStartMarker = "---"
	docEndMarker   = "..."
	nullTag        = "!!null"
)

// Document is a parsed values.yaml that writes back byte-identically.
// The source bytes are the truth; root is only an index whose Line/Column map into body.
type Document struct {
	bom          bool
	headerRaw    []byte
	header       Header
	headerSet    bool
	body         []byte
	tail         []byte
	root         *yaml.Node
	lineStarts   []int
	eol          string
	finalNewline bool
	// src is the tree as parsed, kept unedited once root carries edits; positions are read from it.
	src *yaml.Node
	// reps and groups are the pending edits keyed by path string; Write renders them from root.
	reps   map[string]*replacement
	groups map[string]*group
	lay    *layout
}

// Parse reads src into a Document. It refuses more than one YAML document with
// ErrMultiDocument; an empty or comment-only src gives an empty document.
func Parse(src []byte) (*Document, error) {
	d := &Document{}
	if bytes.HasPrefix(src, []byte(utf8BOM)) {
		d.bom = true
		src = src[len(utf8BOM):]
	}
	d.eol = detectEOL(src)
	d.headerRaw, d.header, src = splitHeader(src)

	lines := splitLines(src)
	tailStart, err := checkMarkers(lines)
	if err != nil {
		return nil, err
	}
	cut := len(src)
	if tailStart < len(lines) {
		cut = lines[tailStart].start
	}
	d.body, d.tail = src[:cut], src[cut:]
	if d.root, err = decodeSingle(src, len(d.tail) > 0); err != nil {
		return nil, err
	}
	d.lineStarts = lineStarts(d.body)
	d.finalNewline = bytes.HasSuffix(d.body, []byte(lineFeed))
	return d, nil
}

// Write returns the document's bytes: BOM, header, body with the pending edits applied, and tail.
// Output with edits is re-parsed and checked against them first. On ErrInvalidHeader,
// ErrPatchConflict or ErrPatchVerification it returns no bytes.
func (d *Document) Write() ([]byte, error) {
	body := d.body
	edited := len(d.reps)+len(d.groups) > 0
	if edited {
		ps, err := d.pendingPatches()
		if err != nil {
			return nil, err
		}
		if body, err = applyPatches(d.body, ps); err != nil {
			return nil, err
		}
	}
	header := d.headerRaw
	if d.headerSet {
		if !d.header.valid() {
			return nil, ErrInvalidHeader
		}
		header = d.header.Render()
		if d.eol == crlf {
			header = bytes.ReplaceAll(header, []byte(lineFeed), []byte(crlf))
		}
	}
	var out bytes.Buffer
	if d.bom {
		out.WriteString(utf8BOM)
	}
	out.Write(header)
	out.Write(body)
	out.Write(d.tail)
	if edited {
		if err := d.verify(out.Bytes()); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

// Header returns the generated header found by Parse, or the one passed to SetHeader.
func (d *Document) Header() Header {
	return d.header
}

// SetHeader replaces the generated header; Write then renders h instead of the parsed bytes.
// A zero h removes the header.
func (d *Document) SetHeader(h Header) {
	d.header = h
	d.headerSet = true
}

// Generated reports whether Parse stripped a generated header, even one whose fields do not parse.
// SetHeader does not change it.
func (d *Document) Generated() bool {
	return d.hasHeader()
}

// FileLine maps a body line (yaml.Node.Line, Leaf.Line) to the line an editor shows for it.
func (d *Document) FileLine(bodyLine int) int {
	if d.hasHeader() {
		return bodyLine + headerLineCount
	}
	return bodyLine
}

// hasHeader is the one test for "the parsed file carries the header"; Header().IsZero() is not,
// since unparseable fields stay empty and SetHeader replaces the value.
func (d *Document) hasHeader() bool {
	return d.headerRaw != nil
}

// detectEOL returns the line ending of the first line break in src, LF when there is none.
func detectEOL(src []byte) string {
	i := bytes.IndexByte(src, '\n')
	if i > 0 && src[i-1] == '\r' {
		return crlf
	}
	return lineFeed
}

// line is the byte span of one source line; text excludes the line break.
type line struct {
	start int
	text  []byte
}

func splitLines(src []byte) []line {
	var lines []line
	for start := 0; start < len(src); {
		end := bytes.IndexByte(src[start:], '\n')
		next := start + end + 1
		if end < 0 {
			next = len(src)
		}
		lines = append(lines, line{start: start, text: bytes.TrimSuffix(bytes.TrimSuffix(src[start:next], []byte(lineFeed)), []byte("\r"))})
		start = next
	}
	return lines
}

// startsMarker reports whether l opens with a document marker, "---" or "...", that is a token
// of its own (followed by the end of the line or whitespace).
func (l line) startsMarker() bool {
	if !bytes.HasPrefix(l.text, []byte(docStartMarker)) && !bytes.HasPrefix(l.text, []byte(docEndMarker)) {
		return false
	}
	rest := l.text[len(docStartMarker):]
	return len(rest) == 0 || rest[0] == ' ' || rest[0] == '\t'
}

// isMarker reports whether l is a bare marker line: a marker and nothing but whitespace after it.
func (l line) isMarker() bool {
	return l.startsMarker() && len(bytes.TrimSpace(l.text[len(docStartMarker):])) == 0
}

func (l line) isBlank() bool {
	return len(bytes.TrimSpace(l.text)) == 0
}

// isPreamble reports whether l may precede a leading "---": blank, comment-only or a directive.
func (l line) isPreamble() bool {
	t := bytes.TrimLeft(l.text, " \t")
	return len(t) == 0 || t[0] == '#' || l.text[0] == '%'
}

// checkMarkers returns the index of the first tail line (len(lines) when there is no tail).
// Any document marker before the tail, other than one leading "---", gives ErrMultiDocument.
func checkMarkers(lines []line) (int, error) {
	leading := -1
	for i, l := range lines {
		if l.isPreamble() {
			continue
		}
		if l.startsMarker() && bytes.HasPrefix(l.text, []byte(docStartMarker)) {
			leading = i
		}
		break
	}
	runStart := len(lines)
	for runStart > leading+1 && (lines[runStart-1].isMarker() || lines[runStart-1].isBlank()) {
		runStart--
	}
	tailStart := len(lines)
	for i := runStart; i < len(lines); i++ {
		if lines[i].isMarker() {
			tailStart = i
			break
		}
	}
	for i := 0; i < tailStart; i++ {
		if i != leading && lines[i].startsMarker() {
			return 0, fmt.Errorf("%w: document marker on line %d", ErrMultiDocument, i+1)
		}
	}
	return tailStart, nil
}

// decodeSingle decodes src, which must hold one document plus, when hasTail is set, at most
// one empty document opened by a trailing "---".
func decodeSingle(src []byte, hasTail bool) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var root yaml.Node
	if err := dec.Decode(&root); err == io.EOF {
		return &yaml.Node{Kind: yaml.DocumentNode}, nil
	} else if err != nil {
		return nil, fmt.Errorf("roundtrip: %w", err)
	}
	for extra := 0; ; extra++ {
		var next yaml.Node
		err := dec.Decode(&next)
		if err == io.EOF {
			return &root, nil
		}
		if err != nil {
			return nil, fmt.Errorf("roundtrip: %w", err)
		}
		if !hasTail || extra > 0 || !isEmptyDocument(&next) {
			return nil, ErrMultiDocument
		}
	}
}

// isEmptyDocument reports whether doc is what yaml.v3 makes of a bare trailing "---".
func isEmptyDocument(doc *yaml.Node) bool {
	if len(doc.Content) != 1 || hasComments(doc) {
		return false
	}
	n := doc.Content[0]
	return n.Kind == yaml.ScalarNode && n.Tag == nullTag && n.Value == "" && !hasComments(n)
}

func hasComments(n *yaml.Node) bool {
	return n.HeadComment != "" || n.LineComment != "" || n.FootComment != ""
}

// lineStarts returns the byte offset at which each line of src starts; after a final line
// break the next, empty line starts at len(src).
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offset maps a yaml.v3 position (1-based line, 1-based column counted in runes) to a byte
// offset into the body. ok is false when the position lies outside the body.
func (d *Document) offset(lineNo, col int) (int, bool) {
	if lineNo < 1 || lineNo > len(d.lineStarts) || col < 1 {
		return 0, false
	}
	off := d.lineStarts[lineNo-1]
	for range col - 1 {
		if off >= len(d.body) || d.body[off] == '\n' {
			return 0, false
		}
		_, size := utf8.DecodeRune(d.body[off:])
		off += size
	}
	return off, true
}
