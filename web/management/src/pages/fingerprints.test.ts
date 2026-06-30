import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { FingerprintsPage } from './fingerprints.js'
import { state, showConfirm } from '../state.js'
import { firstCall, mustQuery } from '../test-utils.js'

vi.mock('../client.js', () => ({
  listFingerprints: vi.fn(),
  createFingerprint: vi.fn(),
  broadcastFingerprints: vi.fn(),
  listRoutingRules: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    constructor(message: string, status: number) {
      super(message)
      this.status = status
    }
  }
}))

vi.mock('../state.js', () => {
  const state = {
    fingerprintsData: null,
    rulesData: null,
    fingerprintsLoading: false,
    fingerprintsShowModal: false,
    fingerprintsModalIsEdit: false,
    fingerprintsModalIsDuplicate: false,
    fingerprintsModalRule: null
  }
  return {
    state,
    setState: vi.fn((changes: Record<string, unknown>) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  }
})

describe('Fingerprints Presets Page', () => {
  let container: HTMLElement

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)

    state.fingerprintsData = [
      {
        id: 'chrome-desktop',
        name: 'Chrome Desktop Preset',
        config: { user_agent: 'Mozilla/5.0 Chrome/120.0.0.0 Safari/537.36' },
        updated_at: '2026-06-30T00:00:00Z'
      },
      {
        id: 'firefox-mobile',
        name: 'Firefox Mobile Preset',
        config: { user_agent: 'Mozilla/5.0 Android Firefox/115.0' },
        updated_at: '2026-06-30T00:00:00Z'
      }
    ]
    state.rulesData = [
      { id: 'rule-1', name: 'Rule 1', priority: 1, fingerprint_preset: 'chrome-desktop' },
      { id: 'rule-2', name: 'Rule 2', priority: 2, fingerprint_preset: 'chrome-desktop' }
    ]
    state.fingerprintsShowModal = false
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
  })

  it('renders presets lists and infers browser families', () => {
    container.innerHTML = FingerprintsPage.render(state)

    expect(container.textContent).toContain('chrome-desktop')
    expect(container.textContent).toContain('Chrome') // Browser family Chrome
    expect(container.textContent).toContain('Firefox') // Browser family Firefox
    expect(container.textContent).toContain('2 rules') // Usage count
    expect(container.textContent).toContain('0 rules') // Usage count for Firefox
  })

  it('opens create modal on button click', () => {
    container.innerHTML = FingerprintsPage.render(state)
    FingerprintsPage.afterRender(state)

    mustQuery<HTMLButtonElement>(container, '#btn-create-preset').click()
    expect(state.fingerprintsShowModal).toBe(true)
    expect(state.fingerprintsModalIsEdit).toBe(false)
  })

  it('triggers NATS broadcast confirmation dialog', () => {
    container.innerHTML = FingerprintsPage.render(state)
    FingerprintsPage.afterRender(state)

    mustQuery<HTMLButtonElement>(container, '#btn-broadcast-presets').click()
    const [options] = firstCall(vi.mocked(showConfirm))
    expect(options.title).toBe('Broadcast Presets')
    expect(options.body).toContain('NATS')
  })
})
