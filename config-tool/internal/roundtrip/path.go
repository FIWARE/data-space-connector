package roundtrip

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	keySeparator = '.'
	indexOpen    = '['
	indexClose   = ']'
	quote        = '"'
	escape       = '\\'
	// keyQuoteChars force a key to be printed quoted, because they delimit segments.
	keyQuoteChars = `.[]"`
)

// Segment is one step of a Path: a mapping key or a sequence index.
type Segment struct {
	key     string
	index   int
	isIndex bool
}

// Key returns the segment that selects the mapping key k.
func Key(k string) Segment {
	return Segment{key: k}
}

// Index returns the segment that selects sequence item i. It panics when i is negative.
func Index(i int) Segment {
	if i < 0 {
		panic(fmt.Sprintf("roundtrip: negative sequence index %d", i))
	}
	return Segment{index: i, isIndex: true}
}

// Key returns the mapping key of s; ok is false for an index segment.
func (s Segment) Key() (k string, ok bool) {
	return s.key, !s.isIndex
}

// Index returns the sequence index of s; ok is false for a key segment.
func (s Segment) Index() (i int, ok bool) {
	return s.index, s.isIndex
}

// Path addresses a node from the document root; the empty Path is the root itself.
type Path []Segment

// String prints p as `a.b[0]`, quoting keys that are empty or hold `.`, `[`, `]` or `"`.
// ParsePath reads the result back into p.
func (p Path) String() string {
	var b strings.Builder
	for i, s := range p {
		if s.isIndex {
			b.WriteByte(indexOpen)
			b.WriteString(strconv.Itoa(s.index))
			b.WriteByte(indexClose)
			continue
		}
		if i > 0 {
			b.WriteByte(keySeparator)
		}
		if s.key != "" && !strings.ContainsAny(s.key, keyQuoteChars) {
			b.WriteString(s.key)
			continue
		}
		b.WriteByte(quote)
		for i := range len(s.key) {
			if s.key[i] == quote || s.key[i] == escape {
				b.WriteByte(escape)
			}
			b.WriteByte(s.key[i])
		}
		b.WriteByte(quote)
	}
	return b.String()
}

// ParsePath is the inverse of Path.String. Malformed input gives ErrInvalidPath.
func ParsePath(s string) (Path, error) {
	var p Path
	for i := 0; i < len(s); {
		var (
			seg Segment
			n   int
			err error
		)
		switch {
		case s[i] == indexOpen:
			seg, n, err = parseIndex(s[i:])
		case len(p) == 0:
			seg, n, err = parseKey(s[i:])
		case s[i] == keySeparator:
			seg, n, err = parseKey(s[i+1:])
			n++
		default:
			err = fmt.Errorf("unexpected %q", s[i])
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %q at byte %d: %v", ErrInvalidPath, s, i, err)
		}
		p = append(p, seg)
		i += n
	}
	return p, nil
}

// parseIndex reads `[n]` from the start of s and returns the segment and the bytes consumed.
func parseIndex(s string) (Segment, int, error) {
	end := strings.IndexByte(s, indexClose)
	if end < 0 {
		return Segment{}, 0, fmt.Errorf("unclosed index")
	}
	digits := s[1:end]
	if digits == "" || digits[0] < '0' || digits[0] > '9' || (len(digits) > 1 && digits[0] == '0') {
		return Segment{}, 0, fmt.Errorf("bad index %q", digits)
	}
	i, err := strconv.Atoi(digits)
	if err != nil {
		return Segment{}, 0, fmt.Errorf("bad index %q", digits)
	}
	return Index(i), end + 1, nil
}

// parseKey reads a bare or quoted key from the start of s and returns the segment and the
// bytes consumed. A key must be followed by the end of s, a separator or an index.
func parseKey(s string) (Segment, int, error) {
	if s != "" && s[0] == quote {
		return parseQuotedKey(s)
	}
	end := strings.IndexAny(s, keyQuoteChars)
	if end < 0 {
		end = len(s)
	}
	if end == 0 {
		return Segment{}, 0, fmt.Errorf("empty bare key")
	}
	if end < len(s) && s[end] != keySeparator && s[end] != indexOpen {
		return Segment{}, 0, fmt.Errorf("unexpected %q in bare key", s[end])
	}
	return Key(s[:end]), end, nil
}

func parseQuotedKey(s string) (Segment, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case escape:
			if i+1 == len(s) || (s[i+1] != quote && s[i+1] != escape) {
				return Segment{}, 0, fmt.Errorf("bad escape in quoted key")
			}
			i++
			b.WriteByte(s[i])
		case quote:
			if i+1 < len(s) && s[i+1] != keySeparator && s[i+1] != indexOpen {
				return Segment{}, 0, fmt.Errorf("unexpected %q after quoted key", s[i+1])
			}
			return Key(b.String()), i + 1, nil
		default:
			b.WriteByte(s[i])
		}
	}
	return Segment{}, 0, fmt.Errorf("unclosed quote")
}
