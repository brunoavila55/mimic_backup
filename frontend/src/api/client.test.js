import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiFetch } from './client'

afterEach(() => vi.restoreAllMocks())

describe('apiFetch', () => {
  it('always includes session credentials and unwraps data', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(
      JSON.stringify({ data: { status: 'ok' } }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    ))

    await expect(apiFetch('/health')).resolves.toEqual({ status: 'ok' })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/health', expect.objectContaining({ credentials: 'include' }))
  })

  it('maps the standard error envelope', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(
      JSON.stringify({ error: { code: 'forbidden', message: 'Access denied.' } }),
      { status: 403, headers: { 'Content-Type': 'application/json' } },
    ))

    const request = apiFetch('/nodes')
    await expect(request).rejects.toEqual(expect.objectContaining({
      name: 'ApiError', code: 'forbidden', status: 403,
    }))
    await request.catch((error) => expect(error).toBeInstanceOf(ApiError))
  })
})
