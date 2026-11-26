package render

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

const (
	caSecretID      = "ca.privateSecret"
	privateCASecret = "private-ca"
	k3sGlob         = "../../../k3s/*.yaml"
	apisixSSL       = "decentralizedIam.odrlAuthorization.apisix.apisix.ssl"
	routeSSLVerify  = "decentralizedIam.odrlAuthorization.apisix.routes[0].plugins.openid-connect.ssl_verify"
	verifierDeploy  = "decentralizedIam.vcAuthentication.vcverifier.deployment"
	verifierMounts  = verifierDeploy + ".additionalVolumeMounts"
	keycloakMounts  = "keycloak.extraVolumeMounts"
	// bundleImage is the vcverifier chart's own caCert bundle image, which the add-private-ca init container reuses.
	bundleImage = "vcAuthentication.vcverifier.deployment.serverCertificates.caCert.bundleImage"
	// k3sVerifier is the verifier block of k3s/provider.yaml (L109-122): root-ca already sits at /etc/ssl/cert.pem.
	k3sVerifier = `decentralizedIam:
  vcAuthentication:
    vcverifier:
      deployment:
        additionalVolumes:
          - {name: signing-key, secret: {secretName: verifier.mp-operations.org-tls}}
          - {name: root-ca, emptyDir: {}}
        additionalVolumeMounts:
          - {name: signing-key, mountPath: /signing-key}
          - {name: root-ca, mountPath: /etc/ssl/cert.pem, subPath: cert.pem}
`
	// foreignTruststore puts a mount the tool does not own at the Keycloak truststore path.
	foreignTruststore = "keycloak:\n  extraVolumeMounts:\n    - {name: my-ca, mountPath: /opt/keycloak/conf/truststores/private-ca.pem}\n"
)

// caMechanisms are the per-component mechanisms, one per PREREQUISITES row besides APISIX.
var caMechanisms = []string{"keycloak-ca", "marketplace-ca", "vc-operator-ca", "verifier-ca"}

// secretVolume is the Secret volume every component mounts, with the item path it reads ca.crt under.
func secretVolume(path string) string {
	return fmt.Sprintf("{name: private-ca, secret: {secretName: %s, items: [{key: ca.crt, path: %s}]}}", privateCASecret, path)
}

// caWant are the expected values at each CA target, as YAML so that the types match the rendered tree.
var caWant = []struct{ path, want string }{
	{apisixSSL, "{existingCASecret: " + privateCASecret + ", certCAFilename: ca.crt, enabled: true}"},
	{routeSSLVerify, "true"},
	{verifierDeploy + ".additionalVolumes", "[" + secretVolume("ca.crt") + ", {name: private-ca-bundle, emptyDir: {}}]"},
	{verifierMounts, "[{name: private-ca-bundle, mountPath: /etc/ssl/cert.pem, subPath: cert.pem}]"},
	{verifierDeploy + ".initContainers", `[{name: add-private-ca, image: "alpine:3.23.3",
	  command: [sh, -c, cat /etc/ssl/cert.pem /private-ca/ca.crt > /bundle/cert.pem],
	  volumeMounts: [{name: private-ca, mountPath: /private-ca, readOnly: true}, {name: private-ca-bundle, mountPath: /bundle}],
	  securityContext: {runAsNonRoot: true, runAsUser: 65534, readOnlyRootFilesystem: true, allowPrivilegeEscalation: false,
	    capabilities: {drop: [ALL]}}}]`},
	{"marketplace.bizEcosystemLogicProxy.extraContainerVolumes", "[" + secretVolume("ca.crt") + "]"},
	{"marketplace.bizEcosystemLogicProxy.extraVolumeMounts", "[{name: private-ca, mountPath: /etc/ssl/private-ca, readOnly: true}]"},
	{logicProxyEnvVars + fmt.Sprintf("[%d]", logicProxyBaseSize), "{name: NODE_EXTRA_CA_CERTS, value: /etc/ssl/private-ca/ca.crt}"},
	{"vc-operator.extraVolumes", "[" + secretVolume("ca.crt") + "]"},
	{"vc-operator.extraVolumeMounts", "[{name: private-ca, mountPath: /etc/ssl/private-ca, readOnly: true}]"},
	{vcOperatorEnvVars, "[{name: SSL_CERT_DIR, value: /etc/ssl/private-ca}]"},
	{"keycloak.extraVolumes", "[" + secretVolume("private-ca.pem") + "]"},
	{keycloakMounts, `[{name: realms, mountPath: /opt/keycloak/data/import},
	  {name: private-ca, mountPath: /opt/keycloak/conf/truststores/private-ca.pem, subPath: private-ca.pem, readOnly: true}]`},
	{"keycloak.extraEnvVars", `[{name: KC_HEAP_SIZE, value: "1024m"},
	  {name: KC_TRUSTSTORE_PATHS, value: /opt/keycloak/conf/truststores/private-ca.pem}]`},
}

func parseYAML(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return v
}

// TestCARender checks that the single ca.privateSecret answer reaches every mechanism, and nothing while unanswered.
func TestCARender(t *testing.T) {
	set := renderShipped(t, map[string]any{caSecretID: privateCASecret}, oneRoute)
	unset := renderShipped(t, map[string]any{}, oneRoute)
	for _, c := range caWant {
		t.Run(c.path, func(t *testing.T) {
			if got, want := at(t, set, c.path), parseYAML(t, c.want); !reflect.DeepEqual(got, want) {
				t.Errorf("set: %s = %#v, want %#v", c.path, got, want)
			}
		})
	}
	for _, l := range unset.Leaves {
		if l.Provenance.Foundation == caSecretID {
			t.Errorf("unanswered: %s renders %#v", l.Path, l.Value)
		}
	}
	if len(set.Withheld) > 0 || len(unset.Withheld) > 0 {
		t.Errorf("withheld = %+v / %+v, want none", set.Withheld, unset.Withheld)
	}
}

// TestCATargetPaths checks that every CA target exists in the layer that supplies its default.
func TestCATargetPaths(t *testing.T) {
	umbrella, fixtures := parseValues(t, umbrellaValues), subchartFixtures(t)
	f, ok := providerBundle(t).Foundation(caSecretID)
	if !ok {
		t.Fatalf("no foundation %s", caSecretID)
	}
	for i, tg := range f.Targets {
		if _, _, ok := chartDefault(t, umbrella, fixtures, tg.Path); !ok {
			t.Errorf("targets[%d] %s: neither the umbrella nor a subchart fixture declares it", i, tg.Path)
		}
	}
}

// TestCABundleImage pins the add-private-ca image and its source path to the vcverifier chart's caCert defaults.
func TestCABundleImage(t *testing.T) {
	f, ok := subchartFixtures(t)["decentralizedIam"]
	if !ok {
		t.Fatal("no decentralized-iam fixture")
	}
	p, err := roundtrip.ParsePath(bundleImage)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := lookupValue(f.Values, p)
	img, _ := v.(map[string]any)
	tree := renderShipped(t, map[string]any{caSecretID: privateCASecret}, "")
	init, _ := at(t, tree, verifierDeploy+".initContainers[0]").(map[string]any)
	if want := fmt.Sprintf("%v:%v", img["repository"], img["tag"]); init["image"] != want {
		t.Errorf("add-private-ca image = %v, want the chart's %s", init["image"], want)
	}
	if mount := at(t, tree, verifierMounts+"[0].mountPath"); mount != img["bundlePath"] {
		t.Errorf("bundle mount = %v, want the chart's bundlePath %v", mount, img["bundlePath"])
	}
}

// TestCARouteVerify checks that the tool writes ssl_verify true only with a CA and pins a loaded false as a hand edit.
func TestCARouteVerify(t *testing.T) {
	route := func(verify string) string {
		return strings.Replace(oneRoute, "discovery:", "ssl_verify: "+verify+", discovery:", 1)
	}
	tests := []struct {
		name    string
		answers map[string]any
		src     string
		want    any
		owned   bool
	}{
		{"a loaded false stays and is pinned", map[string]any{caSecretID: privateCASecret}, route("false"), false, false},
		{"a loaded true is owned", map[string]any{caSecretID: privateCASecret}, route("true"), true, true},
		{"no CA writes nothing", map[string]any{}, oneRoute, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderShipped(t, tc.answers, tc.src)
			if got := at(t, tree, routeSSLVerify); got != tc.want {
				t.Errorf("ssl_verify = %#v, want %#v", got, tc.want)
			}
			for _, l := range tree.Leaves {
				if l.Path.String() != routeSSLVerify {
					continue
				}
				if owned := l.Provenance.Foundation == caSecretID && !l.Provenance.Pinned; owned != tc.owned {
					t.Errorf("ssl_verify provenance = %+v, want owned %v", l.Provenance, tc.owned)
				}
			}
		})
	}
}

// TestCACollision checks that a foreign element at a mechanism's unique mount path withholds that mechanism only.
func TestCACollision(t *testing.T) {
	tests := []struct {
		name, src, mechanism, list, mountPath, foreign string
	}{
		{"k3s root-ca at the verifier bundle path", k3sVerifier, "verifier-ca", verifierMounts, "/etc/ssl/cert.pem", "root-ca"},
		{"a foreign Keycloak truststore", foreignTruststore, "keycloak-ca", keycloakMounts,
			"/opt/keycloak/conf/truststores/private-ca.pem", "my-ca"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderShipped(t, map[string]any{caSecretID: privateCASecret}, tc.src)
			want := []Withheld{{Reason: WithheldCollision, Mechanism: tc.mechanism, Element: elementAt(tc.mechanism),
				List: tc.list, Field: "mountPath", Value: tc.mountPath, ForeignKey: tc.foreign}}
			if !reflect.DeepEqual(tree.Withheld, want) {
				t.Errorf("withheld = %+v, want %+v", tree.Withheld, want)
			}
			at := func(mounts []any) (n int, names []any) {
				for _, m := range mounts {
					if m := m.(map[string]any); m["mountPath"] == tc.mountPath {
						n++
						names = append(names, m["name"])
					}
				}
				return n, names
			}
			mounts, ok := finalList(t, tree, tc.src, mustPath(t, tc.list))
			if !ok {
				t.Fatalf("%s is neither rendered nor loaded", tc.list)
			}
			if n, names := at(mounts); n != 1 || names[0] != tc.foreign {
				t.Errorf("%s at %s = %v, want only %s", tc.list, tc.mountPath, names, tc.foreign)
			}
			assertOnlyWithheld(t, tree, tc.mechanism)
		})
	}
}

// TestCAConflict checks that a loaded chart setting the mechanisms table names withholds verifier-ca only.
func TestCAConflict(t *testing.T) {
	const (
		caCert    = "decentralizedIam.vcAuthentication.vcverifier.deployment.serverCertificates.caCert.enabled"
		clientKey = "decentralizedIam.vcAuthentication.vcverifier.deployment.serverCertificates.clientKey.secretName"
		certPath  = "decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.clientIdentification.certificatePath"
	)
	verifier := func(serverCertificates, clientIdentification string) string {
		return fmt.Sprintf(`decentralizedIam:
  vcAuthentication:
    vcverifier:
      deployment:
        serverCertificates: %s
        verifier: {clientIdentification: %s}
`, serverCertificates, clientIdentification)
	}
	tests := []struct {
		name, src string
		conflict  []string
	}{
		{"the chart's own caCert", verifier("{caCert: {enabled: true, secretName: my-ca}}", "{}"), []string{caCert}},
		{"the chart's mount with a client certificate",
			verifier("{clientKey: {secretName: client-tls}}", "{certificatePath: /certs/tls.crt}"), []string{clientKey, certPath}},
		{"caCert off", verifier("{caCert: {enabled: false}}", "{}"), nil},
		{"a client key without a certificate path", verifier("{clientKey: {secretName: client-tls}}", "{}"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tree := renderShipped(t, map[string]any{caSecretID: privateCASecret}, tc.src)
			var want []Withheld
			mechanism := ""
			if tc.conflict != nil {
				mechanism = "verifier-ca"
				want = []Withheld{{Reason: WithheldConflict, Mechanism: mechanism, Conflict: tc.conflict}}
			}
			if !reflect.DeepEqual(tree.Withheld, want) {
				t.Errorf("withheld = %+v, want %+v", tree.Withheld, want)
			}
			assertOnlyWithheld(t, tree, mechanism)
		})
	}
}

// assertOnlyWithheld checks that withheld, if any, renders no CA leaf in any list it writes and the others render.
func assertOnlyWithheld(t *testing.T, tree Tree, withheld string) {
	t.Helper()
	for _, m := range caMechanisms {
		rendered := slices.ContainsFunc(tree.Leaves, func(l Leaf) bool {
			return l.Provenance.Foundation == caSecretID && strings.HasPrefix(l.Path.String(), mechanismComponent[m]+".")
		})
		if rendered == (m == withheld) {
			t.Errorf("%s rendered = %v, want %v", m, rendered, m != withheld)
		}
	}
}

// finalList is the list at p after the render: the tree's when the render writes it, otherwise the loaded one.
func finalList(t *testing.T, tree Tree, src string, p roundtrip.Path) ([]any, bool) {
	t.Helper()
	if v, ok := subtree(tree.Nested(), p).([]any); ok {
		return v, true
	}
	v, _ := lookupValue(parseYAML(t, src), p)
	l, ok := v.([]any)
	return l, ok
}

// mechanismComponent maps each mechanism to the path prefix of the component it reaches, which covers every list
// the mechanism writes.
var mechanismComponent = map[string]string{
	"verifier-ca":    "decentralizedIam.vcAuthentication.vcverifier",
	"marketplace-ca": "marketplace.bizEcosystemLogicProxy",
	"vc-operator-ca": "vc-operator",
	"keycloak-ca":    "keycloak",
}

// elementAt names the element of a mechanism whose mount collides; both colliding mounts are named so.
func elementAt(mechanism string) string {
	if mechanism == "verifier-ca" {
		return "private-ca-bundle"
	}
	return "private-ca"
}

// TestCAMountsUnique loads every k3s file with the CA set and asserts that each final mount list stays valid.
func TestCAMountsUnique(t *testing.T) {
	files, err := filepath.Glob(k3sGlob)
	if err != nil || len(files) == 0 {
		t.Fatalf("no k3s files: %v", err)
	}
	lists := providerBundle(t).ListTable()
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			// A list the render does not write is the loaded one, untouched; some k3s files repeat keys, so only the
			// written lists are checked.
			nested := renderShipped(t, map[string]any{caSecretID: privateCASecret}, string(src)).Nested()
			for _, l := range lists {
				elems, _ := subtree(nested, l.Path).([]any)
				for _, u := range l.Unique {
					seen := map[any]bool{}
					for _, e := range elems {
						v := e.(map[string]any)[u]
						if seen[v] {
							t.Errorf("%s: %s %v appears twice", l.Path, u, v)
						}
						seen[v] = true
					}
				}
			}
		})
	}
}
