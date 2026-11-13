// Package profile defines the answer set: everything the user decided, which render turns into values.
package profile

import (
	"fmt"
	"regexp"
)

// Profile is the answer set stored in a .dscprofile.yaml.
type Profile struct {
	Target Target `json:"target" yaml:"target"`
}

// TargetType says where the generated values go.
type TargetType string

// Target types offered on the Target stage.
const (
	TargetFile    TargetType = "file"
	TargetCluster TargetType = "cluster"
)

// Environment selects development or production defaults.
type Environment string

// Environments offered on the Target stage.
const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
)

// Length limits of the names a Target carries.
const (
	// MaxNamespaceLength is the DNS-1123 label limit Kubernetes enforces on namespaces.
	MaxNamespaceLength = 63
	// MaxReleaseNameLength is Helm's limit, shorter than a label so derived names still fit.
	MaxReleaseNameLength = 53
)

// Field names reported in FieldError, matching the JSON keys of Target.
const (
	FieldType         = "type"
	FieldContext      = "context"
	FieldNamespace    = "namespace"
	FieldReleaseName  = "releaseName"
	FieldChart        = "chart"
	FieldChartVersion = "chartVersion"
	FieldEnvironment  = "environment"
)

// msgRequired marks an unanswered field, which the UI counts as an open question rather than an error.
const msgRequired = "is required"

// Release names prefix Service names, which must be DNS-1035 labels (leading letter);
// namespaces only need DNS-1123 labels.
var (
	dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	dns1035Label = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
)

const (
	msgDNS1123 = "must consist of lowercase letters, digits and '-', and start and end with a letter or digit"
	msgDNS1035 = "must consist of lowercase letters, digits and '-', start with a letter and end with a letter or digit"
)

// Target is where and how the connector is deployed.
// It holds only the kube context name; the kubeconfig path is session state and never stored.
type Target struct {
	Type            TargetType  `json:"type" yaml:"type"`
	Context         string      `json:"context,omitempty" yaml:"context,omitempty"`
	Namespace       string      `json:"namespace" yaml:"namespace"`
	CreateNamespace bool        `json:"createNamespace" yaml:"createNamespace"`
	ReleaseName     string      `json:"releaseName" yaml:"releaseName"`
	Chart           string      `json:"chart" yaml:"chart"`
	ChartVersion    string      `json:"chartVersion" yaml:"chartVersion"`
	Environment     Environment `json:"environment" yaml:"environment"`
}

// FieldError is a validation failure of a single Target field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements error.
func (e FieldError) Error() string {
	return e.Field + ": " + e.Message
}

// Validate returns every field error of t; an empty result means t is valid.
func (t Target) Validate() []FieldError {
	var errs []FieldError
	switch t.Type {
	case "":
		errs = append(errs, FieldError{FieldType, msgRequired})
	case TargetFile:
	case TargetCluster:
		if t.Context == "" {
			errs = append(errs, FieldError{FieldContext, msgRequired})
		}
	default:
		errs = append(errs, FieldError{FieldType, fmt.Sprintf("must be %q or %q", TargetFile, TargetCluster)})
	}
	if msg := checkLabel(t.Namespace, MaxNamespaceLength, dns1123Label, msgDNS1123); msg != "" {
		errs = append(errs, FieldError{FieldNamespace, msg})
	}
	if msg := checkLabel(t.ReleaseName, MaxReleaseNameLength, dns1035Label, msgDNS1035); msg != "" {
		errs = append(errs, FieldError{FieldReleaseName, msg})
	}
	// Semver and known-chart checks need the model bundle (step 4); presence is enough here.
	if t.Chart == "" {
		errs = append(errs, FieldError{FieldChart, msgRequired})
	}
	if t.ChartVersion == "" {
		errs = append(errs, FieldError{FieldChartVersion, msgRequired})
	}
	switch t.Environment {
	case EnvDevelopment, EnvProduction:
	case "":
		errs = append(errs, FieldError{FieldEnvironment, msgRequired})
	default:
		errs = append(errs, FieldError{FieldEnvironment, fmt.Sprintf("must be %q or %q", EnvDevelopment, EnvProduction)})
	}
	return errs
}

// checkLabel returns why name is not a label matching pattern of at most maxLen characters, or "".
func checkLabel(name string, maxLen int, pattern *regexp.Regexp, patternMsg string) string {
	switch {
	case name == "":
		return msgRequired
	case len(name) > maxLen:
		return fmt.Sprintf("must be at most %d characters", maxLen)
	case !pattern.MatchString(name):
		return patternMsg
	}
	return ""
}
