// Straw Management API Client

import { state, clearSession } from './state.js'
import type {
  ApiKey,
  BillingEstimate,
  CacheStats,
  ClearCacheResult,
  Endpoint,
  FingerprintPreset,
  RoutingRule,
  UsageSummary
} from './types.js'

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'
type JsonPayload = object

interface ApiRequestOptions extends Omit<RequestInit, 'body' | 'headers' | 'method'> {
  body?: BodyInit | JsonPayload
  headers?: Record<string, string>
}

interface ListOptions {
  page?: number
  limit?: number
}

type ListBody<T> = T[] | { data?: T[]; keys?: T[] }

interface UsageOptions {
  start?: string
  end?: string
  api_key_id?: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : 'Network error'
}

export class ApiError extends Error {
  method: HttpMethod
  url: string
  status: number
  code: string | null
  details: unknown

  constructor(
    message: string,
    method: HttpMethod,
    url: string,
    status: number,
    code: string | null,
    details: unknown
  ) {
    super(message)
    this.name = 'ApiError'
    this.method = method
    this.url = url
    this.status = status
    this.code = code
    this.details = details
  }
}

export async function apiRequest<T = unknown>(
  method: HttpMethod,
  path: string,
  options: ApiRequestOptions = {}
): Promise<T> {
  const baseUrl = (state.baseUrl || '').replace(/\/$/, '')
  const url = `${baseUrl}${path}`
  const headers = { ...(options.headers || {}) }

  if (path.startsWith('/management/')) {
    headers['Authorization'] = `Bearer ${state.token}`
  }

  const { body, headers: _headers, ...rest } = options
  const fetchOptions: RequestInit = {
    method,
    headers,
    ...rest
  }

  if (body && typeof body === 'object' && !(body instanceof FormData)) {
    fetchOptions.body = JSON.stringify(body)
    headers['Content-Type'] = 'application/json'
  } else {
    fetchOptions.body = body
  }

  try {
    const res = await fetch(url, fetchOptions)

    if (res.status === 401 && path.startsWith('/management/')) {
      clearSession()
      window.location.hash = '#/login'
      throw new ApiError('Session token rejected', method, url, 401, 'UNAUTHORIZED', null)
    }

    let responseBody: unknown = null
    const contentType = res.headers.get('content-type')
    if (contentType && contentType.includes('application/json')) {
      responseBody = await res.json()
    } else {
      responseBody = await res.text()
    }

    if (!res.ok) {
      const errMessage =
        isRecord(responseBody) && typeof responseBody.error === 'string'
          ? responseBody.error
          : typeof responseBody === 'string' && responseBody
            ? responseBody
            : res.statusText
      const code =
        isRecord(responseBody) && typeof responseBody.code === 'string' ? responseBody.code : null
      const details = isRecord(responseBody) ? responseBody.details : null
      throw new ApiError(errMessage, method, url, res.status, code, details)
    }

    return responseBody as T
  } catch (err) {
    if (err instanceof ApiError) throw err
    throw new ApiError(errorMessage(err), method, url, 0, 'NETWORK_ERROR', err)
  }
}

function listBody<T>(body: ListBody<T>, legacyKey?: 'keys'): T[] {
  if (Array.isArray(body)) return body
  if (body && Array.isArray(body.data)) return body.data
  if (legacyKey && body && Array.isArray(body[legacyKey])) return body[legacyKey]
  return []
}

// Health Check
export async function healthCheck(): Promise<unknown> {
  return apiRequest('GET', '/healthz')
}

// API Keys
export async function listApiKeys({ page = 1, limit = 20 }: ListOptions = {}): Promise<ApiKey[]> {
  const lim = Math.min(Math.max(1, limit), 100)
  return listBody<ApiKey>(
    await apiRequest<ListBody<ApiKey>>('GET', `/management/api-keys?page=${page}&limit=${lim}`),
    'keys'
  )
}

export async function createApiKey(payload: JsonPayload): Promise<ApiKey> {
  return apiRequest<ApiKey>('POST', '/management/api-keys', { body: payload })
}

export async function revokeApiKey(id: string) {
  return apiRequest('DELETE', `/management/api-keys/${id}`)
}

// Routing Rules
export async function listRoutingRules({ page = 1, limit = 20 }: ListOptions = {}): Promise<
  RoutingRule[]
> {
  const lim = Math.min(Math.max(1, limit), 100)
  return listBody<RoutingRule>(
    await apiRequest<ListBody<RoutingRule>>('GET', `/management/rules?page=${page}&limit=${lim}`)
  )
}

export async function getRoutingRule(id: string): Promise<RoutingRule> {
  return apiRequest<RoutingRule>('GET', `/management/rules/${id}`)
}

export async function createRoutingRule(payload: JsonPayload): Promise<RoutingRule> {
  return apiRequest<RoutingRule>('POST', '/management/rules', { body: payload })
}

export async function updateRoutingRule(id: string, payload: JsonPayload): Promise<RoutingRule> {
  return apiRequest<RoutingRule>('PUT', `/management/rules/${id}`, { body: payload })
}

export async function deleteRoutingRule(id: string) {
  return apiRequest('DELETE', `/management/rules/${id}`)
}

// Endpoints
export async function listEndpoints(): Promise<Endpoint[]> {
  return listBody<Endpoint>(await apiRequest<ListBody<Endpoint>>('GET', '/management/endpoints'))
}

export async function drainEndpoint(id: string) {
  return apiRequest('POST', `/management/endpoints/${id}/drain`)
}

// Fingerprints
export async function listFingerprints(): Promise<FingerprintPreset[]> {
  return apiRequest<FingerprintPreset[]>('GET', '/management/fingerprints')
}

export async function createFingerprint(payload: JsonPayload): Promise<FingerprintPreset> {
  return apiRequest<FingerprintPreset>('POST', '/management/fingerprints', { body: payload })
}

export async function broadcastFingerprints() {
  return apiRequest('POST', '/management/fingerprints/broadcast')
}

// Usage and Billing
export async function getUsageSummary({
  start,
  end,
  api_key_id = ''
}: UsageOptions = {}): Promise<UsageSummary> {
  let query = `?start=${start}&end=${end}`
  if (api_key_id) query += `&api_key_id=${api_key_id}`
  return apiRequest<UsageSummary>('GET', `/management/usage/summary${query}`)
}

export async function getBillingEstimate({
  start,
  end,
  api_key_id = ''
}: UsageOptions = {}): Promise<BillingEstimate> {
  let query = `?start=${start}&end=${end}`
  if (api_key_id) query += `&api_key_id=${api_key_id}`
  return apiRequest<BillingEstimate>('GET', `/management/billing/estimate${query}`)
}

// Cache Controls
export async function getCacheStats(): Promise<CacheStats> {
  return apiRequest<CacheStats>('GET', '/management/cache/stats')
}

export async function clearCache(pattern = '*'): Promise<ClearCacheResult> {
  return apiRequest<ClearCacheResult>(
    'POST',
    `/management/cache/clear?pattern=${encodeURIComponent(pattern)}`
  )
}
