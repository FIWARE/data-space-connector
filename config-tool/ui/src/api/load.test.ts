import { describe, expect, it, vi } from 'vitest'
import { REPORT } from '../test/api'
import { CONTENT_TYPE_YAML, loadValues } from './load'

const HTTP_UNPROCESSABLE = 422

function respond(status: number, body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function sent(fetchMock: ReturnType<typeof respond>): { url: string; init: RequestInit } {
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
  return { url, init }
}

describe('loadValues', () => {
  it('posts the File itself as application/yaml and returns the report', async () => {
    const fetchMock = respond(200, REPORT)
    const file = new File(['﻿a: 1\r\n'], 'values.yaml')
    await expect(loadValues(file)).resolves.toEqual(REPORT)
    const { init } = sent(fetchMock)
    expect(init.method).toBe('POST')
    expect(init.body).toBe(file)
    expect(new Headers(init.headers).get('Content-Type')).toBe(CONTENT_TYPE_YAML)
  })

  it.each([
    ['values.yaml', '/api/load?name=values.yaml'],
    ['a&b #1%.yaml', '/api/load?name=a%26b%20%231%25.yaml'],
  ])('encodes the name %s into the query', async (name, url) => {
    const fetchMock = respond(200, REPORT)
    await loadValues(new File([''], name))
    expect(sent(fetchMock).url).toBe(url)
    expect(new URL(url, 'http://localhost').searchParams.get('name')).toBe(name)
  })

  it.each([
    [undefined, null],
    ['', ''],
    ['dsc', 'dsc'],
    ['apps."my app"', 'apps."my app"'],
  ])('sends the root %j only when one is asked for', async (root, expected) => {
    const fetchMock = respond(200, REPORT)
    await loadValues(new File([''], 'values.yaml'), { root })
    expect(new URL(sent(fetchMock).url, 'http://localhost').searchParams.get('root')).toBe(expected)
  })

  it('rejects a refused upload with its status', async () => {
    respond(HTTP_UNPROCESSABLE, { error: 'not one YAML document' })
    await expect(loadValues(new File([''], 'x.yaml'))).rejects.toMatchObject({ name: 'ApiError', status: HTTP_UNPROCESSABLE, message: 'not one YAML document' })
  })
})
