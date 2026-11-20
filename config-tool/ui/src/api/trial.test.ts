import { describe, expect, it } from 'vitest'
import { DEFAULT_DIR, mockApi, refusedTrial, TRIAL } from '../test/api'
import { fetchTrial, writeTrial } from './trial'

const HTTP_CONFLICT = 409
const HTTP_UNPROCESSABLE = 422
const HTTP_INTERNAL = 500

describe('api/trial', () => {
  it('fetches the trial with GET and passes the abort signal on', async () => {
    const fetchMock = mockApi()
    const controller = new AbortController()
    await expect(fetchTrial(controller.signal)).resolves.toEqual(TRIAL)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/trial')
    expect(init.method).toBeUndefined()
    expect(init.signal).toBe(controller.signal)
  })

  it('posts the directory and returns the written paths', async () => {
    const fetchMock = mockApi()
    const written = await writeTrial(DEFAULT_DIR)
    expect(written).toEqual({ dir: DEFAULT_DIR, files: TRIAL.releases.map((release) => `${DEFAULT_DIR}/${release.file}`) })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/trial/write')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ dir: DEFAULT_DIR })
  })

  it.each([HTTP_CONFLICT, HTTP_UNPROCESSABLE])('rejects a refused write with %i and the server message', async (status) => {
    mockApi(undefined, undefined, undefined, undefined, { write: () => status })
    await expect(writeTrial(DEFAULT_DIR)).rejects.toMatchObject({ name: 'ApiError', status, message: refusedTrial(status) })
  })

  it('rejects a trial the server cannot render', async () => {
    mockApi(undefined, undefined, undefined, undefined, { fetch: () => HTTP_INTERNAL })
    await expect(fetchTrial()).rejects.toMatchObject({ status: HTTP_INTERNAL, message: refusedTrial(HTTP_INTERNAL) })
  })
})
