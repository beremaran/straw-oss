// Usage & Billing Page

import { setState, showToast } from '../state.js'
import { getUsageSummary, getBillingEstimate, listApiKeys, ApiError } from '../client.js'
import { validateDate } from '../validation.js'
import { errorMessage, eventValue } from '../utils.js'
import type { Page } from '../types.js'

export const UsagePage = {
  render(state) {
    const usageData = state.usageData || null
    const billingData = state.billingData || null
    const isLoading = state.usageLoading
    const error = state.usageError
    const apiKeys = state.apiKeysSuggestions || []

    // Filters
    const datePreset = state.usageDatePreset || '7d'
    const customStart = state.usageCustomStart || ''
    const customEnd = state.usageCustomEnd || ''
    const selectedApiKey = state.usageApiKeyFilter || ''
    const dateError = state.usageDateError || ''
    const viewMode = state.usageViewMode || 'chart' // 'chart' or 'table'

    // Compute effective dates
    const getFormattedDate = (daysAgo: number) => {
      const d = new Date()
      d.setDate(d.getDate() - daysAgo)
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    }

    const today = getFormattedDate(0)
    let effectiveStart, effectiveEnd

    switch (datePreset) {
      case '7d':
        effectiveStart = getFormattedDate(7)
        effectiveEnd = today
        break
      case '30d':
        effectiveStart = getFormattedDate(30)
        effectiveEnd = today
        break
      case 'mtd':
        effectiveStart = `${new Date().getFullYear()}-${String(new Date().getMonth() + 1).padStart(2, '0')}-01`
        effectiveEnd = today
        break
      case 'custom':
        effectiveStart = customStart
        effectiveEnd = customEnd
        break
      default:
        effectiveStart = getFormattedDate(7)
        effectiveEnd = today
    }

    // Format bytes
    const formatBytes = (bytes?: number) => {
      if (!bytes || bytes === 0) return '0 B'
      const k = 1024
      const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
      const i = Math.floor(Math.log(bytes) / Math.log(k))
      return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
    }

    // Usage totals
    const totalRequests = usageData?.total_requests || 0
    const totalBytes = usageData?.total_bytes || 0
    const totalCostUnits = usageData?.total_cost_units || 0
    const dailyData = usageData?.daily || []

    // Billing totals
    const billingCostUnits = billingData?.total_cost_units || 0
    const billingUsd = billingData?.estimated_usd ?? 0
    const billingCurrency = billingData?.currency || 'USD'

    // Empty state check
    const hasNoData = !usageData && !state.usageError

    // Render chart (bar chart)
    const chartHtml =
      dailyData.length > 0
        ? (() => {
            const maxRequests = Math.max(...dailyData.map((d) => d.requests || 0), 1)
            const barWidth = Math.min(40, Math.floor(600 / dailyData.length))
            const gap = Math.max(
              2,
              Math.floor((600 - barWidth * dailyData.length) / (dailyData.length + 1))
            )
            const bars = dailyData
              .map((d, _) => {
                const requests = d.requests || 0
                const height = Math.max(2, (requests / maxRequests) * 120)
                const dateLabel = d.date ? d.date.slice(5) : '' // MM-DD
                return `
              <div style="display: inline-flex; flex-direction: column; align-items: center; margin: 0 ${gap / 2}px;">
                <span style="font-size: 11px; color: var(--text); margin-bottom: 2px;">${requests.toLocaleString()}</span>
                <div style="width: ${barWidth}px; height: ${height}px; background: var(--accent); border-radius: 2px 2px 0 0; min-height: 2px;"></div>
                <span style="font-size: 10px; color: var(--text); margin-top: 2px;">${dateLabel}</span>
              </div>
            `
              })
              .join('')
            return `<div style="overflow-x: auto; padding: 8px 0;">${bars}</div>`
          })()
        : ''

    // Render table
    const tableRows =
      dailyData.length > 0
        ? dailyData
            .map((d) => {
              const breakdownHtml = (d.breakdown || [])
                .map(
                  (b) =>
                    `<span style="font-size: 12px; color: var(--text);">${b.tier || 'unknown'}: ${b.requests || 0}</span>`
                )
                .join(' &middot; ')
              return `
            <tr>
              <td>${d.date || 'N/A'}</td>
              <td>${(d.requests || 0).toLocaleString()}</td>
              <td>${formatBytes(d.bytes || 0)}</td>
              <td>${(d.cost_units || 0).toFixed(4)}</td>
              <td>${breakdownHtml}</td>
            </tr>
          `
            })
            .join('')
        : ''

    // API key filter options
    const apiKeyOptions = apiKeys
      .map(
        (k) =>
          `<option value="${k.id}" ${k.id === selectedApiKey ? 'selected' : ''}>${k.name || k.id}</option>`
      )
      .join('')

    // Loading state
    if (isLoading && !usageData && !billingData) {
      return `
        <div class="page-header">
          <h2 class="page-title">Usage & Billing</h2>
        </div>
        <div class="card skeleton-card">
          <div class="skeleton skeleton-title"></div>
          <div class="skeleton skeleton-value"></div>
          <div class="skeleton skeleton-text"></div>
        </div>
        <div class="card skeleton-card">
          <div class="skeleton skeleton-title"></div>
          <div class="skeleton skeleton-value"></div>
          <div class="skeleton skeleton-text"></div>
        </div>
      `
    }

    // Error state
    if (error && !usageData && !billingData) {
      return `
        <div class="page-header">
          <h2 class="page-title">Usage & Billing</h2>
        </div>
        <div class="alert alert-error" role="alert">
          <div class="alert-title">Failed to load usage data</div>
          <div class="alert-body">${error}</div>
          <button class="btn btn-secondary btn-sm" id="usage-retry-btn">Retry</button>
        </div>
      `
    }

    // Empty state
    if (hasNoData) {
      return `
        <div class="page-header">
          <h2 class="page-title">Usage & Billing</h2>
        </div>
        <div class="card">
          <div class="empty-state">
            <svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" style="width: 48px; height: 48px;">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3 3h18v18H3z M9 17v-2m3 2v-4m3 4v-6m2 10H7" />
            </svg>
            <h3>No usage data</h3>
            <p>No usage has been summarized for this range. This could mean: new installation, no traffic, or the summary job has not populated data yet.</p>
          </div>
        </div>
      `
    }

    return `
      <div class="page-header">
        <h2 class="page-title">Usage & Billing</h2>
      </div>

      <!-- Filters -->
      <div class="card" style="margin-bottom: 1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Filters</h3>
        </div>
        <div class="filter-row">
          <div class="filter-group">
            <label for="usage-date-preset">Date range</label>
            <select id="usage-date-preset" class="form-control">
              <option value="7d" ${datePreset === '7d' ? 'selected' : ''}>Last 7 days</option>
              <option value="30d" ${datePreset === '30d' ? 'selected' : ''}>Last 30 days</option>
              <option value="mtd" ${datePreset === 'mtd' ? 'selected' : ''}>Month to date</option>
              <option value="custom" ${datePreset === 'custom' ? 'selected' : ''}>Custom</option>
            </select>
          </div>
          <div class="filter-group" id="usage-custom-dates" style="${datePreset !== 'custom' ? 'display: none;' : ''}">
            <label for="usage-custom-start">Start</label>
            <input type="text" id="usage-custom-start" class="form-control" placeholder="YYYY-MM-DD" value="${customStart}" />
            <label for="usage-custom-end" style="margin-top: 8px;">End</label>
            <input type="text" id="usage-custom-end" class="form-control" placeholder="YYYY-MM-DD" value="${customEnd}" />
          </div>
          <div class="filter-group">
            <label for="usage-api-key">API key</label>
            <select id="usage-api-key" class="form-control">
              <option value="">All keys</option>
              ${apiKeyOptions}
            </select>
          </div>
          <div class="filter-group" style="align-self: flex-end;">
            <button class="btn btn-primary" id="usage-apply-btn">Apply</button>
          </div>
        </div>
        ${dateError ? `<div class="alert alert-error" style="margin-top: 8px; margin-bottom: 0;">${dateError}</div>` : ''}
      </div>

      <!-- Usage Summary -->
      <div class="card" style="margin-bottom: 1.5rem;">
        <div class="card-header" style="display: flex; justify-content: space-between; align-items: center;">
          <h3 class="card-title">Usage Summary</h3>
          <div>
            <button class="btn btn-secondary btn-sm ${viewMode === 'chart' ? 'active' : ''}" id="usage-view-chart">Chart</button>
            <button class="btn btn-secondary btn-sm ${viewMode === 'table' ? 'active' : ''}" id="usage-view-table">Table</button>
            <button class="btn btn-secondary btn-sm" id="usage-export-csv">Export CSV</button>
          </div>
        </div>
        <div class="summary-stats">
          <div class="stat-item">
            <span class="stat-label">Total Requests</span>
            <span class="stat-value">${totalRequests.toLocaleString()}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Total Bytes</span>
            <span class="stat-value">${formatBytes(totalBytes)}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Cost Units</span>
            <span class="stat-value">${totalCostUnits.toFixed(4)}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Date Range</span>
            <span class="stat-value" style="font-size: 14px;">${effectiveStart} &mdash; ${effectiveEnd}</span>
          </div>
        </div>
        <div class="chart-area" id="usage-chart-area" style="${viewMode === 'table' ? 'display: none;' : ''}">
          ${chartHtml}
        </div>
        <div class="table-area" id="usage-table-area" style="${viewMode === 'chart' ? 'display: none;' : ''}">
          ${
            dailyData.length > 0
              ? `
            <table class="data-table">
              <thead>
                <tr>
                  <th>Date</th>
                  <th>Requests</th>
                  <th>Bytes</th>
                  <th>Cost Units</th>
                  <th>Breakdown</th>
                </tr>
              </thead>
              <tbody>
                ${tableRows}
              </tbody>
            </table>
          `
              : '<p class="text-muted" style="padding: 1rem;">No daily data for this range.</p>'
          }
        </div>
      </div>

      <!-- Billing Estimate -->
      <div class="card" style="margin-bottom: 1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Billing Estimate</h3>
        </div>
        <div class="summary-stats">
          <div class="stat-item">
            <span class="stat-label">Cost Units</span>
            <span class="stat-value">${billingCostUnits.toFixed(4)}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Estimated USD</span>
            <span class="stat-value">${billingUsd.toFixed(4)}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Currency</span>
            <span class="stat-value">${billingCurrency}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Date Range</span>
            <span class="stat-value" style="font-size: 14px;">${effectiveStart} &mdash; ${effectiveEnd}</span>
          </div>
        </div>
        <p style="margin-top: 1rem; font-size: 13px; color: var(--text);">
          This is an <strong>estimate</strong>, not an invoice. The current backend calculation is <code>estimated_usd = total_cost_units &times; 0.0001</code>.
        </p>
      </div>
    `
  },

  async refresh(state) {
    setState({ usageLoading: true, usageError: null, usageDateError: '' })

    // Load API keys for filter dropdown
    const apiKeysPromise = listApiKeys({ limit: 100 }).catch(() => [])

    // Load usage and billing with current filters
    const datePreset = state.usageDatePreset || '7d'
    const customStart = state.usageCustomStart || ''
    const customEnd = state.usageCustomEnd || ''
    const selectedApiKey = state.usageApiKeyFilter || ''

    const getFormattedDate = (daysAgo: number) => {
      const d = new Date()
      d.setDate(d.getDate() - daysAgo)
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    }

    const today = getFormattedDate(0)
    let start, end

    switch (datePreset) {
      case '7d':
        start = getFormattedDate(7)
        end = today
        break
      case '30d':
        start = getFormattedDate(30)
        end = today
        break
      case 'mtd':
        start = `${new Date().getFullYear()}-${String(new Date().getMonth() + 1).padStart(2, '0')}-01`
        end = today
        break
      case 'custom':
        start = customStart
        end = customEnd
        break
      default:
        start = getFormattedDate(7)
        end = today
    }

    const usagePromise = getUsageSummary({ start, end, api_key_id: selectedApiKey })
    const billingPromise = getBillingEstimate({ start, end, api_key_id: selectedApiKey })

    const [apiKeysResult, usageResult, billingResult] = await Promise.allSettled([
      apiKeysPromise,
      usagePromise,
      billingPromise
    ])

    // Process API keys for suggestions
    if (apiKeysResult.status === 'fulfilled') {
      setState({ apiKeysSuggestions: apiKeysResult.value })
    }

    // Process usage
    if (usageResult.status === 'fulfilled') {
      setState({ usageData: usageResult.value, usageLoading: false, lastRefreshed: Date.now() })
    } else {
      if (usageResult.reason instanceof ApiError && usageResult.reason.status === 400) {
        // Date parse error from backend - set field-level error
        setState({
          usageLoading: false,
          usageDateError: errorMessage(usageResult.reason, 'Invalid date format')
        })
      } else {
        setState({
          usageError: errorMessage(usageResult.reason, 'Failed to load usage data'),
          usageLoading: false
        })
      }
    }

    // Process billing
    if (billingResult.status === 'fulfilled') {
      setState({ billingData: billingResult.value })
    } else {
      if (!(billingResult.reason instanceof ApiError && billingResult.reason.status === 400)) {
        console.warn('Billing estimate failed:', errorMessage(billingResult.reason))
      }
    }
  },

  afterRender(state) {
    if (!state.usageData && !state.usageLoading && !state.usageError) {
      void UsagePage.refresh(state)
      return
    }

    // Date preset toggle
    const presetSelect = document.getElementById('usage-date-preset')
    const customDatesDiv = document.getElementById('usage-custom-dates')
    if (presetSelect && customDatesDiv) {
      presetSelect.addEventListener('change', (e) => {
        const value = eventValue(e)
        customDatesDiv.style.display = value === 'custom' ? '' : 'none'
        setState({ usageDatePreset: value })
      })
    }

    // Apply filters
    const applyBtn = document.getElementById('usage-apply-btn')
    if (applyBtn) {
      applyBtn.addEventListener('click', () => {
        const datePreset = state.usageDatePreset || '7d'
        const customStart =
          (document.getElementById('usage-custom-start') as HTMLInputElement | null)?.value || ''
        const customEnd =
          (document.getElementById('usage-custom-end') as HTMLInputElement | null)?.value || ''
        const selectedApiKey =
          (document.getElementById('usage-api-key') as HTMLSelectElement | null)?.value || ''

        // Validate dates
        let hasError = false
        if (datePreset === 'custom') {
          try {
            validateDate(customStart)
            validateDate(customEnd)
          } catch (err) {
            setState({ usageDateError: errorMessage(err, 'Invalid date') })
            hasError = true
          }
          if (!hasError && customStart && customEnd && customStart > customEnd) {
            setState({ usageDateError: 'Start date must be on or before end date' })
            hasError = true
          }
        }

        if (!hasError) {
          setState({
            usageCustomStart: customStart,
            usageCustomEnd: customEnd,
            usageApiKeyFilter: selectedApiKey,
            usageDateError: ''
          })
          void UsagePage.refresh(state)
        }
      })
    }

    // View mode toggle
    const chartBtn = document.getElementById('usage-view-chart')
    const tableBtn = document.getElementById('usage-view-table')
    if (chartBtn) {
      chartBtn.addEventListener('click', () => {
        setState({ usageViewMode: 'chart' })
        void UsagePage.refresh(state)
      })
    }
    if (tableBtn) {
      tableBtn.addEventListener('click', () => {
        setState({ usageViewMode: 'table' })
        void UsagePage.refresh(state)
      })
    }

    // CSV export
    const csvBtn = document.getElementById('usage-export-csv')
    if (csvBtn) {
      csvBtn.addEventListener('click', () => {
        const dailyData = state.usageData?.daily || []
        if (dailyData.length === 0) {
          showToast('No data to export', 'warning')
          return
        }

        const headers = 'date,requests,bytes,cost_units\n'
        const rows = dailyData
          .map(
            (d) =>
              `${d.date || ''},${d.requests || 0},${d.bytes || 0},${(d.cost_units || 0).toFixed(4)}`
          )
          .join('\n')

        const apiKey = state.usageApiKeyFilter || 'all'
        const start =
          state.usageCustomStart ||
          (state.usageDatePreset === '7d' ? '7d' : state.usageDatePreset || '7d')
        const filename = `usage-${start}-to-${state.usageCustomEnd || 'today'}-key-${apiKey}.csv`

        const blob = new Blob([headers + rows], { type: 'text/csv' })
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = filename
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        showToast('CSV exported', 'success')
      })
    }

    // Retry button
    const retryBtn = document.getElementById('usage-retry-btn')
    if (retryBtn) {
      retryBtn.addEventListener('click', () => {
        void UsagePage.refresh(state)
      })
    }
  }
} satisfies Page
