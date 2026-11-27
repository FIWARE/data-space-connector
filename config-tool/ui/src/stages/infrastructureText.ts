/** The Infrastructure stage's copy (ADR 0026); every dependency, mode and question text is bundle data. */
export const INFRASTRUCTURE_HEADING = 'Infrastructure — what is already there, what should be installed'
export const DEVELOPMENT_BADGE = 'suitable for development'
export const PRODUCTION_LINK = 'For production ↗'
export const WARNING_LINK = 'How to install it once ↗'
export const WARNING_PREFIX = '⚠'
export const DERIVED_BADGE = 'derived'
export const DERIVED_PENDING = 'Derived once the answers it comes from are given.'
export const SECRETS_NOTE = 'The tool does not create these Secrets. They are listed in NEXT_STEPS.md.'
export const NO_DEPENDENCIES = 'This chart version has nothing to integrate with.'

/** The "writes" disclosure's summary, e.g. "writes 3 keys". */
export function writesText(count: number): string {
  return `writes ${count} key${count === 1 ? '' : 's'}`
}

/** The accessible name of a mode's key list. */
export function writesListLabel(mode: string): string {
  return `Keys ${mode} writes`
}

/** The link to a dependency's doc, e.g. "About TLS certificates (cert-manager) ↗". */
export function aboutLink(title: string): string {
  return `About ${title} ↗`
}

/** Names the ingresses whose rows wait for their host: an answer renders nothing until Stage 3 resolves it. */
export function pendingIngressesText(labels: readonly string[]): string {
  return `Asked once the host is known (Stage 3): ${labels.join(', ')}.`
}
