// Overview Dashboard Page

import { state, setState, showToast, showConfirm } from '../state.js';
import { 
  listEndpoints, 
  listRoutingRules, 
  listApiKeys, 
  getUsageSummary, 
  getBillingEstimate, 
  getCacheStats, 
  drainEndpoint,
  ApiError
} from '../client.js';

export const OverviewPage = {
  render(state) {
    const data = state.overviewData || {};
    const isLoading = state.overviewLoading;
    const errors = state.overviewErrors || {};

    if (isLoading && !state.overviewData) {
      return `
        <div class="page-header">
          <h2 class="page-title">Overview Dashboard</h2>
        </div>
        <div class="dashboard-grid">
          ${Array(6).fill(0).map(() => `
            <div class="card skeleton-card">
              <div class="skeleton skeleton-title"></div>
              <div class="skeleton skeleton-value"></div>
              <div class="skeleton skeleton-text"></div>
            </div>
          `).join('')}
        </div>
      `;
    }

    // Process endpoints data
    const endpoints = data.endpoints || [];
    const endpointStates = { healthy: 0, suspect: 0, unhealthy: 0, draining: 0 };
    let totalActiveTasks = 0;
    endpoints.forEach(ep => {
      const st = ep.state || 'unhealthy';
      if (st in endpointStates) endpointStates[st]++;
      totalActiveTasks += ep.active_tasks || 0;
    });

    // Process routing rules data
    const rules = data.rules || [];
    const activeRulesCount = rules.filter(r => r.is_active).length;
    const maxPriority = rules.length ? Math.max(...rules.map(r => r.priority || 0)) : 0;

    // Process API keys data
    const apiKeys = data.apiKeys || [];
    const activeKeysCount = apiKeys.filter(k => k.is_active).length;

    // Process usage & billing data
    const usage = data.usage || { total_requests: 0, total_bytes: 0, daily: [] };
    const billing = data.billing || { total_cost_units: 0, estimated_usd: 0.0 };

    // Format bytes
    const formatBytes = (bytes) => {
      if (!bytes) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    // Cache status
    const cacheStats = data.cacheStats || {};
    const cacheOnline = cacheStats.status === 'online' || cacheStats.redis_connected || false;

    // Client-side Routing Attention Checks
    const attentionList = [];
    const priorities = {};
    const fingerprintIds = new Set((data.fingerprints || []).map(f => f.id));
    const endpointIds = new Set(endpoints.map(e => e.id));

    rules.forEach(rule => {
      if (rule.is_active) {
        // 1. Duplicate priorities
        priorities[rule.priority] = (priorities[rule.priority] || 0) + 1;
        if (priorities[rule.priority] > 1) {
          attentionList.push({
            type: 'warning',
            message: `Active rule "${rule.name}" shares priority ${rule.priority} with another active rule.`
          });
        }
        // 2. No required tags
        if (!rule.required_tags || rule.required_tags.length === 0) {
          attentionList.push({
            type: 'warning',
            message: `Active rule "${rule.name}" matches all traffic (no required tags specified).`
          });
        }
        // 3. Referencing missing fingerprint preset
        if (rule.fingerprint_preset && !fingerprintIds.has(rule.fingerprint_preset)) {
          attentionList.push({
            type: 'danger',
            message: `Rule "${rule.name}" references missing fingerprint preset "${rule.fingerprint_preset}".`
          });
        }
        // 4. Missing endpoint pool matches
        if (rule.endpoint_pools) {
          rule.endpoint_pools.forEach(pool => {
            if (pool.endpoint_ids) {
              pool.endpoint_ids.forEach(epId => {
                if (!endpointIds.has(epId)) {
                  attentionList.push({
                    type: 'danger',
                    message: `Rule "${rule.name}" references offline/missing endpoint "${epId}" in pool tier ${pool.tier}.`
                  });
                }
              });
            }
          });
        }
        // 5. Insecure TLS
        if (rule.allow_insecure_tls) {
          attentionList.push({
            type: 'warning',
            message: `Rule "${rule.name}" has insecure TLS verification enabled.`
          });
        }
      } else {
        // Inactive rules attention check
        attentionList.push({
          type: 'info',
          message: `Rule "${rule.name}" is inactive (deactivated).`
        });
      }
    });

    // Render partial failure alert if some calls failed
    const failedPanels = Object.keys(errors).filter(k => errors[k]);
    const partialFailureHtml = failedPanels.length > 0
      ? `<div class="alert alert-warning" role="alert" style="margin-bottom: 1.5rem;">
          <div class="alert-title" style="display: flex; justify-content: space-between; align-items: center;">
            <span>Some panels could not load due to API errors</span>
            <button class="btn btn-secondary btn-sm" id="retry-failed-panels" style="margin-left: 1rem;">Retry Failed</button>
          </div>
          <div class="alert-body" style="font-size: 0.85rem; margin-top: 0.5rem;">
            Failed sections: ${failedPanels.map(p => `<strong>${p}</strong>`).join(', ')}. Details are logged to the console.
          </div>
         </div>`
      : '';

    // Render Endpoint pressure rows
    const endpointRows = endpoints.map(ep => {
      const tagsHtml = (ep.tags || []).map(t => `<span class="badge badge-secondary badge-sm">${t}</span>`).join(' ');
      const isDraining = ep.state === 'draining';
      const lastSeen = ep.last_seen ? new Date(ep.last_seen).toLocaleTimeString() : 'N/A';
      return `
        <tr>
          <td><code class="symbol-link">${ep.id}</code></td>
          <td><span class="badge badge-${ep.state === 'healthy' ? 'success' : ep.state === 'suspect' ? 'warning' : ep.state === 'draining' ? 'info' : 'danger'}">${ep.state}</span></td>
          <td><strong>${ep.active_tasks || 0}</strong></td>
          <td><div class="tag-chips-cell">${tagsHtml}</div></td>
          <td>${lastSeen}</td>
          <td style="text-align: right;">
            <button class="btn btn-secondary btn-xs btn-drain" data-id="${ep.id}" data-tasks="${ep.active_tasks}" ${isDraining ? 'disabled' : ''}>
              ${isDraining ? 'Draining' : 'Drain'}
            </button>
          </td>
        </tr>
      `;
    }).join('');

    const noEndpointsHtml = endpoints.length === 0 
      ? `<tr><td colspan="6" class="table-empty">No active endpoints registered. Start a worker node.</td></tr>` 
      : '';

    // Render Routing Attention items
    const attentionHtml = attentionList.length === 0
      ? `<div class="attention-empty">All configurations healthy. No warnings detected.</div>`
      : attentionList.map(item => `
          <div class="attention-item alert-${item.type}">
            <span class="attention-dot"></span>
            <span class="attention-message">${item.message}</span>
          </div>
        `).join('');

    // Generate Simple SVG chart of 7-day usage
    const dailyData = usage.daily || [];
    const chartWidth = 500;
    const chartHeight = 150;
    const padding = 20;
    const pointsCount = dailyData.length || 7;
    const maxVal = Math.max(...dailyData.map(d => d.requests || 0), 10);
    
    const barWidth = Math.max(10, Math.floor((chartWidth - padding * 2) / pointsCount) - 10);
    const chartHtml = dailyData.map((d, i) => {
      const r = d.requests || 0;
      const x = padding + i * (barWidth + 10);
      const h = (r / maxVal) * (chartHeight - padding * 2);
      const y = chartHeight - padding - h;
      return `
        <rect x="${x}" y="${y}" width="${barWidth}" height="${h}" fill="var(--color-violet-500)" rx="3" opacity="0.85">
          <title>${d.date}: ${r} requests</title>
        </rect>
        <text x="${x + barWidth / 2}" y="${chartHeight - 5}" font-size="9" fill="var(--color-slate-400)" text-anchor="middle">
          ${d.date ? d.date.split('-').slice(1).join('/') : ''}
        </text>
      `;
    }).join('');

    return `
      <div class="page-header">
        <h2 class="page-title">Overview Dashboard</h2>
      </div>

      ${partialFailureHtml}

      <!-- Metrics Panel -->
      <div class="dashboard-grid">
        <!-- Card 1: Endpoints -->
        <div class="card metric-card">
          <div class="metric-header">
            <span class="metric-title">Endpoints Status</span>
            <span class="badge badge-success">${endpointStates.healthy} healthy</span>
          </div>
          <div class="metric-value">${endpoints.length}</div>
          <div class="metric-meta">
            <span>Draining: <strong>${endpointStates.draining}</strong> | Suspect: <strong>${endpointStates.suspect}</strong></span>
            <span style="margin-left: auto;">Tasks: <strong>${totalActiveTasks}</strong></span>
          </div>
        </div>

        <!-- Card 2: Routing Rules -->
        <div class="card metric-card">
          <div class="metric-header">
            <span class="metric-title">Active Rules</span>
            <span class="badge badge-indigo">Max Priority: ${maxPriority}</span>
          </div>
          <div class="metric-value">${activeRulesCount}<span class="value-slash">/</span>${rules.length}</div>
          <div class="metric-meta">
            <span>Configured routing overrides active</span>
          </div>
        </div>

        <!-- Card 3: API Keys -->
        <div class="card metric-card">
          <div class="metric-header">
            <span class="metric-title">API Keys</span>
            <span class="badge badge-success">${activeKeysCount} active</span>
          </div>
          <div class="metric-value">${apiKeys.length}</div>
          <div class="metric-meta">
            <span>Revoked keys: <strong>${apiKeys.length - activeKeysCount}</strong></span>
          </div>
        </div>

        <!-- Card 4: Usage Summary -->
        <div class="card metric-card">
          <div class="metric-header">
            <span class="metric-title">7-Day Requests</span>
            <span class="badge badge-secondary">Traffic</span>
          </div>
          <div class="metric-value">${(usage.total_requests || 0).toLocaleString()}</div>
          <div class="metric-meta">
            <span>Bytes: <strong>${formatBytes(usage.total_bytes)}</strong></span>
          </div>
        </div>

        <!-- Card 5: Month-to-date Cost -->
        <div class="card metric-card">
          <div class="metric-header">
            <span class="metric-title">Est. Billing (MTD)</span>
            <span class="badge badge-amber">Estimate</span>
          </div>
          <div class="metric-value">$${(billing.estimated_usd || 0.0).toFixed(2)}</div>
          <div class="metric-meta">
            <span>Cost units: <strong>${(billing.total_cost_units || 0).toLocaleString()}</strong></span>
          </div>
        </div>

        <!-- Card 6: Cache Status -->
        <div class="card metric-card">
          <div class="metric-header">
            <span class="metric-title">Redis Cache</span>
            <span class="badge badge-${cacheOnline ? 'success' : 'danger'}">${cacheOnline ? 'Connected' : 'Disconnected'}</span>
          </div>
          <div class="metric-value">${cacheOnline ? 'Online' : 'Offline'}</div>
          <div class="metric-meta">
            <span>Used for active rules caching</span>
          </div>
        </div>
      </div>

      <!-- Main Columns -->
      <div class="overview-columns" style="margin-top: 2rem;">
        <!-- Left Column -->
        <div class="overview-col-left">
          <div class="card table-card">
            <div class="card-header">
              <h3 class="card-title">Endpoint Pressure</h3>
            </div>
            <div class="table-responsive">
              <table class="table dense-table">
                <thead>
                  <tr>
                    <th>Endpoint ID</th>
                    <th>State</th>
                    <th>Tasks</th>
                    <th>Tags</th>
                    <th>Last Seen</th>
                    <th style="text-align: right;">Action</th>
                  </tr>
                </thead>
                <tbody>
                  ${endpointRows}
                  ${noEndpointsHtml}
                </tbody>
              </table>
            </div>
          </div>

          <div class="card chart-card" style="margin-top: 2rem;">
            <div class="card-header">
              <h3 class="card-title">Recent 7-Day Request Traffic</h3>
            </div>
            <div class="chart-body" style="padding: 1rem; display: flex; justify-content: center; align-items: center;">
              <svg width="${chartWidth}" height="${chartHeight}" style="background: transparent;">
                ${chartHtml}
              </svg>
            </div>
          </div>
        </div>

        <!-- Right Column -->
        <div class="overview-col-right">
          <div class="card attention-card">
            <div class="card-header">
              <h3 class="card-title">Routing Attention & Audit</h3>
            </div>
            <div class="attention-body">
              ${attentionHtml}
            </div>
          </div>
        </div>
      </div>
    `;
  },

  async refresh(state) {
    setState({ overviewLoading: true, overviewErrors: null });
    
    // Dates
    const getFormattedDate = (daysAgo) => {
      const d = new Date();
      d.setDate(d.getDate() - daysAgo);
      const year = d.getFullYear();
      const month = String(d.getMonth() + 1).padStart(2, '0');
      const date = String(d.getDate()).padStart(2, '0');
      return `${year}-${month}-${date}`;
    };

    const start7 = getFormattedDate(7);
    const startMTD = `${new Date().getFullYear()}-${String(new Date().getMonth() + 1).padStart(2, '0')}-01`;
    const today = getFormattedDate(0);

    const endpointsPromise = listEndpoints().catch(e => { console.error(e); throw e; });
    const rulesPromise = listRoutingRules({ limit: 100 }).catch(e => { console.error(e); throw e; });
    const apiKeysPromise = listApiKeys({ limit: 100 }).catch(e => { console.error(e); throw e; });
    const usagePromise = getUsageSummary({ start: start7, end: today }).catch(e => { console.error(e); throw e; });
    const billingPromise = getBillingEstimate({ start: startMTD, end: today }).catch(e => { console.error(e); throw e; });
    const cacheStatsPromise = getCacheStats().catch(e => { console.error(e); throw e; });

    // Fingerprints (used for cross-checking rule presets)
    const fingerprintsPromise = import('../client.js').then(m => m.listFingerprints()).catch(() => []);

    const results = await Promise.allSettled([
      endpointsPromise,
      rulesPromise,
      apiKeysPromise,
      usagePromise,
      billingPromise,
      cacheStatsPromise,
      fingerprintsPromise
    ]);

    const data = { ...state.overviewData };
    const errors = {};

    const keys = ['endpoints', 'rules', 'apiKeys', 'usage', 'billing', 'cacheStats', 'fingerprints'];
    results.forEach((res, i) => {
      const key = keys[i];
      if (res.status === 'fulfilled') {
        data[key] = res.value;
        errors[key] = null;
      } else {
        errors[key] = res.reason.message || res.reason;
      }
    });

    setState({
      overviewData: data,
      overviewErrors: failedPanelsCount(errors) > 0 ? errors : null,
      overviewLoading: false,
      lastRefreshed: Date.now()
    });

    // Check if auth failed across everything - if so, client.js already handles redirects, but let's be safe
    const unauthorizedCount = results.filter(res => res.status === 'rejected' && res.reason && res.reason.status === 401).length;
    if (unauthorizedCount > 3) {
      import('../state.js').then(({ clearSession }) => {
        clearSession();
        window.location.hash = '#/login';
      });
    }
  },

  afterRender(state) {
    if (!state.overviewData && !state.overviewLoading) {
      this.refresh(state);
      return;
    }

    // Attach local event listeners (Retry and Drain)
    const retryBtn = document.getElementById('retry-failed-panels');
    if (retryBtn) {
      retryBtn.addEventListener('click', () => {
        this.refresh(state);
      });
    }

    const drainBtns = document.querySelectorAll('.btn-drain');
    drainBtns.forEach(btn => {
      btn.addEventListener('click', (e) => {
        const id = btn.getAttribute('data-id');
        const tasks = btn.getAttribute('data-tasks') || '0';
        
        showConfirm({
          title: 'Drain Endpoint',
          body: `Are you sure you want to drain endpoint <strong>${id}</strong>? Active tasks (<strong>${tasks}</strong>) will be allowed to finish, but no new requests will be routed to this endpoint. This action is irreversible.`,
          confirmText: 'confirm',
          requiresInput: false,
          callback: async () => {
            try {
              await drainEndpoint(id);
              showToast(`Endpoint ${id} is now draining`, 'success');
              
              // Optimistically update status
              if (state.overviewData && state.overviewData.endpoints) {
                state.overviewData.endpoints = state.overviewData.endpoints.map(ep => {
                  if (ep.id === id) {
                    return { ...ep, state: 'draining' };
                  }
                  return ep;
                });
                setState({ overviewData: state.overviewData });
              }
            } catch (err) {
              showToast(`Failed to drain endpoint: ${err.message}`, 'error');
            }
          }
        });
      });
    });
  }
};

function failedPanelsCount(errors) {
  return Object.values(errors).filter(Boolean).length;
}
