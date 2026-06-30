// App Shell Component

import type { AppState } from '../types.js'

export function renderShell(state: AppState, contentHtml: string): string {
  const currentHash = state.currentPage || '#/overview'
  const navItems = [
    {
      hash: '#/overview',
      label: 'Overview',
      icon: 'M4 6a2 2 0 012-2h2a2 2 0 012 2v4a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v4a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v4a2 2 0 01-2 2H6a2 2 0 01-2-2v-4zM14 16a2 2 0 012-2h2a2 2 0 012 2v4a2 2 0 01-2 2h-2a2 2 0 01-2-2v-4z'
    },
    {
      hash: '#/api-keys',
      label: 'API Keys',
      icon: 'M15 7a2 2 0 012 2m-5-3a5 5 0 11-5 5 5 5 0 015-5zM19 12l2 2m-2-2l-2 2'
    },
    {
      hash: '#/routing-rules',
      label: 'Routing Rules',
      icon: 'M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4'
    },
    {
      hash: '#/endpoints',
      label: 'Endpoints',
      icon: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2'
    },
    {
      hash: '#/fingerprints',
      label: 'Fingerprints',
      icon: 'M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 009 11.5c0-.733.08-1.448.23-2.137M12 11a14.022 14.022 0 003.442 9.571M19 10a11.963 11.963 0 00-2.812-7.794M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m0 0A8.999 8.999 0 013 12m18 0a9 9 0 01-9 9m0-9c.274 0 .546-.015.813-.046M12 3a9 9 0 00-9 9'
    },
    {
      hash: '#/usage',
      label: 'Usage & Billing',
      icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z'
    },
    {
      hash: '#/cache',
      label: 'Cache Control',
      icon: 'M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4'
    },
    {
      hash: '#/system',
      label: 'System Info',
      icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z'
    }
  ]

  const lastRefreshed = state.lastRefreshed
    ? new Date(state.lastRefreshed).toLocaleTimeString()
    : 'Never'

  // Build the sidebar nav links
  const sidebarNavHtml = navItems
    .map((item) => {
      const isActive = currentHash.startsWith(item.hash) ? 'active' : ''
      return `
      <a href="${item.hash}" class="sidebar-nav-link ${isActive}" data-hash="${item.hash}" role="link" aria-current="${isActive ? 'page' : 'false'}">
        <svg class="sidebar-nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="${item.icon}" />
        </svg>
        <span class="sidebar-nav-text">${item.label}</span>
      </a>
    `
    })
    .join('')

  // Global Toasts / Dialogs rendering
  const toastHtml = state.toast
    ? `<div class="toast toast-${state.toast.type || 'info'} animate-slide-in" role="alert" aria-live="polite">
        <span class="toast-message">${state.toast.message}</span>
       </div>`
    : ''

  const confirmHtml = state.confirmDialog
    ? `<div class="modal-overlay active" role="dialog" aria-modal="true" aria-labelledby="confirm-dialog-title">
        <div class="modal-card animate-zoom-in">
          <div class="modal-header">
            <h3 class="modal-title" id="confirm-dialog-title">${state.confirmDialog.title}</h3>
          </div>
          <div class="modal-body">
            <p>${state.confirmDialog.body}</p>
            ${
              state.confirmDialog.requiresInput
                ? `<div class="form-group" style="margin-top: 1rem;">
                  <input type="text" id="confirm-input" class="form-control" placeholder="Type '${state.confirmDialog.confirmText}' to confirm..." aria-label="Confirmation text" />
                 </div>`
                : ''
            }
          </div>
          <div class="modal-footer">
            <button class="btn btn-secondary" id="confirm-cancel-btn" aria-label="Cancel confirmation">Cancel</button>
            <button class="btn btn-danger" id="confirm-ok-btn" ${state.confirmDialog.requiresInput ? 'disabled' : ''} aria-label="Confirm action">
              ${state.confirmDialog.loading ? '<span class="spinner"></span>' : 'Confirm'}
            </button>
          </div>
        </div>
       </div>`
    : ''

  return `
    <a href="#app-content" class="skip-link">Skip to main content</a>
    <div class="app-layout">
      <!-- App Sidebar -->
      <aside class="app-sidebar" id="app-sidebar" role="navigation" aria-label="Main navigation">
        <div class="sidebar-brand">
          <svg class="brand-logo" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          <div class="brand-details">
            <span class="brand-name">Straw Console</span>
            <span class="brand-badge">Local Node</span>
          </div>
        </div>
        <nav class="sidebar-nav" aria-label="Pages">
          ${sidebarNavHtml}
        </nav>
      </aside>

      <!-- App Content Wrapper -->
      <div class="app-main">
        <!-- App Top Bar -->
        <header class="app-topbar" role="banner">
          <div class="topbar-left">
            <button class="btn-icon sidebar-toggle-btn" id="sidebar-toggle" aria-label="Toggle Sidebar" title="Toggle navigation">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 20px; height: 20px;" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>
            <div class="connection-status" aria-label="Connection status">
              <span class="status-indicator online" aria-hidden="true"></span>
              <span class="connection-url">${escapeHtml(state.baseUrl)}</span>
            </div>
          </div>
          <div class="topbar-right">
            <span class="refresh-time" aria-label="Last refresh time">Refreshed: ${lastRefreshed}</span>
            <button class="btn btn-secondary btn-sm btn-icon-label" id="shell-refresh" title="Manual Refresh" aria-label="Refresh page data">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px;" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 1121.21 8H16" />
              </svg>
              <span>Refresh</span>
            </button>
            <button class="btn btn-secondary btn-sm" id="shell-sign-out" aria-label="Sign out">Sign Out</button>
          </div>
        </header>

        <!-- App View Main Content -->
        <main class="app-content-view" id="app-content" role="main" aria-label="Main content">
          ${contentHtml}
        </main>
      </div>
      
      <!-- Global Toasts -->
      ${toastHtml}

      <!-- Global Confirm Dialog -->
      ${confirmHtml}
    </div>
  `
}

function escapeHtml(str: string): string {
  if (!str) return ''
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}
