import { describe, expect, it } from 'vitest'
import { PUBLIC_NAMES_URL, QUICK_START_URL, RELEASE_NOTES_URL, releaseNotesUrl, repositoryDocUrl, SIZING_URL } from './links'
import { DEFAULT_CHART } from './state/answers'

describe('releaseNotesUrl', () => {
  it.each([
    { chart: DEFAULT_CHART, version: '10.16.0', want: expect.stringMatching(/\/doc\/release-notes\/10-x\.md$/) },
    { chart: DEFAULT_CHART, version: '', want: RELEASE_NOTES_URL },
    { chart: 'trust-anchor', version: '0.2.0', want: RELEASE_NOTES_URL },
    { chart: 'trust-anchor', version: '10.16.0', want: RELEASE_NOTES_URL },
  ])('$chart $version', ({ chart, version, want }) => {
    expect(releaseNotesUrl(chart, version)).toEqual(want)
  })
})

describe('trial links', () => {
  it.each([
    ['QUICK_START_URL', QUICK_START_URL, /\/blob\/main\/doc\/deployment-integration\/local-deployment\/LOCAL\.MD#quick-start$/],
    ['SIZING_URL', SIZING_URL, /\/blob\/main\/doc\/deployment-integration\/roles\/PREREQUISITES\.md#sizing$/],
    ['PUBLIC_NAMES_URL', PUBLIC_NAMES_URL, /\/PREREQUISITES\.md#public-names-have-to-resolve-inside-the-cluster$/],
  ])('%s points at the repository document', (_, url, path) => {
    expect(url).toMatch(/^https:\/\/github\.com\/FIWARE\/data-space-connector\//)
    expect(url).toMatch(path)
  })
})

describe('repositoryDocUrl', () => {
  it.each([
    ['doc/deployment-integration/roles/consumer/README.md', 'https://github.com/FIWARE/data-space-connector/blob/main/doc/deployment-integration/roles/consumer/README.md'],
    ['doc/deployment-integration/roles/README.md#component-matrix', 'https://github.com/FIWARE/data-space-connector/blob/main/doc/deployment-integration/roles/README.md#component-matrix'],
  ])('%s', (path, want) => {
    expect(repositoryDocUrl(path)).toBe(want)
  })
})
