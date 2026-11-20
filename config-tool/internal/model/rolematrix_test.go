package model

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"

	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	roleGuide        = repoRoot + "/doc/deployment-integration/roles/README.md"
	matrixHeading    = "## Component matrix"
	componentHeader  = "Component"
	cellRequired     = "Required"
	cellOptional     = "Optional"
	cellNotOffered   = "-"
	cellBuiltIntoPre = "Built into "
)

// matrix is the role guide's component matrix with rowspans expanded: every row has a cell per header.
type matrix struct {
	header []string
	rows   [][]string
}

// parseMatrix reads the first <table> under the "Component matrix" heading of the role guide.
func parseMatrix(src string) (matrix, error) {
	start := strings.Index(src, "\n"+matrixHeading+"\n")
	if start < 0 {
		return matrix{}, fmt.Errorf("no %q heading", matrixHeading)
	}
	section := src[start+1:]
	if end := strings.Index(section[len(matrixHeading):], "\n## "); end >= 0 {
		section = section[:len(matrixHeading)+end]
	}
	from, to := strings.Index(section, "<table"), strings.Index(section, "</table>")
	if from < 0 || to < from {
		return matrix{}, fmt.Errorf("no <table> under %q", matrixHeading)
	}
	doc, err := html.Parse(strings.NewReader(section[from : to+len("</table>")]))
	if err != nil {
		return matrix{}, err
	}
	var trs []*html.Node
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "tr" {
			trs = append(trs, n)
		}
	}
	if len(trs) == 0 {
		return matrix{}, errors.New("the matrix has no rows")
	}
	grid := expandRows(trs)
	return matrix{header: grid[0], rows: grid[1:]}, nil
}

// spanned is a cell that still covers the given number of rows below the current one.
type spanned struct {
	text string
	left int
}

// expandRows turns <tr> elements into rows of cell text, repeating rowspan cells downwards.
func expandRows(trs []*html.Node) [][]string {
	pending := map[int]*spanned{}
	fill := func(row []string) []string {
		for p := pending[len(row)]; p != nil && p.left > 0; p = pending[len(row)] {
			row = append(row, p.text)
			p.left--
		}
		return row
	}
	var grid [][]string
	for _, tr := range trs {
		var row []string
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || c.Data != "td" && c.Data != "th" {
				continue
			}
			row = fill(row)
			text := cellText(c)
			if span, err := strconv.Atoi(attr(c, "rowspan")); err == nil && span > 1 {
				pending[len(row)] = &spanned{text: text, left: span - 1}
			}
			row = append(row, text)
		}
		grid = append(grid, fill(row))
	}
	return grid
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// cellText is the text content of a cell with whitespace collapsed.
func cellText(n *html.Node) string {
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// column returns the index of the header whose trimmed text equals name exactly, or -1.
func (m matrix) column(name string) int {
	return slices.Index(m.header, name)
}

// rowOf returns the row whose Component cell is component, or nil.
func (m matrix) rowOf(compCol int, component string) []string {
	for _, r := range m.rows {
		if r[compCol] == component {
			return r
		}
	}
	return nil
}

// offer translates a matrix cell (decision 2 of ADR 0017); "Built into X" takes row X's cell in the same column.
func (m matrix) offer(compCol, col int, row []string) (Offer, error) {
	text := row[col]
	if host, ok := strings.CutPrefix(text, cellBuiltIntoPre); ok {
		hostRow := m.rowOf(compCol, host)
		if hostRow == nil {
			return "", fmt.Errorf("%q names no matrix row", text)
		}
		text = hostRow[col]
	}
	switch text {
	case cellRequired:
		return OfferRecommended, nil
	case cellOptional:
		return OfferAvailable, nil
	case cellNotOffered:
		return OfferNotOffered, nil
	}
	return "", fmt.Errorf("unknown cell text %q", row[col])
}

// matrixOffer is a feature's offer in one role column and the component it was first read from.
type matrixOffer struct {
	offer     Offer
	component string
}

// compareMatrix lists every disagreement between the matrix and the bundle's roles and roleMatrix.
func compareMatrix(m matrix, b Bundle) []string {
	if b.RoleMatrix == nil {
		return []string{"the bundle declares no roleMatrix"}
	}
	compCol := m.column(componentHeader)
	if compCol < 0 {
		return []string{fmt.Sprintf("the matrix has no column %q", componentHeader)}
	}
	var errs []string
	var mapped [][]string
	for _, r := range m.rows {
		comp := r[compCol]
		if _, ok := b.RoleMatrix.Components[comp]; ok {
			mapped = append(mapped, r)
		} else if !slices.Contains(b.RoleMatrix.Infrastructure, comp) {
			errs = append(errs, fmt.Sprintf("component %q is in neither roleMatrix.components nor roleMatrix.infrastructure", comp))
		}
	}
	for _, comp := range slices.Sorted(maps.Keys(b.RoleMatrix.Components)) {
		if m.rowOf(compCol, comp) == nil {
			errs = append(errs, fmt.Sprintf("component %q is mapped in roleMatrix but missing from the matrix", comp))
		}
	}
	for _, role := range b.Roles {
		if role.ID == RoleCustom {
			continue
		}
		col := m.column(role.Title)
		if col < 0 {
			errs = append(errs, fmt.Sprintf("role %q: the matrix has no column %q", role.ID, role.Title))
			continue
		}
		var order []string
		seen := map[string]matrixOffer{}
		conflicted := map[string]bool{}
		for _, r := range mapped {
			comp, feature := r[compCol], b.RoleMatrix.Components[r[compCol]]
			o, err := m.offer(compCol, col, r)
			if err != nil {
				errs = append(errs, fmt.Sprintf("role %q, component %q: %v", role.ID, comp, err))
				conflicted[feature] = true
				continue
			}
			first, ok := seen[feature]
			if !ok {
				seen[feature] = matrixOffer{o, comp}
				order = append(order, feature)
			} else if first.offer != o && !conflicted[feature] {
				errs = append(errs, fmt.Sprintf("role %q, feature %q: components %q and %q disagree", role.ID, feature, first.component, comp))
				conflicted[feature] = true
			}
		}
		for _, feature := range order {
			if conflicted[feature] {
				continue
			}
			want := seen[feature]
			if got := role.Features[feature]; got != want.offer {
				errs = append(errs, fmt.Sprintf("role %q, feature %q: bundle says %s, matrix says %s (component %q)", role.ID, feature, got, want.offer, want.component))
			}
		}
	}
	return errs
}

// latestRolesBundle is the shipped bundle the role catalogue comes from.
func latestRolesBundle(t *testing.T) Bundle {
	t.Helper()
	got, err := Versions(bundles.FS)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RoleCatalogue(got, chart, "")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readRoleGuide(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(roleGuide)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// TestRoleMatrixAgreesWithBundle keeps the shipped presets equal to the role guide's matrix, which is authoritative.
func TestRoleMatrixAgreesWithBundle(t *testing.T) {
	m, err := parseMatrix(readRoleGuide(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range compareMatrix(m, latestRolesBundle(t)) {
		t.Error(e)
	}
}

// TestCustomOffersItsWholeChart pins decision 3 of ADR 0017: custom offers its chart's features, recommends none.
func TestCustomOffersItsWholeChart(t *testing.T) {
	b := latestRolesBundle(t)
	i := slices.IndexFunc(b.Roles, func(r Role) bool { return r.ID == RoleCustom })
	if i < 0 {
		t.Fatal("no custom role")
	}
	custom := b.Roles[i]
	for _, f := range b.Features {
		want := OfferNotOffered
		if f.Chart == custom.Chart {
			want = OfferAvailable
		}
		if got := custom.Features[f.ID]; got != want {
			t.Errorf("custom offers %s as %s, want %s", f.ID, got, want)
		}
	}
}

func TestRoleDocsExist(t *testing.T) {
	for _, r := range latestRolesBundle(t).Roles {
		docs, _, _ := strings.Cut(r.Docs, "#")
		if _, err := os.Stat(repoRoot + "/" + docs); err != nil {
			t.Errorf("role %s: docs %q: %v", r.ID, r.Docs, err)
		}
	}
}

var (
	rowPattern  = regexp.MustCompile(`(?s)<tr>.*?</tr>`)
	cellPattern = regexp.MustCompile(`(?s)<td[^>]*>.*?</td>`)
	tagPattern  = regexp.MustCompile(`<[^>]+>`)
)

// roleCellsFromEnd counts a role column from the end of a <tr>: the role cells are always its last ones,
// whatever rowspans the cells on its left have.
var roleCellsFromEnd = map[string]int{"Consumer": 4, "Provider": 3}

// editRow rewrites the <tr> of the guide whose Component cell is component; it fails when there is none.
func editRow(t *testing.T, src, component string, edit func(row string) string) string {
	t.Helper()
	for _, loc := range rowPattern.FindAllStringIndex(src, -1) {
		row := src[loc[0]:loc[1]]
		for _, cell := range cellPattern.FindAllString(row, -1) {
			if strings.TrimSpace(tagPattern.ReplaceAllString(cell, "")) == component {
				return src[:loc[0]] + edit(row) + src[loc[1]:]
			}
		}
	}
	t.Fatalf("the role guide has no row %q", component)
	return ""
}

// setCell sets the cell of component in a role column.
func setCell(t *testing.T, src, component, role, text string) string {
	return editRow(t, src, component, func(row string) string {
		cells := cellPattern.FindAllStringIndex(row, -1)
		c := cells[len(cells)-roleCellsFromEnd[role]]
		return row[:c[0]] + "<td>" + text + "</td>" + row[c[1]:]
	})
}

// TestRoleMatrixMutations proves that changing the matrix without the bundle fails with a precise message.
func TestRoleMatrixMutations(t *testing.T) {
	const fooRow = "<tr><td>-</td><td>-</td><td>Foo</td><td>x</td><td>-</td><td>-</td><td>-</td><td>-</td></tr>"
	tests := []struct {
		name   string
		mutate func(t *testing.T, src string) string
		want   string
	}{
		{"single-component feature flipped", func(t *testing.T, src string) string {
			return setCell(t, src, "Keycloak", "Provider", cellOptional)
		}, `role "provider", feature "identity.issuer": bundle says recommended, matrix says available (component "Keycloak")`},
		{"one component of several flipped", func(t *testing.T, src string) string {
			return setCell(t, src, "OPA", "Provider", cellOptional)
		}, `role "provider", feature "access.policies": components "APISIX" and "OPA" disagree`},
		{"mapped row dropped", func(t *testing.T, src string) string {
			return editRow(t, src, "DID", func(string) string { return "" })
		}, `component "DID" is mapped in roleMatrix but missing from the matrix`},
		{"unmapped row added", func(t *testing.T, src string) string {
			return editRow(t, src, "DID", func(row string) string { return row + fooRow })
		}, `component "Foo" is in neither roleMatrix.components nor roleMatrix.infrastructure`},
		{"unknown cell text", func(t *testing.T, src string) string {
			return setCell(t, src, "Keycloak", "Consumer", "Maybe")
		}, `role "consumer", component "Keycloak": unknown cell text "Maybe"`},
	}
	b := latestRolesBundle(t)
	src := readRoleGuide(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseMatrix(tc.mutate(t, src))
			if err != nil {
				t.Fatal(err)
			}
			if got := compareMatrix(m, b); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("errors = %q, want [%q]", got, tc.want)
			}
		})
	}
}

func TestParseMatrixRowspans(t *testing.T) {
	const src = "# x\n" + matrixHeading + "\n<table><tr><th>A</th><th>Component</th><th>B</th></tr>" +
		"<tr><td rowspan=\"2\">a</td><td>c1</td><td> b\n 1 </td></tr><tr><td>c2</td><td>b2</td></tr></table>\n## Next\n<table></table>"
	m, err := parseMatrix(src)
	if err != nil {
		t.Fatal(err)
	}
	want := matrix{header: []string{"A", "Component", "B"}, rows: [][]string{{"a", "c1", "b 1"}, {"a", "c2", "b2"}}}
	if !slices.Equal(m.header, want.header) || !slices.EqualFunc(m.rows, want.rows, slices.Equal) {
		t.Errorf("matrix = %q, want %q", m, want)
	}
	if _, err := parseMatrix("# x\n" + matrixHeading + "\nno table\n## Next\n<table></table>"); err == nil {
		t.Error("a table under a later heading was taken")
	}
}
