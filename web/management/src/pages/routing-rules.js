// Routing Rules List and Details Page

import { setState, showToast, showConfirm } from '../state.js'
import {
  listRoutingRules,
  getRoutingRule,
  deleteRoutingRule,
  updateRoutingRule,
  listEndpoints,
  listFingerprints
} from '../client.js'

export const RoutingRulesPage = {
  render(state) {
    const hash = state.currentPage || '#/routing-rules'

    // Check if we are viewing a specific rule's details
    const urlParams = new URLSearchParams(hash.includes('?') ? hash.split('?')[1] : '')
    const ruleId = urlParams.get('id')

    if (ruleId) {
      return this.renderDetails(state, ruleId)
    }

    return this.renderList(state)
  },

  renderList(state) {
    const data = state.rulesData || []

    const filterStatus = state.rulesFilterStatus || 'all'
    const filterTag = (state.rulesFilterTag || '').trim().toLowerCase()
    const filterPreset = state.rulesFilterPreset || 'all'
    const filterSearch = (state.rulesFilterSearch || '').trim().toLowerCase()

    // Client-side filtering
    const filteredRules = data.filter((r) => {
      if (filterStatus === 'active' && !r.is_active) return false
      if (filterStatus === 'inactive' && r.is_active) return false

      if (filterTag) {
        const required = r.required_tags || []
        const excluded = r.excluded_tags || []
        const hasTag = [...required, ...excluded].some((t) => t.toLowerCase().includes(filterTag))
        if (!hasTag) return false
      }

      if (filterPreset !== 'all') {
        if (filterPreset === 'none' && r.fingerprint_preset) return false
        if (filterPreset !== 'none' && r.fingerprint_preset !== filterPreset) return false
      }

      if (filterSearch) {
        const nameMatch = (r.name || '').toLowerCase().includes(filterSearch)
        const idMatch = (r.id || '').toLowerCase().includes(filterSearch)
        if (!nameMatch && !idMatch) return false
      }

      return true
    })

    // Extract presets list for dropdown filter
    const presets = Array.from(new Set(data.map((r) => r.fingerprint_preset).filter(Boolean)))

    // Attention check data (collect endpoints and fingerprints for verification)
    const endpoints = state.endpointsData || []
    const endpointIds = new Set(endpoints.map((e) => e.id))
    const fingerprints = state.fingerprintsData || []
    const fingerprintIds = new Set(fingerprints.map((f) => f.id))

    // Calculate active rule priorities count to detect duplicates
    const activePriorities = {}
    data.forEach((r) => {
      if (r.is_active) {
        activePriorities[r.priority] = (activePriorities[r.priority] || 0) + 1
      }
    })

    // Render table rows
    const rowsHtml = filteredRules
      .map((r) => {
        // Run attention checks
        const warnings = []
        if (r.is_active) {
          if (activePriorities[r.priority] > 1) {
            warnings.push(`Shares priority ${r.priority} with another active rule`)
          }
          if (!r.required_tags || r.required_tags.length === 0) {
            warnings.push('Active rule matches all traffic (no required tags)')
          }
          if (r.allow_insecure_tls) {
            warnings.push('Insecure TLS verification enabled')
          }
          if (r.fingerprint_preset && !fingerprintIds.has(r.fingerprint_preset)) {
            warnings.push(`Missing fingerprint preset: ${r.fingerprint_preset}`)
          }
          if (r.endpoint_pools) {
            r.endpoint_pools.forEach((pool) => {
              if (pool.endpoint_ids) {
                pool.endpoint_ids.forEach((epId) => {
                  if (!endpointIds.has(epId)) {
                    warnings.push(`Missing endpoint "${epId}" in pool tier ${pool.tier}`)
                  }
                })
              }
            })
          }
        }

        const warningIcon =
          warnings.length > 0
            ? `<span class="warning-indicator" title="${warnings.join('\n')}">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px; color: var(--color-amber-500);">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
           </span>`
            : ''

        const requiredTagsHtml = (r.required_tags || [])
          .map((t) => `<span class="badge badge-secondary badge-sm">${t}</span>`)
          .join(' ')
        const statusBadge = r.is_active
          ? `<span class="badge badge-success">Active</span>`
          : `<span class="badge badge-secondary">Inactive</span>`

        const fpMode = r.fingerprint_ab_test
          ? 'A/B Test'
          : r.fingerprint_preset
            ? `Preset (${r.fingerprint_preset})`
            : 'None'
        const rateLimitText = r.rate_limit_per_minute ? `${r.rate_limit_per_minute}/m` : 'None'

        return `
        <tr class="${r.is_active ? '' : 'row-disabled'}">
          <td><strong>${r.priority}</strong></td>
          <td>
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <div>
                <a href="#/routing-rules?id=${r.id}" class="rule-title-link font-medium">${r.name}</a>
                <div style="font-size: 0.75rem; color: var(--color-slate-400); margin-top: 0.15rem;">
                  ID: <code class="key-id">${r.id}</code>
                </div>
              </div>
              ${warningIcon}
            </div>
          </td>
          <td>${statusBadge}</td>
          <td><div class="tag-chips-cell">${requiredTagsHtml || '<span class="text-muted" style="font-size:0.85rem;">None</span>'}</div></td>
          <td>${fpMode}</td>
          <td>${rateLimitText}</td>
          <td>${r.hard_timeout || 'Default'}</td>
          <td>${r.quota_key || '<span class="text-muted">None</span>'}</td>
          <td style="text-align: right;">
            <div class="action-buttons-group">
              <button class="btn btn-secondary btn-xs btn-toggle-rule" data-id="${r.id}" data-active="${r.is_active ? 'true' : 'false'}">
                ${r.is_active ? 'Deactivate' : 'Activate'}
              </button>
              <a href="#/routing-rules/edit?id=${r.id}" class="btn btn-secondary btn-xs">Edit</a>
              <button class="btn btn-secondary btn-xs btn-duplicate-rule" data-id="${r.id}">Duplicate</button>
              <button class="btn btn-secondary btn-xs btn-danger btn-delete-rule" data-id="${r.id}" data-name="${r.name}">Delete</button>
            </div>
          </td>
        </tr>
      `
      })
      .join('')

    const noRulesHtml =
      filteredRules.length === 0
        ? `<tr><td colspan="9" class="table-empty">No routing rules match the current filters.</td></tr>`
        : ''

    return `
      <div class="page-header">
        <h2 class="page-title">Routing Rules</h2>
        <a href="#/routing-rules/new" class="btn btn-primary">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px; margin-right: 6px; display: inline-block; vertical-align: middle;">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          <span>Create Rule</span>
        </a>
      </div>

      <!-- Filters panel -->
      <div class="filter-bar card">
        <div class="filter-inputs">
          <div class="filter-group">
            <label for="filter-status" class="filter-label">Status</label>
            <select id="filter-status" class="form-control select-control">
              <option value="all" ${filterStatus === 'all' ? 'selected' : ''}>All Rules</option>
              <option value="active" ${filterStatus === 'active' ? 'selected' : ''}>Active Only</option>
              <option value="inactive" ${filterStatus === 'inactive' ? 'selected' : ''}>Inactive Only</option>
            </select>
          </div>

          <div class="filter-group">
            <label for="filter-tag" class="filter-label">Tag Contains</label>
            <input type="text" id="filter-tag" class="form-control text-control" value="${filterTag}" placeholder="e.g. residential" />
          </div>

          <div class="filter-group">
            <label for="filter-preset" class="filter-label">Fingerprint Preset</label>
            <select id="filter-preset" class="form-control select-control">
              <option value="all" ${filterPreset === 'all' ? 'selected' : ''}>All Presets</option>
              <option value="none" ${filterPreset === 'none' ? 'selected' : ''}>None</option>
              ${presets.map((p) => `<option value="${p}" ${filterPreset === p ? 'selected' : ''}>${p}</option>`).join('')}
            </select>
          </div>

          <div class="filter-group" style="flex-grow: 1;">
            <label for="filter-search" class="filter-label">Search</label>
            <input type="text" id="filter-search" class="form-control text-control" value="${filterSearch}" placeholder="Search rules by name or ID..." />
          </div>
        </div>
      </div>

      <!-- Rules Table -->
      <div class="card table-card" style="margin-top: 1.5rem;">
        <div class="table-responsive">
          <table class="table">
            <thead>
              <tr>
                <th style="width: 80px;">Priority</th>
                <th>Rule Name & ID</th>
                <th>Status</th>
                <th>Required Tags</th>
                <th>Fingerprints</th>
                <th>Rate Limit</th>
                <th>Timeout</th>
                <th>Quota Key</th>
                <th style="text-align: right; width: 280px;">Actions</th>
              </tr>
            </thead>
            <tbody>
              ${rowsHtml}
              ${noRulesHtml}
            </tbody>
          </table>
        </div>
      </div>
    `
  },

  renderDetails(state, ruleId) {
    const rule = state.selectedRuleDetail
    const isLoading = state.selectedRuleLoading

    if (isLoading || !rule || rule.id !== ruleId) {
      return `
        <div class="page-header">
          <a href="#/routing-rules" class="btn btn-secondary btn-sm" style="margin-right: 1rem;">Back to Rules</a>
          <h2 class="page-title">Loading Details...</h2>
        </div>
        <div class="card skeleton-card" style="height: 300px;">
          <div class="skeleton" style="width: 60%; height: 2rem; margin-bottom: 1rem;"></div>
          <div class="skeleton" style="width: 40%; height: 1.5rem; margin-bottom: 2rem;"></div>
          <div class="skeleton" style="width: 80%; height: 1rem; margin-bottom: 0.5rem;"></div>
          <div class="skeleton" style="width: 70%; height: 1rem;"></div>
        </div>
      `
    }

    const activeTab = state.selectedRuleTab || 'summary'

    const renderTabBtn = (tabId, label) => {
      const activeClass = activeTab === tabId ? 'active' : ''
      return `<button class="tab-btn ${activeClass}" data-tab="${tabId}">${label}</button>`
    }

    // Prepare tab contents
    let tabContentHtml = ''

    if (activeTab === 'summary') {
      tabContentHtml = `
        <div class="detail-grid">
          <div class="detail-field">
            <span class="detail-label">Rule ID</span>
            <span class="detail-value"><code>${rule.id}</code></span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Name</span>
            <span class="detail-value font-medium">${rule.name}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Priority</span>
            <span class="detail-value">${rule.priority}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Version</span>
            <span class="detail-value">${rule.version}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Status</span>
            <span class="detail-value">
              <span class="badge badge-${rule.is_active ? 'success' : 'secondary'}">
                ${rule.is_active ? 'Active' : 'Inactive'}
              </span>
            </span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Quota Key</span>
            <span class="detail-value">${rule.quota_key || 'None'}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Created At</span>
            <span class="detail-value">${rule.created_at ? new Date(rule.created_at).toLocaleString() : 'N/A'}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Last Updated At</span>
            <span class="detail-value">${rule.updated_at ? new Date(rule.updated_at).toLocaleString() : 'N/A'}</span>
          </div>
        </div>
      `
    } else if (activeTab === 'match') {
      const requiredTags = (rule.required_tags || [])
        .map((t) => `<span class="badge badge-secondary">${t}</span>`)
        .join(' ')
      const excludedTags = (rule.excluded_tags || [])
        .map(
          (t) =>
            `<span class="badge badge-secondary alert-danger" style="color:var(--color-rose-500); border-color:transparent;">${t}</span>`
        )
        .join(' ')
      const allowedEpTypes = (rule.allowed_endpoint_types || [])
        .map((t) => `<span class="badge badge-secondary">${t}</span>`)
        .join(' ')
      const requiredEpCaps = (rule.required_endpoint_caps || [])
        .map((t) => `<span class="badge badge-secondary">${t}</span>`)
        .join(' ')

      // Endpoint pools tier list
      const poolsHtml =
        (rule.endpoint_pools || [])
          .map(
            (pool) => `
        <div class="pool-tier card" style="margin-top: 0.5rem; padding: 0.75rem;">
          <div style="display:flex; justify-content:space-between; align-items:center;">
            <strong>Tier ${pool.tier}</strong>
            <span style="font-size:0.85rem; color:var(--color-slate-400);">Max Retries: ${pool.max_retries || 0}</span>
          </div>
          <div style="margin-top:0.5rem; font-size:0.9rem;">
            Endpoints: ${(pool.endpoint_ids || []).map((id) => `<code>${id}</code>`).join(', ') || '<span class="text-muted">None</span>'}
          </div>
        </div>
      `
          )
          .join('') ||
        '<span class="text-muted">No endpoint pools configured. Routing uses tag matching.</span>'

      tabContentHtml = `
        <div style="display:flex; flex-direction:column; gap: 1.5rem;">
          <div>
            <h4 style="margin-bottom:0.5rem; font-weight: 500;">Tag Selection Criteria</h4>
            <div class="detail-grid">
              <div class="detail-field">
                <span class="detail-label">Required Tags</span>
                <span class="detail-value">${requiredTags || '<span class="text-muted">None</span>'}</span>
              </div>
              <div class="detail-field">
                <span class="detail-label">Excluded Tags</span>
                <span class="detail-value">${excludedTags || '<span class="text-muted">None</span>'}</span>
              </div>
            </div>
          </div>

          <div>
            <h4 style="margin-bottom:0.5rem; font-weight: 500;">Endpoint Constraints</h4>
            <div class="detail-grid">
              <div class="detail-field">
                <span class="detail-label">Allowed Endpoint Types</span>
                <span class="detail-value">${allowedEpTypes || '<span class="text-muted">Any</span>'}</span>
              </div>
              <div class="detail-field">
                <span class="detail-label">Required Endpoint Capabilities</span>
                <span class="detail-value">${requiredEpCaps || '<span class="text-muted">None</span>'}</span>
              </div>
            </div>
          </div>

          <div>
            <h4 style="margin-bottom:0.25rem; font-weight: 500;">Endpoint Pools</h4>
            ${poolsHtml}
          </div>
        </div>
      `
    } else if (activeTab === 'limits') {
      tabContentHtml = `
        <div class="detail-grid">
          <div class="detail-field">
            <span class="detail-label">Hard Timeout</span>
            <span class="detail-value">${rule.hard_timeout || 'Default'}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Rate Limit (Per Minute)</span>
            <span class="detail-value">${rule.rate_limit_per_minute || 'None'}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Rate Limit (Per Second)</span>
            <span class="detail-value">${rule.rate_limit_per_second || 'None'}</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Allow Insecure TLS</span>
            <span class="detail-value">${rule.allow_insecure_tls ? '<span class="text-danger font-medium">Yes (Insecure)</span>' : 'No (Secure)'}</span>
          </div>
          <div class="detail-field" style="grid-column: span 2;">
            <span class="detail-label">Pinned Certificate Hash</span>
            <span class="detail-value"><code>${rule.pinned_cert_hash || 'None'}</code></span>
          </div>
        </div>
      `
    } else if (activeTab === 'fingerprints') {
      let fpDetailHtml = '<span class="text-muted">No fingerprinting overrides active.</span>'

      if (rule.fingerprint_preset) {
        fpDetailHtml = `
          <div class="detail-field">
            <span class="detail-label">Fingerprint Mode</span>
            <span class="detail-value">Preset</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Preset ID</span>
            <span class="detail-value"><code>${rule.fingerprint_preset}</code></span>
          </div>
        `
      } else if (rule.fingerprint_ab_test) {
        const ab = rule.fingerprint_ab_test
        const variantsHtml = (ab.variants || [])
          .map(
            (v) => `
          <tr>
            <td><code>${v.preset_id}</code></td>
            <td><strong>${v.weight}</strong></td>
          </tr>
        `
          )
          .join('')

        fpDetailHtml = `
          <div class="detail-field">
            <span class="detail-label">Fingerprint Mode</span>
            <span class="detail-value">A/B Test Overrides</span>
          </div>
          <div class="detail-field">
            <span class="detail-label">Selection Strategy</span>
            <span class="detail-value"><code>${ab.strategy || 'weighted'}</code></span>
          </div>
          <div style="grid-column: span 2; margin-top: 1rem;">
            <strong>Test Variants</strong>
            <table class="table dense-table" style="margin-top:0.5rem;">
              <thead>
                <tr>
                  <th>Preset ID</th>
                  <th>Weight</th>
                </tr>
              </thead>
              <tbody>
                ${variantsHtml}
              </tbody>
            </table>
          </div>
        `
      }

      tabContentHtml = `<div class="detail-grid">${fpDetailHtml}</div>`
    } else if (activeTab === 'filters') {
      const filters = rule.request_filters || {}
      const blockDomains = (filters.block_domains || [])
        .map((d) => `<span class="badge badge-secondary">${d}</span>`)
        .join(' ')
      const blockContentTypes = (filters.block_content_types || [])
        .map((c) => `<span class="badge badge-secondary">${c}</span>`)
        .join(' ')

      tabContentHtml = `
        <div style="display:flex; flex-direction:column; gap: 1.25rem;">
          <div class="detail-grid">
            <div class="detail-field">
              <span class="detail-label">Enable AdBlock Filters</span>
              <span class="detail-value">${filters.enable_adblock ? 'Enabled' : 'Disabled'}</span>
            </div>
          </div>
          
          <div class="detail-field">
            <span class="detail-label">AdBlock Filter Lists</span>
            <pre class="raw-json-panel" style="font-size:0.8rem; max-height: 100px;">${(filters.adblock_lists || []).join('\n') || 'None'}</pre>
          </div>

          <div class="detail-grid">
            <div class="detail-field">
              <span class="detail-label">Block Content Types</span>
              <span class="detail-value">${blockContentTypes || 'None'}</span>
            </div>
          </div>

          <div class="detail-field">
            <span class="detail-label">Blocked URL Patterns</span>
            <pre class="raw-json-panel" style="font-size:0.8rem; max-height: 100px;">${(filters.block_url_patterns || []).join('\n') || 'None'}</pre>
          </div>

          <div class="detail-field">
            <span class="detail-label">Blocked Domains</span>
            <span class="detail-value">${blockDomains || 'None'}</span>
          </div>
        </div>
      `
    } else if (activeTab === 'raw') {
      const ruleJson = JSON.stringify(rule, null, 2)
      tabContentHtml = `
        <div class="raw-json-actions" style="margin-bottom:0.75rem; display:flex; justify-content:flex-end;">
          <button class="btn btn-secondary btn-sm" id="btn-copy-rule-json" data-json="${encodeURIComponent(ruleJson)}">Copy JSON Payload</button>
        </div>
        <pre class="raw-json-panel"><code>${ruleJson}</code></pre>
      `
    }

    return `
      <div class="page-header">
        <div style="display:flex; align-items:center;">
          <a href="#/routing-rules" class="btn btn-secondary btn-sm" style="margin-right: 1.5rem;">Back to Rules</a>
          <div>
            <h2 class="page-title">${rule.name}</h2>
            <div style="font-size: 0.85rem; color: var(--color-slate-400); margin-top: 0.25rem;">
              Priority ${rule.priority} &bull; ID: <code>${rule.id}</code>
            </div>
          </div>
        </div>
        <div class="action-buttons-group">
          <a href="#/routing-rules/edit?id=${rule.id}" class="btn btn-primary">Edit Configuration</a>
        </div>
      </div>

      <div class="card detail-card" style="margin-top: 1.5rem;">
        <div class="tabs-container">
          ${renderTabBtn('summary', 'Summary')}
          ${renderTabBtn('match', 'Match & Routing')}
          ${renderTabBtn('limits', 'Limits & TLS')}
          ${renderTabBtn('fingerprints', 'Fingerprints')}
          ${renderTabBtn('filters', 'Request Filters')}
          ${renderTabBtn('raw', 'Raw JSON')}
        </div>
        
        <div class="tab-body" style="padding: 1.5rem;">
          ${tabContentHtml}
        </div>
      </div>
    `
  },

  async refresh(_) {
    setState({ rulesLoading: true, rulesError: null })
    try {
      const rules = await listRoutingRules({ limit: 100 })
      // Fetch endpoints and fingerprints to check rule validity warnings
      const endpoints = await listEndpoints().catch(() => [])
      const fingerprints = await listFingerprints().catch(() => [])

      setState({
        rulesData: rules,
        endpointsData: endpoints,
        fingerprintsData: fingerprints,
        rulesLoading: false
      })
    } catch (err) {
      setState({ rulesError: err.message, rulesLoading: false })
      showToast(`Failed to load routing rules: ${err.message}`, 'error')
    }
  },

  afterRender(state) {
    const hash = state.currentPage || '#/routing-rules'
    const urlParams = new URLSearchParams(hash.includes('?') ? hash.split('?')[1] : '')
    const ruleId = urlParams.get('id')

    if (ruleId) {
      this.afterRenderDetails(state, ruleId)
      return
    }

    this.afterRenderList(state)
  },

  afterRenderList(state) {
    if (!state.rulesData && !state.rulesLoading) {
      this.refresh(state)
      return
    }

    // Bind filters
    const bindFilter = (id, stateKey) => {
      const el = document.getElementById(id)
      if (el) {
        el.addEventListener('input', (e) => {
          setState({ [stateKey]: e.target.value })
        })
      }
    }
    bindFilter('filter-status', 'rulesFilterStatus')
    bindFilter('filter-tag', 'rulesFilterTag')
    bindFilter('filter-preset', 'rulesFilterPreset')
    bindFilter('filter-search', 'rulesFilterSearch')

    // Rule toggle active
    const toggleBtns = document.querySelectorAll('.btn-toggle-rule')
    toggleBtns.forEach((btn) => {
      btn.addEventListener('click', async () => {
        const id = btn.getAttribute('data-id')
        const isActive = btn.getAttribute('data-active') === 'true'

        try {
          const rule = state.rulesData.find((r) => r.id === id)
          if (!rule) return

          await updateRoutingRule(id, {
            ...rule,
            is_active: !isActive
          })

          showToast(`Rule "${rule.name}" ${!isActive ? 'activated' : 'deactivated'}`, 'success')
          this.refresh(state)
        } catch (err) {
          showToast(`Failed to update rule status: ${err.message}`, 'error')
        }
      })
    })

    // Rule Delete action
    const deleteBtns = document.querySelectorAll('.btn-delete-rule')
    deleteBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-id')
        const name = btn.getAttribute('data-name')

        showConfirm({
          title: 'Delete Routing Rule',
          body: `Are you sure you want to delete routing rule <strong>${name}</strong>? This is a soft-delete and sets the rule status to inactive.`,
          confirmText: 'confirm',
          callback: async () => {
            try {
              await deleteRoutingRule(id)
              showToast(`Routing rule "${name}" deleted`, 'success')
              this.refresh(state)
            } catch (err) {
              showToast(`Failed to delete rule: ${err.message}`, 'error')
            }
          }
        })
      })
    })

    // Rule Duplication action
    const duplicateBtns = document.querySelectorAll('.btn-duplicate-rule')
    duplicateBtns.forEach((btn) => {
      btn.addEventListener('click', async () => {
        const id = btn.getAttribute('data-id')
        try {
          const rule = await getRoutingRule(id)

          // Sanitize rule data: remove identity/timestamps for create flow
          const {
            id: _id,
            version: _version,
            created_at: _createdAt,
            updated_at: _updatedAt,
            ...sanitized
          } = rule

          // Save in state for the editor flow
          setState({
            editingRule: sanitized,
            editingRuleId: null // clear ID to mark as new
          })

          showToast(`Rule "${rule.name}" duplicated. Redirecting to form...`, 'success')
          window.location.hash = '#/routing-rules/new'
        } catch (err) {
          showToast(`Failed to duplicate rule: ${err.message}`, 'error')
        }
      })
    })
  },

  async afterRenderDetails(state, ruleId) {
    if (!state.selectedRuleDetail || state.selectedRuleDetail.id !== ruleId) {
      setState({ selectedRuleLoading: true })
      try {
        const rule = await getRoutingRule(ruleId)
        setState({ selectedRuleDetail: rule, selectedRuleLoading: false })
      } catch (err) {
        setState({ selectedRuleLoading: false })
        showToast(`Failed to load rule details: ${err.message}`, 'error')
        window.location.hash = '#/routing-rules'
        return
      }
    }

    // Bind tab clicks
    const tabBtns = document.querySelectorAll('.tab-btn')
    tabBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const tabId = btn.getAttribute('data-tab')
        setState({ selectedRuleTab: tabId })
      })
    })

    // Copy JSON button
    const copyJsonBtn = document.getElementById('btn-copy-rule-json')
    if (copyJsonBtn) {
      copyJsonBtn.addEventListener('click', () => {
        const json = decodeURIComponent(copyJsonBtn.getAttribute('data-json'))
        navigator.clipboard.writeText(json)
        showToast('Rule JSON payload copied to clipboard', 'success')
      })
    }
  }
}
