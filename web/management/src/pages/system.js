// System Diagnostics Page

import { state, setState, clearSession, showToast } from '../state.js';
import { healthCheck, ApiError } from '../client.js';

// Known backend gaps from the UI spec
const BACKEND_GAPS = [
  'User accounts, SSO, and per-role permissions (no auth endpoints)',
  'Audit-log viewer (backend writes admin_audit_log but no read endpoint exists)',
  'API key update, rotation, reactivation, or explicit expiration editing',
  'Endpoint creation, deletion, undrain, restart, or live log viewing',
  'Fingerprint deletion',
  'Cost multiplier management (API exists but not exposed in UI)',
  'Saved reports, scheduled exports, alerts, and notification preferences',
  'Hourly drilldown, invoices, payments, and organizations in usage'
];

export const SystemPage = {
  render(state) {
    const healthData = state.systemHealth || null;
    const healthError = state.systemHealthError || null;
    const healthLoading = state.systemHealthLoading || false;
    const healthResponseTime = state.systemHealthResponseTime || null;
    const capabilities = state.systemCapabilities || {};
    const lastChecked = state.systemLastChecked || null;

    // Capability labels
    const capabilityItems = [
      { key: 'cache', label: 'Cache Controls', available: capabilities.cache },
      { key: 'fingerprints', label: 'Fingerprints', available: capabilities.fingerprints },
      { key: 'usage', label: 'Usage & Billing', available: capabilities.usage },
      { key: 'rules', label: 'Routing Rules', available: capabilities.rules },
      { key: 'endpoints', label: 'Endpoints', available: capabilities.endpoints },
      { key: 'apiKeys', label: 'API Keys', available: capabilities.apiKeys }
    ];

    const capabilityHtml = capabilityItems.map(item =>
      `<div style="display:flex;justify-content:space-between;align-items:center;padding:0.5rem 0;border-bottom:1px solid var(--border);">
         <span>${item.label}</span>
         <span class="badge badge-${item.available ? 'success' : 'danger'}">${item.available ? 'Available' : 'Unavailable'}</span>
       </div>`
    ).join('');

    // Backend gaps
    const gapsHtml = BACKEND_GAPS.map(gap =>
      `<li style="margin-bottom:0.5rem;padding:0.5rem 0;border-bottom:1px solid var(--border);font-size:0.875rem;">${gap}</li>`
    ).join('');

    // Health status
    const healthStatusHtml = healthLoading
      ? `<div class="skeleton skeleton-text" style="width:80px;"></div>`
      : healthError
        ? `<span class="badge badge-danger">Error</span>`
        : healthData
          ? `<span class="badge badge-success">Healthy</span>`
          : `<span class="badge badge-warning">Unknown</span>`;

    // Response time
    const responseTimeHtml = healthResponseTime !== null
      ? `${healthResponseTime}ms`
      : '';

    return `
      <div class="page-header">
        <h2 class="page-title">System Info</h2>
      </div>

      <!-- Connection Details -->
      <div class="card" style="margin-bottom:1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Connection</h3>
          <button class="btn btn-danger btn-sm" id="system-sign-out" title="Sign Out">Sign Out</button>
        </div>
        <div style="padding:1rem;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
            <span style="font-size:0.875rem;color:var(--text);">Management API URL</span>
            <code style="font-size:0.8rem;">${escapeHtml(state.baseUrl)}</code>
          </div>
          <div style="display:flex;justify-content:space-between;align-items:center;">
            <span style="font-size:0.875rem;color:var(--text);">Token Status</span>
            <span class="badge badge-success">Authenticated</span>
          </div>
          <div style="font-size:0.75rem;color:var(--text);margin-top:0.5rem;">
            Warning: Management token is stored in localStorage and may be accessible to browser extensions.
          </div>
        </div>
      </div>

      <!-- Health Check -->
      <div class="card" style="margin-bottom:1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Health Check</h3>
          <button class="btn btn-secondary btn-sm" id="system-health-refresh" ${healthLoading ? 'disabled' : ''}>Refresh</button>
        </div>
        <div style="padding:1rem;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
            <span style="font-size:0.875rem;">Status</span>
            ${healthStatusHtml}
          </div>
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
            <span style="font-size:0.875rem;">Response Time</span>
            <code style="font-size:0.8rem;">${responseTimeHtml || 'N/A'}</code>
          </div>
          ${healthData && !healthError
            ? `<div style="font-size:0.875rem;">
                 <strong>Response:</strong> <code>${escapeHtml(typeof healthData === 'string' ? healthData : JSON.stringify(healthData))}</code>
               </div>`
            : ''}
          ${healthError
            ? `<div style="font-size:0.875rem;color:var(--text);margin-top:0.5rem;">
                 <strong>Error:</strong> ${escapeHtml(healthError)}
               </div>`
            : ''}
          ${lastChecked
            ? `<div style="font-size:0.75rem;color:var(--text);margin-top:0.5rem;">
                 Last checked: ${new Date(lastChecked).toLocaleString()}
               </div>`
            : ''}
        </div>
      </div>

      <!-- Detected Capabilities -->
      <div class="card" style="margin-bottom:1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Detected Capabilities</h3>
          <button class="btn btn-secondary btn-sm" id="system-capabilities-refresh">Re-detect</button>
        </div>
        <div style="padding:1rem;">
          ${capabilityHtml}
        </div>
      </div>

      <!-- Documentation Links -->
      <div class="card" style="margin-bottom:1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Documentation</h3>
        </div>
        <div style="padding:1rem;">
          <ul style="list-style:none;padding:0;margin:0;">
            <li style="margin-bottom:0.5rem;"><a href="https://github.com/beremaran/straw/blob/main/docs/management-api.md" target="_blank" rel="noopener noreferrer" style="color:var(--text-h);text-decoration:none;">Management API Documentation</a></li>
            <li style="margin-bottom:0.5rem;"><a href="https://github.com/beremaran/straw/blob/main/api/openapi.yaml" target="_blank" rel="noopener noreferrer" style="color:var(--text-h);text-decoration:none;">OpenAPI Reference (yaml)</a></li>
            <li style="margin-bottom:0.5rem;"><a href="https://github.com/beremaran/straw/blob/main/docs/architecture.md" target="_blank" rel="noopener noreferrer" style="color:var(--text-h);text-decoration:none;">Architecture Documentation</a></li>
          </ul>
        </div>
      </div>

      <!-- Backend Gaps -->
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">First-Release Backend Gaps</h3>
        </div>
        <div style="padding:1rem;">
          <p style="font-size:0.875rem;margin-bottom:0.75rem;color:var(--text);">
            These features are documented in the spec but not yet exposed as UI controls because the backend API is unavailable:
          </p>
          <ul style="list-style:none;padding:0;margin:0;">
            ${gapsHtml}
          </ul>
        </div>
      </div>
    `;
  },

  async refresh(state) {
    setState({ systemHealthLoading: true, systemHealthError: null });
    const start = performance.now();
    try {
      const result = await healthCheck();
      const elapsed = Math.round(performance.now() - start);
      setState({
        systemHealth: result,
        systemHealthLoading: false,
        systemHealthResponseTime: elapsed,
        systemLastChecked: Date.now()
      });
    } catch (err) {
      setState({
        systemHealthLoading: false,
        systemHealthError: err.message || 'Health check failed'
      });
    }
  },

  async detectCapabilities(state) {
    setState({ systemCapabilitiesLoading: true });
    const results = await Promise.allSettled([
      import('../client.js').then(m => m.getCacheStats().catch(() => null)),
      import('../client.js').then(m => m.listFingerprints().catch(() => null)),
      import('../client.js').then(m => m.getUsageSummary({ start: '2026-01-01', end: '2026-01-02' }).catch(() => null)),
      import('../client.js').then(m => m.listRoutingRules({ limit: 1 }).catch(() => null)),
      import('../client.js').then(m => m.listEndpoints().catch(() => null)),
      import('../client.js').then(m => m.listApiKeys({ limit: 1 }).catch(() => null))
    ]);

    const keys = ['cache', 'fingerprints', 'usage', 'rules', 'endpoints', 'apiKeys'];
    const caps = {};
    results.forEach((res, i) => {
      caps[keys[i]] = res.status === 'fulfilled' && res.value !== null;
    });

    setState({ systemCapabilities: caps, systemCapabilitiesLoading: false });
  },

  afterRender(state) {
    // Sign out
    const signOutBtn = document.getElementById('system-sign-out');
    if (signOutBtn) {
      signOutBtn.addEventListener('click', () => {
        clearSession();
        window.location.hash = '#/login';
      });
    }

    // Health refresh
    const healthRefreshBtn = document.getElementById('system-health-refresh');
    if (healthRefreshBtn) {
      healthRefreshBtn.addEventListener('click', () => this.refresh(state));
    }

    // Capabilities re-detect
    const capsRefreshBtn = document.getElementById('system-capabilities-refresh');
    if (capsRefreshBtn) {
      capsRefreshBtn.addEventListener('click', () => this.detectCapabilities(state));
    }

    // Initial load
    if (!state.systemHealth && !state.systemHealthLoading) {
      this.refresh(state);
    }
    if (!state.systemCapabilities) {
      this.detectCapabilities(state);
    }
  }
};

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
