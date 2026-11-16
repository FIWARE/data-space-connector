/** External documentation links shown on S0 and S1. */

const REPOSITORY = 'https://github.com/FIWARE/data-space-connector'

/** The connector's documentation. */
export const DOCUMENTATION_URL = `${REPOSITORY}#readme`
/** The index of all release notes. */
export const RELEASE_NOTES_URL = `${REPOSITORY}/tree/main/doc/release-notes`

/** Release notes of a chart version; they are kept per major version as `<major>-x.md`. */
export function releaseNotesUrl(version: string): string {
  const major = version.split('.')[0]
  return major ? `${REPOSITORY}/blob/main/doc/release-notes/${major}-x.md` : RELEASE_NOTES_URL
}
