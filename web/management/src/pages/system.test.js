import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { SystemPage } from './system.js'
import { state, clearSession } from '../state.js'

vi.mock('../client.js', () => ({
  healthCheck: vi.fn(),
  getCacheStats: vi.fn(),
  listFingerprints: vi.fn(),
  getUsageSummary: vi.fn(),
  listRoutingRules: vi.fn(),
  listEndpoints: vi.fn(),
  listApiKeys: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(message, status) {
      super(message)
      this.status = status
    }
  }
}))

vi.mock('../state.js', () => {
  const state = {}
  return {
    state,
    setState: vi.fn((changes) => Object.assign(state, changes)),
    showToast: vi.fn(),
    clearSession: vi.fn()
  }
})

describe('System Diagnostics Page', () => {
  let container

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    state.baseUrl = 'http://localhost:8081'
    state.token = 'secret-management-token-12345'
    state.systemHealth = null
    state.systemHealthError = null
    state.systemHealthLoading = false
    state.systemHealthResponseTime = null
    state.systemCapabilities = null
    state.systemLastChecked = null
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
  })

  it('renders connection details with base URL and sign out button', () => {
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('Connection')
    expect(container.textContent).toContain('Management API URL')
    expect(container.textContent).toContain('http://localhost:8081')
    expect(container.textContent).toContain('Sign Out')
    expect(container.textContent).toContain('Authenticated')
  })

  it('shows health check loading state', () => {
    state.systemHealthLoading = true
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('Health Check')
    expect(container.querySelector('.skeleton-text')).toBeTruthy()
  })

  it('shows health check error state', () => {
    state.systemHealthError = 'Connection refused'
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('Health Check')
    expect(container.textContent).toContain('Error')
    expect(container.textContent).toContain('Connection refused')
  })

  it('shows health check success state', () => {
    state.systemHealth = 'OK'
    state.systemHealthResponseTime = 42
    state.systemLastChecked = Date.now()
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('Healthy')
    expect(container.textContent).toContain('42ms')
    expect(container.textContent).toContain('OK')
  })

  it('renders capability detection panel', () => {
    state.systemCapabilities = {
      cache: true,
      fingerprints: true,
      usage: false,
      rules: true,
      endpoints: true,
      apiKeys: true
    }
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('Detected Capabilities')
    expect(container.textContent).toContain('Cache Controls')
    expect(container.textContent).toContain('Available')
    expect(container.textContent).toContain('Usage & Billing')
  })

  it('renders documentation links', () => {
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('Documentation')
    expect(container.textContent).toContain('Management API Documentation')
    expect(container.textContent).toContain('OpenAPI Reference')
    expect(container.textContent).toContain('Architecture Documentation')
  })

  it('renders backend gaps list', () => {
    container.innerHTML = SystemPage.render(state)
    expect(container.textContent).toContain('First-Release Backend Gaps')
    expect(container.textContent).toContain('Audit-log viewer')
    expect(container.textContent).toContain('Fingerprint deletion')
    expect(container.textContent).toContain('Cost multiplier management')
  })

  it('does not expose secret token value in rendered output', () => {
    state.baseUrl = 'http://localhost:8081'
    state.systemHealth = { status: 'ok' }
    container.innerHTML = SystemPage.render(state)
    const rendered = container.innerHTML
    expect(rendered).not.toContain(state.token)
    expect(rendered).not.toContain('Bearer')
    expect(rendered).not.toContain('secret-management-token-12345')
  })

  it('renders sign out button and triggers clearSession on click', () => {
    container.innerHTML = SystemPage.render(state)
    SystemPage.afterRender(state)

    const signOutBtn = container.querySelector('#system-sign-out')
    expect(signOutBtn).toBeTruthy()
    signOutBtn.click()

    expect(clearSession).toHaveBeenCalled()
  })
})
