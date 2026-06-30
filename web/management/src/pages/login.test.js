import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { LoginPage } from './login.js';
import { state } from '../state.js';
import * as client from '../client.js';

vi.mock('../client.js', () => ({
  healthCheck: vi.fn(),
  listApiKeys: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(message, method, url, status) {
      super(message);
      this.status = status;
      this.method = method;
      this.url = url;
    }
  }
}));

describe('Login Page', () => {
  let container;
  
  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    state.baseUrl = 'http://localhost:8081';
    state.token = '';
    state.loginError = null;
    vi.clearAllMocks();
  });

  afterEach(() => {
    container.remove();
  });

  it('renders login form properly', () => {
    container.innerHTML = LoginPage.render(state);
    expect(container.querySelector('input[type="url"]').value).toBe('http://localhost:8081');
    expect(container.querySelector('input[type="password"]')).toBeTruthy();
  });

  it('validates missing url and token', async () => {
    container.innerHTML = LoginPage.render(state);
    LoginPage.afterRender(state);
    
    const form = container.querySelector('form');
    container.querySelector('input[type="url"]').value = '';
    
    form.dispatchEvent(new Event('submit'));
    
    expect(container.querySelector('#baseUrl-error').textContent).toContain('API URL is required');
  });

  it('handles successful login', async () => {
    client.healthCheck.mockResolvedValueOnce('OK');
    client.listApiKeys.mockResolvedValueOnce({ keys: [] });

    container.innerHTML = LoginPage.render(state);
    LoginPage.afterRender(state);

    container.querySelector('input[type="url"]').value = 'http://localhost:8081';
    container.querySelector('input[type="password"]').value = 'secret-token';

    const form = container.querySelector('form');
    
    // Trigger submit and wait for async promise chain to run
    form.dispatchEvent(new Event('submit'));
    await new Promise((r) => setTimeout(r, 20));

    expect(state.token).toBe('secret-token');
    expect(state.baseUrl).toBe('http://localhost:8081');
  });

  it('handles 401 error as Invalid management token', async () => {
    client.healthCheck.mockResolvedValueOnce('OK');
    client.listApiKeys.mockRejectedValueOnce(new client.ApiError('Unauthorized', 'GET', 'http://localhost:8081/keys', 401));

    container.innerHTML = LoginPage.render(state);
    LoginPage.afterRender(state);

    container.querySelector('input[type="url"]').value = 'http://localhost:8081';
    container.querySelector('input[type="password"]').value = 'invalid-token';

    const form = container.querySelector('form');
    form.dispatchEvent(new Event('submit'));
    await new Promise((r) => setTimeout(r, 20));

    expect(state.token).toBe('');
    expect(state.loginError).toContain('Invalid management token');
  });
});
