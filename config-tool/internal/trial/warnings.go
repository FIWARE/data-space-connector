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
// Evidence holds at least one entry per release in Releases and none for other releases; a test enforces it.
type Warning struct {
	ID       string     `json:"id"`
	Releases []string   `json:"releases"`
	Message  string     `json:"message"`
	Evidence []Evidence `json:"evidence"`
	// Link points to guidance; empty when there is none.
	Link string `json:"link,omitempty"`
}

// Evidence proves that a warning applies to one release, so the warning cannot outlive the setting (ADR 0016 §4).
// With DefaultKey empty, Text occurs in the release's bundled values. Otherwise the bundled values leave the
// dotted DefaultKey unset and the chart's own values.yaml sets it to Text.
type Evidence struct {
	Release    string `json:"release"`
	Text       string `json:"text"`
	DefaultKey string `json:"defaultKey,omitempty"`
}

var allReleases = []string{releaseTrustAnchor, releaseProvider, releaseConsumer}
var connectors = []string{releaseProvider, releaseConsumer}

// same returns one evidence entry per release, all with the same text.
func same(releases []string, text string) []Evidence {
	out := make([]Evidence, len(releases))
	for i, r := range releases {
		out[i] = Evidence{Release: r, Text: text}
	}
	return out
}

// warnings is hard-coded until Epic 6 validates values (ADR 0016 §4).
var warnings = []Warning{
	{
		ID:       "self-signed-certificates",
		Releases: allReleases,
		Message: "TLS certificates come from cert-manager's selfsigned-issuer. Nobody outside this machine trusts them; " +
			"use an issuer of a public or organisational CA.",
		Evidence: same(allReleases, `cert-manager.io/cluster-issuer: "selfsigned-issuer"`),
	},
	{
		ID:       "chart-deployed-databases",
		Releases: allReleases,
		Message: "PostgreSQL (and the provider's MongoDB) run in the cluster through the operators, with default sizes " +
			"and no backups. Size them, or use managed databases.",
		Evidence: []Evidence{
			// The trust-anchor values set no database key; its PostgreSQL comes from the chart default.
			{Release: releaseTrustAnchor, Text: "true", DefaultKey: "managedPostgres.enabled"},
			{Release: releaseProvider, Text: "    managedPostgres:\n      enabled: true"},
			{Release: releaseProvider, Text: "managedMongo:\n  enabled: true"},
			{Release: releaseConsumer, Text: "    managedPostgres:\n      enabled: true"},
		},
	},
	{
		ID:       "local-host-names",
		Releases: allReleases,
		Message: "Host names under 127.0.0.1.nip.io only resolve on this machine. Replace them with public names " +
			"that also resolve inside the cluster.",
		Evidence: same(allReleases, ".127.0.0.1.nip.io"),
	},
	{
		ID:       "trust-all-certificates",
		Releases: connectors,
		Message:  "tlsTrustAll: true makes the status-list client trust every certificate. Remove it once a trusted CA issues them.",
		Evidence: same(connectors, "tlsTrustAll: true"),
	},
	{
		ID:       "example-identities",
		Releases: connectors,
		Message: "The participants use the example identities did:web:fancy-marketplace.biz and mp-operations.org. " +
			"Replace them with your organisation's identifiers.",
		Evidence: same(connectors, "did:web:fancy-marketplace.biz"),
	},
	{
		ID:       "demo-keystore-password",
		Releases: connectors,
		Message: "The keystore password is read from the keystore-password Secret, which the Quick Start creates with a " +
			"published value. Create your own secret.",
		Evidence: same(connectors, "name: keystore-password"),
	},
	{
		ID:       "local-proxy",
		Releases: connectors,
		Message: "Outbound traffic goes through the local squid proxy (squid-proxy.infra). Remove the proxy settings " +
			"or point them at your own proxy.",
		Evidence: same(connectors, "squid-proxy.infra"),
	},
	{
		ID:       "laptop-memory-limits",
		Releases: allReleases,
		Message: "Memory requests and limits are sized for a laptop, and most components set none. " +
			"Size them from measurements under load.",
		// The largest memory limit of each release.
		Evidence: []Evidence{
			{Release: releaseTrustAnchor, Text: "memory: 512Mi"},
			{Release: releaseProvider, Text: "memory: 1Gi"},
			{Release: releaseConsumer, Text: `memory: "256Mi"`},
		},
		Link: memoryLimitsLink,
	},
}

// Warnings returns a copy of the trial's production warnings, in display order.
func Warnings() []Warning {
	out := make([]Warning, len(warnings))
	for i, w := range warnings {
		w.Releases = slices.Clone(w.Releases)
		w.Evidence = slices.Clone(w.Evidence)
		out[i] = w
	}
	return out
}
