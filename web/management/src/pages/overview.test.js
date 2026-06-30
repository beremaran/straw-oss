import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { OverviewPage } from './overview.js'
import { state, showConfirm } from '../state.js'

vi.mock('../client.js', () => ({
  listEndpoints: vi.fn(),
  listRoutingRules: vi.fn(),
  listApiKeys: vi.fn(),
  getUsageSummary: vi.fn(),
  getBillingEstimate: vi.fn(),
  getCacheStats: vi.fn(),
  drainEndpoint: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(message, status) {
      super(message)
      this.status = status
    }
  }
}))

vi.mock('../state.js', () => {
  const state = {
    overviewData: null,
    overviewLoading: false,
    overviewErrors: null,
    confirmDialog: null
  }
  return {
    state,
    setState: vi.fn((changes) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  }
})

describe('Overview Dashboard Page', () => {
  let container

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    state.overviewData = {
      endpoints: [
        { id: 'ep-1', state: 'healthy', active_tasks: 2, tags: ['residential'] },
        { id: 'ep-2', state: 'draining', active_tasks: 0, tags: ['mobile'] }
      ],
      rules: [
        { name: 'Rule 1', is_active: true, priority: 10, required_tags: ['residential'] },
        { name: 'Rule 2', is_active: false, priority: 5, required_tags: [] }
      ],
      apiKeys: [
        { id: 'key-1', is_active: true },
        { id: 'key-2', is_active: false }
      ],
      usage: { total_requests: 1200, total_bytes: 409600, daily: [] },
      billing: { estimated_usd: 1.5 },
      cacheStats: { status: 'online' },
      fingerprints: []
    }
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
  })

  it('calculates metrics properly and renders them', () => {
    container.innerHTML = OverviewPage.render(state)

    expect(container.textContent).toContain('1 healthy')
    expect(container.textContent).toContain('Tasks: 2')
    expect(container.textContent).toContain('1/2') // Active rules count / total rules
    expect(container.textContent).toContain('$1.50')
    expect(container.textContent).toContain('Online')
  })

  it('renders endpoint rows and attention indicators', () => {
    container.innerHTML = OverviewPage.render(state)

    expect(container.querySelector('.badge-success').textContent).toBe('1 healthy') // endpoint healthy badge
    expect(container.textContent).toContain('Rule "Rule 2" is inactive')
  })

  it('triggers confirmation dialog for draining', () => {
    container.innerHTML = OverviewPage.render(state)
    OverviewPage.afterRender(state)

    const drainBtn = container.querySelector('.btn-drain')
    drainBtn.click()

    expect(showConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Drain Endpoint',
        body: expect.stringContaining('ep-1')
      })
    )
  })
})
