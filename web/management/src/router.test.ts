import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { handleRouteChange } from './router.js'
import { state, subscribe } from './state.js'

describe('Router & App Shell Guarding', () => {
  let appDiv: HTMLElement

  beforeEach(() => {
    appDiv = document.createElement('div')
    appDiv.id = 'app'
    document.body.appendChild(appDiv)
    state.baseUrl = 'http://localhost:8081'
    state.token = ''
    state.currentPage = '#/overview'
    window.location.hash = ''
  })

  afterEach(() => {
    appDiv.remove()
  })

  it('redirects to #/login if token is missing and trying to access a protected route', () => {
    window.location.hash = '#/overview'
    handleRouteChange()
    expect(window.location.hash).toBe('#/login')
  })

  it('redirects to #/overview if token is present and trying to access #/login', () => {
    state.token = 'valid-token'
    window.location.hash = '#/login'
    handleRouteChange()
    expect(window.location.hash).toBe('#/overview')
  })

  it('renders app shell and current page content for authenticated routes', () => {
    state.token = 'valid-token'
    window.location.hash = '#/overview'
    handleRouteChange()

    expect(document.querySelector('.app-layout')).toBeTruthy()
    expect(document.querySelector('.app-sidebar')).toBeTruthy()
    expect(document.getElementById('shell-sign-out')).toBeTruthy()
  })

  it('does not recurse when a state subscriber rerenders the current route', () => {
    state.token = 'valid-token'
    state.currentPage = '#/login'
    state.overviewData = {
      endpoints: [],
      rules: [],
      apiKeys: [],
      usage: {},
      billing: {},
      cacheStats: {},
      fingerprints: []
    }
    window.location.hash = '#/overview'

    const unsubscribe = subscribe(() => handleRouteChange())
    try {
      expect(() => handleRouteChange()).not.toThrow()
      expect(state.currentPage).toBe('#/overview')
    } finally {
      unsubscribe()
    }
  })
})
