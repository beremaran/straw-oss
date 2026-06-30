// Fingerprint Presets Management Page

import { state, setState, showToast, showConfirm } from '../state.js';
import { 
  listFingerprints, 
  createFingerprint, 
  broadcastFingerprints, 
  listRoutingRules, 
  ApiError 
} from '../client.js';
import { validateJsonObject } from '../validation.js';

export const FingerprintsPage = {
  render(state) {
    const presets = state.fingerprintsData || [];
    const rules = state.rulesData || [];
    const isLoading = state.fingerprintsLoading;
    const error = state.fingerprintsError;

    // Build map of preset usage in rules
    const usageCounts = {};
    rules.forEach(r => {
      if (r.fingerprint_preset) {
        usageCounts[r.fingerprint_preset] = (usageCounts[r.fingerprint_preset] || 0) + 1;
      }
      if (r.fingerprint_ab_test && r.fingerprint_ab_test.variants) {
        r.fingerprint_ab_test.variants.forEach(v => {
          if (v.preset_id) {
            usageCounts[v.preset_id] = (usageCounts[v.preset_id] || 0) + 1;
          }
        });
      }
    });

    const rowsHtml = presets.map(p => {
      const config = p.config || {};
      const userAgent = config.user_agent || config.UserAgent || 'N/A';
      
      // Infer browser family
      let browserFamily = 'Unknown';
      const uaLower = userAgent.toLowerCase();
      if (uaLower.includes('firefox')) browserFamily = 'Firefox';
      else if (uaLower.includes('chrome')) browserFamily = 'Chrome';
      else if (uaLower.includes('safari')) browserFamily = 'Safari';
      else if (uaLower.includes('edge')) browserFamily = 'Edge';

      const usageCount = usageCounts[p.id] || 0;
      const updatedStr = p.updated_at ? new Date(p.updated_at).toLocaleDateString() : 'N/A';

      return `
        <tr>
          <td><code class="symbol-link">${p.id}</code></td>
          <td><strong class="font-medium">${p.name || p.id}</strong></td>
          <td><span class="badge badge-secondary">${browserFamily}</span></td>
          <td>
            <div class="user-agent-cell" title="${userAgent}">
              ${userAgent !== 'N/A' ? userAgent : '<span class="text-muted">None</span>'}
            </div>
          </td>
          <td>${updatedStr}</td>
          <td><strong>${usageCount}</strong> rules</td>
          <td style="text-align: right;">
            <div class="action-buttons-group" style="justify-content: flex-end;">
              <button class="btn btn-secondary btn-xs btn-copy-preset-json" data-id="${p.id}" data-config="${encodeURIComponent(JSON.stringify(p.config, null, 2))}">Copy JSON</button>
              <button class="btn btn-secondary btn-xs btn-edit-preset" data-id="${p.id}">Edit</button>
              <button class="btn btn-secondary btn-xs btn-duplicate-preset" data-id="${p.id}">Duplicate</button>
            </div>
          </td>
        </tr>
      `;
    }).join('');

    const noPresetsHtml = presets.length === 0 
      ? `<tr><td colspan="7" class="table-empty">No fingerprint presets registered. Create one below.</td></tr>` 
      : '';

    // Create/Edit Modal
    const modalShow = state.fingerprintsShowModal;
    const modalIsEdit = state.fingerprintsModalIsEdit;
    const modalRule = state.fingerprintsModalRule || {};
    const modalTitle = modalIsEdit 
      ? 'Edit Fingerprint Preset' 
      : (state.fingerprintsModalIsDuplicate ? 'Duplicate Fingerprint Preset' : 'Create Fingerprint Preset');

    const modalHtml = modalShow
      ? `<div class="modal-overlay active">
          <div class="modal-card animate-zoom-in" style="max-width: 600px;">
            <div class="modal-header">
              <h3 class="modal-title">${modalTitle}</h3>
            </div>
            <form id="preset-form" novalidate>
              <div class="modal-body">
                <div class="form-grid-2">
                  <div class="form-group">
                    <label for="preset-id" class="form-label">Preset ID (Stable slug)</label>
                    <input type="text" id="preset-id" class="form-control" value="${modalRule.id || ''}" placeholder="e.g. chrome-desktop-120" ${modalIsEdit ? 'readonly style="background:var(--color-slate-800); color:var(--color-slate-400);"' : ''} required />
                    <div class="invalid-feedback" id="preset-id-error"></div>
                  </div>
                  <div class="form-group">
                    <label for="preset-name" class="form-label">Preset Name</label>
                    <input type="text" id="preset-name" class="form-control" value="${modalRule.name || ''}" placeholder="e.g. Chrome Windows Desktop" required />
                    <div class="invalid-feedback" id="preset-name-error"></div>
                  </div>
                </div>

                <div class="form-group" style="margin-top: 1rem;">
                  <label for="preset-config" class="form-label">Configuration (JSON Object)</label>
                  <textarea id="preset-config" class="form-control text-control" rows="8" style="font-family:monospace; font-size:0.85rem;" placeholder="{\n  &quot;user_agent&quot;: &quot;...&quot;,\n  &quot;ja3&quot;: &quot;...&quot;\n}" required>${modalRule.config ? JSON.stringify(modalRule.config, null, 2) : ''}</textarea>
                  <div class="invalid-feedback" id="preset-config-error"></div>
                </div>
              </div>
              <div class="modal-footer">
                <button type="button" class="btn btn-secondary" id="btn-preset-cancel">Cancel</button>
                <button type="submit" class="btn btn-primary">Save Preset</button>
              </div>
            </form>
          </div>
         </div>`
      : '';

    return `
      <div class="page-header">
        <h2 class="page-title">Fingerprint Presets</h2>
        <div class="action-buttons-group">
          <button class="btn btn-secondary" id="btn-broadcast-presets" style="border-color: var(--color-indigo-500); color: var(--color-indigo-400);">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px; margin-right: 6px; display: inline-block; vertical-align: middle;">
              <path stroke-linecap="round" stroke-linejoin="round" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071a9.004 9.004 0 0112.728 0m-18.384-7.07a14 14 0 000 19.8m20.124-19.8a14 14 0 010 19.8" />
            </svg>
            <span>Broadcast Presets</span>
          </button>
          <button class="btn btn-primary" id="btn-create-preset">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 14px; height: 14px; margin-right: 6px; display: inline-block; vertical-align: middle;">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            <span>Create Preset</span>
          </button>
        </div>
      </div>

      <!-- Presets Table -->
      <div class="card table-card" style="margin-top: 1.5rem;">
        <div class="table-responsive">
          <table class="table">
            <thead>
              <tr>
                <th>Preset ID</th>
                <th>Name</th>
                <th>Browser Family</th>
                <th>User Agent (Config)</th>
                <th>Updated</th>
                <th>Usage</th>
                <th style="text-align: right; width: 260px;">Actions</th>
              </tr>
            </thead>
            <tbody>
              ${rowsHtml}
              ${noPresetsHtml}
            </tbody>
          </table>
        </div>
      </div>

      <!-- Info Notice for Delete Gap -->
      <div class="card info-card" style="margin-top: 2rem; border-left: 4px solid var(--color-slate-500); background: var(--color-slate-800); opacity: 0.85;">
        <div style="display:flex; gap:0.75rem; align-items:flex-start;">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 20px; height: 20px; color: var(--color-slate-400); flex-shrink:0; margin-top:0.1rem;">
            <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div>
            <strong style="display:block; font-size:0.9rem; margin-bottom:0.25rem;">Control Plane Notice</strong>
            <span style="font-size:0.825rem; color:var(--color-slate-300);">
              Fingerprint preset deletion is currently not supported via the API Control Plane. Presets once created remain stored in PostgreSQL; you can deactivate or update their configurations freely.
            </span>
          </div>
        </div>
      </div>

      <!-- Modal -->
      ${modalHtml}
    `;
  },

  async refresh(state) {
    setState({ fingerprintsLoading: true, fingerprintsError: null });
    try {
      const presets = await listFingerprints();
      const rules = await listRoutingRules({ limit: 100 }).catch(() => []);
      setState({ 
        fingerprintsData: presets, 
        rulesData: rules,
        fingerprintsLoading: false 
      });
    } catch (err) {
      setState({ fingerprintsLoading: false, fingerprintsError: err.message });
      showToast(`Failed to load presets: ${err.message}`, 'error');
    }
  },

  afterRender(state) {
    if (!state.fingerprintsData && !state.fingerprintsLoading) {
      this.refresh(state);
      return;
    }

    // Copy JSON config trigger
    const copyBtns = document.querySelectorAll('.btn-copy-preset-json');
    copyBtns.forEach(btn => {
      btn.addEventListener('click', () => {
        const configStr = decodeURIComponent(btn.getAttribute('data-config'));
        navigator.clipboard.writeText(configStr);
        showToast('Preset config JSON copied', 'success');
      });
    });

    // Create Preset trigger
    const createBtn = document.getElementById('btn-create-preset');
    if (createBtn) {
      createBtn.addEventListener('click', () => {
        setState({
          fingerprintsShowModal: true,
          fingerprintsModalIsEdit: false,
          fingerprintsModalIsDuplicate: false,
          fingerprintsModalRule: { config: {} }
        });
      });
    }

    // Edit Preset trigger
    const editBtns = document.querySelectorAll('.btn-edit-preset');
    editBtns.forEach(btn => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-id');
        const preset = state.fingerprintsData.find(p => p.id === id);
        if (preset) {
          setState({
            fingerprintsShowModal: true,
            fingerprintsModalIsEdit: true,
            fingerprintsModalIsDuplicate: false,
            fingerprintsModalRule: { ...preset }
          });
        }
      });
    });

    // Duplicate Preset trigger
    const duplicateBtns = document.querySelectorAll('.btn-duplicate-preset');
    duplicateBtns.forEach(btn => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-id');
        const preset = state.fingerprintsData.find(p => p.id === id);
        if (preset) {
          setState({
            fingerprintsShowModal: true,
            fingerprintsModalIsEdit: false,
            fingerprintsModalIsDuplicate: true,
            fingerprintsModalRule: {
              id: `${preset.id}-copy`,
              name: `${preset.name || preset.id} Copy`,
              config: JSON.parse(JSON.stringify(preset.config || {}))
            }
          });
        }
      });
    });

    // Broadcast Presets trigger
    const broadcastBtn = document.getElementById('btn-broadcast-presets');
    if (broadcastBtn) {
      broadcastBtn.addEventListener('click', () => {
        showConfirm({
          title: 'Broadcast Presets',
          body: 'Are you sure you want to broadcast all fingerprint presets? This pushes current configs to active workers over NATS.',
          confirmText: 'confirm',
          callback: async () => {
            try {
              await broadcastFingerprints();
              showToast('Broadcast requested successfully', 'success');
            } catch (err) {
              showToast(`Failed to broadcast: ${err.message}`, 'error');
            }
          }
        });
      });
    }

    // Modal forms event bindings
    if (state.fingerprintsShowModal) {
      const cancelBtn = document.getElementById('btn-preset-cancel');
      if (cancelBtn) {
        cancelBtn.addEventListener('click', () => {
          setState({ fingerprintsShowModal: false });
        });
      }

      const form = document.getElementById('preset-form');
      if (form) {
        form.addEventListener('submit', async (e) => {
          e.preventDefault();
          
          const idInput = document.getElementById('preset-id');
          const nameInput = document.getElementById('preset-name');
          const configInput = document.getElementById('preset-config');

          const idErr = document.getElementById('preset-id-error');
          const nameErr = document.getElementById('preset-name-error');
          const configErr = document.getElementById('preset-config-error');

          idErr.textContent = '';
          nameErr.textContent = '';
          configErr.textContent = '';
          idInput.classList.remove('is-invalid');
          nameInput.classList.remove('is-invalid');
          configInput.classList.remove('is-invalid');

          let isValid = true;
          const idVal = idInput.value.trim();
          const nameVal = nameInput.value.trim();
          const configVal = configInput.value.trim();

          if (!state.fingerprintsModalIsEdit) {
            if (!idVal) {
              idErr.textContent = 'Preset ID is required.';
              idInput.classList.add('is-invalid');
              isValid = false;
            } else if (!/^[a-z0-9-_]+$/.test(idVal)) {
              idErr.textContent = 'Preset ID must be alphanumeric and slug format (only lowercase, numbers, dashes, underscores).';
              idInput.classList.add('is-invalid');
              isValid = false;
            }
          }

          if (!nameVal) {
            nameErr.textContent = 'Preset Name is required.';
            nameInput.classList.add('is-invalid');
            isValid = false;
          }

          let parsedConfig = null;
          try {
            parsedConfig = validateJsonObject(configVal);
          } catch (err) {
            configErr.textContent = err.message;
            configInput.classList.add('is-invalid');
            isValid = false;
          }

          if (!isValid) {
            const firstInvalid = form.querySelector('.is-invalid');
            if (firstInvalid) firstInvalid.focus();
            return;
          }

          try {
            await createFingerprint({
              id: state.fingerprintsModalIsEdit ? state.fingerprintsModalRule.id : idVal,
              name: nameVal,
              config: parsedConfig
            });

            showToast('Preset saved successfully', 'success');
            setState({ fingerprintsShowModal: false });
            this.refresh(state);
          } catch (err) {
            showToast(`Failed to save preset: ${err.message}`, 'error');
          }
        });
      }
    }
  }
};
