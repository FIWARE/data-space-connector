import { describe, expect, it } from 'vitest'
import { emptyAnswers, TargetType } from '../state/answers'
import { initialState, reducer, type AppState } from '../state/reducer'
import { mockApi, REPORT, type SaveBody } from '../test/api'
import { defaultProfilePath, fromProfile, profileName, saveProfile, sinceSaved, toProfile } from './profile'

const PATH = '/home/me/provider.dscprofile.yaml'
const NOW = Date.parse('2026-11-19T12:00:00Z')

const ENTERED: AppState = {
  ...reducer(initialState, { type: 'newConfiguration' }),
  answers: { target: { ...emptyAnswers().target, type: TargetType.Cluster, namespace: 'Prod_NS', releaseName: 'provider' } },
  open: ['target.context', 'target.namespace'],
}

async function sentBody(state: AppState): Promise<SaveBody> {
  const fetchMock = mockApi()
  await saveProfile(state, PATH, false)
  const init = fetchMock.mock.calls.find(([input]) => String(input).endsWith('/api/profile'))?.[1]
  return JSON.parse(String(init?.body)) as SaveBody
}

describe('api/profile', () => {
  it('round-trips target and open markers through the profile shape', () => {
    const profile = toProfile(ENTERED)
    expect(profile).toMatchObject({ answers: {}, secrets: {}, open: [{ field: 'target.context' }, { field: 'target.namespace' }] })
    expect(fromProfile(profile)).toEqual({ answers: ENTERED.answers, open: ENTERED.open })
  })

  it('drops open ids the UI does not know and fills target keys the profile lacks', () => {
    const profile = { ...toProfile(ENTERED), target: { namespace: 'x' } as AppState['answers']['target'], open: [{ field: 'nope' }, { field: 'target.namespace' }] }
    expect(fromProfile(profile)).toEqual({ answers: { target: { ...emptyAnswers().target, namespace: 'x' } }, open: ['target.namespace'] })
  })

  it('sends the held report as the source to reference', async () => {
    const loaded = reducer(initialState, { type: 'fileLoaded', report: REPORT, file: new File([''], REPORT.source) })
    expect((await sentBody(loaded)).sourceSha256).toBe(REPORT.sha256)
  })

  it('references no source after New configuration', async () => {
    const loaded = reducer(initialState, { type: 'fileLoaded', report: REPORT, file: new File([''], REPORT.source) })
    expect(await sentBody(reducer(loaded, { type: 'newConfiguration' }))).not.toHaveProperty('sourceSha256')
  })

  it.each([
    ['/home/me', 'provider', '/home/me/provider.dscprofile.yaml'],
    ['/home/me/', '', '/home/me/connector.dscprofile.yaml'],
    ['C:\\Users\\me', 'p', 'C:\\Users\\me\\p.dscprofile.yaml'],
    ['', 'p', 'p.dscprofile.yaml'],
  ])('puts a new profile in %j for release %j at %s', (dir, release, expected) => {
    expect(defaultProfilePath(dir, release)).toBe(expected)
  })

  it.each([
    [PATH, 'provider'],
    ['C:\\dsc\\consumer.dscprofile.yaml', 'consumer'],
  ])('names %s %s', (path, name) => {
    expect(profileName(path)).toBe(name)
  })

  it.each([
    ['2026-11-19T11:55:00Z', '5 minutes ago'],
    ['2026-11-19T09:00:00Z', '3 hours ago'],
    ['2026-11-18T12:00:00Z', 'yesterday'],
    ['2026-09-19T12:00:00Z', '2 months ago'],
    ['2020-01-01T00:00:00Z', '7 years ago'],
    ['not a time', 'not a time'],
  ])('says %s was saved %s', (savedAt, expected) => {
    expect(sinceSaved(savedAt, NOW)).toBe(expected)
  })
})
