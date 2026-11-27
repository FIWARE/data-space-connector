package model

import (
	"reflect"
	"testing"
	"text/template"
)

func TestInvert(t *testing.T) {
	tests := []struct {
		name string
		tpls []string
		sep  string
		s    string
		want map[string]string
		ok   bool
	}{
		{name: "one answer", tpls: []string{"{{ .proxy.https }}"}, s: "http://p:3128", want: map[string]string{"proxy.https": "http://p:3128"}, ok: true},
		{name: "text around an answer", tpls: []string{"{{ .a }}.svc"}, s: "dsc.svc", want: map[string]string{"a": "dsc"}, ok: true},
		{name: "text differs", tpls: []string{"{{ .a }}.svc"}, s: "dsc.local", ok: false},
		{name: "regexp metacharacters are text", tpls: []string{"{{ .a }}(x)"}, s: "v(x)", want: map[string]string{"a": "v"}, ok: true},
		{name: "one answer twice, agreeing", tpls: []string{"x{{ .a }}-{{ .a }}"}, s: "x1-1", want: map[string]string{"a": "1"}, ok: true},
		{name: "one answer twice, disagreeing", tpls: []string{"x{{ .a }}-{{ .a }}"}, s: "x1-2", ok: false},
		{name: "fullname answers nothing", tpls: []string{`{{ fullname .target.release "keycloak" }},{{ .a }}`}, s: "rel-keycloak,v",
			want: map[string]string{"a": "v"}, ok: true},
		{name: "join", tpls: []string{"{{ .a }}", "{{ .b }}"}, sep: ",", s: "1,2", want: map[string]string{"a": "1", "b": "2"}, ok: true},
		{name: "join without its separator", tpls: []string{"{{ .a }}", "{{ .b }}"}, sep: ",", s: "1;2", ok: false},
		{name: "literal", tpls: []string{"ca.crt"}, s: "ca.crt", want: map[string]string{}, ok: true},
		{name: "literal differs", tpls: []string{"ca.crt"}, s: "tls.crt", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tpls := make([]*template.Template, len(tc.tpls))
			for i, s := range tc.tpls {
				tpls[i] = template.Must(template.New("").Funcs(template.FuncMap{"fullname": Fullname}).Parse(s))
			}
			got, ok := Invert(tpls, tc.sep, tc.s)
			if ok != tc.ok || ok && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Invert(%q) = %v, %v; want %v, %v", tc.s, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestElementLeaves(t *testing.T) {
	tpl := template.Must(template.New("").Parse("{{ .a }}"))
	tests := []struct {
		name  string
		value map[string]any
		want  []string
	}{
		{name: "flat", value: map[string]any{"value": tpl, "mountPath": "/x"}, want: []string{"mountPath", "value"}},
		{name: "nested", value: map[string]any{"secret": map[string]any{"secretName": tpl}}, want: []string{"secret.secretName"}},
		{name: "empty collections", value: map[string]any{"emptyDir": map[string]any{}, "args": []any{}}, want: []string{"args", "emptyDir"}},
		{name: "list items", value: map[string]any{"args": []any{"-c", tpl}}, want: []string{"args[0]", "args[1]"}},
		{name: "key only", value: map[string]any{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, p := range (CompiledElement{KeyField: "name", Key: "k", value: tc.value}).Leaves() {
				got = append(got, p.String())
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Leaves() = %v, want %v", got, tc.want)
			}
		})
	}
}
