// Straw Management UI Hash Router

import { state, setState } from './state.js'
import { LoginPage } from './pages/login.js'
import { OverviewPage } from './pages/overview.js'
import { ApiKeysPage } from './pages/api-keys.js'
import { RoutingRulesPage } from './pages/routing-rules.js'
import { RoutingRuleEditorPage } from './pages/routing-rule-editor.js'
import { EndpointsPage } from './pages/endpoints.js'
import { FingerprintsPage } from './pages/fingerprints.js'
import { UsagePage } from './pages/usage.js'
import { CachePage } from './pages/cache.js'
import { SystemPage } from './pages/system.js'
import { renderShell } from './components/shell.js'
import { eventValue } from './utils.js'
import type { Page } from './types.js'

const routes: Record<string, Page> = {
  '#/login': LoginPage,
  '#/overview': OverviewPage,
  '#/api-keys': ApiKeysPage,
  '#/routing-rules': RoutingRulesPage,
  '#/routing-rules/new': RoutingRuleEditorPage,
  '#/routing-rules/edit': RoutingRuleEditorPage,
  '#/endpoints': EndpointsPage,
  '#/fingerprints': FingerprintsPage,
  '#/usage': UsagePage,
  '#/cache': CachePage,
  '#/system': SystemPage
}

export function initRouter() {
  window.addEventListener('hashchange', handleRouteChange)
  handleRouteChange()
}

export function handleRouteChange() {
  const hash = window.location.hash || '#/overview'

  let routeKey = hash
  if (hash.startsWith('#/routing-rules/edit')) {
    routeKey = '#/routing-rules/edit'
  } else if (
    hash.startsWith('#/routing-rules/') &&
    hash !== '#/routing-rules/new' &&
    !hash.includes('?')
  ) {
    routeKey = '#/routing-rules/edit'
  }

  const hasToken = !!state.token
  if (!hasToken && routeKey !== '#/login') {
    window.location.hash = '#/login'
    return
  }
  if (hasToken && routeKey === '#/login') {
    window.location.hash = '#/overview'
    return
  }

  const page = routes[routeKey] || OverviewPage
  if (state.currentPage !== hash) {
    setState({ currentPage: hash })
  }

  const appDiv = document.querySelector<HTMLElement>('#app')
  if (!appDiv) return
  if (routeKey === '#/login') {
    appDiv.innerHTML = page.render(state)
    if (page.afterRender) void page.afterRender(state)
  } else {
    appDiv.innerHTML = renderShell(state, page.render(state))
    attachShellEvents()
    if (page.afterRender) void page.afterRender(state)
  }
}

function attachShellEvents() {
  // Mobile sidebar toggle
  const toggleBtn = document.getElementById('sidebar-toggle')
  const sidebar = document.getElementById('app-sidebar')
  if (toggleBtn && sidebar) {
    toggleBtn.addEventListener('click', () => {
      sidebar.classList.toggle('active')
    })
  }

  // Sign out
  const signOutBtn = document.getElementById('shell-sign-out')
  if (signOutBtn) {
    signOutBtn.addEventListener('click', (e) => {
      e.preventDefault()
      void import('./state.js').then(({ clearSession }) => {
        clearSession()
        window.location.hash = '#/login'
      })
    })
  }

  // Refresh button
  const refreshBtn = document.getElementById('shell-refresh')
  if (refreshBtn) {
    refreshBtn.addEventListener('click', (e) => {
      e.preventDefault()
      const hash = window.location.hash || '#/overview'
      let routeKey = hash
      if (
        hash.startsWith('#/routing-rules/edit') ||
        (hash.startsWith('#/routing-rules/') &&
          hash !== '#/routing-rules/new' &&
          !hash.includes('?'))
      ) {
        routeKey = '#/routing-rules/edit'
      }
      const page = routes[routeKey]
      if (page && page.refresh) {
        void page.refresh(state)
      }
    })
  }

  // Confirmation dialog actions
  const confirmCancel = document.getElementById('confirm-cancel-btn')
  if (confirmCancel) {
    confirmCancel.addEventListener('click', () => {
      void import('./state.js').then(({ closeConfirm }) => closeConfirm())
    })
  }

  const confirmInput = document.getElementById('confirm-input')
  const confirmOk = document.getElementById('confirm-ok-btn') as HTMLButtonElement | null
  if (confirmInput && confirmOk) {
    confirmInput.addEventListener('input', (e) => {
      confirmOk.disabled = eventValue(e) !== state.confirmDialog?.confirmText
    })
  }

  if (confirmOk) {
    confirmOk.addEventListener('click', () => {
      const dialog = state.confirmDialog
      if (dialog?.callback) {
        // Set loading state on the button
        confirmOk.disabled = true
        confirmOk.innerHTML = '<span class="spinner"></span>'

        void Promise.resolve(dialog.callback()).finally(() => {
          void import('./state.js').then(({ closeConfirm }) => closeConfirm())
        })
      }
    })
  }
}
