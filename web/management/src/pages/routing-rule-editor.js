// Routing Rule Create and Edit Page

import { state, setState, showToast, showConfirm } from '../state.js';
import { 
  getRoutingRule, 
  createRoutingRule, 
  updateRoutingRule, 
  listEndpoints, 
  listFingerprints,
  ApiError
} from '../client.js';
import { parseTag, validateDuration, validatePositiveInteger, validateNonNegativeInteger } from '../validation.js';

export const RoutingRuleEditorPage = {
  render(state) {
    const hash = state.currentPage || '';
    const urlParams = new URLSearchParams(hash.includes('?') ? hash.split('?')[1] : '');
    const ruleId = urlParams.get('id') || state.editingRuleId;
    const isEdit = !!ruleId;

    const rule = state.editingRule || {};
    const presets = state.fingerprintsData || [];
    const endpoints = state.endpointsData || [];

    const jsonError = state.ruleJsonError || '';

    // Construct form content
    return `
      <div class="page-header">
        <div style="display:flex; align-items:center;">
          <a href="#/routing-rules" class="btn btn-secondary btn-sm" style="margin-right: 1.5rem;">Cancel</a>
          <h2 class="page-title">${isEdit ? 'Edit Routing Rule' : 'Create Routing Rule'}</h2>
        </div>
      </div>

      <div class="editor-layout">
        <!-- Main Form Column -->
        <div class="editor-main-col">
          <form id="rule-form" novalidate>
            <!-- Section 1: Basic Information -->
            <div class="card section-card">
              <h3 class="section-title">Basic Information</h3>
              <div class="form-grid-2">
                <div class="form-group">
                  <label for="rule-name" class="form-label">Rule Name</label>
                  <input type="text" id="rule-name" class="form-control" value="${rule.name || ''}" placeholder="e.g. US Residential Proxy Bypass" required />
                  <div class="invalid-feedback" id="rule-name-error"></div>
                </div>

                <div class="form-group">
                  <label for="rule-priority" class="form-label">Priority</label>
                  <input type="number" id="rule-priority" class="form-control" value="${rule.priority !== undefined ? rule.priority : 0}" required />
                  <span class="help-text">Higher numbers are evaluated first.</span>
                  <div class="invalid-feedback" id="rule-priority-error"></div>
                </div>

                <div class="form-group">
                  <label for="rule-quota-key" class="form-label">Quota Key</label>
                  <input type="text" id="rule-quota-key" class="form-control" value="${rule.quota_key || ''}" placeholder="Optional custom quota key identifier" />
                </div>

                <div class="form-group form-check-group" style="align-self: center; margin-top: 1rem;">
                  <label class="form-check-label">
                    <input type="checkbox" id="rule-active" ${rule.is_active !== false ? 'checked' : ''} />
                    <strong>Rule Active (Traffic matching enabled)</strong>
                  </label>
                </div>
              </div>
            </div>

            <!-- Section 2: Match Criteria -->
            <div class="card section-card" style="margin-top: 1.5rem;">
              <h3 class="section-title">Match Criteria & Selection</h3>
              
              <div class="form-group">
                <label class="form-label">Required Tags (Key:Value)</label>
                <div class="chip-input-container">
                  <div id="required-tags-chips" class="chip-input-chips"></div>
                  <input type="text" id="required-tags-input" class="chip-input-field" placeholder="Type tag (e.g. region:us) and press Enter..." />
                </div>
                <div class="invalid-feedback" id="required-tags-error" style="display:block;"></div>
              </div>

              <div class="form-group" style="margin-top: 1rem;">
                <label class="form-label">Excluded Tags (Key:Value)</label>
                <div class="chip-input-container">
                  <div id="excluded-tags-chips" class="chip-input-chips"></div>
                  <input type="text" id="excluded-tags-input" class="chip-input-field" placeholder="Type tag to exclude..." />
                </div>
                <div class="invalid-feedback" id="excluded-tags-error" style="display:block;"></div>
              </div>
            </div>

            <!-- Section 3: Limits & Constraints -->
            <div class="card section-card" style="margin-top: 1.5rem;">
              <h3 class="section-title">Limits & Timeouts</h3>
              <div class="form-grid-3">
                <div class="form-group">
                  <label for="rule-timeout" class="form-label">Hard Timeout</label>
                  <input type="text" id="rule-timeout" class="form-control" value="${rule.hard_timeout || ''}" placeholder="e.g. 30s, 1m" />
                  <div class="invalid-feedback" id="rule-timeout-error"></div>
                </div>

                <div class="form-group">
                  <label for="rule-limit-min" class="form-label">Rate Limit Override / Minute</label>
                  <input type="number" id="rule-limit-min" class="form-control" value="${rule.rate_limit_per_minute || ''}" min="1" placeholder="None" />
                  <div class="invalid-feedback" id="rule-limit-min-error"></div>
                </div>

                <div class="form-group">
                  <label for="rule-limit-sec" class="form-label">Rate Limit Override / Second</label>
                  <input type="number" id="rule-limit-sec" class="form-control" value="${rule.rate_limit_per_second || ''}" min="1" placeholder="None" />
                  <div class="invalid-feedback" id="rule-limit-sec-error"></div>
                </div>
              </div>
            </div>

            <!-- Section 4: Endpoints pool -->
            <div class="card section-card" style="margin-top: 1.5rem;">
              <h3 class="section-title">Endpoint Pool Assignment</h3>
              <div class="form-grid-2">
                <div class="form-group">
                  <label class="form-label">Allowed Endpoint Types</label>
                  <div class="chip-input-container">
                    <div id="ep-types-chips" class="chip-input-chips"></div>
                    <input type="text" id="ep-types-input" class="chip-input-field" placeholder="Type type name..." />
                  </div>
                </div>

                <div class="form-group">
                  <label class="form-label">Required Endpoint Capabilities</label>
                  <div class="chip-input-container">
                    <div id="ep-caps-chips" class="chip-input-chips"></div>
                    <input type="text" id="ep-caps-input" class="chip-input-field" placeholder="Type capability..." />
                  </div>
                </div>
              </div>

              <!-- Endpoint pools tiers repeatable groups -->
              <div style="margin-top: 1.5rem;">
                <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom: 0.5rem;">
                  <strong class="form-label" style="margin: 0;">Fallback Tier Pools</strong>
                  <button type="button" class="btn btn-secondary btn-xs" id="btn-add-pool-tier">+ Add Tier Pool</button>
                </div>
                <div id="pool-tiers-list"></div>
              </div>
            </div>

            <!-- Section 5: Fingerprints -->
            <div class="card section-card" style="margin-top: 1.5rem;">
              <h3 class="section-title">Browser & Device Fingerprinting</h3>
              <div class="form-group">
                <label for="rule-fp-mode" class="form-label">Fingerprint Override Mode</label>
                <select id="rule-fp-mode" class="form-control select-control">
                  <option value="none" ${!rule.fingerprint_preset && !rule.fingerprint_ab_test ? 'selected' : ''}>None (Use defaults)</option>
                  <option value="preset" ${rule.fingerprint_preset ? 'selected' : ''}>Static Preset Override</option>
                  <option value="ab" ${rule.fingerprint_ab_test ? 'selected' : ''}>A/B Test Multi-variant</option>
                </select>
              </div>

              <!-- Preset fields -->
              <div id="fp-preset-group" class="fp-override-subgroup" style="display: none; margin-top: 1rem;">
                <div class="form-group">
                  <label for="rule-fp-preset-select" class="form-label">Select Fingerprint Preset</label>
                  <select id="rule-fp-preset-select" class="form-control select-control">
                    <option value="">-- Choose Preset --</option>
                    ${presets.map(p => `<option value="${p.id}" ${rule.fingerprint_preset === p.id ? 'selected' : ''}>${p.name || p.id}</option>`).join('')}
                  </select>
                </div>
              </div>

              <!-- A/B testing fields -->
              <div id="fp-ab-group" class="fp-override-subgroup" style="display: none; margin-top: 1rem;">
                <div class="form-group">
                  <label for="rule-ab-strategy" class="form-label">Selection Strategy</label>
                  <input type="text" id="rule-ab-strategy" class="form-control" value="${(rule.fingerprint_ab_test || {}).strategy || 'weighted'}" placeholder="e.g. weighted" />
                </div>
                <div style="margin-top:1rem;">
                  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom: 0.5rem;">
                    <strong>A/B Test Variants</strong>
                    <button type="button" class="btn btn-secondary btn-xs" id="btn-add-ab-variant">+ Add Variant</button>
                  </div>
                  <table class="table dense-table">
                    <thead>
                      <tr>
                        <th>Fingerprint Preset ID</th>
                        <th>Weight</th>
                        <th>% Share</th>
                        <th style="width: 40px;"></th>
                      </tr>
                    </thead>
                    <tbody id="ab-variants-rows"></tbody>
                  </table>
                  <div class="invalid-feedback" id="ab-variants-error" style="display:block; margin-top: 0.5rem;"></div>
                </div>
              </div>
            </div>

            <!-- Section 6: Request Filters -->
            <div class="card section-card" style="margin-top: 1.5rem;">
              <h3 class="section-title">Request Filtering & Blocklists</h3>
              <div class="form-group form-check-group" style="margin-bottom: 1rem;">
                <label class="form-check-label">
                  <input type="checkbox" id="rule-filters-adblock" ${((rule.request_filters || {}).enable_adblock) ? 'checked' : ''} />
                  <strong>Enable AdBlock Engine lists</strong>
                </label>
              </div>

              <div class="form-group">
                <label for="rule-filters-lists" class="form-label">AdBlock Filter Lists (one URL per line)</label>
                <textarea id="rule-filters-lists" class="form-control text-control" rows="3" placeholder="https://easylist.to/easylist/easylist.txt">${((rule.request_filters || {}).adblock_lists || []).join('\n')}</textarea>
              </div>

              <div class="form-group" style="margin-top: 1rem;">
                <label class="form-label">Block Content Types (MIME)</label>
                <div class="chip-input-container">
                  <div id="block-types-chips" class="chip-input-chips"></div>
                  <input type="text" id="block-types-input" class="chip-input-field" placeholder="image/gif, video/*, etc." />
                </div>
              </div>

              <div class="form-group" style="margin-top: 1rem;">
                <label for="rule-filters-patterns" class="form-label">Block URL Patterns (one glob/regex per line)</label>
                <textarea id="rule-filters-patterns" class="form-control text-control" rows="3" placeholder="*ads*.*&#10;.*google-analytics\\.com.*">${((rule.request_filters || {}).block_url_patterns || []).join('\n')}</textarea>
              </div>

              <div class="form-group" style="margin-top: 1rem;">
                <label class="form-label">Blocked Domains</label>
                <div class="chip-input-container">
                  <div id="block-domains-chips" class="chip-input-chips"></div>
                  <input type="text" id="block-domains-input" class="chip-input-field" placeholder="doubleclick.net, facebook.com, etc." />
                </div>
              </div>
            </div>

            <!-- Section 7: TLS / Certificate Pinning -->
            <div class="card section-card" style="margin-top: 1.5rem;">
              <h3 class="section-title">TLS & Certificate Pinning</h3>
              <div class="form-group form-check-group" style="margin-bottom: 1rem;">
                <label class="form-check-label">
                  <input type="checkbox" id="rule-insecure-tls" ${rule.allow_insecure_tls ? 'checked' : ''} />
                  <strong style="color: var(--color-rose-500);">Allow Insecure TLS (Skip domain/cert check)</strong>
                </label>
                <div class="checkbox-warning">
                  Security Warning: Skipping cert checks allows Man-In-The-Middle attacks.
                </div>
              </div>

              <div class="form-group">
                <label for="rule-cert-hash" class="form-label">Pinned Certificate SHA256 Hash</label>
                <input type="text" id="rule-cert-hash" class="form-control" value="${rule.pinned_cert_hash || ''}" placeholder="SHA-256 hex string..." />
                <div class="invalid-feedback" id="rule-cert-hash-error"></div>
              </div>
            </div>

            <!-- Form submission actions -->
            <div style="margin-top: 2rem; display:flex; gap:1rem; align-items:center;">
              <button type="submit" class="btn btn-primary btn-lg" id="btn-save-rule">Save Routing Rule</button>
              <a href="#/routing-rules" class="btn btn-secondary btn-lg">Cancel</a>
            </div>
          </form>
        </div>

        <!-- Right Side Panel: Raw JSON sync editor -->
        <div class="editor-side-col">
          <div class="card sticky-card">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom: 0.75rem;">
              <h3 class="card-title" style="margin:0;">Raw API Payload JSON</h3>
              <button class="btn btn-secondary btn-xs" id="btn-sync-raw-json">Sync JSON to Form</button>
            </div>
            
            <textarea id="raw-json-editor" class="raw-json-panel" style="flex-grow:1; font-family: monospace; font-size: 0.85rem; height: 450px;" placeholder="Parsing form data..."></textarea>
            
            ${jsonError ? `<div class="alert alert-error" style="margin-top:0.75rem; font-size:0.8rem; padding:0.5rem 0.75rem;">${jsonError}</div>` : ''}
          </div>
        </div>
      </div>
    `;
  },

  async refresh(state) {
    const hash = state.currentPage || '';
    const urlParams = new URLSearchParams(hash.includes('?') ? hash.split('?')[1] : '');
    const ruleId = urlParams.get('id');

    setState({ rulesLoading: true });
    try {
      const endpoints = await listEndpoints().catch(() => []);
      const presets = await listFingerprints().catch(() => []);
      
      setState({ 
        endpointsData: endpoints,
        fingerprintsData: presets
      });

      if (ruleId) {
        const rule = await getRoutingRule(ruleId);
        setState({ 
          editingRule: rule,
          editingRuleId: ruleId,
          rulesLoading: false
        });
      } else {
        // If not editing, keep whatever editingRule was set by duplicate, or default to empty
        if (!state.editingRule) {
          setState({ editingRule: { priority: 0, is_active: true } });
        }
        setState({ editingRuleId: null, rulesLoading: false });
      }
    } catch (err) {
      setState({ rulesLoading: false });
      showToast(`Failed to initialize editor data: ${err.message}`, 'error');
    }
  },

  afterRender(state) {
    const hash = state.currentPage || '';
    const urlParams = new URLSearchParams(hash.includes('?') ? hash.split('?')[1] : '');
    const ruleId = urlParams.get('id') || state.editingRuleId;
    const isEdit = !!ruleId;

    if (!state.endpointsData && !state.rulesLoading) {
      this.refresh(state);
      return;
    }

    const form = document.getElementById('rule-form');
    if (!form) return;

    // Track dynamic form values in memory
    const formState = {
      required_tags: [...(state.editingRule?.required_tags || [])],
      excluded_tags: [...(state.editingRule?.excluded_tags || [])],
      allowed_endpoint_types: [...(state.editingRule?.allowed_endpoint_types || [])],
      required_endpoint_caps: [...(state.editingRule?.required_endpoint_caps || [])],
      endpoint_pools: JSON.parse(JSON.stringify(state.editingRule?.endpoint_pools || [])),
      ab_variants: JSON.parse(JSON.stringify(state.editingRule?.fingerprint_ab_test?.variants || []))
    };

    // Helper functions for chip lists
    const setupChipList = (inputId, containerId, listKey) => {
      const input = document.getElementById(inputId);
      const container = document.getElementById(containerId);
      if (!input || !container) return;

      const render = () => {
        container.innerHTML = formState[listKey].map((c, i) => `
          <span class="chip">
            <span>${c}</span>
            <button type="button" class="btn-remove-chip" data-idx="${i}">&times;</button>
          </span>
        `).join('');
        
        container.querySelectorAll('.btn-remove-chip').forEach(btn => {
          btn.addEventListener('click', () => {
            const idx = parseInt(btn.getAttribute('data-idx'));
            formState[listKey].splice(idx, 1);
            render();
            updateRawJsonPayload();
          });
        });
      };
      
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ',') {
          e.preventDefault();
          const val = input.value.trim();
          if (val) {
            if (listKey === 'required_tags' || listKey === 'excluded_tags') {
              try {
                parseTag(val);
              } catch (err) {
                showToast(`Tag error: ${err.message}`, 'error');
                return;
              }
            }
            if (!formState[listKey].includes(val)) {
              formState[listKey].push(val);
              render();
              updateRawJsonPayload();
            }
            input.value = '';
          }
        }
      });

      render();
    };

    setupChipList('required-tags-input', 'required-tags-chips', 'required_tags');
    setupChipList('excluded-tags-input', 'excluded-tags-chips', 'excluded_tags');
    setupChipList('ep-types-input', 'ep-types-chips', 'allowed_endpoint_types');
    setupChipList('ep-caps-input', 'ep-caps-chips', 'required_endpoint_caps');
    
    // Blocked content types and blocked domains
    const filters = state.editingRule?.request_filters || {};
    formState.block_content_types = [...(filters.block_content_types || [])];
    formState.block_domains = [...(filters.block_domains || [])];
    
    setupChipList('block-types-input', 'block-types-chips', 'block_content_types');
    setupChipList('block-domains-input', 'block-domains-chips', 'block_domains');

    // Repeater lists renders
    const poolsContainer = document.getElementById('pool-tiers-list');
    const renderPoolTiers = () => {
      poolsContainer.innerHTML = formState.endpoint_pools.map((pool, idx) => {
        const epOptions = (state.endpointsData || []).map(ep => {
          const selected = (pool.endpoint_ids || []).includes(ep.id) ? 'selected' : '';
          return `<option value="${ep.id}" ${selected}>${ep.id}</option>`;
        }).join('');

        return `
          <div class="card pool-tier-item" style="margin-top:0.75rem; padding:1rem; position:relative;">
            <button type="button" class="btn-remove-pool btn-remove-chip" data-idx="${idx}" style="position:absolute; top:0.5rem; right:0.5rem; font-size:1.25rem;">&times;</button>
            <div class="form-grid-3">
              <div class="form-group">
                <label class="form-label">Tier Priority</label>
                <input type="number" class="form-control pool-tier-input" data-idx="${idx}" value="${pool.tier}" required />
              </div>
              <div class="form-group">
                <label class="form-label">Max Retries</label>
                <input type="number" class="form-control pool-retries-input" data-idx="${idx}" value="${pool.max_retries || 0}" required />
              </div>
              <div class="form-group">
                <label class="form-label">Endpoints (Command/Ctrl select)</label>
                <select class="form-control pool-eps-select" data-idx="${idx}" multiple style="height: 60px;">
                  ${epOptions}
                </select>
              </div>
            </div>
          </div>
        `;
      }).join('');

      // Bind dynamic change listeners for pools
      poolsContainer.querySelectorAll('.btn-remove-pool').forEach(btn => {
        btn.addEventListener('click', () => {
          const idx = parseInt(btn.getAttribute('data-idx'));
          formState.endpoint_pools.splice(idx, 1);
          renderPoolTiers();
          updateRawJsonPayload();
        });
      });

      poolsContainer.querySelectorAll('.pool-tier-input').forEach(input => {
        input.addEventListener('change', (e) => {
          const idx = parseInt(input.getAttribute('data-idx'));
          formState.endpoint_pools[idx].tier = parseInt(e.target.value) || 0;
          updateRawJsonPayload();
        });
      });

      poolsContainer.querySelectorAll('.pool-retries-input').forEach(input => {
        input.addEventListener('change', (e) => {
          const idx = parseInt(input.getAttribute('data-idx'));
          formState.endpoint_pools[idx].max_retries = parseInt(e.target.value) || 0;
          updateRawJsonPayload();
        });
      });

      poolsContainer.querySelectorAll('.pool-eps-select').forEach(sel => {
        sel.addEventListener('change', () => {
          const idx = parseInt(sel.getAttribute('data-idx'));
          const selected = Array.from(sel.selectedOptions).map(opt => opt.value);
          formState.endpoint_pools[idx].endpoint_ids = selected;
          updateRawJsonPayload();
        });
      });
    };

    const addPoolBtn = document.getElementById('btn-add-pool-tier');
    if (addPoolBtn) {
      addPoolBtn.addEventListener('click', () => {
        formState.endpoint_pools.push({
          tier: formState.endpoint_pools.length + 1,
          max_retries: 3,
          endpoint_ids: []
        });
        renderPoolTiers();
        updateRawJsonPayload();
      });
    }

    renderPoolTiers();

    // Fingerprint Mode toggles
    const fpModeSelect = document.getElementById('rule-fp-mode');
    const fpPresetGroup = document.getElementById('fp-preset-group');
    const fpAbGroup = document.getElementById('fp-ab-group');

    const updateFpVisibility = () => {
      const mode = fpModeSelect.value;
      if (mode === 'none') {
        fpPresetGroup.style.display = 'none';
        fpAbGroup.style.display = 'none';
      } else if (mode === 'preset') {
        fpPresetGroup.style.display = 'block';
        fpAbGroup.style.display = 'none';
      } else if (mode === 'ab') {
        fpPresetGroup.style.display = 'none';
        fpAbGroup.style.display = 'block';
      }
    };

    fpModeSelect.addEventListener('change', () => {
      updateFpVisibility();
      updateRawJsonPayload();
    });
    updateFpVisibility();

    // A/B test variants list render
    const abVariantsContainer = document.getElementById('ab-variants-rows');
    const renderAbVariants = () => {
      const presetsList = state.fingerprintsData || [];
      abVariantsContainer.innerHTML = formState.ab_variants.map((v, idx) => {
        const presetOptions = presetsList.map(p => `
          <option value="${p.id}" ${v.preset_id === p.id ? 'selected' : ''}>${p.name || p.id}</option>
        `).join('');

        return `
          <tr>
            <td>
              <select class="form-control select-control ab-preset-select" data-idx="${idx}">
                <option value="">-- Choose Preset --</option>
                ${presetOptions}
              </select>
            </td>
            <td>
              <input type="number" class="form-control ab-weight-input" data-idx="${idx}" value="${v.weight}" min="1" required />
            </td>
            <td class="ab-percentage-cell" data-idx="${idx}">0%</td>
            <td>
              <button type="button" class="btn-remove-chip btn-remove-variant" data-idx="${idx}">&times;</button>
            </td>
          </tr>
        `;
      }).join('');

      calculatePercentages();

      // Bind variant changes
      abVariantsContainer.querySelectorAll('.btn-remove-variant').forEach(btn => {
        btn.addEventListener('click', () => {
          const idx = parseInt(btn.getAttribute('data-idx'));
          formState.ab_variants.splice(idx, 1);
          renderAbVariants();
          updateRawJsonPayload();
        });
      });

      abVariantsContainer.querySelectorAll('.ab-preset-select').forEach(sel => {
        sel.addEventListener('change', (e) => {
          const idx = parseInt(sel.getAttribute('data-idx'));
          formState.ab_variants[idx].preset_id = e.target.value;
          updateRawJsonPayload();
        });
      });

      abVariantsContainer.querySelectorAll('.ab-weight-input').forEach(input => {
        input.addEventListener('change', (e) => {
          const idx = parseInt(input.getAttribute('data-idx'));
          formState.ab_variants[idx].weight = parseInt(e.target.value) || 0;
          calculatePercentages();
          updateRawJsonPayload();
        });
      });
    };

    const calculatePercentages = () => {
      const totalWeight = formState.ab_variants.reduce((acc, curr) => acc + (curr.weight || 0), 0);
      abVariantsContainer.querySelectorAll('.ab-percentage-cell').forEach(cell => {
        const idx = parseInt(cell.getAttribute('data-idx'));
        const weight = formState.ab_variants[idx].weight || 0;
        const pct = totalWeight > 0 ? ((weight / totalWeight) * 100).toFixed(1) : '0';
        cell.textContent = `${pct}%`;
      });
    };

    const addVariantBtn = document.getElementById('btn-add-ab-variant');
    if (addVariantBtn) {
      addVariantBtn.addEventListener('click', () => {
        formState.ab_variants.push({
          preset_id: '',
          weight: 1
        });
        renderAbVariants();
        updateRawJsonPayload();
      });
    }

    renderAbVariants();

    // Assemble form data into single request JSON payload
    const getPayloadFromInputs = () => {
      const name = document.getElementById('rule-name').value.trim();
      const priority = parseInt(document.getElementById('rule-priority').value) || 0;
      const is_active = document.getElementById('rule-active').checked;
      const quota_key = document.getElementById('rule-quota-key').value.trim() || null;

      const hard_timeout = document.getElementById('rule-timeout').value.trim() || null;
      const rate_limit_per_minute = parseInt(document.getElementById('rule-limit-min').value) || null;
      const rate_limit_per_second = parseInt(document.getElementById('rule-limit-sec').value) || null;

      const allow_insecure_tls = document.getElementById('rule-insecure-tls').checked;
      const pinned_cert_hash = document.getElementById('rule-cert-hash').value.trim() || null;

      // Fingerprinting
      let fingerprint_preset = null;
      let fingerprint_ab_test = null;
      const fpMode = fpModeSelect.value;
      if (fpMode === 'preset') {
        fingerprint_preset = document.getElementById('rule-fp-preset-select').value || null;
      } else if (fpMode === 'ab') {
        fingerprint_ab_test = {
          strategy: document.getElementById('rule-ab-strategy').value.trim() || 'weighted',
          variants: formState.ab_variants.filter(v => v.preset_id)
        };
      }

      // Filters
      const request_filters = {
        enable_adblock: document.getElementById('rule-filters-adblock').checked,
        adblock_lists: document.getElementById('rule-filters-lists').value.split('\n').map(l => l.trim()).filter(Boolean),
        block_content_types: formState.block_content_types,
        block_url_patterns: document.getElementById('rule-filters-patterns').value.split('\n').map(p => p.trim()).filter(Boolean),
        block_domains: formState.block_domains
      };

      const payload = {
        name,
        priority,
        is_active,
        quota_key,
        required_tags: formState.required_tags,
        excluded_tags: formState.excluded_tags,
        allowed_endpoint_types: formState.allowed_endpoint_types,
        required_endpoint_caps: formState.required_endpoint_caps,
        endpoint_pools: formState.endpoint_pools.filter(p => p.endpoint_ids && p.endpoint_ids.length > 0),
        hard_timeout,
        rate_limit_per_minute,
        rate_limit_per_second,
        fingerprint_preset,
        fingerprint_ab_test,
        request_filters,
        allow_insecure_tls,
        pinned_cert_hash
      };

      if (isEdit && state.editingRule) {
        payload.version = state.editingRule.version;
      }

      return payload;
    };

    const rawJsonTextarea = document.getElementById('raw-json-editor');

    const updateRawJsonPayload = () => {
      const payload = getPayloadFromInputs();
      rawJsonTextarea.value = JSON.stringify(payload, null, 2);
    };

    // Bind inputs changes to raw JSON display updates
    const inputsToBind = [
      'rule-name', 'rule-priority', 'rule-active', 'rule-quota-key',
      'rule-timeout', 'rule-limit-min', 'rule-limit-sec',
      'rule-fp-preset-select', 'rule-ab-strategy',
      'rule-filters-adblock', 'rule-filters-lists', 'rule-filters-patterns',
      'rule-insecure-tls', 'rule-cert-hash'
    ];
    inputsToBind.forEach(id => {
      const el = document.getElementById(id);
      if (el) el.addEventListener('input', updateRawJsonPayload);
    });

    updateRawJsonPayload();

    // Sync Raw JSON Textarea back to inputs
    const syncRawJsonBtn = document.getElementById('btn-sync-raw-json');
    if (syncRawJsonBtn) {
      syncRawJsonBtn.addEventListener('click', () => {
        setState({ ruleJsonError: null });
        try {
          const parsed = JSON.parse(rawJsonTextarea.value);
          if (typeof parsed !== 'object' || parsed === null) {
            throw new Error('Payload must be a JSON object');
          }

          // Load parsed data back into local form state
          document.getElementById('rule-name').value = parsed.name || '';
          document.getElementById('rule-priority').value = parsed.priority || 0;
          document.getElementById('rule-active').checked = parsed.is_active !== false;
          document.getElementById('rule-quota-key').value = parsed.quota_key || '';

          document.getElementById('rule-timeout').value = parsed.hard_timeout || '';
          document.getElementById('rule-limit-min').value = parsed.rate_limit_per_minute || '';
          document.getElementById('rule-limit-sec').value = parsed.rate_limit_per_second || '';

          formState.required_tags = [...(parsed.required_tags || [])];
          formState.excluded_tags = [...(parsed.excluded_tags || [])];
          formState.allowed_endpoint_types = [...(parsed.allowed_endpoint_types || [])];
          formState.required_endpoint_caps = [...(parsed.required_endpoint_caps || [])];
          formState.endpoint_pools = JSON.parse(JSON.stringify(parsed.endpoint_pools || []));

          // Filters
          const filters = parsed.request_filters || {};
          document.getElementById('rule-filters-adblock').checked = !!filters.enable_adblock;
          document.getElementById('rule-filters-lists').value = (filters.adblock_lists || []).join('\n');
          formState.block_content_types = [...(filters.block_content_types || [])];
          document.getElementById('rule-filters-patterns').value = (filters.block_url_patterns || []).join('\n');
          formState.block_domains = [...(filters.block_domains || [])];

          // TLS
          document.getElementById('rule-insecure-tls').checked = !!parsed.allow_insecure_tls;
          document.getElementById('rule-cert-hash').value = parsed.pinned_cert_hash || '';

          // Fingerprinting
          if (parsed.fingerprint_preset) {
            fpModeSelect.value = 'preset';
            const selPreset = document.getElementById('rule-fp-preset-select');
            if (selPreset) selPreset.value = parsed.fingerprint_preset;
          } else if (parsed.fingerprint_ab_test) {
            fpModeSelect.value = 'ab';
            document.getElementById('rule-ab-strategy').value = parsed.fingerprint_ab_test.strategy || 'weighted';
            formState.ab_variants = JSON.parse(JSON.stringify(parsed.fingerprint_ab_test.variants || []));
          } else {
            fpModeSelect.value = 'none';
          }

          // Re-render subelements
          setupChipList('required-tags-input', 'required-tags-chips', 'required_tags');
          setupChipList('excluded-tags-input', 'excluded-tags-chips', 'excluded_tags');
          setupChipList('ep-types-input', 'ep-types-chips', 'allowed_endpoint_types');
          setupChipList('ep-caps-input', 'ep-caps-chips', 'required_endpoint_caps');
          setupChipList('block-types-input', 'block-types-chips', 'block_content_types');
          setupChipList('block-domains-input', 'block-domains-chips', 'block_domains');
          
          renderPoolTiers();
          updateFpVisibility();
          renderAbVariants();

          showToast('Form synced with JSON successfully', 'success');
        } catch (err) {
          setState({ ruleJsonError: err.message });
          showToast('JSON Sync Failed', 'error');
        }
      });
    }

    // Submit Action
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      
      // Clean error borders
      form.querySelectorAll('.is-invalid').forEach(el => el.classList.remove('is-invalid'));
      form.querySelectorAll('.invalid-feedback').forEach(el => el.textContent = '');

      let isValid = true;
      const nameInput = document.getElementById('rule-name');
      const priorityInput = document.getElementById('rule-priority');
      const timeoutInput = document.getElementById('rule-timeout');
      
      const nameErr = document.getElementById('rule-name-error');
      const priorityErr = document.getElementById('rule-priority-error');
      const timeoutErr = document.getElementById('rule-timeout-error');

      if (!nameInput.value.trim()) {
        nameErr.textContent = 'Rule name is required.';
        nameInput.classList.add('is-invalid');
        isValid = false;
      }

      if (priorityInput.value.trim() === '') {
        priorityErr.textContent = 'Priority integer is required.';
        priorityInput.classList.add('is-invalid');
        isValid = false;
      }

      if (timeoutInput.value.trim()) {
        try {
          validateDuration(timeoutInput.value);
        } catch (err) {
          timeoutErr.textContent = err.message;
          timeoutInput.classList.add('is-invalid');
          isValid = false;
        }
      }

      // Validate A/B testing variants if mode active
      const abErr = document.getElementById('ab-variants-error');
      if (abErr) abErr.textContent = '';
      if (fpModeSelect.value === 'ab') {
        const validVariants = formState.ab_variants.filter(v => v.preset_id && v.weight > 0);
        if (validVariants.length < 2) {
          abErr.textContent = 'A/B tests require at least 2 presets with positive weights.';
          isValid = false;
        }
      }

      if (!isValid) {
        const firstInvalid = form.querySelector('.is-invalid');
        if (firstInvalid) firstInvalid.focus();
        return;
      }

      // Proceed to save
      const payload = getPayloadFromInputs();
      const saveBtn = document.getElementById('btn-save-rule');
      saveBtn.disabled = true;
      saveBtn.innerHTML = '<span class="spinner"></span> Saving...';

      try {
        if (isEdit) {
          await updateRoutingRule(ruleId, payload);
          showToast('Routing rule updated successfully', 'success');
        } else {
          await createRoutingRule(payload);
          showToast('Routing rule created successfully', 'success');
        }
        
        // Clear editing state and redirect
        setState({ editingRule: null, editingRuleId: null });
        window.location.hash = '#/routing-rules';
      } catch (err) {
        saveBtn.disabled = false;
        saveBtn.textContent = 'Save Routing Rule';

        // Conflict check: optimistic lock version clash
        if (err instanceof ApiError && err.status === 500 && (err.message.includes('routing rule not found') || err.message.includes('not found'))) {
          // Rule version conflict
          showConfirm({
            title: 'Version Conflict Detected',
            body: `Another operator has modified rule <strong>${payload.name}</strong> since you loaded it. <br/><br/>Would you like to fetch the latest database version to review changes, or force-overwrite the database rules?`,
            confirmText: 'Review Latest',
            callback: async () => {
              // Fetch latest rule from database and overlay on editor
              try {
                const latestRule = await getRoutingRule(ruleId);
                setState({ editingRule: latestRule });
                showToast('Loaded latest version for review', 'info');
                
                // Reload page view
                const appDiv = document.querySelector('#app');
                import('../components/shell.js').then(({ renderShell }) => {
                  appDiv.innerHTML = renderShell(state, RoutingRuleEditorPage.render(state));
                  RoutingRuleEditorPage.afterRender(state);
                });
              } catch (e) {
                showToast(`Failed to fetch latest: ${e.message}`, 'error');
              }
            }
          });
        } else {
          showToast(`Save failed: ${err.message}`, 'error');
        }
      }
    });
  }
};
