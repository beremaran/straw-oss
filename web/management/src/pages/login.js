// Login Page implementation

import { setState, showToast } from '../state.js'
import { healthCheck, listApiKeys, ApiError } from '../client.js'

export const LoginPage = {
  render(state) {
    const errorHtml = state.loginError
      ? `<div class="alert alert-error" role="alert" style="margin-bottom: 1.5rem;">
          <div class="alert-title">Connection Failed</div>
          <div class="alert-body" style="font-size: 0.875rem;">${state.loginError}</div>
         </div>`
      : ''

    return `
      <div class="login-wrapper">
        <div class="login-card">
          <div class="login-header">
            <svg class="login-logo" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
            <h1 class="login-title">Straw Console</h1>
            <p class="login-subtitle">Management Control Surface</p>
          </div>
          
          ${errorHtml}

          <form id="login-form" novalidate>
            <div class="form-group">
              <label for="baseUrl" class="form-label">Management API URL</label>
              <input type="url" id="baseUrl" class="form-control" value="${state.baseUrl || 'http://localhost:8081'}" required />
              <div class="invalid-feedback" id="baseUrl-error"></div>
            </div>

            <div class="form-group">
              <label for="token" class="form-label">Management Token</label>
              <input type="password" id="token" class="form-control" placeholder="Enter bearer token..." required />
              <div class="invalid-feedback" id="token-error"></div>
            </div>

            <div class="form-group form-check-group">
              <label class="form-check-label">
                <input type="checkbox" id="remember" ${state.remember ? 'checked' : ''} />
                <span>Remember this connection</span>
              </label>
              <div class="checkbox-warning">
                Warning: Credentials will be stored in localStorage and may be accessible to browser extensions.
              </div>
            </div>

            <button type="submit" class="btn btn-primary btn-block" id="btn-login-submit" style="margin-top: 1.5rem;">
              <span class="btn-text">Connect to Node</span>
              <span class="spinner" id="login-spinner" style="display: none;"></span>
            </button>
          </form>
        </div>
      </div>
    `
  },

  afterRender(state) {
    const form = document.getElementById('login-form')
    if (!form) return

    form.addEventListener('submit', async (e) => {
      e.preventDefault()

      const baseUrlInput = document.getElementById('baseUrl')
      const tokenInput = document.getElementById('token')
      const rememberCheckbox = document.getElementById('remember')
      const submitBtn = document.getElementById('btn-login-submit')
      const spinner = document.getElementById('login-spinner')

      // Clean previous errors
      setState({ loginError: null })
      document.getElementById('baseUrl-error').textContent = ''
      document.getElementById('token-error').textContent = ''
      baseUrlInput.classList.remove('is-invalid')
      tokenInput.classList.remove('is-invalid')

      let isValid = true

      const baseUrlVal = baseUrlInput.value.trim()
      const tokenVal = tokenInput.value.trim()

      if (!baseUrlVal) {
        document.getElementById('baseUrl-error').textContent = 'API URL is required.'
        baseUrlInput.classList.add('is-invalid')
        isValid = false
      } else {
        try {
          new URL(baseUrlVal)
        } catch {
          document.getElementById('baseUrl-error').textContent =
            'API URL must include a protocol (e.g. http:// or https://).'
          baseUrlInput.classList.add('is-invalid')
          isValid = false
        }
      }

      if (!tokenVal) {
        document.getElementById('token-error').textContent = 'Management token is required.'
        tokenInput.classList.add('is-invalid')
        isValid = false
      }

      if (!isValid) {
        // Focus first invalid field
        const firstInvalid = form.querySelector('.is-invalid')
        if (firstInvalid) firstInvalid.focus()
        return
      }

      // Start loading
      submitBtn.disabled = true
      spinner.style.display = 'inline-block'

      // Temporarily set baseUrl and token for verification calls
      const oldBaseUrl = state.baseUrl
      const oldToken = state.token

      state.baseUrl = baseUrlVal
      state.token = tokenVal

      try {
        // Step 1: Health check (public endpoint)
        try {
          await healthCheck()
        } catch (err) {
          throw new Error(
            `Reachability check failed: Could not connect to API at ${baseUrlVal}. Verify the URL, port, CORS settings, or network configuration.`,
            { cause: err }
          )
        }

        // Step 2: Authenticated test request
        try {
          await listApiKeys({ limit: 1 })
        } catch (err) {
          if (err instanceof ApiError && err.status === 401) {
            throw new Error('Invalid management token. Access denied by Straw Node.', {
              cause: err
            })
          }
          throw err
        }

        // Verification successful! Save settings
        const remember = rememberCheckbox.checked
        setState({
          baseUrl: baseUrlVal,
          token: tokenVal,
          remember,
          loginError: null
        })

        if (remember) {
          localStorage.setItem('straw_baseUrl', baseUrlVal)
          localStorage.setItem('straw_token', tokenVal)
          localStorage.setItem('straw_remember', 'true')
        } else {
          localStorage.setItem('straw_baseUrl', baseUrlVal)
          localStorage.setItem('straw_remember', 'false')
          localStorage.removeItem('straw_token')
        }

        showToast('Connected to Straw Node', 'success')
        window.location.hash = '#/overview'
      } catch (err) {
        // Restore old credentials
        state.baseUrl = oldBaseUrl
        state.token = oldToken

        setState({ loginError: err.message })
        submitBtn.disabled = false
        spinner.style.display = 'none'

        // Re-render to show error message
        const appDiv = document.querySelector('#app') || form.parentElement
        if (appDiv) {
          appDiv.innerHTML = LoginPage.render(state)
          LoginPage.afterRender(state)
        }
      }
    })
  }
}
