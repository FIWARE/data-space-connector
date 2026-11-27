package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/check"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/requirements"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
)

const (
	renderDomain      = "example.org"
	otherRenderDomain = "example.com"
	keycloakURLPath   = "keycloak.realm.frontendUrl"
	handEditedURL     = "https://login.hand.example"
	// handEditedValues sets a target of host.keycloak by hand, so a render must keep it.
	handEditedValues = "keycloak:\n  realm:\n    frontendUrl: " + handEditedURL + "\n"
	tirRule          = "tir-address-for-verification"
	tirAnswer        = "dataspace.tirAddress"
	loadedTIR        = "https://tir.example.org/"
	// tirValues sets the verifier's TIR address, which a load derives dataspace.tirAddress from.
	tirValues = "decentralizedIam:\n  vcAuthentication:\n    vcverifier:\n      deployment:\n        verifier:\n" +
		"          tirAddress: " + loadedTIR + "\n"
)

// verificationProfile is domainProfile with credential verification switched as given.
func verificationProfile(on bool) profile.Profile {
	p := domainProfile(renderDomain)
	p.Role = model.RoleProvider
	p.Features = map[string]profile.FeatureState{"access.verification": {Enabled: on, Source: profile.SourceUser}}
	return p
}

// findingRules lists the rule ids of findings.
func findingRules(findings []check.Finding) []string {
	rules := []string{}
	for _, f := range findings {
		rules = append(rules, f.Rule)
	}
	return rules
}

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
	Derived  render.AnswerDiff      `json:"derived"`
	HandSet  []render.HandSetAnswer `json:"handSet"`
	Findings []check.Finding        `json:"findings"`
	Secrets  []requirements.Secret  `json:"secrets"`
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

func TestRenderListsSecrets(t *testing.T) {
	const caSecret = "corp-ca"
	tests := []struct {
		name    string
		answers map[string]any
		want    []string
	}{
		{name: "no private CA", answers: map[string]any{"domain.base": renderDomain}, want: []string{}},
		{name: "private CA", answers: map[string]any{"domain.base": renderDomain, "ca.privateSecret": caSecret},
			want: []string{completeTarget().Namespace + "/" + caSecret + "#ca.crt"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			status, raw, got := ps.render(profile.Profile{Target: completeTarget(), Answers: tc.answers}, "")
			if status != http.StatusOK || !strings.Contains(raw, `"secrets":[`) {
				t.Fatalf("status %d, body %s, want a secrets list", status, raw)
			}
			secrets := []string{}
			for _, s := range got.Secrets {
				if s.Purpose == "" {
					t.Errorf("secret %+v has no purpose", s)
				}
				secrets = append(secrets, s.Namespace+"/"+s.Name+"#"+strings.Join(s.Keys, ","))
			}
			if !slices.Equal(secrets, tc.want) {
				t.Errorf("secrets = %v, want %v", secrets, tc.want)
			}
		})
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

// TestRenderTIRAddressFinding builds the profile the way the UI does after a load: the report's foundations-stage
// answers go into answers, so a loaded TIR address satisfies tir-address-for-verification.
func TestRenderTIRAddressFinding(t *testing.T) {
	tests := []struct {
		name         string
		verification bool
		load         bool
		want         []string
	}{
		{name: "verification on, TIR loaded", verification: true, load: true, want: []string{}},
		{name: "verification on, nothing loaded", verification: true, want: []string{tirRule}},
		{name: "verification off, nothing loaded", want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			p, sha := verificationProfile(tc.verification), ""
			if tc.load {
				report := ps.load(valuesName, tirValues)
				sha = report.SHA256
				for _, a := range report.Answers {
					if a.Stage == string(model.StageFoundations) {
						p.Answers[a.ID] = a.Value
					}
				}
				if p.Answers[tirAnswer] != loadedTIR {
					t.Fatalf("answers = %v, want the loaded %s = %s", p.Answers, tirAnswer, loadedTIR)
				}
			}
			got := ps.mustRender(p, sha)
			if rules := findingRules(got.Findings); !slices.Equal(rules, tc.want) {
				t.Errorf("findings = %v, want %v", rules, tc.want)
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

const (
	pinnedHost    = "host.keycloak"
	hostPrefix    = "host."
	collidingHost = "host.marketplace"
	collisionRule = "host-collision"
	// switchedVersion is a second DSC version, served from a copy of the latest bundle.
	switchedVersion = "10.17.0"
)

// derivedHost is the host id derives under domain; host.keycloak derives keycloak.<domain>.
func derivedHost(domain string) string {
	return "keycloak." + domain
}

// onDomain reports whether host is domain or one of its sub-domains (host.did derives the bare domain).
func onDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// hostIDs lists the host foundations of the shipped DSC bundle in declaration order.
func hostIDs(t *testing.T, srv *Server) []string {
	t.Helper()
	ids := []string{}
	for _, id := range latestBundle(t, mustBundles(t, srv), dscChart).FoundationOrder() {
		if strings.HasPrefix(id, hostPrefix) {
			ids = append(ids, id)
		}
	}
	return ids
}

func TestRenderDerivedDiff(t *testing.T) {
	ps := newProfileSession(t)
	if status, raw, _ := ps.render(domainProfile(renderDomain), ""); status != http.StatusOK ||
		!strings.Contains(raw, `"derived":{"rederived":[],"pinned":[]}`) {
		t.Fatalf("first render: status %d, body %s, want an empty derived diff", status, raw)
	}
	others := slices.DeleteFunc(hostIDs(t, ps.srv), func(id string) bool { return id == pinnedHost })
	tests := []struct {
		name          string
		domain        string
		pin           bool
		wantPinned    []render.PinnedAnswer
		wantRederived []string
	}{
		{name: "pin at the derived value", domain: renderDomain, pin: true, wantPinned: []render.PinnedAnswer{}},
		{name: "domain change keeps the pinned host", domain: otherRenderDomain, pin: true,
			wantPinned: []render.PinnedAnswer{{ID: pinnedHost, Kept: derivedHost(renderDomain),
				Before: derivedHost(renderDomain), After: derivedHost(otherRenderDomain)}},
			wantRederived: others},
		{name: "reset to derived", domain: otherRenderDomain, wantPinned: []render.PinnedAnswer{}, wantRederived: []string{pinnedHost}},
	}
	for _, tc := range tests {
		p := domainProfile(tc.domain)
		if tc.pin {
			p.Answers[pinnedHost] = derivedHost(renderDomain)
		}
		got := ps.mustRender(p, "").Derived
		if !slices.Equal(got.Pinned, tc.wantPinned) {
			t.Errorf("%s: pinned = %+v, want %+v", tc.name, got.Pinned, tc.wantPinned)
		}
		hosts := []string{}
		for _, c := range got.Rederived {
			if !strings.HasPrefix(c.ID, hostPrefix) {
				continue
			}
			hosts = append(hosts, c.ID)
			if before, after := c.Before.(string), c.After.(string); !onDomain(before, renderDomain) || !onDomain(after, otherRenderDomain) {
				t.Errorf("%s: re-derived %+v, want a move from %s to %s", tc.name, c, renderDomain, otherRenderDomain)
			}
		}
		if !slices.Equal(hosts, append([]string{}, tc.wantRederived...)) {
			t.Errorf("%s: re-derived hosts = %v, want %v", tc.name, hosts, tc.wantRederived)
		}
	}
}

// switchModels serves the shipped bundles plus a copy of the latest DSC bundle as switchedVersion.
func switchModels(t *testing.T) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	err := fs.WalkDir(bundles.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(bundles.FS, path)
		fsys[path] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	latest := fsys[latestChart+"/bundle.yaml"].Data
	copied := strings.Replace(string(latest), "\nversion: "+latestChart+"\n", "\nversion: "+switchedVersion+"\n", 1)
	if copied == string(latest) {
		t.Fatalf("bundle %s declares no version line", latestChart)
	}
	fsys[switchedVersion+"/bundle.yaml"] = &fstest.MapFile{Data: []byte(copied)}
	return fsys
}

// A load, a profile open and a switch of chart or version each drop the previous answers, so the next render
// reports no derived change and diffs its leaves against an empty tree.
func TestRenderDerivedBaselineReset(t *testing.T) {
	switched := func(chart, version string) profile.Profile {
		p := domainProfile(otherRenderDomain)
		p.Target.Chart, p.Target.ChartVersion = chart, version
		return p
	}
	tests := []struct {
		name   string
		action func(ps *profileSession)
		next   profile.Profile
	}{
		{name: "load", action: func(ps *profileSession) { ps.load(valuesName, smallValues) }, next: domainProfile(otherRenderDomain)},
		{name: "profile open", action: func(ps *profileSession) {
			saved := ps.mustSave(saveRequest{Path: filepath.Join(ps.t.TempDir(), profileName), Profile: domainProfile(renderDomain)})
			if status, raw, _ := ps.open(saved.Path); status != http.StatusOK {
				ps.t.Fatalf("open status = %d: %s", status, raw)
			}
		}, next: domainProfile(otherRenderDomain)},
		{name: "chart switch", next: switched(trustAnchorChart, "")},
		{name: "version switch", next: switched(dscChart, switchedVersion)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, builtUI)
			srv.models = switchModels(t)
			serve(t, srv)
			ps := &profileSession{t: t, srv: srv, cookie: login(t, srv)}
			baseline := domainProfile(renderDomain)
			baseline.Answers[pinnedHost] = derivedHost(renderDomain)
			ps.mustRender(baseline, "")
			if tc.action != nil {
				tc.action(ps)
			}
			got := ps.mustRender(tc.next, "")
			if len(got.Derived.Rederived) != 0 || len(got.Derived.Pinned) != 0 {
				t.Errorf("derived = %+v, want empty", got.Derived)
			}
			if n := got.changes(); n[render.StatusAdded] != len(got.Tree.Leaves) || len(n) > 1 {
				t.Errorf("changes = %v, want all %d leaves added", n, len(got.Tree.Leaves))
			}
		})
	}
}

func TestRenderCollisionNamesRelatedField(t *testing.T) {
	ps := newProfileSession(t)
	p := domainProfile(renderDomain)
	p.Answers[collidingHost] = derivedHost(renderDomain)
	got := ps.mustRender(p, "")
	i := slices.IndexFunc(got.Findings, func(f check.Finding) bool { return f.Rule == collisionRule })
	if i < 0 {
		t.Fatalf("findings = %v, want %s", findingRules(got.Findings), collisionRule)
	}
	if f := got.Findings[i]; f.Field != "answers."+collidingHost || f.Related != "answers."+pinnedHost ||
		f.Severity != model.SeverityError {
		t.Errorf("finding = %+v, want an error on answers.%s related to answers.%s", f, collidingHost, pinnedHost)
	}
}

// TestRenderMemoryLimitReadsLoad pins that /api/render evaluates java-memory-limit over the loaded file: a loaded
// limit takes its component off the list, and a loaded null drops Keycloak's chart default.
func TestRenderMemoryLimitReadsLoad(t *testing.T) {
	const (
		memoryRule   = "java-memory-limit"
		memoryPrefix = "Without a memory limit, the JVM sizes its heap from the node: "
		unlimited    = "TMForum API, Trusted Issuers List, contract-management"
	)
	tests := []struct {
		name   string
		values string
		want   string
	}{
		{name: "nothing loaded", want: "Scorpio, " + unlimited},
		{name: "loaded Scorpio limit", values: "scorpio:\n  resources:\n    limits:\n      memory: 1Gi\n", want: unlimited},
		{name: "loaded Keycloak null", values: "keycloak:\n  resources: null\n", want: "Keycloak, Scorpio, " + unlimited},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := newProfileSession(t)
			sha := ""
			if tc.values != "" {
				sha = ps.load(valuesName, tc.values).SHA256
			}
			p := domainProfile(renderDomain)
			p.Target.Environment = profile.EnvProduction
			p.Answers["resources.profile"] = "custom"
			got := ps.mustRender(p, sha)
			i := slices.IndexFunc(got.Findings, func(f check.Finding) bool { return f.Rule == memoryRule })
			if i < 0 {
				t.Fatalf("findings = %v, want %s", findingRules(got.Findings), memoryRule)
			}
			if want := memoryPrefix + tc.want + "."; got.Findings[i].Message != want {
				t.Errorf("message = %q, want %q", got.Findings[i].Message, want)
			}
		})
	}
}
