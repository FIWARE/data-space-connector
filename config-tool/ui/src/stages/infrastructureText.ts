/** The Infrastructure stage's copy (ADR 0026); every dependency, mode and question text is bundle data. */
export const INFRASTRUCTURE_HEADING = 'Infrastructure — what is already there, what should be installed'
export const DEVELOPMENT_BADGE = 'suitable for development'
export const PRODUCTION_LINK = 'For production ↗'
export const WARNING_LINK = 'How to install it once ↗'
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
