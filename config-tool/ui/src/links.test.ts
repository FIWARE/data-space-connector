import { describe, expect, it } from 'vitest'
import { RELEASE_NOTES_URL, releaseNotesUrl } from './links'
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
