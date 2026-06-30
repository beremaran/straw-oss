import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { RoutingRuleEditorPage } from './routing-rule-editor.js';
import { state, showConfirm } from '../state.js';
import * as client from '../client.js';

vi.mock('../client.js', () => ({
  getRoutingRule: vi.fn(),
  createRoutingRule: vi.fn(),
  updateRoutingRule: vi.fn(),
  listEndpoints: vi.fn(),
  listFingerprints: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(message, status) {
      super(message);
      this.status = status;
    }
  }
}));

vi.mock('../state.js', () => {
  const state = {
    editingRule: null,
    editingRuleId: null,
    endpointsData: null,
    fingerprintsData: null,
    ruleJsonError: null,
    rulesLoading: false
  };
  return {
    state,
    setState: vi.fn((changes) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  };
});

describe('Routing Rule Editor Page', () => {
  let container;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    
    state.endpointsData = [
      { id: 'ep-1', state: 'healthy' }
    ];
    state.fingerprintsData = [
      { id: 'fp-chrome', name: 'Chrome Preset' }
    ];
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
    };
    state.editingRuleId = 'rule-123';
    vi.clearAllMocks();
  });

  afterEach(() => {
    container.remove();
  });

  it('renders form inputs with state values', () => {
    container.innerHTML = RoutingRuleEditorPage.render(state);
    
    expect(container.querySelector('#rule-name').value).toBe('Test Rule');
    expect(container.querySelector('#rule-priority').value).toBe('10');
    expect(container.querySelector('#rule-active').checked).toBe(true);
    expect(container.querySelector('#rule-fp-preset-select').value).toBe('fp-chrome');
  });

  it('displays validation error on invalid timeout duration', async () => {
    container.innerHTML = RoutingRuleEditorPage.render(state);
    RoutingRuleEditorPage.afterRender(state);

    const timeoutInput = container.querySelector('#rule-timeout');
    timeoutInput.value = '30 seconds'; // Natural language is invalid

    const form = container.querySelector('form');
    form.dispatchEvent(new Event('submit'));
    
    expect(container.querySelector('#rule-timeout-error').textContent).toContain('Invalid Go duration');
  });

  it('synchronizes input edits with raw JSON view', () => {
    container.innerHTML = RoutingRuleEditorPage.render(state);
    RoutingRuleEditorPage.afterRender(state);

    const nameInput = container.querySelector('#rule-name');
    nameInput.value = 'Updated Name';
    nameInput.dispatchEvent(new Event('input'));

    const jsonTextarea = container.querySelector('#raw-json-editor');
    expect(jsonTextarea.value).toContain('Updated Name');
  });

  it('triggers conflict confirmation on optimistic locking error (status 500)', async () => {
    client.updateRoutingRule.mockRejectedValueOnce(new client.ApiError('routing rule not found', 500));
    
    container.innerHTML = RoutingRuleEditorPage.render(state);
    RoutingRuleEditorPage.afterRender(state);

    const form = container.querySelector('form');
    form.dispatchEvent(new Event('submit'));
    
    await new Promise(r => setTimeout(r, 20));

    expect(showConfirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Version Conflict Detected',
        confirmText: 'Review Latest'
      })
    );
  });
});
