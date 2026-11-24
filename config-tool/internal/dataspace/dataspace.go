// Package dataspace reads a data space's descriptor into a preview of the dataspace.* answers and gives later
// stories typed access to those answers (ADR 0022). It never touches the network; internal/server fetches.
package dataspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
)

// MaxDescriptorBytes caps a descriptor; a real one is a few hundred bytes.
const MaxDescriptorBytes = 64 << 10

// Answer ids of the dataspace foundations in the bundle.
const (
	TIRAddressID       = "dataspace.tirAddress"
	CredentialTypesID  = "dataspace.credentialTypes"
	AutoRegistrationID = "dataspace.autoRegistration"
)

// Descriptor keys the tool imports (ADR 0022, Decision 4).
const (
	KeyTIRAddress              = "tir_address"
	KeyCredentialTypes         = "credential_types"
	KeyParticipantRegistration = "participant_registration"
)

// Descriptor keys the chart's dsconfig-cm.yaml publishes; shown, never applied.
const (
	keySupportedModels         = "supported_models"
	keySupportedProtocols      = "supported_protocols"
	keyAuthenticationProtocols = "authentication_protocols"
)

// Values of participant_registration.
const (
	registrationAutomatic = "automatic"
	registrationManual    = "manual"
)

// NothingToImport is the message of a preview whose descriptor carries none of the imported keys. Today every
// publisher, the chart included, omits them, so it is information, not a failure.
const NothingToImport = "This data space does not publish a TIR address, credential types or a registration mode yet. Enter them below."

// importSpec ties an imported descriptor key to its answer.
type importSpec struct{ key, id string }

// imports lists the imported keys in the order a preview lists them.
var imports = []importSpec{
	{KeyTIRAddress, TIRAddressID},
	{KeyCredentialTypes, CredentialTypesID},
	{KeyParticipantRegistration, AutoRegistrationID},
}

var chartKeys = []string{keySupportedModels, keySupportedProtocols, keyAuthenticationProtocols}

// Descriptor is a parsed data-space configuration document.
type Descriptor struct {
	// Imported holds the raw values of the imported keys the document carries, by descriptor key.
	Imported map[string]any
	// Known holds the chart's own keys the document carries; shown to the user, never applied.
	Known map[string]any
	// Unused lists every other top-level key, sorted.
	Unused []string
}

// CredentialType is one element of the dataspace.credentialTypes answer.
type CredentialType struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Change is an imported value that differs from the profile's given answer.
type Change struct {
	ID string `json:"id"`
	// Current is the given answer; nil when unanswered.
	Current  any `json:"current,omitempty"`
	Imported any `json:"imported"`
}

// Problem is an imported value the answer's type refuses; it is not offered as a change.
type Problem struct {
	ID string `json:"id"`
	// Key is the descriptor key the value came from.
	Key     string `json:"key"`
	Message string `json:"message"`
}

// PreviewResult is what applying a descriptor would change; nothing is applied until the user says so.
type PreviewResult struct {
	Changes  []Change       `json:"changes"`
	Problems []Problem      `json:"problems"`
	Known    map[string]any `json:"known,omitempty"`
	Unused   []string       `json:"unused"`
	// Message is NothingToImport when the descriptor carries none of the imported keys; empty otherwise.
	Message string `json:"message,omitempty"`
}

// ParseDescriptor reads a descriptor. Its errors never echo the document.
func ParseDescriptor(data []byte) (Descriptor, error) {
	if len(data) > MaxDescriptorBytes {
		return Descriptor{}, fmt.Errorf("the descriptor is larger than %d bytes", MaxDescriptorBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Descriptor{}, errors.New("the descriptor is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var v any
	if err := dec.Decode(&v); err != nil {
		return Descriptor{}, errors.New("the descriptor is not valid JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return Descriptor{}, errors.New("the descriptor is not valid JSON: it continues after its first value")
	}
	doc, ok := v.(map[string]any)
	if !ok {
		return Descriptor{}, fmt.Errorf("the descriptor must be a JSON object, not %s", jsonKind(v))
	}
	d := Descriptor{Imported: map[string]any{}, Known: map[string]any{}, Unused: []string{}}
	for _, k := range slices.Sorted(maps.Keys(doc)) {
		switch {
		case slices.ContainsFunc(imports, func(i importSpec) bool { return i.key == k }):
			d.Imported[k] = doc[k]
		case slices.Contains(chartKeys, k):
			d.Known[k] = doc[k]
		default:
			d.Unused = append(d.Unused, k)
		}
	}
	return d, nil
}

// Preview compares d's imported values with p's given answers. A key whose answer b does not declare is a problem.
func Preview(b model.Bundle, p profile.Profile, d Descriptor) PreviewResult {
	r := PreviewResult{Changes: []Change{}, Problems: []Problem{}, Unused: d.Unused}
	if len(d.Known) > 0 {
		r.Known = d.Known
	}
	if r.Unused == nil {
		r.Unused = []string{}
	}
	if len(d.Imported) == 0 {
		r.Message = NothingToImport
		return r
	}
	for _, i := range imports {
		raw, ok := d.Imported[i.key]
		if !ok {
			continue
		}
		f, ok := b.Foundation(i.id)
		if !ok {
			r.Problems = append(r.Problems, Problem{ID: i.id, Key: i.key, Message: fmt.Sprintf("is not asked for chart %s %s", b.Chart, b.Version)})
			continue
		}
		v, err := answerOf(i.key, raw)
		if err == nil {
			err = f.Check(v)
		}
		if err != nil {
			r.Problems = append(r.Problems, Problem{ID: i.id, Key: i.key, Message: err.Error()})
			continue
		}
		cur, answered := p.Answers[i.id]
		if answered && sameJSON(cur, v) {
			continue
		}
		r.Changes = append(r.Changes, Change{ID: i.id, Current: cur, Imported: v})
	}
	return r
}

// CredentialTypes returns the dataspace.credentialTypes answer in the user's order; nil when b does not declare
// it or p leaves it unanswered or invalid.
func CredentialTypes(b model.Bundle, p profile.Profile) []CredentialType {
	v, ok := validAnswer(b, p, CredentialTypesID)
	if !ok {
		return nil
	}
	list := v.([]any)
	out := make([]CredentialType, len(list))
	for i, e := range list {
		m := e.(map[string]any)
		desc, _ := m[model.CredentialDescriptionKey].(string)
		out[i] = CredentialType{Type: m[model.CredentialTypeKey].(string), Description: desc}
	}
	return out
}

// AutoRegistration returns the dataspace.autoRegistration answer; false as second value when b does not declare
// it or p leaves it unanswered or invalid.
func AutoRegistration(b model.Bundle, p profile.Profile) (bool, bool) {
	v, ok := validAnswer(b, p, AutoRegistrationID)
	if !ok {
		return false, false
	}
	return v.(bool), true
}

// validAnswer returns p's given answer id when b declares it and its type accepts it, so callers may assert.
func validAnswer(b model.Bundle, p profile.Profile, id string) (any, bool) {
	f, ok := b.Foundation(id)
	if !ok {
		return nil, false
	}
	v, ok := p.Answers[id]
	if !ok || f.Check(v) != nil {
		return nil, false
	}
	return v, true
}

// answerOf turns a descriptor value into the answer shape its foundation checks; the check reports the rest.
func answerOf(key string, raw any) (any, error) {
	switch key {
	case KeyCredentialTypes:
		return credentialTypesOf(raw)
	case KeyParticipantRegistration:
		switch raw {
		case registrationAutomatic:
			return true, nil
		case registrationManual:
			return false, nil
		}
		return nil, fmt.Errorf("must be %q or %q", registrationAutomatic, registrationManual)
	}
	return raw, nil
}

// credentialTypesOf turns a string element into {type}; an object stays as it is for the type's check.
func credentialTypesOf(raw any) (any, error) {
	list, ok := raw.([]any)
	if !ok {
		return raw, nil
	}
	out := make([]any, len(list))
	for i, e := range list {
		switch e := e.(type) {
		case string:
			out[i] = map[string]any{model.CredentialTypeKey: e}
		case map[string]any:
			out[i] = e
		default:
			return nil, fmt.Errorf("[%d] must be a type name or an object with %s and an optional %s", i, model.CredentialTypeKey, model.CredentialDescriptionKey)
		}
	}
	return out, nil
}

// sameJSON compares answers by their JSON form, so a []map[string]any equals the []any a decoder produced.
func sameJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}

func jsonKind(v any) string {
	switch v.(type) {
	case []any:
		return "an array"
	case string:
		return "a string"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	case nil:
		return "null"
	}
	return "another value"
}
