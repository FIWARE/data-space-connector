package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/check"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
)

const (
	renderDomain      = "example.org"
	otherRenderDomain = "example.com"
	keycloakURLPath   = "keycloak.realm.frontendUrl"
	handEditedURL     = "https://login.hand.example"
	// handEditedValues sets a target of host.keycloak by hand, so a render must keep it.
	handEditedValues = "keycloak:\n  realm:\n    frontendUrl: " + handEditedURL + "\n"
)

// renderLeaf is a decoded tree leaf.
type renderLeaf struct {
	Path       string            `json:"path"`
	Value      any               `json:"value"`
	Provenance render.Provenance `json:"provenance"`
}

// renderBody is the decoded POST /api/render response, paths kept as strings.
type renderBody struct {
	Tree struct {
		Leaves []renderLeaf `json:"leaves"`
	} `json:"tree"`
	Diff struct {
		Groups []struct {
			Foundation string `json:"foundation"`
			Changes    []struct {
				Path   string        `json:"path"`
				Status render.Status `json:"status"`
				Before any           `json:"before"`
				After  any           `json:"after"`
				Kept   any           `json:"kept"`
			} `json:"changes"`
		} `json:"groups"`
	} `json:"diff"`
	HandSet  []render.HandSetAnswer `json:"handSet"`
	Findings []check.Finding        `json:"findings"`
}

// changes counts the diff's changes per status.
func (b renderBody) changes() map[render.Status]int {
	n := map[render.Status]int{}
	for _, g := range b.Diff.Groups {
		for _, c := range g.Changes {
			n[c.Status]++
		}
	}
	return n
}

// domainProfile is a complete DSC profile that answers only domain.base.
func domainProfile(domain string) profile.Profile {
	return profile.Profile{Target: completeTarget(), Answers: map[string]any{"domain.base": domain}}
}

// render posts p with the load sha names to /api/render and returns the status, the raw body and, on
// 200, the decoded body.
func (ps *profileSession) render(p profile.Profile, sha string) (int, string, renderBody) {
	ps.t.Helper()
	data, err := json.Marshal(renderRequest{Profile: p, SourceSHA256: sha})
	if err != nil {
		ps.t.Fatal(err)
	}
	resp, raw := do(ps.t, ps.srv, request{method: http.MethodPost, path: "/api/render", origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(data)})
	var got renderBody
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			ps.t.Fatalf("body %q is not a render response: %v", raw, err)
		}
	}
	return resp.StatusCode, raw, got
}

func (ps *profileSession) mustRender(p profile.Profile, sha string) renderBody {
	ps.t.Helper()
	status, raw, got := ps.render(p, sha)
	if status != http.StatusOK {
		ps.t.Fatalf("render status = %d: %s", status, raw)
	}
	return got
}

func TestRenderRefusals(t *testing.T) {
	tests := []struct {
		name      string
		answers   map[string]any
		sha       string
		unauth    bool
		want      int
		wantField string
	}{
		{name: "missing token", answers: map[string]any{"domain.base": renderDomain}, unauth: true, want: http.StatusUnauthorized},
		{name: "malformed domain", answers: map[string]any{"domain.base": "Not A Domain"}, want: http.StatusUnprocessableEntity, wantField: "answers.domain.base"},
		{name: "unknown answer", answers: map[string]any{"db.password": "x"}, want: http.StatusUnprocessableEntity, wantField: "answers.db.password"},
		{name: "stale sourceSha256", answers: map[string]any{"domain.base": renderDomain}, sha: "0000", want: http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			p := profile.Profile{Target: completeTarget(), Answers: tc.answers}
			data, err := json.Marshal(renderRequest{Profile: p, SourceSHA256: tc.sha})
			if err != nil {
				t.Fatal(err)
			}
			req := request{method: http.MethodPost, path: "/api/render", origin: ownOrigin(ps.srv), cookie: ps.cookie, body: string(data)}
			if tc.unauth {
				req.cookie = nil
			}
			resp, raw := do(t, ps.srv, req)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, raw)
			}
			if tc.wantField != "" && !strings.Contains(raw, `"`+tc.wantField+`"`) {
				t.Errorf("body %s does not name field %s", raw, tc.wantField)
			}
		})
	}
}

func TestRenderDiffsAgainstPreviousRender(t *testing.T) {
	ps := newProfileSession(t)
	first := ps.mustRender(domainProfile(renderDomain), "")
	leaves := len(first.Tree.Leaves)
	if leaves == 0 {
		t.Fatal("first render has no leaves")
	}
	if got := first.changes(); got[render.StatusAdded] != leaves || len(got) != 1 {
		t.Errorf("first render: changes = %v, want all %d leaves added", got, leaves)
	}
	if got := ps.mustRender(domainProfile(renderDomain), "").changes(); len(got) != 0 {
		t.Errorf("same render: changes = %v, want none", got)
	}
	second := ps.mustRender(domainProfile(otherRenderDomain), "")
	changed := 0
	for _, l := range first.Tree.Leaves {
		if s, ok := l.Value.(string); ok && strings.Contains(s, renderDomain) {
			changed++
		}
	}
	if got := second.changes(); got[render.StatusChanged] != changed || len(got) != 1 {
		t.Errorf("domain change: changes = %v, want %d changed", got, changed)
	}
}

func TestRenderListsHandSetAnswers(t *testing.T) {
	const handDID = "did:web:id.acme.com"
	ps := newProfileSession(t)
	if status, raw, _ := ps.render(domainProfile(renderDomain), ""); status != http.StatusOK || !strings.Contains(raw, `"handSet":[]`) {
		t.Errorf("derived answers only: status %d, body %s, want an empty handSet", status, raw)
	}
	for _, d := range []string{renderDomain, otherRenderDomain} {
		p := domainProfile(d)
		p.Answers["did.value"] = handDID
		got := ps.mustRender(p, "")
		want := []render.HandSetAnswer{{ID: "did.value", Value: handDID, Derived: "did:web:" + d}}
		if !slices.Equal(got.HandSet, want) {
			t.Errorf("domain %s: handSet = %+v, want %+v", d, got.HandSet, want)
		}
	}
}

func TestRenderReportsFindings(t *testing.T) {
	const keyDID = "did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK"
	tests := []struct {
		name   string
		env    profile.Environment
		method string
		want   []string
	}{
		{name: "development did:web", env: profile.EnvDevelopment, method: "did:web"},
		{name: "development did:key", env: profile.EnvDevelopment, method: "did:key"},
		{name: "production did:web", env: profile.EnvProduction, method: "did:web"},
		{name: "production did:key", env: profile.EnvProduction, method: "did:key", want: []string{"did-key-in-production"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			p := domainProfile(renderDomain)
			p.Target.Environment = tc.env
			p.Answers["did.method"] = tc.method
			if tc.method == "did:key" {
				p.Answers["did.value"] = keyDID
			}
			// Rendered twice: the second tree is unchanged, and its findings must still come back.
			for range 2 {
				got := ps.mustRender(p, "")
				if len(got.Tree.Leaves) == 0 {
					t.Fatal("a render with findings has no tree")
				}
				rules := []string{}
				for _, f := range got.Findings {
					rules = append(rules, f.Rule)
					if f.Severity != model.SeverityWarning || f.Stage != model.StageFoundations || f.Field != "answers.did.method" {
						t.Errorf("finding = %+v, want a foundations warning on answers.did.method", f)
					}
				}
				if !slices.Equal(rules, append([]string{}, tc.want...)) {
					t.Errorf("findings = %v, want %v", rules, tc.want)
				}
			}
		})
	}
}

func TestRenderPinsLoadedHandEdit(t *testing.T) {
	ps := newProfileSession(t)
	ps.mustRender(domainProfile(renderDomain), "")
	sha := ps.load(valuesName, handEditedValues).SHA256

	first := ps.mustRender(domainProfile(renderDomain), sha)
	i := slices.IndexFunc(first.Tree.Leaves, func(l renderLeaf) bool { return l.Path == keycloakURLPath })
	if i < 0 {
		t.Fatalf("tree has no leaf %s", keycloakURLPath)
	}
	if l := first.Tree.Leaves[i]; !l.Provenance.Pinned || l.Value != handEditedURL || l.Provenance.Rendered != "https://keycloak."+renderDomain {
		t.Errorf("leaf = %+v, want the hand edit kept and pinned", l)
	}
	// The load emptied lastRender, so every leaf is new and the pinned one is reported once.
	if got := first.changes(); got[render.StatusAdded] != len(first.Tree.Leaves)-1 || got[render.StatusPinned] != 1 {
		t.Errorf("render after load: changes = %v, want a reset diff with one pinned leaf", got)
	}

	second := ps.mustRender(domainProfile(otherRenderDomain), sha)
	var pinned []string
	for _, g := range second.Diff.Groups {
		for _, c := range g.Changes {
			if c.Status == render.StatusPinned {
				pinned = append(pinned, c.Path)
				if c.Kept != handEditedURL || c.After != "https://keycloak."+otherRenderDomain {
					t.Errorf("pinned change = %+v, want the new rendered value and the hand edit kept", c)
				}
			}
		}
	}
	if !slices.Equal(pinned, []string{keycloakURLPath}) {
		t.Errorf("domain change: pinned = %v, want [%s]", pinned, keycloakURLPath)
	}
	if got := ps.mustRender(domainProfile(otherRenderDomain), sha).changes(); len(got) != 0 {
		t.Errorf("unchanged render: changes = %v, want none", got)
	}
}

// A profile that names no source (a new configuration after a load) neither pins the loaded file's
// hand edits nor diffs against the renders of that file, and switching back starts afresh too.
func TestRenderWithoutSourceIgnoresLoad(t *testing.T) {
	ps := newProfileSession(t)
	sha := ps.load(valuesName, handEditedValues).SHA256
	ps.mustRender(domainProfile(renderDomain), sha)

	for _, step := range []struct {
		name       string
		sha        string
		wantPinned int
	}{
		{name: "no source", sha: "", wantPinned: 0},
		{name: "loaded source again", sha: sha, wantPinned: 1},
	} {
		got := ps.mustRender(domainProfile(renderDomain), step.sha)
		pinned := 0
		for _, l := range got.Tree.Leaves {
			if l.Provenance.Pinned {
				pinned++
			}
		}
		if pinned != step.wantPinned {
			t.Errorf("%s: %d pinned leaves, want %d", step.name, pinned, step.wantPinned)
		}
		if n := got.changes(); n[render.StatusAdded] != len(got.Tree.Leaves)-step.wantPinned || n[render.StatusPinned] != step.wantPinned {
			t.Errorf("%s: changes = %v, want a diff against an empty tree", step.name, n)
		}
	}
}

func TestRenderResetOnProfileOpen(t *testing.T) {
	ps := newProfileSession(t)
	saved := ps.mustSave(saveRequest{Path: filepath.Join(t.TempDir(), profileName), Profile: domainProfile(renderDomain)})
	ps.mustRender(domainProfile(renderDomain), "")
	if status, raw, _ := ps.open(saved.Path); status != http.StatusOK {
		t.Fatalf("open status = %d: %s", status, raw)
	}
	got := ps.mustRender(domainProfile(renderDomain), "")
	if n := got.changes(); n[render.StatusAdded] != len(got.Tree.Leaves) {
		t.Errorf("render after open: changes = %v, want every leaf added", n)
	}
}
