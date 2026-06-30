import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ApiError, listApiKeys, healthCheck } from './client.js'
import { state } from './state.js'

describe('API Client', () => {
  beforeEach(() => {
    state.baseUrl = 'http://localhost:8081'
    state.token = 'test-token'
    vi.stubGlobal('fetch', vi.fn())
  })

  it('does not send authorization header to /healthz', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'text/plain' },
      text: async () => 'OK'
    })

    await healthCheck()

    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:8081/healthz',
      expect.objectContaining({
        headers: expect.not.objectContaining({
          Authorization: expect.any(String)
        })
      })
    )
  })

  it('sends authorization header to /management/*', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'application/json' },
      json: async () => ({ data: [] })
    })

    await listApiKeys()

    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:8081/management/api-keys?page=1&limit=20',
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer test-token'
        })
      })
    )
  })

  it('unwraps paginated list responses', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: true,
      headers: { get: () => 'application/json' },
      json: async () => ({ data: [{ id: 'key-1' }], total: 1, page: 1, limit: 20 })
    })

    await expect(listApiKeys()).resolves.toEqual([{ id: 'key-1' }])
  })

  it('normalizes error payloads', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      headers: { get: () => 'application/json' },
      json: async () => ({
        error: 'invalid name',
        code: 'INVALID_INPUT',
        details: ['name must be trimmed']
      })
    })

    try {
      await listApiKeys()
      expect.fail('should have thrown ApiError')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      expect(err.message).toBe('invalid name')
      expect(err.status).toBe(400)
      expect(err.code).toBe('INVALID_INPUT')
      expect(err.details).toEqual(['name must be trimmed'])
    }
  })
})
