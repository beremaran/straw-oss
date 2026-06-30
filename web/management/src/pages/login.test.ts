import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { LoginPage } from './login.js'
import { state } from '../state.js'
import * as client from '../client.js'
import { mustQuery } from '../test-utils.js'

vi.mock('../client.js', () => ({
  healthCheck: vi.fn(),
  listApiKeys: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    method: string
    url: string

    constructor(message: string, method: string, url: string, status: number) {
      super(message)
      this.status = status
      this.method = method
      this.url = url
    }
  }
}))

describe('Login Page', () => {
  let container: HTMLElement

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    state.baseUrl = 'http://localhost:8081'
    state.token = ''
    state.loginError = null
    vi.clearAllMocks()
  })

  afterEach(() => {
    container.remove()
  })

  it('renders login form properly', () => {
    container.innerHTML = LoginPage.render(state)
    expect(mustQuery<HTMLInputElement>(container, 'input[type="url"]').value).toBe(
      'http://localhost:8081'
    )
    expect(mustQuery<HTMLInputElement>(container, 'input[type="password"]')).toBeTruthy()
  })

  it('validates missing url and token', async () => {
    container.innerHTML = LoginPage.render(state)
    LoginPage.afterRender(state)

    const form = mustQuery<HTMLFormElement>(container, 'form')
    mustQuery<HTMLInputElement>(container, 'input[type="url"]').value = ''

    form.dispatchEvent(new Event('submit'))

    expect(mustQuery<HTMLElement>(container, '#baseUrl-error').textContent).toContain(
      'API URL is required'
    )
  })

  it('handles successful login', async () => {
    vi.mocked(client.healthCheck).mockResolvedValueOnce('OK')
    vi.mocked(client.listApiKeys).mockResolvedValueOnce([])

    container.innerHTML = LoginPage.render(state)
    LoginPage.afterRender(state)

    mustQuery<HTMLInputElement>(container, 'input[type="url"]').value = 'http://localhost:8081'
    mustQuery<HTMLInputElement>(container, 'input[type="password"]').value = 'secret-token'

    const form = mustQuery<HTMLFormElement>(container, 'form')

    // Trigger submit and wait for async promise chain to run
    form.dispatchEvent(new Event('submit'))
    await new Promise((r) => setTimeout(r, 20))

    expect(state.token).toBe('secret-token')
    expect(state.baseUrl).toBe('http://localhost:8081')
  })

  it('handles 401 error as Invalid management token', async () => {
    vi.mocked(client.healthCheck).mockResolvedValueOnce('OK')
    vi.mocked(client.listApiKeys).mockRejectedValueOnce(
      new client.ApiError('Unauthorized', 'GET', 'http://localhost:8081/keys', 401, null, null)
    )

    container.innerHTML = LoginPage.render(state)
    LoginPage.afterRender(state)

    mustQuery<HTMLInputElement>(container, 'input[type="url"]').value = 'http://localhost:8081'
    mustQuery<HTMLInputElement>(container, 'input[type="password"]').value = 'invalid-token'

    const form = mustQuery<HTMLFormElement>(container, 'form')
    form.dispatchEvent(new Event('submit'))
    await new Promise((r) => setTimeout(r, 20))

    expect(state.token).toBe('')
    expect(state.loginError).toContain('Invalid management token')
  })
})
