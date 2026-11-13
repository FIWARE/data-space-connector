import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, NotImplementedError, UnauthorizedError, api } from './client'

function respond(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })))
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('api', () => {
  it('decodes a JSON answer from /api', async () => {
    respond(200, { version: 'dev' })
    await expect(api('/version')).resolves.toEqual({ version: 'dev' })
    expect(fetch).toHaveBeenCalledWith('/api/version', expect.objectContaining({ credentials: 'same-origin' }))
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
