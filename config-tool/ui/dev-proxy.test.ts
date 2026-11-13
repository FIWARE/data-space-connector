import type { IncomingHttpHeaders } from 'node:http'
import { describe, expect, it } from 'vitest'
import { DEV_PORT, devOrigins, devProxyGuard, parseConfigUrl } from './dev-proxy.ts'

const OWN = devOrigins(DEV_PORT)
const UPGRADE: IncomingHttpHeaders = { upgrade: 'websocket', connection: 'Upgrade' }

describe('devProxyGuard', () => {
  it.each<[string, string, IncomingHttpHeaders]>([
    ['GET from 127.0.0.1', 'GET', { origin: `http://127.0.0.1:${DEV_PORT}` }],
    ['GET from localhost', 'GET', { origin: `http://localhost:${DEV_PORT}` }],
    ['POST from own origin', 'POST', { origin: `http://127.0.0.1:${DEV_PORT}`, 'sec-fetch-site': 'same-origin' }],
    ['upgrade from own origin', 'GET', { ...UPGRADE, origin: `http://localhost:${DEV_PORT}` }],
    ['GET without Origin', 'GET', {}],
    ['HEAD without Origin', 'HEAD', {}],
  ])('lets %s pass', (_name, method, headers) => {
    expect(devProxyGuard(method, headers, OWN)).toBeUndefined()
  })

  it.each<[string, string, IncomingHttpHeaders]>([
    ['a foreign port', 'GET', { origin: 'http://127.0.0.1:8080' }],
    ['a remote origin', 'POST', { origin: 'https://evil.example' }],
    ['an opaque origin', 'GET', { origin: 'null' }],
    ['a foreign-origin upgrade', 'GET', { ...UPGRADE, origin: 'http://localhost:3000' }],
    ['a POST without Origin', 'POST', {}],
    ['a PUT without Origin', 'PUT', {}],
    ['an upgrade without Origin', 'GET', UPGRADE],
    ['Sec-Fetch-Site cross-site', 'GET', { 'sec-fetch-site': 'cross-site' }],
    ['Sec-Fetch-Site same-site', 'GET', { origin: `http://127.0.0.1:${DEV_PORT}`, 'sec-fetch-site': 'same-site' }],
    ['Sec-Fetch-Site none', 'GET', { 'sec-fetch-site': 'none' }],
  ])('answers 403 to %s', (_name, method, headers) => {
    expect(devProxyGuard(method, headers, OWN)).toBeDefined()
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
