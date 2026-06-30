// Straw Management API Client

import { state, clearSession } from './state.js'

export class ApiError extends Error {
  constructor(message, method, url, status, code, details) {
    super(message)
    this.name = 'ApiError'
    this.method = method
    this.url = url
    this.status = status
    this.code = code
    this.details = details
  }
}

export async function apiRequest(method, path, options = {}) {
  const baseUrl = (state.baseUrl || '').replace(/\/$/, '')
  const url = `${baseUrl}${path}`
  const headers = { ...(options.headers || {}) }

  if (path.startsWith('/management/')) {
    headers['Authorization'] = `Bearer ${state.token}`
  }

  const fetchOptions = {
    method,
    headers,
    ...options
  }

  if (options.body && typeof options.body === 'object' && !(options.body instanceof FormData)) {
    fetchOptions.body = JSON.stringify(options.body)
    headers['Content-Type'] = 'application/json'
  }

  try {
    const res = await fetch(url, fetchOptions)

    if (res.status === 401 && path.startsWith('/management/')) {
      clearSession()
      window.location.hash = '#/login'
      throw new ApiError('Session token rejected', method, url, 401, 'UNAUTHORIZED', null)
    }

    let body = null
    const contentType = res.headers.get('content-type')
    if (contentType && contentType.includes('application/json')) {
      body = await res.json()
    } else {
      body = await res.text()
    }

    if (!res.ok) {
      const errMessage =
        body && typeof body === 'object' && body.error
          ? body.error
          : typeof body === 'string' && body
            ? body
            : res.statusText
      const code = body && typeof body === 'object' && body.code ? body.code : null
      const details = body && typeof body === 'object' && body.details ? body.details : null
      throw new ApiError(errMessage, method, url, res.status, code, details)
    }

    return body
  } catch (err) {
    if (err instanceof ApiError) throw err
    throw new ApiError(err.message || 'Network error', method, url, 0, 'NETWORK_ERROR', err)
  }
}

// Health Check
export async function healthCheck() {
  return apiRequest('GET', '/healthz')
}

// API Keys
export async function listApiKeys({ page = 1, limit = 20 } = {}) {
  const lim = Math.min(Math.max(1, limit), 100)
  return apiRequest('GET', `/management/api-keys?page=${page}&limit=${lim}`)
}

export async function createApiKey(payload) {
  return apiRequest('POST', '/management/api-keys', { body: payload })
}

export async function revokeApiKey(id) {
  return apiRequest('DELETE', `/management/api-keys/${id}`)
}

// Routing Rules
export async function listRoutingRules({ page = 1, limit = 20 } = {}) {
  const lim = Math.min(Math.max(1, limit), 100)
  return apiRequest('GET', `/management/rules?page=${page}&limit=${lim}`)
}

export async function getRoutingRule(id) {
  return apiRequest('GET', `/management/rules/${id}`)
}

export async function createRoutingRule(payload) {
  return apiRequest('POST', '/management/rules', { body: payload })
}

export async function updateRoutingRule(id, payload) {
  return apiRequest('PUT', `/management/rules/${id}`, { body: payload })
}

export async function deleteRoutingRule(id) {
  return apiRequest('DELETE', `/management/rules/${id}`)
}

// Endpoints
export async function listEndpoints() {
  return apiRequest('GET', '/management/endpoints')
}

export async function drainEndpoint(id) {
  return apiRequest('POST', `/management/endpoints/${id}/drain`)
}

// Fingerprints
export async function listFingerprints() {
  return apiRequest('GET', '/management/fingerprints')
}

export async function createFingerprint(payload) {
  return apiRequest('POST', '/management/fingerprints', { body: payload })
}

export async function broadcastFingerprints() {
  return apiRequest('POST', '/management/fingerprints/broadcast')
}

// Usage and Billing
export async function getUsageSummary({ start, end, api_key_id = '' } = {}) {
  let query = `?start=${start}&end=${end}`
  if (api_key_id) query += `&api_key_id=${api_key_id}`
  return apiRequest('GET', `/management/usage/summary${query}`)
}

export async function getBillingEstimate({ start, end, api_key_id = '' } = {}) {
  let query = `?start=${start}&end=${end}`
  if (api_key_id) query += `&api_key_id=${api_key_id}`
  return apiRequest('GET', `/management/billing/estimate${query}`)
}

// Cache Controls
export async function getCacheStats() {
  return apiRequest('GET', '/management/cache/stats')
}

export async function clearCache(pattern = '*') {
  return apiRequest('POST', `/management/cache/clear?pattern=${encodeURIComponent(pattern)}`)
}
