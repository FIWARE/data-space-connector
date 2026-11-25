import { describe, expect, it } from 'vitest'
import { initialState } from '../state/reducer'
import { mockApi, refusedFoundations, REPORT, type RenderBody } from '../test/api'
import type { AnswerValue } from '../state/answers'
import { toProfile } from './profile'
import { fetchRender, type AnswerDiff } from './render'

const HTTP_UNPROCESSABLE = 422
const BASE = 'example.org'
const OTHER_BASE = 'example.com'
const PINNED_HOST = 'host.keycloak'
const withAnswers = (values: Record<string, AnswerValue>) => toProfile({ ...initialState, answers: { ...initialState.answers, values } })
const PROFILE = withAnswers({ 'domain.base': BASE })
const NO_CHANGE: AnswerDiff = { rederived: [], pinned: [] }

describe('api/render', () => {
  it.each<[string, string | undefined]>([
    ['without a load', undefined],
    ['naming the held load', REPORT.sha256],
  ])('posts the profile %s and passes the abort signal on', async (_, sha) => {
    const fetchMock = mockApi()
    const controller = new AbortController()
    await fetchRender(PROFILE, sha, controller.signal)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/render')
    expect(init.method).toBe('POST')
    expect(init.signal).toBe(controller.signal)
    expect(JSON.parse(String(init.body))).toEqual<RenderBody>({ profile: PROFILE, sourceSha256: sha })
  })

  it('returns the tree and the diff against the previous render', async () => {
    mockApi()
    const first = await fetchRender(PROFILE)
    expect(first.tree.leaves.map((l) => l.value)).toEqual([`keycloak.${BASE}`, `https://keycloak.${BASE}`])
    expect(first.diff.groups).toEqual([
      {
        foundation: 'host.keycloak',
        changes: [
          { path: 'keycloak.ingress.hosts[0].host', status: 'added', after: `keycloak.${BASE}` },
          { path: 'keycloak.realm.frontendUrl', status: 'added', after: `https://keycloak.${BASE}` },
        ],
      },
    ])
    const again = await fetchRender(PROFILE)
    expect(again.diff.groups).toEqual([])
  })

  it.each<[string, Record<string, AnswerValue>, AnswerDiff]>([
    ['pinning at the derived value', { 'domain.base': BASE, [PINNED_HOST]: `keycloak.${BASE}` }, NO_CHANGE],
    [
      'a domain change past a pinned host',
      { 'domain.base': OTHER_BASE, [PINNED_HOST]: `keycloak.${BASE}` },
      { rederived: [], pinned: [{ id: PINNED_HOST, kept: `keycloak.${BASE}`, before: `keycloak.${BASE}`, after: `keycloak.${OTHER_BASE}` }] },
    ],
    [
      'a domain change re-deriving the host',
      { 'domain.base': OTHER_BASE },
      { rederived: [{ id: PINNED_HOST, before: `keycloak.${BASE}`, after: `keycloak.${OTHER_BASE}` }], pinned: [] },
    ],
  ])('returns the derived diff after %s', async (_, values, want) => {
    mockApi()
    expect((await fetchRender(PROFILE)).derived).toEqual(NO_CHANGE)
    expect((await fetchRender(withAnswers(values))).derived).toEqual(want)
  })

  it('rejects an invalid profile with 422 and the server message', async () => {
    mockApi(undefined, undefined, undefined, undefined, undefined, undefined, { render: () => HTTP_UNPROCESSABLE })
    await expect(fetchRender(PROFILE)).rejects.toMatchObject({
      name: 'ApiError',
      status: HTTP_UNPROCESSABLE,
      message: refusedFoundations(HTTP_UNPROCESSABLE),
    })
  })
})
