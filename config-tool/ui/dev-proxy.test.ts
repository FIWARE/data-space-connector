import type { IncomingHttpHeaders, IncomingMessage, ServerResponse } from 'node:http'
import { describe, expect, it, vi } from 'vitest'
import { API_PREFIX, DEV_PORT, devProxy, devProxyGuard, parseConfigUrl } from './dev-proxy.ts'

const OWN_HOST: IncomingHttpHeaders = { host: `127.0.0.1:${DEV_PORT}` }
const UPGRADE: IncomingHttpHeaders = { ...OWN_HOST, upgrade: 'websocket', connection: 'Upgrade' }

describe('devProxyGuard', () => {
  it.each<[string, string, IncomingHttpHeaders]>([
    ['GET from 127.0.0.1', 'GET', { ...OWN_HOST, origin: `http://127.0.0.1:${DEV_PORT}` }],
    ['GET from localhost', 'GET', { host: `localhost:${DEV_PORT}`, origin: `http://localhost:${DEV_PORT}` }],
    ['POST from own origin', 'POST', { ...OWN_HOST, origin: `http://127.0.0.1:${DEV_PORT}`, 'sec-fetch-site': 'same-origin' }],
    ['upgrade from own origin', 'GET', { ...UPGRADE, origin: `http://localhost:${DEV_PORT}` }],
    ['GET without Origin', 'GET', OWN_HOST],
    ['HEAD without Origin', 'HEAD', OWN_HOST],
  ])('lets %s pass', (_name, method, headers) => {
    expect(devProxyGuard(method, headers, DEV_PORT)).toBeUndefined()
  })

  it.each<[string, string, IncomingHttpHeaders]>([
    ['a foreign port', 'GET', { ...OWN_HOST, origin: 'http://127.0.0.1:8080' }],
    ['a remote origin', 'POST', { ...OWN_HOST, origin: 'https://evil.example' }],
    ['an opaque origin', 'GET', { ...OWN_HOST, origin: 'null' }],
    ['a foreign-origin upgrade', 'GET', { ...UPGRADE, origin: 'http://localhost:3000' }],
    ['a POST without Origin', 'POST', OWN_HOST],
    ['a PUT without Origin', 'PUT', OWN_HOST],
    ['an upgrade without Origin', 'GET', UPGRADE],
    ['Sec-Fetch-Site cross-site', 'GET', { ...OWN_HOST, 'sec-fetch-site': 'cross-site' }],
    ['Sec-Fetch-Site same-site', 'GET', { ...OWN_HOST, origin: `http://127.0.0.1:${DEV_PORT}`, 'sec-fetch-site': 'same-site' }],
    ['Sec-Fetch-Site none', 'GET', { ...OWN_HOST, 'sec-fetch-site': 'none' }],
    ['a rebound Host on a same-origin GET', 'GET', { host: `evil.example:${DEV_PORT}`, 'sec-fetch-site': 'same-origin' }],
    ['a foreign-port Host', 'GET', { host: '127.0.0.1:8080' }],
    ['a missing Host', 'GET', {}],
  ])('answers 403 to %s', (_name, method, headers) => {
    expect(devProxyGuard(method, headers, DEV_PORT)).toBeDefined()
  })
})

describe('devProxy bypass', () => {
  const TARGET = { port: 41234, token: 'run-token' }
  const BACKEND = `http://127.0.0.1:${TARGET.port}`
  const OWN_ORIGIN = `http://127.0.0.1:${DEV_PORT}`
  const FOREIGN_ORIGIN = 'http://127.0.0.1:8080'
  const URL_PATH = `${API_PREFIX}/model`

  /** Fakes what Vite hands to bypass: res for HTTP, none for a WebSocket upgrade. */
  function call(method: string, headers: IncomingHttpHeaders, upgrade: boolean) {
    const socketEnd = vi.fn()
    const req = { method, url: URL_PATH, headers: { ...headers }, socket: { end: socketEnd } }
    const res = { writeHead: vi.fn(), end: vi.fn() }
    res.writeHead.mockReturnValue(res)
    const bypass = devProxy(TARGET, DEV_PORT)[API_PREFIX]?.bypass
    if (!bypass) {
      throw new Error(`devProxy has no ${API_PREFIX} bypass`)
    }
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = bypass(
      req as unknown as IncomingMessage,
      (upgrade ? undefined : res) as unknown as ServerResponse,
      {},
    )
    return { result, req, res, socketEnd }
  }

  it.each<[string, string, IncomingHttpHeaders, boolean]>([
    ['HTTP POST', 'POST', { ...OWN_HOST, origin: OWN_ORIGIN }, false],
    ['upgrade', 'GET', { ...UPGRADE, origin: OWN_ORIGIN }, true],
  ])('proxies a passing %s with the token and the backend Origin', (_name, method, headers, upgrade) => {
    const { result, req, res, socketEnd } = call(method, headers, upgrade)
    expect(result).toBeUndefined()
    expect(req.headers.authorization).toBe(`Bearer ${TARGET.token}`)
    expect(req.headers.origin).toBe(BACKEND)
    expect(res.writeHead).not.toHaveBeenCalled()
    expect(socketEnd).not.toHaveBeenCalled()
  })

  it('leaves a missing Origin missing on a passing GET', () => {
    const { result, req } = call('GET', OWN_HOST, false)
    expect(result).toBeUndefined()
    expect(req.headers.authorization).toBe(`Bearer ${TARGET.token}`)
    expect(req.headers).not.toHaveProperty('origin')
  })

  it.each<[string, string, IncomingHttpHeaders, boolean]>([
    ['HTTP POST with a foreign Origin', 'POST', { ...OWN_HOST, origin: FOREIGN_ORIGIN }, false],
    ['HTTP POST without Origin', 'POST', OWN_HOST, false],
    ['HTTP GET with a rebound Host', 'GET', { host: `evil.example:${DEV_PORT}` }, false],
    ['upgrade with a foreign Origin', 'GET', { ...UPGRADE, origin: FOREIGN_ORIGIN }, true],
    ['upgrade without Origin', 'GET', UPGRADE, true],
  ])('answers 403 to a %s without adding the token', (_name, method, headers, upgrade) => {
    const { result, req, res, socketEnd } = call(method, headers, upgrade)
    expect(typeof result).toBe('string')
    expect(req.headers).not.toHaveProperty('authorization')
    expect(req.headers.origin).toBe(headers.origin)
    if (upgrade) {
      expect(socketEnd).toHaveBeenCalledWith(expect.stringMatching(/^HTTP\/1\.1 403 /))
    } else {
      expect(res.writeHead).toHaveBeenCalledWith(403)
      expect(res.end).toHaveBeenCalled()
    }
  })
})

describe('parseConfigUrl', () => {
  it('takes port and token from the printed URL', () => {
    expect(parseConfigUrl('http://127.0.0.1:41234/?token=abc')).toEqual({ port: 41234, token: 'abc' })
  })

  it.each([
    ['missing', undefined],
    ['empty', ''],
    ['not a URL', 'nope'],
    ['without token', 'http://127.0.0.1:41234/'],
    ['without port', 'http://127.0.0.1/?token=abc'],
    ['non-loopback', 'http://example.com:41234/?token=abc'],
    ['https', 'https://127.0.0.1:41234/?token=abc'],
  ])('rejects a %s URL', (_name, raw) => {
    expect(() => parseConfigUrl(raw)).toThrow(/DSC_CONFIG_URL/)
  })
})
