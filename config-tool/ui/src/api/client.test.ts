import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, NotImplementedError, UnauthorizedError, api, apiNoContent, connectProgress } from './client'

function respond(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('api', () => {
  it('decodes a JSON answer from /api', async () => {
    respond(200, { version: 'dev' })
    await expect(api('/version')).resolves.toEqual({ version: 'dev' })
    expect(fetch).toHaveBeenCalledWith('/api/version', expect.objectContaining({ credentials: 'same-origin' }))
  })

  it.each([
    ['204 No Content', 204, null],
    ['empty 200', 200, ''],
    ['200 with a body', 200, '{"saved":true}'],
  ])('apiNoContent resolves a %s', async (_name, status, body) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { status })))
    await expect(apiNoContent('/profile', { method: 'PUT', body: '{}' })).resolves.toBeUndefined()
  })

  it.each([
    ['204 No Content', 204, null],
    ['empty 200', 200, ''],
  ])('api rejects a %s with an ApiError', async (_name, status, body) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { status })))
    await expect(api('/model')).rejects.toMatchObject({ name: 'ApiError', status })
  })

  it.each([
    [401, UnauthorizedError],
    [501, NotImplementedError],
    [500, ApiError],
  ])('maps %i to %o', async (status, type) => {
    respond(status, { error: 'nope', ticket: 'CT-2' })
    const error = await api('/model').catch((e: unknown) => e)
    expect(error).toBeInstanceOf(type)
    expect(error).toMatchObject({ status, message: 'nope' })
  })

  it('keeps the ticket of a 501', async () => {
    respond(501, { error: 'later', ticket: 'CT-4' })
    await expect(api('/profile')).rejects.toMatchObject({ ticket: 'CT-4' })
  })
})

/** Stands in for the browser WebSocket so a test can push frames into the listener. */
class FakeWebSocket extends EventTarget {
  constructor(readonly url: string) {
    super()
  }

  receive(data: string) {
    this.dispatchEvent(new MessageEvent('message', { data }))
  }
}

describe('connectProgress', () => {
  const location = { protocol: 'http:', host: '127.0.0.1:41234' } as Location

  function connect() {
    vi.stubGlobal('WebSocket', FakeWebSocket)
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    const onEvent = vi.fn()
    const socket = connectProgress(onEvent, location) as unknown as FakeWebSocket
    return { onEvent, socket }
  }

  it('connects to /api/ws on the page origin', () => {
    const { socket } = connect()
    expect(socket.url).toBe('ws://127.0.0.1:41234/api/ws')
  })

  it('hands a well-formed event to the caller', () => {
    const { onEvent, socket } = connect()
    socket.receive('{"type":"ready","version":"dev"}')
    expect(onEvent).toHaveBeenCalledWith({ type: 'ready', version: 'dev' })
  })

  it.each([
    ['not JSON', 'ready'],
    ['JSON null', 'null'],
    ['a JSON string', '"ready"'],
    ['an object without type', '{"version":"dev"}'],
  ])('drops a frame that is %s without throwing', (_name, frame) => {
    const { onEvent, socket } = connect()
    expect(() => socket.receive(frame)).not.toThrow()
    expect(onEvent).not.toHaveBeenCalled()
    expect(console.warn).toHaveBeenCalled()
  })
})
