// API Key Management Page

import { setState, showToast, showConfirm } from '../state.js'
import {
  listApiKeys,
  createApiKey,
  revokeApiKey,
  listEndpoints,
  listRoutingRules,
  ApiError
} from '../client.js'
import { validateScope, validatePositiveInteger } from '../validation.js'
import { attr, errorMessage, eventChecked, eventValue } from '../utils.js'
import type { ApiKey, AppState, Page } from '../types.js'

function apiKeyRows(data: AppState['apiKeysData']): ApiKey[] {
  if (Array.isArray(data)) return data
  return data?.keys || data?.data || []
}

export const ApiKeysPage = {
  render(state) {
    const keys = apiKeyRows(state.apiKeysData)

    // Filters from state
    const filterStatus = state.apiKeysFilterStatus || 'all'
    const filterScope = (state.apiKeysFilterScope || '').trim().toLowerCase()
    const filterSearch = (state.apiKeysFilterSearch || '').trim().toLowerCase()

    // Client-side filtering
    const filteredKeys = keys.filter((k) => {
      // 1. Status
      if (filterStatus === 'active' && !k.is_active) return false
      if (filterStatus === 'revoked' && k.is_active) return false

      // 2. Scope search
      if (filterScope) {
        const scopes = k.scopes || []
        const hasMatch = scopes.some((s) => s.toLowerCase().includes(filterScope))
        if (!hasMatch) return false
      }

      // 3. Name or ID search
      if (filterSearch) {
        const nameMatch = (k.name || '').toLowerCase().includes(filterSearch)
        const idMatch = (k.id || '').toLowerCase().includes(filterSearch)
        if (!nameMatch && !idMatch) return false
      }

      return true
    })

    const isBulkSelected = state.apiKeysBulkSelected || []
    const isAllSelected =
      filteredKeys.length > 0 &&
      filteredKeys.every((k) => k.is_active && isBulkSelected.includes(k.id))

    // Render Table rows
    const rowsHtml = filteredKeys
      .map((k) => {
        const isSelected = isBulkSelected.includes(k.id)
        const rowChecked = isSelected ? 'checked' : ''
        const scopesHtml = (k.scopes || [])
          .map((s) => `<span class="badge badge-secondary badge-sm">${s}</span>`)
          .join(' ')
        const rateLimitHtml = k.rate_limit_override
          ? `<strong>${k.rate_limit_override}</strong>/m`
          : '<span class="text-muted">None</span>'

        const createdStr = k.created_at ? new Date(k.created_at).toLocaleDateString() : 'N/A'
        const expiresStr = k.expires_at
          ? new Date(k.expires_at).toLocaleDateString()
          : '<span class="text-muted">Never</span>'
        const statusBadge = k.is_active
          ? `<span class="badge badge-success">Active</span>`
          : `<span class="badge badge-danger">Revoked</span>`

        return `
        <tr class="${k.is_active ? '' : 'row-disabled'}">
          <td>
            ${
              k.is_active
                ? `<input type="checkbox" class="key-select-check" data-id="${k.id}" ${rowChecked} />`
                : `<input type="checkbox" disabled />`
            }
          </td>
          <td>
            <div class="key-name-cell">
              <span class="key-name font-medium">${k.name}</span>
              <div class="key-id-row">
                <code class="key-id">${k.id}</code>
                <button class="btn-copy-id" data-copy="${k.id}" title="Copy Key ID">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 12px; height: 12px;">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                  </svg>
                </button>
              </div>
            </div>
          </td>
          <td>${statusBadge}</td>
          <td><div class="tag-chips-cell">${scopesHtml}</div></td>
          <td>${rateLimitHtml}</td>
          <td>${createdStr}</td>
          <td>${expiresStr}</td>
          <td style="text-align: right;">
            ${
              k.is_active
                ? `<button class="btn btn-secondary btn-xs btn-danger btn-revoke-key" data-id="${k.id}" data-name="${k.name}">Revoke</button>`
                : `<span class="text-muted" style="font-size: 0.85rem;">Inactive</span>`
            }
          </td>
        </tr>
      `
      })
      .join('')

    const noKeysHtml =
      filteredKeys.length === 0
        ? `<tr><td colspan="8" class="table-empty">No API keys match the current filters.</td></tr>`
        : ''

    // Suggestions collected from endpoints and rules
    const liveTags = state.apiKeysScopeSuggestions || []
    const suggestionsHtml = liveTags
      .map(
        (tag) => `
      <button type="button" class="btn btn-secondary btn-xs tag-suggestion-btn" data-tag="${tag}">${tag}</button>
    `
      )
      .join(' ')

    // Create Modal HTML
    const createModalHtml = state.apiKeysShowCreateModal
      ? `<div class="modal-overlay active">
          <div class="modal-card animate-zoom-in" style="max-width: 500px;">
            <div class="modal-header">
              <h3 class="modal-title">Create API Key</h3>
            </div>
            <form id="create-key-form" novalidate>
              <div class="modal-body">
                <div class="form-group">
                  <label for="new-key-name" class="form-label">Key Name</label>
                  <input type="text" id="new-key-name" class="form-control" placeholder="Production crawler key, Scraping agent, etc." required />
                  <div class="invalid-feedback" id="new-key-name-error"></div>
                </div>

                <div class="form-group">
                  <label class="form-label">Scopes (Endpoint / Traffic Tags)</label>
                  <div class="chip-input-container">
                    <div id="new-key-chips" class="chip-input-chips"></div>
                    <input type="text" id="new-key-scope-input" class="chip-input-field" placeholder="Type tag (e.g. region:us) and press Enter..." />
                  </div>
                  <div class="invalid-feedback" id="new-key-scope-error" style="display: block; margin-top: 0.25rem;"></div>
                  
                  <div class="suggestions-box" style="margin-top: 0.5rem;">
                    <div class="suggestions-label">Suggestions from live tags:</div>
                    <div class="suggestions-list">${suggestionsHtml || '<span class="text-muted">No live tags detected</span>'}</div>
                  </div>
                </div>

                <div class="form-group">
                  <label for="new-key-rate-limit" class="form-label">Rate Limit Override (requests/min)</label>
                  <input type="number" id="new-key-rate-limit" class="form-control" placeholder="Default node rate limit applies if blank" min="1" />
                  <div class="invalid-feedback" id="new-key-rate-limit-error"></div>
                </div>
              </div>
              <div class="modal-footer">
                <button type="button" class="btn btn-secondary" id="btn-create-cancel">Cancel</button>
                <button type="submit" class="btn btn-primary">Generate Credentials</button>
              </div>
            </form>
          </div>
         </div>`
      : ''

    // Success Modal HTML (displays raw_key once)
    const successModalHtml = state.apiKeysRawKey
      ? `<div class="modal-overlay active">
          <div class="modal-card animate-zoom-in" style="max-width: 500px;">
            <div class="modal-header">
              <h3 class="modal-title" style="color: var(--color-emerald-500);">Save this API key now</h3>
            </div>
            <div class="modal-body">
              <p style="margin-bottom: 1.25rem; font-size: 0.9rem;">
                This key is only returned <strong>once</strong>. Store it securely; you will not be able to retrieve or view it again.
              </p>
              
              <div class="form-group">
                <label class="form-label">Raw API Key</label>
                <div class="raw-key-box">
                  <input type="password" id="raw-key-display" class="form-control" value="${state.apiKeysRawKey}" readonly />
                  <button class="btn btn-secondary btn-sm" id="btn-reveal-raw-key">Reveal</button>
                  <button class="btn btn-secondary btn-sm" id="btn-copy-raw-key">Copy</button>
                </div>
              </div>

              <div class="form-group form-check-group" style="margin-top: 1.5rem;">
                <label class="form-check-label">
                  <input type="checkbox" id="raw-key-saved-check" />
                  <strong>I have saved this API key in a secure location</strong>
                </label>
              </div>
            </div>
            <div class="modal-footer">
              <button class="btn btn-secondary" id="btn-download-raw-key">Download Key File</button>
              <button class="btn btn-success" id="btn-close-raw-key" disabled>Close</button>
            </div>
          </div>
         </div>`
      : ''

    return `
      <div class="page-header">
        <h2 class="page-title">Client API Keys</h2>
        <button class="btn btn-primary" id="btn-open-create-key">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px; margin-right: 6px; display: inline-block; vertical-align: middle;">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          <span>Create API Key</span>
        </button>
      </div>

      <!-- Filters & Bulk actions bar -->
      <div class="filter-bar card">
        <div class="filter-inputs">
          <div class="filter-group">
            <label for="filter-status" class="filter-label">Status</label>
            <select id="filter-status" class="form-control select-control">
              <option value="all" ${filterStatus === 'all' ? 'selected' : ''}>All Keys</option>
              <option value="active" ${filterStatus === 'active' ? 'selected' : ''}>Active</option>
              <option value="revoked" ${filterStatus === 'revoked' ? 'selected' : ''}>Revoked</option>
            </select>
          </div>

          <div class="filter-group">
            <label for="filter-scope" class="filter-label">Scope Contains</label>
            <input type="text" id="filter-scope" class="form-control text-control" value="${filterScope}" placeholder="e.g. region:us" />
          </div>

          <div class="filter-group" style="flex-grow: 1;">
            <label for="filter-search" class="filter-label">Search</label>
            <input type="text" id="filter-search" class="form-control text-control" value="${filterSearch}" placeholder="Search by name or key ID..." />
          </div>
        </div>

        ${
          isBulkSelected.length > 0
            ? `<div class="bulk-actions-strip animate-fade-in">
              <span><strong>${isBulkSelected.length}</strong> keys selected</span>
              <button class="btn btn-danger btn-xs" id="btn-bulk-revoke">Bulk Revoke</button>
             </div>`
            : ''
        }
      </div>

      <!-- Keys Table -->
      <div class="card table-card" style="margin-top: 1.5rem;">
        <div class="table-responsive">
          <table class="table">
            <thead>
              <tr>
                <th style="width: 40px;">
                  <input type="checkbox" id="check-all-keys" ${rowChecked(isAllSelected)} ${filteredKeys.length === 0 ? 'disabled' : ''} />
                </th>
                <th>API Key Name & ID</th>
                <th>Status</th>
                <th>Scopes</th>
                <th>Rate Limit</th>
                <th>Created</th>
                <th>Expires</th>
                <th style="text-align: right;">Actions</th>
              </tr>
            </thead>
            <tbody>
              ${rowsHtml}
              ${noKeysHtml}
            </tbody>
          </table>
        </div>
      </div>

      <!-- Modals -->
      ${createModalHtml}
      ${successModalHtml}
    `
  },

  async refresh(_) {
    setState({ apiKeysLoading: true, apiKeysError: null })
    try {
      const keys = await listApiKeys({ limit: 100 })
      setState({ apiKeysData: keys, apiKeysLoading: false })

      // Gather tags suggestions asynchronously
      const endpoints = await listEndpoints().catch(() => [])
      const rules = await listRoutingRules({ limit: 100 }).catch(() => [])

      const suggestions = new Set([
        '*',
        'target:*',
        'region:us',
        'region:eu',
        'type:residential',
        'type:datacenter'
      ])
      endpoints.forEach((ep) => {
        ;(ep.tags || []).forEach((t) => suggestions.add(t))
      })
      rules.forEach((r) => {
        ;(r.required_tags || []).forEach((t) => suggestions.add(t))
        ;(r.excluded_tags || []).forEach((t) => suggestions.add(t))
      })
      setState({ apiKeysScopeSuggestions: Array.from(suggestions) })
    } catch (err) {
      setState({ apiKeysError: errorMessage(err), apiKeysLoading: false })
      showToast(`Failed to load API keys: ${errorMessage(err)}`, 'error')
    }
  },

  afterRender(state) {
    if (!state.apiKeysData && !state.apiKeysLoading) {
      void ApiKeysPage.refresh(state)
      return
    }

    // Bind filters
    const bindFilter = (id: string, stateKey: keyof AppState) => {
      const el = document.getElementById(id)
      if (el) {
        el.addEventListener('input', (e) => {
          setState({ [stateKey]: eventValue(e) })
        })
      }
    }
    bindFilter('filter-status', 'apiKeysFilterStatus')
    bindFilter('filter-scope', 'apiKeysFilterScope')
    bindFilter('filter-search', 'apiKeysFilterSearch')

    // Create modal open
    const openCreateBtn = document.getElementById('btn-open-create-key')
    if (openCreateBtn) {
      openCreateBtn.addEventListener('click', () => {
        setState({ apiKeysShowCreateModal: true, apiKeysNewChips: [] })
      })
    }

    // Modal forms and input elements
    if (state.apiKeysShowCreateModal) {
      const cancelBtn = document.getElementById('btn-create-cancel')
      if (cancelBtn) {
        cancelBtn.addEventListener('click', () => {
          setState({ apiKeysShowCreateModal: false })
        })
      }

      const scopeInput = document.getElementById('new-key-scope-input') as HTMLInputElement
      const chipsContainer = document.getElementById('new-key-chips') as HTMLElement
      const scopeError = document.getElementById('new-key-scope-error') as HTMLElement

      const renderChips = () => {
        const chips = state.apiKeysNewChips || []
        chipsContainer.innerHTML = chips
          .map(
            (c, i) => `
          <span class="chip">
            <span>${c}</span>
            <button type="button" class="btn-remove-chip" data-idx="${i}">&times;</button>
          </span>
        `
          )
          .join('')

        // Bind removals
        chipsContainer.querySelectorAll('.btn-remove-chip').forEach((btn) => {
          btn.addEventListener('click', () => {
            const idx = parseInt(attr(btn, 'data-idx'))
            const updated = [...(state.apiKeysNewChips || [])]
            updated.splice(idx, 1)
            setState({ apiKeysNewChips: updated })
          })
        })
      }
      renderChips()

      // Suggestions binding
      const suggestionBtns = document.querySelectorAll('.tag-suggestion-btn')
      suggestionBtns.forEach((btn) => {
        btn.addEventListener('click', () => {
          const tag = attr(btn, 'data-tag')
          const chips = state.apiKeysNewChips || []
          if (!chips.includes(tag)) {
            try {
              validateScope(tag)
              setState({ apiKeysNewChips: [...chips, tag] })
            } catch (err) {
              scopeError.textContent = errorMessage(err)
            }
          }
        })
      })

      scopeInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ',') {
          e.preventDefault()
          const val = scopeInput.value.trim()
          if (val) {
            try {
              validateScope(val)
              scopeError.textContent = ''
              const chips = state.apiKeysNewChips || []
              if (!chips.includes(val)) {
                setState({ apiKeysNewChips: [...chips, val] })
              }
              scopeInput.value = ''
            } catch (err) {
              scopeError.textContent = errorMessage(err)
            }
          }
        }
      })

      const createForm = document.getElementById('create-key-form')
      if (createForm) {
        createForm.addEventListener('submit', async (e) => {
          e.preventDefault()
          const nameInput = document.getElementById('new-key-name') as HTMLInputElement
          const rateLimitInput = document.getElementById('new-key-rate-limit') as HTMLInputElement

          const nameErr = document.getElementById('new-key-name-error') as HTMLElement
          const limitErr = document.getElementById('new-key-rate-limit-error') as HTMLElement

          nameErr.textContent = ''
          limitErr.textContent = ''
          nameInput.classList.remove('is-invalid')
          rateLimitInput.classList.remove('is-invalid')

          let isValid = true
          const nameVal = nameInput.value.trim()
          const limitVal = rateLimitInput.value.trim()

          if (!nameVal) {
            nameErr.textContent = 'Key Name is required.'
            nameInput.classList.add('is-invalid')
            isValid = false
          } else if (nameVal.length > 120) {
            nameErr.textContent = 'Name must be 120 characters or less.'
            nameInput.classList.add('is-invalid')
            isValid = false
          }

          if (limitVal) {
            try {
              validatePositiveInteger(limitVal, 'Rate limit override')
            } catch (err) {
              limitErr.textContent = errorMessage(err)
              rateLimitInput.classList.add('is-invalid')
              isValid = false
            }
          }

          if (!isValid) {
            const firstInvalid = createForm.querySelector<HTMLElement>('.is-invalid')
            if (firstInvalid) firstInvalid.focus()
            return
          }

          try {
            const payload: { name: string; scopes: string[]; rate_limit_override?: number } = {
              name: nameVal,
              scopes: state.apiKeysNewChips || []
            }
            if (limitVal) {
              payload.rate_limit_override = parseInt(limitVal)
            }

            const response = await createApiKey(payload)
            showToast('API Key generated successfully', 'success')

            // Success! Close creation modal and open raw key display modal
            setState({
              apiKeysShowCreateModal: false,
              apiKeysRawKey: response.raw_key
            })
            // Reload keys list in background
            void ApiKeysPage.refresh(state)
          } catch (err) {
            showToast(`Failed to create API key: ${errorMessage(err)}`, 'error')
          }
        })
      }
    }

    // Success Modal logic
    if (state.apiKeysRawKey) {
      const displayInput = document.getElementById('raw-key-display') as HTMLInputElement | null
      const revealBtn = document.getElementById('btn-reveal-raw-key')
      const copyBtn = document.getElementById('btn-copy-raw-key')
      const savedCheckbox = document.getElementById(
        'raw-key-saved-check'
      ) as HTMLInputElement | null
      const closeBtn = document.getElementById('btn-close-raw-key') as HTMLButtonElement | null
      const downloadBtn = document.getElementById('btn-download-raw-key')

      if (revealBtn && displayInput) {
        revealBtn.addEventListener('click', () => {
          if (displayInput.type === 'password') {
            displayInput.type = 'text'
            revealBtn.textContent = 'Hide'
          } else {
            displayInput.type = 'password'
            revealBtn.textContent = 'Reveal'
          }
        })
      }

      if (copyBtn) {
        copyBtn.addEventListener('click', () => {
          void navigator.clipboard.writeText(state.apiKeysRawKey || '')
          showToast('Raw key copied to clipboard', 'success')
        })
      }

      if (downloadBtn) {
        downloadBtn.addEventListener('click', () => {
          const blob = new Blob([state.apiKeysRawKey || ''], { type: 'text/plain' })
          const a = document.createElement('a')
          a.href = URL.createObjectURL(blob)
          a.download = `straw-key-${Date.now()}.txt`
          document.body.appendChild(a)
          a.click()
          document.body.removeChild(a)
        })
      }

      if (savedCheckbox && closeBtn) {
        savedCheckbox.addEventListener('change', (e) => {
          closeBtn.disabled = !eventChecked(e)
        })

        closeBtn.addEventListener('click', () => {
          setState({ apiKeysRawKey: null })
        })
      }
    }

    // Row selection and check-all
    const checkAll = document.getElementById('check-all-keys')
    const rowChecks = document.querySelectorAll<HTMLInputElement>('.key-select-check')

    if (checkAll) {
      checkAll.addEventListener('change', (e) => {
        if (eventChecked(e)) {
          // Select all visible active keys
          const keys = apiKeyRows(state.apiKeysData)
          const activeIds = keys.filter((k) => k.is_active).map((k) => k.id)
          setState({ apiKeysBulkSelected: activeIds })
        } else {
          setState({ apiKeysBulkSelected: [] })
        }
      })
    }

    rowChecks.forEach((ch) => {
      ch.addEventListener('change', (_) => {
        const id = attr(ch, 'data-id')
        let selected = [...(state.apiKeysBulkSelected || [])]
        if (ch.checked) {
          if (!selected.includes(id)) selected.push(id)
        } else {
          selected = selected.filter((sid) => sid !== id)
        }
        setState({ apiKeysBulkSelected: selected })
      })
    })

    // Copy Key ID triggers
    const copyIdBtns = document.querySelectorAll('.btn-copy-id')
    copyIdBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const val = attr(btn, 'data-copy')
        void navigator.clipboard.writeText(val)
        showToast('ID copied to clipboard', 'success')
      })
    })

    // Single Key Revocation
    const revokeBtns = document.querySelectorAll('.btn-revoke-key')
    revokeBtns.forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = attr(btn, 'data-id')
        const name = attr(btn, 'data-name')

        showConfirm({
          title: 'Revoke API Key',
          body: `Are you sure you want to revoke API key <strong>${name}</strong>? Clients using this credential will lose proxy access immediately. This action is irreversible.`,
          confirmText: 'confirm',
          callback: async () => {
            try {
              await revokeApiKey(id)
              showToast(`API Key ${name} revoked`, 'success')

              // Optimistically update key in loaded list
              if (state.apiKeysData) {
                setState({
                  apiKeysData: apiKeyRows(state.apiKeysData).map((k) =>
                    k.id === id ? { ...k, is_active: false } : k
                  )
                })
              }
            } catch (err) {
              if (err instanceof ApiError && err.status === 404) {
                showToast(`Key already removed or revoked`, 'warning')
                void ApiKeysPage.refresh(state)
              } else {
                showToast(`Failed to revoke key: ${errorMessage(err)}`, 'error')
              }
            }
          }
        })
      })
    })

    // Bulk Revoke action
    const bulkRevokeBtn = document.getElementById('btn-bulk-revoke')
    if (bulkRevokeBtn) {
      bulkRevokeBtn.addEventListener('click', () => {
        const selectedIds = state.apiKeysBulkSelected || []
        showConfirm({
          title: 'Bulk Revoke API Keys',
          body: `Are you sure you want to revoke all <strong>${selectedIds.length}</strong> selected API keys? Clients using these credentials will lose proxy access immediately. This action is irreversible.`,
          confirmText: 'confirm',
          callback: async () => {
            let succeeded = 0
            let failed = 0
            const errors: string[] = []

            for (const id of selectedIds) {
              try {
                await revokeApiKey(id)
                succeeded++
              } catch (err) {
                failed++
                errors.push(`${id}: ${errorMessage(err)}`)
              }
            }

            if (failed === 0) {
              showToast(`Successfully revoked ${succeeded} keys`, 'success')
            } else if (succeeded > 0) {
              showToast(`Partial failure: Revoked ${succeeded} keys, ${failed} failed.`, 'warning')
            } else {
              showToast(`Failed to revoke selected keys.`, 'error')
            }

            setState({ apiKeysBulkSelected: [] })
            void ApiKeysPage.refresh(state)
          }
        })
      })
    }
  }
} satisfies Page

function rowChecked(val: boolean): string {
  return val ? 'checked' : ''
}
