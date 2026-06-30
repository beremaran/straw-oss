// Cache Control Page

import { state, setState, showToast, showConfirm } from '../state.js';
import { getCacheStats, clearCache, ApiError } from '../client.js';

export const CachePage = {
  render(state) {
    const cacheData = state.cacheData || null;
    const cacheError = state.cacheError || null;
    const cacheUnavailable = state.cacheUnavailable || false;
    const isLoading = state.cacheLoading || false;
    const clearPattern = state.cacheClearPattern || '*';
    const clearConfirmText = state.cacheClearConfirmText || '';
    const clearLoading = state.cacheClearLoading || false;
    const clearResult = state.cacheClearResult || null;
    const infoSearch = state.cacheInfoSearch || '';

    // Parse quick facts from raw Redis INFO
    const quickFacts = cacheData ? parseQuickFacts(cacheData.info) : {};

    // Filter raw info text
    const filteredInfo = infoSearch
      ? cacheData?.info
          .split('\n')
          .filter(line => line.toLowerCase().includes(infoSearch.toLowerCase()))
          .join('\n')
      : cacheData?.info || '';

    // Availability state
    if (cacheUnavailable) {
      return `
        <div class="page-header">
          <h2 class="page-title">Cache Control</h2>
        </div>
        <div class="card" style="border-left: 4px solid var(--color-slate-500); background: var(--color-slate-800); opacity: 0.85;">
          <div style="display:flex; gap:0.75rem; align-items:flex-start;">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:20px;height:20px;color:var(--color-slate-400);flex-shrink:0;margin-top:0.1rem;" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <div>
              <strong style="display:block;font-size:0.9rem;margin-bottom:0.25rem;">Cache Unavailable</strong>
              <span style="font-size:0.825rem;">Redis is not configured on this node. Cache statistics and clearing are not available.</span>
            </div>
          </div>
        </div>
      `;
    }

    // Error state
    if (cacheError) {
      return `
        <div class="page-header">
          <h2 class="page-title">Cache Control</h2>
        </div>
        <div class="alert alert-error" role="alert">
          <div class="alert-title">Failed to Load Cache Stats</div>
          <div class="alert-body" style="font-size:0.875rem;margin-top:0.5rem;">${cacheError}</div>
          <button class="btn btn-secondary btn-sm" id="cache-retry-btn" style="margin-top:0.75rem;">Retry</button>
        </div>
      `;
    }

    // Loading state
    if (isLoading && !cacheData) {
      return `
        <div class="page-header">
          <h2 class="page-title">Cache Control</h2>
        </div>
        <div class="card skeleton-card">
          <div class="skeleton skeleton-title"></div>
          <div class="skeleton skeleton-value"></div>
          <div class="skeleton skeleton-text"></div>
        </div>
      `;
    }

    // Quick facts summary
    const factsHtml = Object.entries(quickFacts).map(([label, value]) =>
      value !== null && value !== undefined
        ? `<div style="text-align:center;padding:0.5rem 0;">
             <div style="font-size:1.25rem;font-weight:600;color:var(--text-h);">${value}</div>
             <div style="font-size:0.75rem;color:var(--text);text-transform:uppercase;letter-spacing:0.05em;">${label}</div>
           </div>`
        : ''
    ).filter(Boolean).join('');

    // Clear result
    const clearResultHtml = clearResult
      ? `<div class="alert alert-success" role="status" style="margin-top:1rem;">
           <div class="alert-title">Cache Cleared</div>
           <div class="alert-body" style="font-size:0.875rem;margin-top:0.5rem;">
             Pattern: <code>${escapeHtml(clearResult.pattern)}</code> &middot; Deleted: <strong>${clearResult.deleted.toLocaleString()}</strong> keys
           </div>
         </div>`
      : '';

    // Is wildcard clear
    const isWildcard = clearPattern === '*';

    return `
      <div class="page-header">
        <h2 class="page-title">Cache Control</h2>
      </div>

      <!-- Quick Facts -->
      <div class="card" style="margin-bottom:1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Quick Facts</h3>
        </div>
        <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:0.5rem;padding:1rem;">
          ${factsHtml}
        </div>
      </div>

      <!-- Raw Redis INFO Panel -->
      <div class="card" style="margin-bottom:1.5rem;">
        <div class="card-header">
          <h3 class="card-title">Redis INFO</h3>
          <div style="display:flex;gap:0.5rem;align-items:center;">
            <input type="text" id="cache-info-search" class="form-control" placeholder="Search INFO..." style="width:180px;" aria-label="Search Redis INFO output" />
            <button class="btn btn-secondary btn-sm btn-copy-info" id="cache-copy-info-btn" title="Copy INFO text">Copy</button>
          </div>
        </div>
        <pre id="cache-info-text" style="margin:0;padding:1rem;font-size:0.8rem;max-height:400px;overflow:auto;background:var(--code-bg);border-radius:4px;white-space:pre-wrap;word-break:break-all;">${escapeHtml(filteredInfo)}</pre>
      </div>

      <!-- Clear Cache Form -->
      <div class="card">
        <div class="card-header">
          <h3 class="card-title">Clear Cache</h3>
        </div>
        <div style="padding:1rem;">
          <div class="form-group">
            <label for="cache-clear-pattern" class="form-label">Pattern</label>
            <input type="text" id="cache-clear-pattern" class="form-control" value="${escapeHtml(clearPattern)}" ${isWildcard ? 'placeholder="*"' : ''} aria-describedby="cache-pattern-help" />
            <div id="cache-pattern-help" style="font-size:0.75rem;color:var(--text);margin-top:0.25rem;">
              ${isWildcard
                ? 'Wildcard (*) clears all keys. Requires typing CLEAR ALL to confirm.'
                : 'Pattern-specific clear. Type the exact pattern to confirm.'}
            </div>
          </div>

          ${isWildcard
            ? `<div class="form-group" style="margin-top:1rem;">
                 <label for="cache-clear-confirm" class="form-label">Type <code>CLEAR ALL</code> to confirm</label>
                 <input type="text" id="cache-clear-confirm" class="form-control" placeholder="Type CLEAR ALL" aria-describedby="cache-clear-confirm-help" />
                 <div id="cache-clear-confirm-help" style="font-size:0.75rem;color:var(--text);margin-top:0.25rem;">This prevents accidental full cache flush.</div>
               </div>`
            : `<div class="form-group" style="margin-top:1rem;">
                 <label for="cache-clear-confirm-other" class="form-label">Confirm pattern</label>
                 <input type="text" id="cache-clear-confirm-other" class="form-control" value="${escapeHtml(clearPattern)}" placeholder="Type the pattern again" aria-describedby="cache-clear-confirm-other-help" />
                 <div id="cache-clear-confirm-other-help" style="font-size:0.75rem;color:var(--text);margin-top:0.25rem;">Retype the exact pattern to confirm.</div>
               </div>`}

          <button class="btn btn-danger" id="cache-clear-btn" style="margin-top:1rem;" ${clearLoading ? 'disabled' : ''}>
            ${clearLoading ? '<span class="spinner"></span> Clearing...' : 'Clear Cache'}
          </button>

          ${clearResultHtml}
        </div>
      </div>
    `;
  },

  async refresh(state) {
    setState({ cacheLoading: true, cacheError: null, cacheUnavailable: false, cacheClearResult: null });
    try {
      const data = await getCacheStats();
      setState({ cacheData: data, cacheLoading: false, lastRefreshed: Date.now() });
    } catch (err) {
      setState({ cacheLoading: false });
      if (err instanceof ApiError) {
        if (err.status === 500 || err.status === 503) {
          // Redis unavailable - show unavailable state, not error
          setState({ cacheUnavailable: true });
        } else {
          setState({ cacheError: err.message });
        }
      } else {
        setState({ cacheError: err.message || 'Failed to load cache stats' });
      }
    }
  },

  afterRender(state) {
    if (!state.cacheData && !state.cacheLoading && !state.cacheError && !state.cacheUnavailable) {
      this.refresh(state);
      return;
    }

    // Retry button
    const retryBtn = document.getElementById('cache-retry-btn');
    if (retryBtn) {
      retryBtn.addEventListener('click', () => this.refresh(state));
    }

    // INFO search
    const searchInput = document.getElementById('cache-info-search');
    if (searchInput) {
      searchInput.addEventListener('input', (e) => {
        setState({ cacheInfoSearch: e.target.value });
        CachePage.refresh(state);
      });
    }

    // Copy INFO button
    const copyInfoBtn = document.getElementById('cache-copy-info-btn');
    if (copyInfoBtn) {
      copyInfoBtn.addEventListener('click', () => {
        const infoText = state.cacheData?.info || '';
        navigator.clipboard.writeText(infoText).then(() => {
          showToast('Redis INFO copied to clipboard', 'success');
        }).catch(() => {
          showToast('Failed to copy INFO text', 'error');
        });
      });
    }

    // Clear cache button
    const clearBtn = document.getElementById('cache-clear-btn');
    if (clearBtn) {
      clearBtn.addEventListener('click', async () => {
        const patternInput = document.getElementById('cache-clear-pattern');
        const pattern = patternInput?.value.trim() || '*';
        const isWildcard = pattern === '*';

        // Validation
        if (!pattern) {
          showToast('Pattern cannot be empty', 'error');
          return;
        }

        // Wildcard requires CLEAR ALL confirmation
        if (isWildcard) {
          const confirmInput = document.getElementById('cache-clear-confirm');
          if (confirmInput?.value.trim() !== 'CLEAR ALL') {
            showToast('Type CLEAR ALL to confirm wildcard clear', 'error');
            return;
          }
        } else {
          // Non-wildcard requires retype confirmation
          const confirmInput = document.getElementById('cache-clear-confirm-other');
          if (confirmInput?.value.trim() !== pattern) {
            showToast(`Type the exact pattern "${pattern}" to confirm`, 'error');
            return;
          }
        }

        // Set loading state
        setState({ cacheClearLoading: true });
        clearBtn.disabled = true;
        clearBtn.innerHTML = '<span class="spinner"></span> Clearing...';

        try {
          const result = await clearCache(pattern);
          setState({
            cacheClearLoading: false,
            cacheClearResult: { pattern, deleted: result.deleted }
          });
          showToast(`Cleared ${result.deleted} keys matching "${pattern}"`, 'success');
          // Refresh stats
          this.refresh(state);
        } catch (err) {
          setState({ cacheClearLoading: false });
          showToast(`Cache clear failed: ${err.message}`, 'error');
          clearBtn.disabled = false;
          clearBtn.innerHTML = 'Clear Cache';
        }
      });
    }
  }
};

// Parse quick facts from raw Redis INFO text
function parseQuickFacts(info) {
  if (!info || typeof info !== 'string') return {};

  const facts = {};

  const extract = (key) => {
    const match = info.match(new RegExp(`^${key}:([\\S\\s]*?)$`, 'm'));
    return match ? match[1].trim() : null;
  };

  // Redis version
  const version = extract('redis_version');
  if (version) facts['Redis Version'] = version;

  // Used memory
  const usedMem = extract('used_memory_human');
  if (usedMem) facts['Used Memory'] = usedMem;

  // Connected clients
  const clients = extract('connected_clients');
  if (clients) facts['Connected Clients'] = parseInt(clients, 10);

  // Keyspace hits
  const hits = extract('keyspace_hits');
  if (hits) facts['Keyspace Hits'] = parseInt(hits, 10);

  // Keyspace misses
  const misses = extract('keyspace_misses');
  if (misses) facts['Keyspace Misses'] = parseInt(misses, 10);

  return facts;
}

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
