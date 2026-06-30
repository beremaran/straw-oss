import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { RoutingRuleEditorPage } from './routing-rule-editor.js'
import { state, showConfirm } from '../state.js'
import * as client from '../client.js'
import { mustQuery } from '../test-utils.js'

vi.mock('../client.js', () => ({
  getRoutingRule: vi.fn(),
  createRoutingRule: vi.fn(),
  updateRoutingRule: vi.fn(),
  listEndpoints: vi.fn(),
  listFingerprints: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    constructor(
      message: string,
      _methodOrStatus: string | number,
      _url = '',
      status = typeof _methodOrStatus === 'number' ? _methodOrStatus : 0
    ) {
      super(message)
      this.status = status
    }
  }
}))

vi.mock('../state.js', () => {
  const state = {
    editingRule: null,
    editingRuleId: null,
    endpointsData: null,
    fingerprintsData: null,
    ruleJsonError: null,
    rulesLoading: false
  }
  return {
    state,
    setState: vi.fn((changes: Record<string, unknown>) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  }
})

describe('Routing Rule Editor Page', () => {
  let container: HTMLElement

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)

    state.endpointsData = [{ id: 'ep-1', state: 'healthy' }]
    state.fingerprintsData = [{ id: 'fp-chrome', name: 'Chrome Preset' }]
    state.editingRule = {
      name: 'Test Rule',
      priority: 10,
      is_active: true,
      required_tags: ['residential'],
      excluded_tags: [],
      endpoint_pools: [],
      fingerprint_preset: 'fp-chrome',
      request_filters: {
        enable_adblock: true,
        adblock_lists: [],
        block_content_types: [],
        block_url_patterns: [],
        block_domains: []
      }
    }
    state.editingRuleId = 'rule-123'
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
  })

  it('renders form inputs with state values', () => {
    container.innerHTML = RoutingRuleEditorPage.render(state)

    expect(mustQuery<HTMLInputElement>(container, '#rule-name').value).toBe('Test Rule')
    expect(mustQuery<HTMLInputElement>(container, '#rule-priority').value).toBe('10')
    expect(mustQuery<HTMLInputElement>(container, '#rule-active').checked).toBe(true)
    expect(mustQuery<HTMLSelectElement>(container, '#rule-fp-preset-select').value).toBe(
      'fp-chrome'
    )
  })

  it('displays validation error on invalid timeout duration', async () => {
    container.innerHTML = RoutingRuleEditorPage.render(state)
    RoutingRuleEditorPage.afterRender(state)

    const timeoutInput = mustQuery<HTMLInputElement>(container, '#rule-timeout')
    timeoutInput.value = '30 seconds' // Natural language is invalid

    const form = mustQuery<HTMLFormElement>(container, 'form')
    form.dispatchEvent(new Event('submit'))

    expect(mustQuery<HTMLElement>(container, '#rule-timeout-error').textContent).toContain(
      'Invalid Go duration'
    )
  })

  it('synchronizes input edits with raw JSON view', () => {
    container.innerHTML = RoutingRuleEditorPage.render(state)
    RoutingRuleEditorPage.afterRender(state)

    const nameInput = mustQuery<HTMLInputElement>(container, '#rule-name')
    nameInput.value = 'Updated Name'
    nameInput.dispatchEvent(new Event('input'))

    const jsonTextarea = mustQuery<HTMLTextAreaElement>(container, '#raw-json-editor')
    expect(jsonTextarea.value).toContain('Updated Name')
  })

  it('triggers conflict confirmation on optimistic locking error (status 500)', async () => {
    vi.mocked(client.updateRoutingRule).mockRejectedValueOnce(
      new client.ApiError(
        'routing rule not found',
        'PUT',
        '/management/rules/rule-123',
        500,
        null,
        null
      )
    )

    container.innerHTML = RoutingRuleEditorPage.render(state)
    RoutingRuleEditorPage.afterRender(state)

    const form = mustQuery<HTMLFormElement>(container, 'form')
    form.dispatchEvent(new Event('submit'))

    await new Promise((r) => setTimeout(r, 20))

    expect(showConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Version Conflict Detected',
        confirmText: 'Review Latest'
      })
    )
  })
})
