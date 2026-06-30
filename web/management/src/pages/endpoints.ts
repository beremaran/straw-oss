// Endpoints Monitoring Page

import { setState, showToast, showConfirm } from '../state.js'
import { listEndpoints, drainEndpoint } from '../client.js'
import { attr, errorMessage, eventValue } from '../utils.js'
import type { Page } from '../types.js'

export const EndpointsPage = {
  render(state) {
    const endpoints = state.endpointsData || []

    const filterStatus = state.endpointsFilterStatus || 'all'
    const filterTag = (state.endpointsFilterTag || '').trim().toLowerCase()
    const filterVersion = (state.endpointsFilterVersion || '').trim().toLowerCase()
    const filterAge = state.endpointsFilterAge || 'all'
    const filterSearch = (state.endpointsFilterSearch || '').trim().toLowerCase()

    // Client-side filtering
    const filteredEndpoints = endpoints.filter((ep) => {
      // 1. Status
      if (filterStatus !== 'all' && ep.state !== filterStatus) return false

      // 2. Tag
      if (filterTag) {
        const tags = ep.tags || []
        const hasTag = tags.some((t) => t.toLowerCase().includes(filterTag))
        if (!hasTag) return false
      }

      // 3. Version
      if (filterVersion) {
        const ver = (ep.version || '').toLowerCase()
        if (!ver.includes(filterVersion)) return false
      }

      // 4. Stale Age (Redis TTL default 60s)
      if (filterAge !== 'all' && ep.last_seen) {
        const lastSeenMs = Date.now() - new Date(ep.last_seen).getTime()
        if (filterAge === 'active' && lastSeenMs > 30000) return false
        if (filterAge === 'stale' && (lastSeenMs <= 30000 || lastSeenMs > 60000)) return false
        if (filterAge === 'disconnected' && lastSeenMs <= 60000) return false
      }

      // 5. Search ID
      if (filterSearch) {
        const id = (ep.id || '').toLowerCase()
        if (!id.includes(filterSearch)) return false
      }

      return true
    })

    const rowsHtml = filteredEndpoints
      .map((ep) => {
        const isDraining = ep.state === 'draining'
        const lastSeen = ep.last_seen ? new Date(ep.last_seen).toLocaleTimeString() : 'N/A'
        const tagsHtml = (ep.tags || [])
          .map(
            (t) => `
        <span class="badge badge-secondary badge-sm ep-tag-chip" data-tag="${t}" style="cursor: pointer;" title="Copy Tag">${t}</span>
      `
          )
          .join(' ')

        // Check if stale
        let staleIndicator = ''
        if (ep.last_seen) {
          const ageMs = Date.now() - new Date(ep.last_seen).getTime()
          if (ageMs > 60000) {
            staleIndicator = `<span class="badge badge-danger badge-sm" style="margin-left: 0.5rem;">Disconnected</span>`
          } else if (ageMs > 30000) {
            staleIndicator = `<span class="badge badge-warning badge-sm" style="margin-left: 0.5rem;">Stale</span>`
          }
        }

        return `
        <tr class="${isDraining ? 'row-disabled' : ''}">
          <td>
            <div style="display:flex; align-items:center; gap:0.5rem;">
              <code class="symbol-link btn-copy-id" data-copy="${ep.id}" style="cursor:pointer;" title="Copy ID">${ep.id}</code>
            </div>
          </td>
          <td>
            <span class="badge badge-${ep.state === 'healthy' ? 'success' : ep.state === 'suspect' ? 'warning' : ep.state === 'draining' ? 'info' : 'danger'}">${ep.state}</span>
            ${staleIndicator}
          </td>
          <td><strong>${ep.active_tasks || 0}</strong></td>
          <td><div class="tag-chips-cell">${tagsHtml || '<span class="text-muted">None</span>'}</div></td>
          <td><code>${ep.version || 'unknown'}</code></td>
          <td>${lastSeen}</td>
          <td style="text-align: right;">
            <div class="action-buttons-group" style="justify-content: flex-end;">
              <button class="btn btn-secondary btn-xs btn-copy-tags" data-tags="${(ep.tags || []).join(',')}" title="Copy Tag List">Copy Tags</button>
              <button class="btn btn-secondary btn-xs btn-find-rules" data-tags="${(ep.tags || []).join(',')}" title="Find matching rules">Matching Rules</button>
              <button class="btn btn-secondary btn-xs btn-danger btn-drain-endpoint" data-id="${ep.id}" data-tasks="${ep.active_tasks}" ${isDraining ? 'disabled' : ''}>
                ${isDraining ? 'Draining' : 'Drain'}
              </button>
            </div>
          </td>
        </tr>
      `
      })
      .join('')

    const noEndpointsHtml =
      filteredEndpoints.length === 0
        ? `<tr><td colspan="7" class="table-empty">No active endpoint nodes monitored.</td></tr>`
        : ''

    return `
      <div class="page-header">
        <h2 class="page-title">Active Endpoint Nodes</h2>
      </div>

      <!-- Filters Panel -->
      <div class="filter-bar card">
        <div class="filter-inputs">
          <div class="filter-group">
            <label for="filter-status" class="filter-label">State</label>
            <select id="filter-status" class="form-control select-control">
              <option value="all" ${filterStatus === 'all' ? 'selected' : ''}>All States</option>
              <option value="healthy" ${filterStatus === 'healthy' ? 'selected' : ''}>Healthy</option>
              <option value="suspect" ${filterStatus === 'suspect' ? 'selected' : ''}>Suspect</option>
              <option value="unhealthy" ${filterStatus === 'unhealthy' ? 'selected' : ''}>Unhealthy</option>
              <option value="draining" ${filterStatus === 'draining' ? 'selected' : ''}>Draining</option>
            </select>
          </div>

          <div class="filter-group">
            <label for="filter-tag" class="filter-label">Tag Name</label>
            <input type="text" id="filter-tag" class="form-control text-control" value="${filterTag}" placeholder="Filter by tag..." />
          </div>

          <div class="filter-group">
            <label for="filter-version" class="filter-label">Version</label>
            <input type="text" id="filter-version" class="form-control text-control" value="${filterVersion}" placeholder="Filter by version..." />
          </div>

          <div class="filter-group">
            <label for="filter-age" class="filter-label">Stale Age</label>
            <select id="filter-age" class="form-control select-control">
              <option value="all" ${filterAge === 'all' ? 'selected' : ''}>All Ages</option>
              <option value="active" ${filterAge === 'active' ? 'selected' : ''}>Active (&lt;30s)</option>
              <option value="stale" ${filterAge === 'stale' ? 'selected' : ''}>Stale (30s-60s)</option>
              <option value="disconnected" ${filterAge === 'disconnected' ? 'selected' : ''}>Offline (&gt;60s)</option>
            </select>
          </div>

          <div class="filter-group" style="flex-grow: 1;">
            <label for="filter-search" class="filter-label">Search ID</label>
            <input type="text" id="filter-search" class="form-control text-control" value="${filterSearch}" placeholder="Search endpoint by ID..." />
          </div>
        </div>
      </div>

      <!-- Endpoints Table -->
      <div class="card table-card" style="margin-top: 1.5rem;">
        <div class="table-responsive">
          <table class="table">
            <thead>
              <tr>
                <th>Endpoint ID</th>
                <th>State</th>
                <th>Active Tasks</th>
                <th>Tags</th>
                <th>Version</th>
                <th>Last Seen</th>
                <th style="text-align: right; width: 260px;">Actions</th>
              </tr>
            </thead>
            <tbody>
              ${rowsHtml}
              ${noEndpointsHtml}
            </tbody>
          </table>
        </div>
      </div>

      <!-- Info Footer for Unsupported Actions -->
      <div class="card info-card" style="margin-top: 2rem; border-left: 4px solid var(--color-slate-500); background: var(--color-slate-800); opacity: 0.85;">
        <div style="display:flex; gap:0.75rem; align-items:flex-start;">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 20px; height: 20px; color: var(--color-slate-400); flex-shrink:0; margin-top:0.1rem;">
            <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div>
            <strong style="display:block; font-size:0.9rem; margin-bottom:0.25rem;">Control Plane Notice</strong>
            <span style="font-size:0.825rem; color:var(--color-slate-300);">
              Undrain, node creation, node deletion, restart execution, live log streams, and metrics details are not supported first-release API actions. Operators must perform these tasks directly on worker hosts.
            </span>
          </div>
        </div>
      </div>
    `
  },

  async refresh(_) {
    setState({ endpointsLoading: true })
    try {
      const endpoints = await listEndpoints()
      setState({ endpointsData: endpoints, endpointsLoading: false })
    } catch (err) {
      setState({ endpointsLoading: false })
      showToast(`Failed to load endpoints: ${errorMessage(err)}`, 'error')
    }
  },

  afterRender(state) {
    if (!state.endpointsData && !state.endpointsLoading) {
      void EndpointsPage.refresh(state)
      return
    }

    // Bind filters
    const bindFilter = (id: string, stateKey: keyof typeof state) => {
      const el = document.getElementById(id)
      if (el) {
        el.addEventListener('input', (e) => {
          setState({ [stateKey]: eventValue(e) })
        })
      }
    }
    bindFilter('filter-status', 'endpointsFilterStatus')
    bindFilter('filter-tag', 'endpointsFilterTag')
    bindFilter('filter-version', 'endpointsFilterVersion')
    bindFilter('filter-age', 'endpointsFilterAge')
    bindFilter('filter-search', 'endpointsFilterSearch')

    // Copy ID triggers
    const copyIdBtns = document.querySelectorAll('.btn-copy-id')
    copyIdBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const val = attr(btn, 'data-copy')
        void navigator.clipboard.writeText(val)
        showToast('Endpoint ID copied', 'success')
      })
    })

    // Copy Tags triggers
    const copyTagsBtns = document.querySelectorAll('.btn-copy-tags')
    copyTagsBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const tags = attr(btn, 'data-tags')
        void navigator.clipboard.writeText(tags)
        showToast('Endpoint tags copied', 'success')
      })
    })

    // Individual chip copy trigger
    const tagChips = document.querySelectorAll('.ep-tag-chip')
    tagChips.forEach((chip) => {
      chip.addEventListener('click', (e) => {
        e.stopPropagation()
        const tag = attr(chip, 'data-tag')
        void navigator.clipboard.writeText(tag)
        showToast(`Tag '${tag}' copied`, 'success')
      })
    })

    // Find rules routing redirect
    const findRulesBtns = document.querySelectorAll('.btn-find-rules')
    findRulesBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const tags = attr(btn, 'data-tags')
        if (tags) {
          const firstTag = tags.split(',')[0]
          // Redirect to rules page with tag filter preset
          setState({ rulesFilterTag: firstTag })
          window.location.hash = '#/routing-rules'
        } else {
          showToast('Endpoint has no tags to match', 'warning')
        }
      })
    })

    // Drain endpoint trigger
    const drainBtns = document.querySelectorAll('.btn-drain-endpoint')
    drainBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = attr(btn, 'data-id')
        const tasks = attr(btn, 'data-tasks') || '0'

        showConfirm({
          title: 'Drain Endpoint',
          body: `Are you sure you want to drain endpoint node <strong>${id}</strong>? Active tasks (<strong>${tasks}</strong>) will be allowed to finish, but no new proxy traffic will route here. This action is irreversible.`,
          confirmText: 'confirm',
          callback: async () => {
            try {
              await drainEndpoint(id)
              showToast(`Endpoint ${id} is now draining`, 'success')

              // Optimistically update local view
              if (state.endpointsData) {
                state.endpointsData = state.endpointsData.map((ep) => {
                  if (ep.id === id) return { ...ep, state: 'draining' }
                  return ep
                })
                setState({ endpointsData: state.endpointsData })
              }
            } catch (err) {
              showToast(`Failed to drain endpoint: ${errorMessage(err)}`, 'error')
            }
          }
        })
      })
    })
  }
} satisfies Page
