import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Mock state and client before importing pages
vi.mock('../../state.js', () => {
  const state = {
    currentPage: '#/overview',
    baseUrl: 'http://localhost:8081',
    token: 'test-token'
  }
  return {
    state,
    setState: vi.fn((changes: Record<string, unknown>) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn(),
    clearSession: vi.fn()
  }
})

vi.mock('../../client.js', () => ({
  ApiError: class ApiError extends Error {
    status: number
    constructor(message: string, status: number) {
      super(message)
      this.status = status
    }
  }
}))

// Import all pages
import { OverviewPage } from './overview.js'
import { ApiKeysPage } from './api-keys.js'
import { RoutingRulesPage } from './routing-rules.js'
import { RoutingRuleEditorPage } from './routing-rule-editor.js'
import { EndpointsPage } from './endpoints.js'
import { FingerprintsPage } from './fingerprints.js'
import { UsagePage } from './usage.js'
import { CachePage } from './cache.js'
import { SystemPage } from './system.js'
import { LoginPage } from './login.js'
import { testState } from '../test-utils.js'

describe('Route Smoke Tests', () => {
  let container: HTMLElement

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    container.remove()
  })

  it('OverviewPage renders without crashing', () => {
    container.innerHTML = OverviewPage.render(
      testState({ overviewData: null, overviewLoading: false })
    )
    expect(container.textContent).toContain('Overview Dashboard')
  })

  it('ApiKeysPage renders without crashing', () => {
    container.innerHTML = ApiKeysPage.render(
      testState({ apiKeysData: null, apiKeysLoading: false })
    )
    expect(container.textContent).toContain('API Keys')
  })

  it('RoutingRulesPage renders without crashing', () => {
    container.innerHTML = RoutingRulesPage.render(
      testState({ rulesData: null, rulesLoading: false })
    )
    expect(container.textContent).toContain('Routing Rules')
  })

  it('RoutingRuleEditorPage renders without crashing', () => {
    container.innerHTML = RoutingRuleEditorPage.render(testState())
    expect(container.textContent).toContain('Routing Rule')
  })

  it('EndpointsPage renders without crashing', () => {
    container.innerHTML = EndpointsPage.render(
      testState({ endpointsData: null, endpointsLoading: false })
    )
    expect(container.textContent).toContain('Active Endpoint Nodes')
  })

  it('FingerprintsPage renders without crashing', () => {
    container.innerHTML = FingerprintsPage.render(
      testState({
        fingerprintsData: null,
        fingerprintsLoading: false
      })
    )
    expect(container.textContent).toContain('Fingerprint Presets')
  })

  it('UsagePage renders without crashing', () => {
    container.innerHTML = UsagePage.render(testState({ usageData: null, usageLoading: false }))
    expect(container.textContent).toContain('Usage & Billing')
  })

  it('CachePage renders without crashing', () => {
    container.innerHTML = CachePage.render(testState({ cacheData: null, cacheLoading: false }))
    expect(container.textContent).toContain('Cache Control')
  })

  it('SystemPage renders without crashing', () => {
    container.innerHTML = SystemPage.render(
      testState({ systemHealth: null, systemHealthLoading: false })
    )
    expect(container.textContent).toContain('System Info')
  })

  it('LoginPage renders without crashing', () => {
    container.innerHTML = LoginPage.render(testState({ loginError: null }))
    expect(container.textContent).toContain('Straw Console')
  })
})
