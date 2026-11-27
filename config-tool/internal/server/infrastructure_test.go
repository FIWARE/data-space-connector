package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/FIWARE/data-space-connector/config-tool/internal/load"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

const (
	certManager    = "certManager"
	modeBundled    = "bundled"
	modeCluster    = "cluster"
	modeExternal   = "external"
	clusterIssuer  = "letsencrypt-corp"
	keycloakSecret = "keycloak-corp-tls"
	keycloakIssuer = `keycloak.ingress.annotations."cert-manager.io/cluster-issuer"`
	keycloakTLS    = "keycloak.ingress.tls[0].secretName"
)

// infrastructureJSON is the decoded GET /api/infrastructure response.
type infrastructureJSON struct {
	Dependencies []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Doc         string `json:"doc"`
		ClusterWide bool   `json:"clusterWide"`
		Modes       []struct {
			ID      string `json:"id"`
			Label   string `json:"label"`
			Warning string `json:"warning"`
			Doc     string `json:"doc"`
			Keys    []struct {
				Path string `json:"path"`
			} `json:"keys"`
			Questions []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Default  string `json:"default"`
				ReadOnly bool   `json:"readOnly"`
				When     *struct {
					Question string `json:"question"`
					Equals   string `json:"equals"`
				} `json:"when"`
				Ingress *struct {
					ID    string `json:"id"`
					Label string `json:"label"`
				} `json:"ingress"`
			} `json:"questions"`
		} `json:"modes"`
	} `json:"dependencies"`
}

// getInfrastructure fetches the dependencies of chart@version and decodes a 200 response.
func (ps *profileSession) getInfrastructure(chart, version string) (int, infrastructureJSON) {
	ps.t.Helper()
	q := url.Values{}
	q.Set(queryChart, chart)
	if version != "" {
		q.Set(queryVersion, version)
	}
	resp, raw := do(ps.t, ps.srv, request{path: "/api/infrastructure?" + q.Encode(), cookie: ps.cookie})
	var got infrastructureJSON
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			ps.t.Fatalf("body %q is not an infrastructure response: %v", raw, err)
		}
	}
	return resp.StatusCode, got
}

func TestInfrastructureEndpointStatus(t *testing.T) {
	ps := newProfileSession(t)
	taVersion := latestBundle(t, mustBundles(t, ps.srv), trustAnchorChart).Version
	tests := []struct {
		name     string
		chart    string
		version  string
		want     int
		wantDeps int
	}{
		{name: "connector", chart: load.DefaultChart, version: latestChart, want: http.StatusOK, wantDeps: 1},
		{name: "trust anchor has none", chart: trustAnchorChart, version: taVersion, want: http.StatusOK},
		{name: "unknown version", chart: load.DefaultChart, version: "0.0.1", want: http.StatusNotFound},
		{name: "unknown chart", chart: "nope", want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, got := ps.getInfrastructure(tc.chart, tc.version)
			if status != tc.want {
				t.Fatalf("status = %d, want %d", status, tc.want)
			}
			if status == http.StatusOK && (got.Dependencies == nil || len(got.Dependencies) != tc.wantDeps) {
				t.Errorf("dependencies = %+v, want %d (never null)", got.Dependencies, tc.wantDeps)
			}
		})
	}
	resp, _ := do(t, ps.srv, request{path: "/api/infrastructure?chart=" + load.DefaultChart})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a token: status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestInfrastructureEndpointShape(t *testing.T) {
	ps := newProfileSession(t)
	_, got := ps.getInfrastructure(load.DefaultChart, latestChart)
	d := got.Dependencies[0]
	if d.ID != certManager || !d.ClusterWide || d.Title == "" {
		t.Fatalf("dependency = %+v, want cluster-wide %s with a title", d, certManager)
	}
	modes := []string{}
	for _, m := range d.Modes {
		modes = append(modes, m.ID)
	}
	if want := []string{modeBundled, modeCluster, modeExternal}; !slices.Equal(modes, want) {
		t.Fatalf("modes = %v, want %v", modes, want)
	}
	bundled, cluster, external := d.Modes[0], d.Modes[1], d.Modes[2]
	if bundled.Warning == "" || bundled.Doc == "" {
		t.Errorf("bundled carries warning %q and doc %q, want both (clusterWide)", bundled.Warning, bundled.Doc)
	}

	keys := func(i int) []string {
		var paths []string
		for _, k := range d.Modes[i].Keys {
			paths = append(paths, k.Path)
		}
		return paths
	}
	for i, want := range map[int][]string{
		0: {"cert-manager.enabled", "cert-manager.crds.enabled", "certManagerResources.type", keycloakIssuer},
		1: {"cert-manager.enabled", keycloakIssuer},
		2: {"cert-manager.enabled", keycloakTLS},
	} {
		for _, k := range want {
			if !slices.Contains(keys(i), k) {
				t.Errorf("mode %s keys %v lack %s", d.Modes[i].ID, keys(i), k)
			}
		}
	}
	if slices.Contains(keys(1), keycloakTLS) {
		t.Errorf("cluster keys %v hold %s, which only external writes", keys(1), keycloakTLS)
	}

	byID := map[string]int{}
	for i, q := range bundled.Questions {
		byID[q.ID] = i
	}
	acme := bundled.Questions[byID["acmeEmail"]]
	if acme.Type != string(model.TypeEmail) || acme.When == nil || acme.When.Question != "type" || acme.When.Equals != "prod" {
		t.Errorf("acmeEmail = %+v, want an email asked when type equals prod", acme)
	}
	if ns := bundled.Questions[byID["namespace"]]; !ns.ReadOnly || ns.Default == "" {
		t.Errorf("namespace = %+v, want read-only with a default", ns)
	}
	if len(cluster.Questions) != 1 || cluster.Questions[0].ID != "issuer" || cluster.Questions[0].Ingress != nil {
		t.Errorf("cluster questions = %+v, want only issuer", cluster.Questions)
	}

	var ingresses []string
	for _, q := range external.Questions {
		if q.Ingress == nil || q.ID != model.IngressQuestionID("secret", q.Ingress.ID) || q.Ingress.Label == "" {
			t.Fatalf("external question %+v is not an expanded per-ingress question", q)
		}
		ingresses = append(ingresses, q.Ingress.ID)
		switch q.Ingress.ID {
		case "keycloak":
			if !strings.Contains(q.Default, ".host.keycloak") || !strings.HasSuffix(q.Default, "-tls") {
				t.Errorf("keycloak secret default = %q, want the host foundation's <host>-tls template", q.Default)
			}
		case "apisix":
			if q.Default != "apisix-provider-tls" {
				t.Errorf("apisix secret default = %q, want the replaced const", q.Default)
			}
		}
	}
	b := latestBundle(t, mustBundles(t, ps.srv), load.DefaultChart)
	if len(ingresses) != len(b.Ingresses) {
		t.Errorf("external asks for ingresses %v, want one per row of %d", ingresses, len(b.Ingresses))
	}
}

// certManagerProfile is domainProfile with cert-manager in mode and answers.
func certManagerProfile(mode string, answers map[string]any) profile.Profile {
	p := domainProfile(renderDomain)
	p.Infrastructure = map[string]profile.InfraChoice{certManager: {Mode: mode, Answers: answers}}
	return p
}

func TestRenderInfrastructureModes(t *testing.T) {
	ps := newProfileSession(t)
	tests := []struct {
		name    string
		mode    string
		answers map[string]any
		want    map[string]any
		// absent are paths the mode must not write.
		absent []string
	}{
		{
			name: "no mode renders nothing", absent: []string{"cert-manager.enabled", keycloakIssuer},
			want: map[string]any{keycloakTLS: "keycloak." + renderDomain + "-tls"},
		},
		{
			name: modeBundled, mode: modeBundled,
			want: map[string]any{"cert-manager.enabled": true, "cert-manager.crds.enabled": true,
				"certManagerResources.enabled": true, "certManagerResources.type": "selfsigned",
				keycloakIssuer: "selfsigned-issuer"},
			absent: []string{"certManagerResources.acmeEmail", "certManagerResources.ingressClass"},
		},
		{
			name: modeCluster, mode: modeCluster, answers: map[string]any{"issuer": clusterIssuer},
			want: map[string]any{"cert-manager.enabled": false, "certManagerResources.enabled": false,
				keycloakIssuer: clusterIssuer, keycloakTLS: "keycloak." + renderDomain + "-tls"},
			absent: []string{"certManagerResources.type"},
		},
		{
			name: modeExternal, mode: modeExternal,
			answers: map[string]any{model.IngressQuestionID("secret", "keycloak"): keycloakSecret},
			want:    map[string]any{"cert-manager.enabled": false, keycloakTLS: keycloakSecret},
			absent:  []string{keycloakIssuer},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ps.mustRender(certManagerProfile(tc.mode, tc.answers), "")
			leaves := map[string]any{}
			for _, l := range got.Tree.Leaves {
				leaves[l.Path] = l.Value
			}
			for path, want := range tc.want {
				if v, ok := leaves[path]; !ok || v != want {
					t.Errorf("%s = %v (present %t), want %v", path, v, ok, want)
				}
			}
			for _, path := range tc.absent {
				if v, ok := leaves[path]; ok {
					t.Errorf("%s = %v, want it absent", path, v)
				}
			}
		})
	}
}

func TestSaveRefusesInvalidInfrastructure(t *testing.T) {
	ps := newProfileSession(t)
	tests := []struct {
		name      string
		choice    map[string]profile.InfraChoice
		wantField string
	}{
		{name: "unknown mode", choice: map[string]profile.InfraChoice{certManager: {Mode: "operator"}},
			wantField: model.InfrastructureFieldPrefix + certManager},
		{name: "unknown dependency", choice: map[string]profile.InfraChoice{"postgres": {Mode: modeBundled}},
			wantField: model.InfrastructureFieldPrefix + "postgres"},
		{name: "unknown question", choice: map[string]profile.InfraChoice{certManager: {Mode: modeCluster,
			Answers: map[string]any{"zone": "x"}}}, wantField: model.InfrastructureFieldPrefix + certManager + ".zone"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := profile.Profile{Target: completeTarget(), Infrastructure: tc.choice}
			path := t.TempDir() + "/" + profileName
			status, raw, _ := ps.save(saveRequest{Path: path, Profile: p})
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d: %s", status, http.StatusUnprocessableEntity, raw)
			}
			var body errorBody
			if err := json.Unmarshal([]byte(raw), &body); err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(body.Fields, func(e profile.FieldError) bool { return e.Field == tc.wantField }) {
				t.Errorf("fields = %+v, want one on %s", body.Fields, tc.wantField)
			}
		})
	}
}
