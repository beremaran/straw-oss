import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ApiKeysPage } from './api-keys.js'
import { state, showConfirm } from '../state.js'

vi.mock('../client.js', () => ({
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  revokeApiKey: vi.fn(),
  listEndpoints: vi.fn(),
  listRoutingRules: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(message, status) {
      super(message)
      this.status = status
    }
  }
}))

vi.mock('../state.js', () => {
  const state = {
    apiKeysData: null,
    apiKeysLoading: false,
    apiKeysError: null,
    apiKeysShowCreateModal: false,
    apiKeysRawKey: null,
    apiKeysNewChips: [],
    apiKeysBulkSelected: [],
    apiKeysSuggestions: []
  }
  return {
    state,
    setState: vi.fn((changes) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  }
})

describe('API Keys Management Page', () => {
  let container

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    state.apiKeysData = {
      keys: [
        {
          id: 'key-1',
          name: 'Production Key',
          is_active: true,
          scopes: ['region:us', 'type:residential'],
          rate_limit_override: 100
        },
        {
          id: 'key-2',
          name: 'Test Key',
          is_active: false,
          scopes: ['region:eu'],
          rate_limit_override: 0
        }
      ]
    }
    state.apiKeysFilterStatus = 'all'
    state.apiKeysFilterScope = ''
    state.apiKeysFilterSearch = ''
    state.apiKeysBulkSelected = []
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
  })

  it('renders keys list table properly', () => {
    container.innerHTML = ApiKeysPage.render(state)
    expect(container.textContent).toContain('Production Key')
    expect(container.textContent).toContain('Test Key')
    expect(container.textContent).toContain('region:us')
    expect(container.textContent).toContain('100/m')
  })

  it('filters keys by status and search query client-side', () => {
    state.apiKeysFilterStatus = 'active'
    container.innerHTML = ApiKeysPage.render(state)

    expect(container.textContent).toContain('Production Key')
    expect(container.textContent).not.toContain('Test Key')

    state.apiKeysFilterStatus = 'all'
    state.apiKeysFilterSearch = 'Production'
    container.innerHTML = ApiKeysPage.render(state)

    expect(container.textContent).toContain('Production Key')
    expect(container.textContent).not.toContain('Test Key')
  })

  it('verifies confirmation and call for revoking a key', () => {
    container.innerHTML = ApiKeysPage.render(state)
    ApiKeysPage.afterRender(state)

    const revokeBtn = container.querySelector('.btn-revoke-key')
    revokeBtn.click()

    expect(showConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Revoke API Key',
        body: expect.stringContaining('Production Key')
      })
    )
  })

  it('verifies bulk check selecting active keys', () => {
    container.innerHTML = ApiKeysPage.render(state)
    ApiKeysPage.afterRender(state)

    const checkAll = container.querySelector('#check-all-keys')
    checkAll.checked = true
    checkAll.dispatchEvent(new Event('change'))

    // Check if state is set to contains active key ID
    expect(vi.mocked(state.apiKeysBulkSelected)).toContain('key-1')
    expect(vi.mocked(state.apiKeysBulkSelected)).not.toContain('key-2') // inactive should not be selected
  })
})
