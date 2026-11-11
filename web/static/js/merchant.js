import { showToast } from './api.js';

function escapeHtml(value) { const element = document.createElement('span'); element.textContent = String(value); return element.innerHTML; }
let csrfToken = '';
sessionStorage.removeItem('fairdeal-merchant-token');
const merchantReady = fetch('/api/merchant/session', { credentials: 'same-origin', cache: 'no-store' })
  .then(async response => {
    if (!response.ok) { window.location.replace('/merchant/login'); throw new Error('Sign in to continue'); }
    const session = await response.json();
    csrfToken = session.csrf_token;
    const label = document.getElementById('merchant-email');
    if (label) label.textContent = session.email;
  });
export async function merchantFetch(url, options = {}) {
  await merchantReady;
  const headers = new Headers(options.headers || {});
  if (options.method && options.method !== 'GET') {
    headers.set('Content-Type', 'application/json');
    headers.set('X-CSRF-Token', csrfToken);
  }
  const response = await fetch(url, { ...options, credentials: 'same-origin', cache: 'no-store', headers });
  if (response.status === 401) { window.location.replace('/merchant/login'); throw new Error('Session expired'); }
  return response;
}

class MerchantDashboard {
  constructor() {
    this.currentPolicy = null;
    this.currentDraft = null;
    this.pollInterval = null;

    this.initDOMElements();
 window.addEventListener("fairdeal-policy-updated",()=>this.loadPolicy());
    this.bindEvents();
    const logout = document.getElementById('merchant-logout');
    if (logout) logout.addEventListener('click', async () => {
      try {
        const response = await merchantFetch('/api/merchant/logout', { method: 'POST' });
        if (!response.ok) throw new Error('Could not sign out. Please retry.');
        csrfToken = '';
        window.location.replace('/merchant/login');
      } catch (error) { showToast(error.message); }
    });
    merchantReady.then(() => { this.loadAll(); this.startPolling(); }).catch(error => showToast(error.message));
  }

  initDOMElements() {
    // Metrics
    this.metricRescued = document.getElementById('metric-rescued');
    this.metricGMV = document.getElementById('metric-gmv');
    this.metricPayLater = document.getElementById('metric-paylater');
    this.metricMargin = document.getElementById('metric-margin');
    this.metricDeadstock = document.getElementById('metric-deadstock');
    this.metricLatency = document.getElementById('metric-latency');

    // Policy AI
    this.policyPrompt = document.getElementById('policy-prompt');
    this.btnCompile = document.getElementById('btn-compile-policy');
    this.diffContainer = document.getElementById('policy-diff-container');
    this.diffTableBody = document.getElementById('diff-table-body');
    this.adjustmentsList = document.getElementById('adjustments-list');
    this.policyRationale = document.getElementById('policy-rationale');
    this.btnActivate = document.getElementById('btn-activate-policy');

    // Audit Log
    this.agreementsTableBody = document.getElementById('agreements-table-body');

    // Demo reset
    this.btnResetDemo = document.getElementById('btn-reset-demo');
  }

  bindEvents() {
    this.btnCompile.addEventListener('click', () => this.compilePolicy());
    this.btnActivate.addEventListener('click', () => this.activatePolicy());

    if (this.btnResetDemo) {
      this.btnResetDemo.addEventListener('click', async () => {
        if (!confirm('Are you sure you want to reset all demo negotiations and restore default seed policy?')) {
          return;
        }
        try {
          const res = await merchantFetch('/api/merchant/demo/reset', { method: 'POST' });
          if (!res.ok) throw new Error('Reset failed');
          showToast('Demo data reset successfully!');
          this.loadAll();
        } catch (err) {
          showToast(err.message || 'Reset failed');
        }
      });
    }
  }

  async loadAll() {
    await Promise.all([
      this.loadMetrics(),
      this.loadPolicy(),
      this.loadAgreements()
    ]);
  }

  startPolling() {
    this.pollInterval = setInterval(() => {
      this.loadMetrics();
      this.loadAgreements();
    }, 5000);
  }

  async loadMetrics() {
    try {
      const res = await merchantFetch('/api/merchant/metrics');
      if (!res.ok) return;
      const m = await res.json();

      this.metricRescued.textContent = m.rescued_carts;
      this.metricGMV.textContent = `$${m.recovered_gmv}`;
      const comparison = document.getElementById('compare-winwin-gmv');
      if (comparison) comparison.textContent = `$${m.recovered_gmv} captured`;
      this.metricPayLater.textContent = `${Math.round(m.pay_later_share * 100)}%`;
      this.metricMargin.textContent = `${(m.avg_contribution_margin_pct * 100).toFixed(1)}%`;
      this.metricDeadstock.textContent = m.deadstock_units_cleared;
      this.metricLatency.textContent = `${(m.solver_latency_p50_us / 1000).toFixed(2)} ms`;
    } catch (err) {
      console.warn('Metrics fetch error', err);
    }
  }

  async loadPolicy() {
    try {
      const res = await merchantFetch('/api/merchant/policy');
      if (!res.ok) return;
      const data = await res.json();
      this.currentPolicy = data.active?.policy;
      this.renderActivePolicySummary(data.active);
    } catch (err) {
      console.warn('Policy fetch error', err);
    }
  }

  renderActivePolicySummary(active) {
    const el = document.getElementById('active-policy-summary');
    if (!el || !active) return;
    const p = active.policy;
    el.innerHTML = `
      <span class="badge" style="background:rgba(63,185,80,0.15);color:var(--green);border-color:rgba(63,185,80,0.3);">ACTIVE</span>
      <span>Min Margin: <strong>${(p.min_margin_pct * 100).toFixed(0)}%</strong></span> &bull;
      <span>Max Discount: <strong>${(p.max_discount_pct * 100).toFixed(0)}%</strong></span> &bull;
      <span>Buyer Power (&alpha;): <strong>${p.buyer_power.toFixed(2)}</strong></span> &bull;
      <span>Pay Later: <strong>${p.pay_later_enabled ? 'Enabled' : 'Disabled'}</strong></span> &bull;
      <span>Bundle SKU: <strong>${escapeHtml(p.bundle_sku || 'None')}</strong></span>
    `;
  }

  async compilePolicy() {
    const text = this.policyPrompt.value.trim();
    if (!text) {
      showToast('Please enter merchant strategy instructions');
      return;
    }

    this.btnCompile.disabled = true;
    this.btnCompile.textContent = 'Compiling with AI...';

    try {
      const res = await merchantFetch('/api/merchant/policy/compile', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error?.message || 'Compile failed');
      }

      const data = await res.json();
      this.currentDraft = data.draft;
      this.renderDiff(this.currentPolicy, data.draft.policy, data.adjustments);
      showToast('Policy draft generated with risk bounds applied!');
    } catch (err) {
      showToast(err.message || 'Failed to compile policy');
    } finally {
      this.btnCompile.disabled = false;
      this.btnCompile.textContent = 'Compile policy with AI';
    }
  }

  renderDiff(cur, draft, adjustments) {
    if (!draft) return;
    this.diffContainer.style.display = 'block';
    this.diffTableBody.innerHTML = '';

    const fields = [
      { key: 'min_margin_pct', label: 'Minimum Margin', fmt: v => `${(v * 100).toFixed(0)}%` },
      { key: 'max_discount_pct', label: 'Max Discount Ceiling', fmt: v => `${(v * 100).toFixed(0)}%` },
      { key: 'buyer_power', label: 'Bargaining Power (α)', fmt: v => v.toFixed(2) },
      { key: 'allow_slow_shipping', label: 'Allow Ground Shipping', fmt: v => v ? 'Yes' : 'No' },
      { key: 'pay_later_enabled', label: 'Enable Pay Later', fmt: v => v ? 'Yes' : 'No' },
      { key: 'bundle_sku', label: 'Deadstock Clearance SKU', fmt: v => v || 'None' }
    ];

    for (const f of fields) {
      const curVal = cur ? cur[f.key] : null;
      const draftVal = draft[f.key];
      const changed = curVal !== draftVal;

      const tr = document.createElement('tr');
      if (changed) tr.style.background = 'rgba(88, 166, 255, 0.08)';

      tr.innerHTML = `
        <td style="font-weight:600;color:var(--heading);">${f.label}</td>
        <td>${escapeHtml(curVal !== null ? f.fmt(curVal) : '—')}</td>
        <td style="${changed ? 'color:var(--accent);font-weight:700;' : ''}">${escapeHtml(f.fmt(draftVal))}</td>
      `;
      this.diffTableBody.appendChild(tr);
    }

    // Adjustments list (Guardrails)
    this.adjustmentsList.innerHTML = '';
    if (adjustments && adjustments.length > 0) {
      for (const adj of adjustments) {
        const li = document.createElement('li');
        li.textContent = 'Guardrail: ' + adj;
        this.adjustmentsList.appendChild(li);
      }
    } else {
      const li = document.createElement('li');
      li.textContent = 'All parameters fell cleanly within safety bounds (no clamping required).';
      this.adjustmentsList.appendChild(li);
    }

    this.policyRationale.textContent = draft.rationale || 'Risk-bounded commercial strategy';
    this.diffContainer.scrollIntoView({ behavior: 'smooth' });
  }

  async activatePolicy() {
    if (!this.currentDraft) return;

    this.btnActivate.disabled = true;
    this.btnActivate.textContent = 'Activating...';

    try {
      const res = await merchantFetch(`/api/merchant/policy/${this.currentDraft.id}/activate`, {
        method: 'POST'
      });
      if (!res.ok) throw new Error('Activation failed');

      showToast('Policy activated! Subsequent negotiations will use these parameters.');
      this.diffContainer.style.display = 'none';
      await this.loadPolicy();
 window.dispatchEvent(new Event("fairdeal-ai-policy-updated"));
    } catch (err) {
      showToast(err.message || 'Activation failed');
    } finally {
      this.btnActivate.disabled = false;
      this.btnActivate.textContent = 'Activate Policy';
    }
  }

  async loadAgreements() {
    try {
      const res = await merchantFetch('/api/merchant/agreements?limit=15');
      if (!res.ok) return;
      const list = await res.json();
      this.renderAgreementsTable(list);
    } catch (err) {
      console.warn('Agreements fetch error', err);
    }
  }

  renderAgreementsTable(list) {
    this.agreementsTableBody.innerHTML = '';
    if (!list || list.length === 0) {
      this.agreementsTableBody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);">No negotiations recorded yet. Try negotiating on the store view!</td></tr>';
      return;
    }

    for (const a of list) {
      const tr = document.createElement('tr');
      const timeStr = new Date(a.created_at * 1000).toLocaleTimeString();
      const statusColor = a.status === 'CAPTURED' ? 'var(--green)' : a.status === 'OFFERED' ? 'var(--yellow)' : 'var(--text-muted)';

      tr.innerHTML = `
        <td style="font-family:var(--font-mono);font-size:0.8rem;">${timeStr}</td>
        <td><strong>${escapeHtml(a.sku)}</strong></td>
        <td style="font-weight:700;color:var(--heading);">$${(a.price_cents / 100).toFixed(2)}</td>
        <td style="font-size:0.82rem;">
          ${a.shipping_tier === 'CONSOLIDATED_GROUND' ? 'Ground' : 'Express'} &bull;
          ${a.pay_later ? 'Pay in 4' : 'Card'} &bull;
          ${a.bundle_sku ? 'Bundle' : 'Single'}
        </td>
        <td><span style="color:${statusColor};font-weight:700;font-size:0.8rem;">${escapeHtml(a.status)}</span></td>
        <td style="font-family:var(--font-mono);font-size:0.75rem;color:var(--text-muted);">${escapeHtml(a.paypal_order_id || '—')}</td>
      `;
      const cell=document.createElement('td');const details=document.createElement('details');const summary=document.createElement('summary');summary.textContent='View economics';const content=document.createElement('div');summary.addEventListener('click',()=>{if(!details.open){window.dispatchEvent(new CustomEvent('fairdeal-show-economics',{detail:{id:a.id,target:content}}));}});details.append(summary,content);cell.append(details);tr.append(cell);
 this.agreementsTableBody.appendChild(tr);
    }
  }
}

window.addEventListener('DOMContentLoaded', () => {
  new MerchantDashboard();
});
