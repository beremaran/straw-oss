import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { CachePage } from './cache.js';
import { state, showConfirm, showToast } from '../state.js';
import * as client from '../client.js';

vi.mock('../client.js', () => ({
  getCacheStats: vi.fn(),
  clearCache: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(message, status) {
      super(message);
      this.status = status;
    }
  }
}));

vi.mock('../state.js', () => {
  const state = {};
  return {
    state,
    setState: vi.fn((changes) => Object.assign(state, changes)),
    showToast: vi.fn(),
    showConfirm: vi.fn()
  };
});

describe('Cache Control Page', () => {
  let container;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    // Reset all cache-related state
    state.cacheData = null;
    state.cacheLoading = false;
    state.cacheError = null;
    state.cacheUnavailable = false;
    state.cacheClearPattern = '*';
    state.cacheClearConfirmText = '';
    state.cacheClearLoading = false;
    state.cacheClearResult = null;
    state.cacheInfoSearch = '';
    vi.clearAllMocks();
  });

  afterEach(() => {
    container.remove();
  });

  it('renders unavailable state when Redis is not configured', () => {
    state.cacheUnavailable = true;
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Cache Unavailable');
    expect(container.textContent).toContain('Redis is not configured');
  });

  it('renders error state with retry button', () => {
    state.cacheError = 'Connection refused';
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Failed to Load Cache Stats');
    expect(container.textContent).toContain('Retry');
  });

  it('renders loading skeleton when fetching', () => {
    state.cacheLoading = true;
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Cache Control');
    expect(container.querySelector('.skeleton-card')).toBeTruthy();
  });

  it('renders quick facts from Redis INFO', () => {
    state.cacheData = {
      info: 'redis_version:7.2.0\nused_memory_human:1.50G\nconnected_clients:42\nkeyspace_hits:100000\nkeyspace_misses:5000'
    };
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Redis Version');
    expect(container.textContent).toContain('7.2.0');
    expect(container.textContent).toContain('Used Memory');
    expect(container.textContent).toContain('1.50G');
    expect(container.textContent).toContain('Connected Clients');
    expect(container.textContent).toContain('42');
    expect(container.textContent).toContain('Keyspace Hits');
    expect(container.textContent).toContain('100000');
    expect(container.textContent).toContain('Keyspace Misses');
    expect(container.textContent).toContain('5000');
  });

  it('renders clear cache form with wildcard confirmation', () => {
    state.cacheData = { info: 'redis_version:7.2.0' };
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Clear Cache');
    expect(container.textContent).toContain('CLEAR ALL');
    expect(container.textContent).toContain('prevents accidental full cache flush');
  });

  it('renders clear cache form with pattern confirmation', () => {
    state.cacheData = { info: 'redis_version:7.2.0' };
    state.cacheClearPattern = 'prefix:*';
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Clear Cache');
    expect(container.textContent).toContain('Confirm pattern');
    // Check the pattern is in the input value attribute
    const patternInput = container.querySelector('#cache-clear-pattern');
    expect(patternInput?.value).toBe('prefix:*');
  });

  it('filters Redis INFO text on search', () => {
    state.cacheData = {
      info: 'redis_version:7.2.0\nused_memory_human:1.50G\nconnected_clients:42'
    };
    state.cacheInfoSearch = 'memory';
    container.innerHTML = CachePage.render(state);
    const infoText = container.querySelector('#cache-info-text')?.textContent || '';
    expect(infoText).toContain('used_memory_human');
    expect(infoText).not.toContain('redis_version');
    expect(infoText).not.toContain('connected_clients');
  });

  it('shows clear result with deleted count', () => {
    state.cacheData = { info: 'redis_version:7.2.0' };
    state.cacheClearResult = { pattern: '*', deleted: 150 };
    container.innerHTML = CachePage.render(state);
    expect(container.textContent).toContain('Cache Cleared');
    expect(container.textContent).toContain('150');
    expect(container.textContent).toContain('*');
  });
});
