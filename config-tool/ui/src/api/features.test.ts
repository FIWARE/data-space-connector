import { describe, expect, it } from 'vitest'
import { initialState } from '../state/reducer'
import { featureChoices, mockApi, refusedRole, REPORT, type FeaturesBody } from '../test/api'
import { fetchFeatures, type FeatureToggle } from './features'
import { toProfile } from './profile'

const HTTP_UNPROCESSABLE = 422
const PROVIDER = { ...toProfile(initialState), role: 'provider', features: { 'identity.issuer': { enabled: true, source: 'preset' as const } } }
const TURN_OFF: FeatureToggle = { feature: 'identity.issuer', enabled: false }

describe('api/features', () => {
  it.each<[string, string | undefined, FeatureToggle | undefined]>([
    ['reads the choices without a load or toggle', undefined, undefined],
    ['records a toggle, naming the held load', REPORT.sha256, TURN_OFF],
  ])('%s with POST', async (_, sha, toggle) => {
    const fetchMock = mockApi()
    await fetchFeatures(PROVIDER, sha, toggle)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/features')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual<FeaturesBody>({ profile: PROVIDER, sourceSha256: sha, toggle: toggle ?? null })
  })

  it('returns the profile and choices the server answers', async () => {
    mockApi()
    const result = await fetchFeatures(PROVIDER, undefined, TURN_OFF)
    expect(result.profile.features).toEqual({ 'identity.issuer': { enabled: false, source: 'user' } })
    expect(result.choices).toEqual(featureChoices(result.profile))
    expect(result.choices[0]).toEqual({ id: 'identity.issuer', offer: 'recommended', preset: true, enabled: false, deliberate: true, source: 'user' })
  })

  it('rejects a refused toggle with 422 and the server message', async () => {
    mockApi(undefined, undefined, undefined, undefined, undefined, { features: () => HTTP_UNPROCESSABLE })
    await expect(fetchFeatures(PROVIDER, undefined, { feature: 'ta.registry', enabled: true })).rejects.toMatchObject({
      name: 'ApiError',
      status: HTTP_UNPROCESSABLE,
      message: refusedRole(HTTP_UNPROCESSABLE),
    })
  })
})
