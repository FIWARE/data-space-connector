import { describe, expect, it } from 'vitest'
import { initialState } from '../state/reducer'
import { mockApi, refusedRole, REPORT, ROLES, type PresetBody } from '../test/api'
import { toProfile } from './profile'
import { applyPreset, fetchRoles } from './roles'

const HTTP_NOT_FOUND = 404
const HTTP_UNPROCESSABLE = 422
const DSC_CHART = 'data-space-connector'
const PROFILE = toProfile(initialState)

describe('api/roles', () => {
  it.each([
    ['10.16.0', '/api/roles?chart=data-space-connector&version=10.16.0'],
    ['', '/api/roles?chart=data-space-connector&version='],
  ])('fetches the catalogue of version %j with GET and passes the abort signal on', async (version, path) => {
    const fetchMock = mockApi()
    const controller = new AbortController()
    await expect(fetchRoles(DSC_CHART, version, controller.signal)).resolves.toEqual(ROLES)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe(path)
    expect(init.method).toBeUndefined()
    expect(init.signal).toBe(controller.signal)
  })

  it('rejects a chart without a bundle with 404', async () => {
    mockApi(undefined, undefined, undefined, undefined, undefined, { roles: () => HTTP_NOT_FOUND })
    await expect(fetchRoles('other', '')).rejects.toMatchObject({ status: HTTP_NOT_FOUND, message: refusedRole(HTTP_NOT_FOUND) })
  })

  it.each([
    ['without a load', undefined],
    ['naming the held load', REPORT.sha256],
  ])('posts profile, role and source %s', async (_, sha) => {
    const fetchMock = mockApi()
    const result = await applyPreset(PROFILE, 'provider', sha)
    expect(result).toEqual({ profile: { ...PROFILE, role: 'provider', features: {} }, chartSwitch: null })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/preset')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual<PresetBody>({ profile: PROFILE, role: 'provider', sourceSha256: sha })
  })

  it('returns the chart switch the server reports', async () => {
    const chartSwitch = { fromChart: DSC_CHART, fromVersion: '10.16.0', toChart: 'trust-anchor', droppedChoices: [] }
    mockApi(undefined, undefined, undefined, undefined, undefined, { preset: (body) => ({ profile: body.profile, chartSwitch }) })
    await expect(applyPreset(PROFILE, 'operator')).resolves.toMatchObject({ chartSwitch })
  })

  it('rejects a refused preset with 422 and the server message', async () => {
    mockApi(undefined, undefined, undefined, undefined, undefined, { preset: () => HTTP_UNPROCESSABLE })
    await expect(applyPreset(PROFILE, 'consumer', REPORT.sha256)).rejects.toMatchObject({
      name: 'ApiError',
      status: HTTP_UNPROCESSABLE,
      message: refusedRole(HTTP_UNPROCESSABLE),
    })
  })
})
