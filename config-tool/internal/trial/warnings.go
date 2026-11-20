package trial

import "slices"

// Release names the warnings refer to; they match the releases of the embedded descriptor.
const (
	releaseTrustAnchor = "trust-anchor"
	releaseProvider    = "provider"
	releaseConsumer    = "consumer"
)

// memoryLimitsLink is the sizing guidance the memory warning points to.
const memoryLimitsLink = "https://github.com/FIWARE/data-space-connector/blob/main/doc/deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components"

// Warning is a production warning of the trial. It is always shown and cannot be dismissed.
// Evidence occurs in the bundled values of every release in Releases; a test enforces it.
type Warning struct {
	ID       string   `json:"id"`
	Releases []string `json:"releases"`
	Message  string   `json:"message"`
	Evidence string   `json:"evidence"`
	// Link points to guidance; empty when there is none.
	Link string `json:"link,omitempty"`
}

var allReleases = []string{releaseTrustAnchor, releaseProvider, releaseConsumer}
var connectors = []string{releaseProvider, releaseConsumer}

// warnings is hard-coded until Epic 6 validates values (ADR 0016 §4).
var warnings = []Warning{
	{
		ID:       "self-signed-certificates",
		Releases: allReleases,
		Message: "TLS certificates come from cert-manager's selfsigned-issuer. Nobody outside this machine trusts them; " +
			"use an issuer of a public or organisational CA.",
		Evidence: `cert-manager.io/cluster-issuer: "selfsigned-issuer"`,
	},
	{
		ID:       "chart-deployed-databases",
		Releases: allReleases,
		Message: "PostgreSQL (and the provider's MongoDB) run in the cluster through the operators, with default sizes " +
			"and no backups. Size them, or use managed databases.",
		Evidence: "postgres-operator:",
	},
	{
		ID:       "local-host-names",
		Releases: allReleases,
		Message: "Host names under 127.0.0.1.nip.io only resolve on this machine. Replace them with public names " +
			"that also resolve inside the cluster.",
		Evidence: ".127.0.0.1.nip.io",
	},
	{
		ID:       "trust-all-certificates",
		Releases: connectors,
		Message:  "tlsTrustAll: true makes the status-list client trust every certificate. Remove it once a trusted CA issues them.",
		Evidence: "tlsTrustAll: true",
	},
	{
		ID:       "example-identities",
		Releases: connectors,
		Message: "The participants use the example identities did:web:fancy-marketplace.biz and mp-operations.org. " +
			"Replace them with your organisation's identifiers.",
		Evidence: "did:web:fancy-marketplace.biz",
	},
	{
		ID:       "demo-keystore-password",
		Releases: connectors,
		Message: "The keystore password is read from the keystore-password Secret, which the Quick Start creates with a " +
			"published value. Create your own secret.",
		Evidence: "name: keystore-password",
	},
	{
		ID:       "local-proxy",
		Releases: connectors,
		Message: "Outbound traffic goes through the local squid proxy (squid-proxy.infra). Remove the proxy settings " +
			"or point them at your own proxy.",
		Evidence: "squid-proxy.infra",
	},
	{
		ID:       "laptop-memory-limits",
		Releases: allReleases,
		Message: "Memory requests and limits are sized for a laptop, and most components set none. " +
			"Size them from measurements under load.",
		Evidence: "memory: ",
		Link:     memoryLimitsLink,
	},
}

// Warnings returns a copy of the trial's production warnings, in display order.
func Warnings() []Warning {
	out := make([]Warning, len(warnings))
	for i, w := range warnings {
		w.Releases = slices.Clone(w.Releases)
		out[i] = w
	}
	return out
}
