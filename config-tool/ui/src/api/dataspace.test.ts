import { describe, expect, it } from 'vitest'
import { DESCRIPTOR_PATH, IMPORTED, mockApi, refusedDataspace, TIR_STATUS, type DataspaceFakes } from '../test/api'
import { checkTIR, importDescriptor } from './dataspace'
import { initialState } from '../state/reducer'
import { toProfile } from './profile'

const TIR = 'https://tir.example.org'
const BASE = 'https://dataspace.example.org'
const HTTP_UNPROCESSABLE = 422
const HTTP_BAD_GATEWAY = 502
const PROFILE = toProfile(initialState)

const fakes = (dataspace: DataspaceFakes) => mockApi(undefined, undefined, undefined, {}, {}, {}, {}, dataspace)

describe('api/dataspace', () => {
  it('posts the TIR address and returns the reachability', async () => {
    const fetchMock = fakes({})
    const controller = new AbortController()
    await expect(checkTIR(TIR, controller.signal)).resolves.toEqual({ status: 'reachable', httpStatus: TIR_STATUS })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/dataspace/check')
    expect(init.method).toBe('POST')
    expect(init.signal).toBe(controller.signal)
    expect(JSON.parse(String(init.body))).toEqual({ url: TIR })
  })

  it('posts the base URL with the profile and returns the preview', async () => {
    const fetchMock = fakes({})
    const result = await importDescriptor(BASE, PROFILE)
    expect(result.url).toBe(`${BASE}${DESCRIPTOR_PATH}`)
    expect(result.preview.changes.map((c) => c.id)).toEqual(Object.keys(IMPORTED))
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/dataspace/import')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({ url: BASE, profile: PROFILE })
  })

  const refusals: [string, () => Promise<unknown>, DataspaceFakes, number][] = [
    ['check', () => checkTIR(TIR), { check: () => HTTP_UNPROCESSABLE }, HTTP_UNPROCESSABLE],
    ['import', () => importDescriptor(BASE, PROFILE), { import: () => HTTP_UNPROCESSABLE }, HTTP_UNPROCESSABLE],
    ['import', () => importDescriptor(BASE, PROFILE), { import: () => HTTP_BAD_GATEWAY }, HTTP_BAD_GATEWAY],
  ]

  it.each(refusals)('rejects a refused %s with the status and the server message', async (_, call, dataspace, status) => {
    fakes(dataspace)
    await expect(call()).rejects.toMatchObject({ name: 'ApiError', status, message: refusedDataspace(status) })
  })
})
