package load

import (
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
)

// elementsBundle is the render package's keyed-element fixture (ADR 0025).
const elementsBundle = "../render/testdata/elements.yaml"

func elementFixture(t *testing.T) model.Bundle {
	t.Helper()
	data, err := os.ReadFile(elementsBundle)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := model.Versions(fstest.MapFS{fixtureVersion + "/bundle.yaml": {Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	return bs[0]
}

func TestLoadElements(t *testing.T) {
	const (
		proxy      = "http://proxy:3128"
		notField   = "not a field of the HTTPS_PROXY element proxy.https writes"
		noValue    = "the element lacks value, which proxy.https writes"
		notCA      = "is not what ca.secret writes here"
		differsCA  = "differs from ca.secret = first"
		verifyPath = ".plugins.openid-connect.ssl_verify"
		volume     = "verifier:\n  volumes:\n    - {name: ca-bundle, secret: {secretName: s}}\n"
		inactiveCA = "ca.secret is not answered, so ca.secret does not write here"
	)
	tests := []struct {
		name  string
		src   string
		owned []string
		// unknown maps every unknown leaf to its note.
		unknown map[string]string
		answers map[string]any
	}{
		{
			name: "owned element among foreign ones",
			src: "keycloak:\n  extraEnvVars:\n    - {name: KC_HEAP_SIZE, value: 1g}\n    - {name: HTTPS_PROXY, value: '" + proxy + "'}\n" +
				"    - {name: NODE_TLS_REJECT_UNAUTHORIZED, value: '0'}\n",
			owned: []string{"keycloak.extraEnvVars[1].name", "keycloak.extraEnvVars[1].value"},
			unknown: map[string]string{"keycloak.extraEnvVars[0].name": "", "keycloak.extraEnvVars[0].value": "",
				"keycloak.extraEnvVars[2].name": "", "keycloak.extraEnvVars[2].value": ""},
			answers: map[string]any{"proxy.https": proxy},
		},
		{
			name: "first element in document order wins, the other is pinned",
			src: "verifier:\n  volumes:\n    - {name: ca-bundle, secret: {secretName: first}}\n" +
				"    - {name: extra-ca, secret: {secretName: second}}\n" +
				"  mounts:\n    - {name: extra-ca, mountPath: /etc/extra, subPath: x.crt}\n",
			owned: []string{"verifier.volumes[0].name", "verifier.volumes[0].secret.secretName", "verifier.volumes[1].name",
				"verifier.mounts[0].name", "verifier.mounts[0].mountPath", "verifier.mounts[0].subPath"},
			unknown: map[string]string{"verifier.volumes[1].secret.secretName": differsCA},
			answers: map[string]any{"ca.secret": "first", "ca.extra": "x.crt"},
		},
		{
			name: "a field the element does not write",
			src:  "keycloak:\n  extraEnvVars:\n    - name: HTTPS_PROXY\n      valueFrom: {secretKeyRef: {name: s, key: k}}\n",
			unknown: map[string]string{"keycloak.extraEnvVars[0].name": noValue,
				"keycloak.extraEnvVars[0].valueFrom.secretKeyRef.key":  notField,
				"keycloak.extraEnvVars[0].valueFrom.secretKeyRef.name": notField},
			answers: map[string]any{},
		},
		{
			name:    "a field the element writes is missing",
			src:     "keycloak:\n  extraEnvVars:\n    - {name: HTTPS_PROXY}\n",
			unknown: map[string]string{"keycloak.extraEnvVars[0].name": noValue},
			answers: map[string]any{},
		},
		{
			name: "a nested field the element writes is missing",
			src:  "verifier:\n  volumes:\n    - {name: ca-bundle, secret: {}}\n",
			unknown: map[string]string{"verifier.volumes[0].name": "the element lacks secret.secretName, which ca.secret writes",
				"verifier.volumes[0].secret": notCA},
			answers: map[string]any{},
		},
		{
			name: "a literal that differs",
			src:  volume + "  mounts:\n    - {name: ca-bundle, mountPath: /other, subPath: ca.crt}\n",
			owned: []string{"verifier.volumes[0].name", "verifier.volumes[0].secret.secretName",
				"verifier.mounts[0].name", "verifier.mounts[0].subPath"},
			unknown: map[string]string{"verifier.mounts[0].mountPath": notCA},
			answers: map[string]any{"ca.secret": "s"},
		},
		{
			name: "a template inside a literal field",
			src: "verifier:\n  volumes:\n    - {name: extra-ca, secret: {secretName: s}}\n" +
				"  mounts:\n    - {name: extra-ca, mountPath: /etc/extra, subPath: x.crt}\n",
			owned: []string{"verifier.volumes[0].name", "verifier.volumes[0].secret.secretName",
				"verifier.mounts[0].name", "verifier.mounts[0].mountPath", "verifier.mounts[0].subPath"},
			unknown: map[string]string{},
			answers: map[string]any{"ca.extra": "x.crt", "ca.secret": "s"},
		},
		{
			name: "a target whose requires is not answered",
			src:  "verifier:\n  mounts:\n    - {name: ca-bundle, mountPath: /etc/ssl/cert.pem, subPath: ca.crt}\n",
			unknown: map[string]string{"verifier.mounts[0].name": inactiveCA,
				"verifier.mounts[0].mountPath": "", "verifier.mounts[0].subPath": ""},
			answers: map[string]any{},
		},
		{
			name: "a mechanism target whose own answer derives but another target's does not",
			src:  "verifier:\n  mounts:\n    - {name: extra-ca, mountPath: /etc/extra, subPath: x.crt}\n",
			unknown: map[string]string{"verifier.mounts[0].name": "ca.secret is not answered, so ca.extra does not write here",
				"verifier.mounts[0].mountPath": "", "verifier.mounts[0].subPath": ""},
			answers: map[string]any{},
		},
		{
			name: "each leaf under every element with the parent",
			src: volume + "apisix:\n  routes:\n    - plugins:\n        openid-connect: {ssl_verify: true, discovery: d}\n" +
				"    - plugins:\n        opa: {}\n    - plugins:\n        openid-connect: {ssl_verify: false}\n",
			owned: []string{"verifier.volumes[0].name", "verifier.volumes[0].secret.secretName", "apisix.routes[0]" + verifyPath},
			unknown: map[string]string{"apisix.routes[0].plugins.openid-connect.discovery": "", "apisix.routes[1].plugins.opa": "",
				"apisix.routes[2]" + verifyPath: notCA},
			answers: map[string]any{"ca.secret": "s"},
		},
		{
			name:    "each leaf while its requires is not answered",
			src:     "apisix:\n  routes:\n    - plugins:\n        openid-connect: {ssl_verify: true}\n",
			unknown: map[string]string{"apisix.routes[0]" + verifyPath: inactiveCA},
			answers: map[string]any{},
		},
		{
			name:    "not a list",
			src:     "keycloak:\n  extraEnvVars: {name: HTTPS_PROXY, value: x}\n",
			unknown: map[string]string{"keycloak.extraEnvVars.name": "", "keycloak.extraEnvVars.value": ""},
			answers: map[string]any{},
		},
	}
	b := elementFixture(t)
	root := roundtrip.Path{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Load([]byte(tc.src), "values.yaml", []model.Bundle{b}, Options{Root: &root, Bundle: &b})
			if err != nil {
				t.Fatal(err)
			}
			var owned []string
			for _, e := range r.Report.Owned {
				owned = append(owned, e.Path)
			}
			if !reflect.DeepEqual(owned, tc.owned) {
				t.Errorf("owned = %v, want %v", owned, tc.owned)
			}
			unknown := map[string]string{}
			for _, e := range r.Report.Unknown {
				unknown[e.Path] = e.Note
			}
			if !reflect.DeepEqual(unknown, tc.unknown) {
				t.Errorf("unknown = %v, want %v", unknown, tc.unknown)
			}
			answers := map[string]any{}
			for _, a := range r.Report.Answers {
				answers[a.ID] = a.Value
				if a.From != FromOwned || a.Stage != string(model.StageFoundations) {
					t.Errorf("answer %s from %s in stage %s, want owned in foundations", a.ID, a.From, a.Stage)
				}
			}
			if !reflect.DeepEqual(answers, tc.answers) {
				t.Errorf("answers = %v, want %v", answers, tc.answers)
			}
		})
	}
}
