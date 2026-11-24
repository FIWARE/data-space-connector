import { describe, expect, it } from 'vitest'
import { FOUNDATIONS, HTTP_NOT_FOUND, mockApi, refusedFoundations } from '../test/api'
import { fetchFoundations } from './foundations'

const DSC_CHART = 'data-space-connector'

describe('api/foundations', () => {
  it.each([
    ['10.16.0', '/api/foundations?chart=data-space-connector&version=10.16.0'],
    ['', '/api/foundations?chart=data-space-connector&version='],
  ])('fetches the foundations of version %j with GET and passes the abort signal on', async (version, path) => {
    const fetchMock = mockApi()
    const controller = new AbortController()
    await expect(fetchFoundations(DSC_CHART, version, controller.signal)).resolves.toEqual(FOUNDATIONS)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe(path)
    expect(init.method).toBeUndefined()
    expect(init.signal).toBe(controller.signal)
  })

  it('rejects a chart without a bundle with 404', async () => {
    mockApi(undefined, undefined, undefined, undefined, undefined, undefined, { foundations: () => HTTP_NOT_FOUND })
    await expect(fetchFoundations('unknown', '')).rejects.toMatchObject({
      name: 'ApiError',
      status: HTTP_NOT_FOUND,
      message: refusedFoundations(HTTP_NOT_FOUND),
    })
  })
})
