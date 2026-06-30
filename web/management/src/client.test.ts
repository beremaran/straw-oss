import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ApiError, listApiKeys, healthCheck } from './client.js'
import { state } from './state.js'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  const headers = new Headers(init.headers)
  headers.set('content-type', 'application/json')
  return new Response(JSON.stringify(body), { ...init, headers })
}

describe('API Client', () => {
  let fetchMock: ReturnType<typeof vi.fn<typeof fetch>>

  beforeEach(() => {
    state.baseUrl = 'http://localhost:8081'
    state.token = 'test-token'
    fetchMock = vi.fn<typeof fetch>()
    vi.stubGlobal('fetch', fetchMock)
  })

  function firstFetchCall(): [RequestInfo | URL, RequestInit] {
    const call = fetchMock.mock.calls[0]
    if (!call || !call[1]) throw new Error('Expected fetch to be called with options')
    return [call[0], call[1]]
  }

  function plainHeaders(init: RequestInit): Record<string, string> {
    if (!init.headers || init.headers instanceof Headers || Array.isArray(init.headers)) {
      throw new Error('Expected plain fetch headers')
    }
    return init.headers
  }

  it('does not send authorization header to /healthz', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response('OK', { headers: { 'content-type': 'text/plain' } })
    )

    await healthCheck()

    const [url, init] = firstFetchCall()
    expect(url).toBe('http://localhost:8081/healthz')
    expect(plainHeaders(init)).not.toHaveProperty('Authorization')
  })

  it('sends authorization header to /management/*', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ data: [] }))

    await listApiKeys()

    const [url, init] = firstFetchCall()
    expect(url).toBe('http://localhost:8081/management/api-keys?page=1&limit=20')
    expect(plainHeaders(init).Authorization).toBe('Bearer test-token')
  })

  it('unwraps paginated list responses', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ data: [{ id: 'key-1' }], total: 1, page: 1, limit: 20 })
    )

    await expect(listApiKeys()).resolves.toEqual([{ id: 'key-1' }])
  })

  it('normalizes error payloads', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(
        {
          error: 'invalid name',
          code: 'INVALID_INPUT',
          details: ['name must be trimmed']
        },
        {
          status: 400,
          statusText: 'Bad Request'
        }
      )
    )

    try {
      await listApiKeys()
      expect.fail('should have thrown ApiError')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      if (!(err instanceof ApiError)) throw err
      expect(err.message).toBe('invalid name')
      expect(err.status).toBe(400)
      expect(err.code).toBe('INVALID_INPUT')
      expect(err.details).toEqual(['name must be trimmed'])
    }
  })
})
