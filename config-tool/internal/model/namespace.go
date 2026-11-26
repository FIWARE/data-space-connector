package model

import (
	"strings"
	"text/template"
)

// Execute runs a bundle template over the namespace ns.
func Execute(t *template.Template, ns map[string]any) (string, error) {
	var b strings.Builder
	if err := t.Execute(&b, ns); err != nil {
		return "", err
	}
	return b.String(), nil
}

// SetNested sets the dotted answer id in the namespace templates read ({{ .domain.base }}).
func SetNested(ns map[string]any, id string, v any) {
	segs := strings.Split(id, ".")
	m := ns
	for _, s := range segs[:len(segs)-1] {
		next, ok := m[s].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[s] = next
		}
		m = next
	}
	m[segs[len(segs)-1]] = v
}
