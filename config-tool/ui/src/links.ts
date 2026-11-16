/** External documentation links shown on S0 and S1. */
import { DEFAULT_CHART } from './state/answers'

const REPOSITORY = 'https://github.com/FIWARE/data-space-connector'

/** The connector's documentation. */
export const DOCUMENTATION_URL = `${REPOSITORY}#readme`
/** The index of all release notes. */
export const RELEASE_NOTES_URL = `${REPOSITORY}/tree/main/doc/release-notes`

/** Release notes of a chart version; only the connector keeps them per major as `<major>-x.md`. */
export function releaseNotesUrl(chart: string, version: string): string {
  const major = version.split('.')[0]
  return chart === DEFAULT_CHART && major ? `${REPOSITORY}/blob/main/doc/release-notes/${major}-x.md` : RELEASE_NOTES_URL
}
