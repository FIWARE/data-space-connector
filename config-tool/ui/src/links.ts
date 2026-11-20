/** External documentation links shown on S0, S1, S2 and Review. */
import { DEFAULT_CHART } from './state/answers'

const REPOSITORY = 'https://github.com/FIWARE/data-space-connector'

/** The connector's documentation. */
export const DOCUMENTATION_URL = `${REPOSITORY}#readme`
/** The Quick Start the local trial is equivalent to; it applies everything the trial leaves out. */
export const QUICK_START_URL = `${REPOSITORY}/blob/main/doc/deployment-integration/local-deployment/LOCAL.MD#quick-start`
/** Memory and disk each role needs; the UI links it and never restates a figure. */
export const SIZING_URL = `${REPOSITORY}/blob/main/doc/deployment-integration/roles/PREREQUISITES.md#sizing`
/** The index of all release notes. */
export const RELEASE_NOTES_URL = `${REPOSITORY}/tree/main/doc/release-notes`

/** Release notes of a chart version; only the connector keeps them per major as `<major>-x.md`. */
export function releaseNotesUrl(chart: string, version: string): string {
  const major = version.split('.')[0]
  return chart === DEFAULT_CHART && major ? `${REPOSITORY}/blob/main/doc/release-notes/${major}-x.md` : RELEASE_NOTES_URL
}

/** A repository document named by its repo-relative path, as the model bundle's `docs` fields give it. */
export function repositoryDocUrl(path: string): string {
  return `${REPOSITORY}/blob/main/${path}`
}
