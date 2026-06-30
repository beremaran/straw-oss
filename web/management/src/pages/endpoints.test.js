import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EndpointsPage } from './endpoints.js'
import { state, showConfirm } from '../state.js'

vi.mock('../client.js', () => ({
  listEndpoints: vi.fn(),
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
    endpointsData: null,
    endpointsLoading: false,
    endpointsError: null
  }
  return {
    state,
    setState: vi.fn((changes) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  }
})

describe('Endpoints Monitoring Page', () => {
  let container

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)

    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-30T08:34:52.000Z'))

    state.endpointsData = [
      {
        id: 'ep-active',
        state: 'healthy',
        active_tasks: 5,
        tags: ['residential'],
        version: 'v1.0.0',
        last_seen: '2026-06-30T08:34:40.000Z'
      }, // 12s ago (Active)
      {
        id: 'ep-stale',
        state: 'suspect',
        active_tasks: 0,
        tags: ['mobile'],
        version: 'v1.0.0',
        last_seen: '2026-06-30T08:34:10.000Z'
      }, // 42s ago (Stale)
      {
        id: 'ep-draining',
        state: 'draining',
        active_tasks: 2,
        tags: ['datacenter'],
        version: 'v1.0.0',
        last_seen: '2026-06-30T08:34:45.000Z'
      }
    ]

    state.endpointsFilterStatus = 'all'
    state.endpointsFilterTag = ''
    state.endpointsFilterVersion = ''
    state.endpointsFilterAge = 'all'
    state.endpointsFilterSearch = ''
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
    vi.useRealTimers()
  })

  it('renders endpoints list properly', () => {
    container.innerHTML = EndpointsPage.render(state)

    expect(container.textContent).toContain('ep-active')
    expect(container.textContent).toContain('ep-stale')
    expect(container.textContent).toContain('Stale') // ep-stale tag status
    expect(container.textContent).toContain('draining')
  })

  it('filters endpoints by status client-side', () => {
    state.endpointsFilterStatus = 'draining'
    container.innerHTML = EndpointsPage.render(state)

    expect(container.textContent).not.toContain('ep-active')
    expect(container.textContent).toContain('ep-draining')
  })

  it('filters endpoints by stale age client-side', () => {
    state.endpointsFilterAge = 'stale'
    container.innerHTML = EndpointsPage.render(state)

    expect(container.textContent).not.toContain('ep-active')
    expect(container.textContent).toContain('ep-stale')
  })

  it('triggers confirmation dialog for draining', () => {
    container.innerHTML = EndpointsPage.render(state)
    EndpointsPage.afterRender(state)

    const drainBtn = container.querySelector('.btn-drain-endpoint')
    drainBtn.click()

    expect(showConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Drain Endpoint',
        body: expect.stringContaining('ep-active')
      })
    )
  })
})
