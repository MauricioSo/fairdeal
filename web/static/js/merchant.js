import { showToast } from './api.js';

function escapeHtml(value) {
  const element = document.createElement('span');
  element.textContent = String(value);
  return element.innerHTML;
}

let csrfToken = '';
sessionStorage.removeItem('fairdeal-merchant-token');

const merchantReady = fetch('/api/merchant/session', { credentials: 'same-origin', cache: 'no-store' })
  .then(async response => {
    if (!response.ok) {
      window.location.replace('/merchant/login');
      throw new Error('Sign in to continue');
    }
    const session = await response.json();
    csrfToken = session.csrf_token;
    const label = document.getElementById('merchant-email');
    if (label) label.textContent = session.email;
    const storeDisplay = document.getElementById('store-title-display');
    if (storeDisplay && session.merchant_name) {
      storeDisplay.textContent = session.merchant_name + ' • Strategy Center';
    }
    const navIdentity = document.getElementById('active-merchant-name');
    if (navIdentity) {
      navIdentity.textContent = session.merchant_name || session.email;
    }
  });

export async function merchantFetch(url, options = {}) {
  await merchantReady;
  const headers = new Headers(options.headers || {});
  if (options.method && options.method !== 'GET') {
    headers.set('Content-Type', 'application/json');
    headers.set('X-CSRF-Token', csrfToken);
  }
  const response = await fetch(url, { ...options, credentials: 'same-origin', cache: 'no-store', headers });
  if (response.status === 401) {
    window.location.replace('/merchant/login');
    throw new Error('Session expired');
  }
  return response;
}

class MerchantDashboard {
  constructor() {
    this.currentPolicy = null;
    this.currentDraft = null;
    this.pollInterval = null;

    this.initDOMElements();
    this.initTabs();
    this.initCopilot();

    window.addEventListener("fairdeal-policy-updated", () => this.loadPolicy());
    this.bindEvents();

    const logout = document.getElementById('merchant-logout');
    if (logout) {
      logout.addEventListener('click', async () => {
        try {
          const response = await merchantFetch('/api/merchant/logout', { method: 'POST' });
          if (!response.ok) throw new Error('Could not sign out. Please retry.');
          csrfToken = '';
          window.location.replace('/merchant/login');
        } catch (error) {
          showToast(error.message);
        }
      });
    }

    merchantReady.then(() => {
      this.loadAll();
      this.startPolling();
    }).catch(error => showToast(error.message));
  }

  initDOMElements() {
    // Metrics
    this.metricRescued = document.getElementById('metric-rescued');
    this.metricGMV = document.getElementById('metric-gmv');
    this.metricPayLater = document.getElementById('metric-paylater');
    this.metricMargin = document.getElementById('metric-margin');
    this.metricDeadstock = document.getElementById('metric-deadstock');
    this.metricLatency = document.getElementById('metric-latency');

    // Audit Log
    this.agreementsTableBody = document.getElementById('agreements-table-body');

    // Demo reset
    this.btnResetDemo = document.getElementById('btn-reset-demo');
  }

  initTabs() {
    const tabButtons = document.querySelectorAll('.merchant-tab-btn');
    const tabPanels = document.querySelectorAll('.merchant-tab-panel');

    tabButtons.forEach(btn => {
      btn.addEventListener('click', () => {
        const targetTab = btn.getAttribute('data-tab');

        tabButtons.forEach(b => b.classList.remove('active'));
        tabPanels.forEach(p => p.classList.remove('active'));

        btn.classList.add('active');
        const targetPanel = document.getElementById(`panel-${targetTab}`);
        if (targetPanel) {
          targetPanel.classList.add('active');
        }
      });
    });
  }

  initCopilot() {
    this.copilotMessages = document.getElementById('copilot-messages');
    this.copilotInput = document.getElementById('copilot-input');
    this.copilotForm = document.getElementById('copilot-form');
    this.copilotSendBtn = document.getElementById('btn-copilot-send');

    // Chips
    const chips = document.querySelectorAll('.copilot-chip');
    chips.forEach(chip => {
      chip.addEventListener('click', () => {
        const prompt = chip.getAttribute('data-prompt');
        if (prompt && this.copilotInput) {
          this.copilotInput.value = prompt;
          this.sendCopilotMessage(prompt);
        }
      });
    });

    if (this.copilotForm) {
      this.copilotForm.addEventListener('submit', (e) => {
        e.preventDefault();
        const text = this.copilotInput.value.trim();
        if (text) {
          this.sendCopilotMessage(text);
          this.copilotInput.value = '';
        }
      });
    }
  }

  bindEvents() {
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
      this.loadAgreements(),
      this.loadCatalogSummary()
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

      if (this.metricRescued) this.metricRescued.textContent = m.rescued_carts;
      if (this.metricGMV) this.metricGMV.textContent = `$${m.recovered_gmv}`;
      const comparison = document.getElementById('compare-winwin-gmv');
      if (comparison) comparison.textContent = `$${m.recovered_gmv} captured`;
      if (this.metricPayLater) this.metricPayLater.textContent = `${Math.round(m.pay_later_share * 100)}%`;
      if (this.metricMargin) this.metricMargin.textContent = `${(m.avg_contribution_margin_pct * 100).toFixed(1)}%`;
      if (this.metricDeadstock) this.metricDeadstock.textContent = m.deadstock_units_cleared;
      if (this.metricLatency) this.metricLatency.textContent = `${(m.solver_latency_p50_us / 1000).toFixed(2)} ms`;
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
      this.updateCopilotBounds(data.active);
    } catch (err) {
      console.warn('Policy fetch error', err);
    }
  }

  renderActivePolicySummary(active) {
    const el = document.getElementById('active-policy-summary');
    if (!el || !active) return;
    const p = active.policy;
    el.innerHTML = `
      <span class="badge" style="background:rgba(46,125,50,0.15);color:var(--green);border-color:rgba(46,125,50,0.3);font-weight:700;">ACTIVE POLICY #${active.id}</span>
      <span>Min Margin: <strong>${(p.min_margin_pct * 100).toFixed(0)}%</strong></span> &bull;
      <span>Max Discount: <strong>${(p.max_discount_pct * 100).toFixed(0)}%</strong></span> &bull;
      <span>Buyer Power (&alpha;): <strong>${p.buyer_power.toFixed(2)}</strong></span> &bull;
      <span>Pay Later: <strong>${p.pay_later_enabled ? 'Enabled' : 'Disabled'}</strong></span> &bull;
      <span>Bundle SKU: <strong>${escapeHtml(p.bundle_sku || 'None')}</strong></span>
    `;
  }

  updateCopilotBounds(active) {
    if (!active) return;
    const p = active.policy;
    const minMarginEl = document.getElementById('copilot-min-margin');
    if (minMarginEl) minMarginEl.textContent = `${(p.min_margin_pct * 100).toFixed(1)}%`;
    const maxDiscountEl = document.getElementById('copilot-max-discount');
    if (maxDiscountEl) maxDiscountEl.textContent = `${(p.max_discount_pct * 100).toFixed(1)}%`;
    const bundleSkuEl = document.getElementById('copilot-bundle-sku');
    if (bundleSkuEl) bundleSkuEl.textContent = p.bundle_sku || 'None';
  }

  async loadCatalogSummary() {
    try {
      const res = await merchantFetch('/api/merchant/catalog');
      if (!res.ok) return;
      const data = await res.json();
      const countEl = document.getElementById('copilot-items-count');
      if (countEl) countEl.textContent = `${data.products.length} items`;

      const listEl = document.getElementById('copilot-inventory-list');
      if (listEl && data.products.length > 0) {
        listEl.innerHTML = '';
        data.products.forEach(p => {
          const item = document.createElement('div');
          item.className = 'inventory-mini-item' + (p.is_deadstock ? ' deadstock-alert' : '');
          const marginPct = p.cost_cents > 0 ? Math.round(((p.list_price_cents - p.cost_cents) / p.list_price_cents) * 100) : 0;
          item.innerHTML = `
            <div>
              <strong style="color:${p.is_deadstock ? '#b45309' : 'var(--heading)'};">${escapeHtml(p.name)}</strong>
              <div style="font-size:0.72rem;color:var(--text-muted);">$${(p.list_price_cents / 100).toFixed(2)} list &bull; $${(p.cost_cents / 100).toFixed(2)} cost (${marginPct}% margin)</div>
            </div>
            <div style="text-align:right;">
              ${p.is_deadstock ? '<span class="deadstock-pill">Deadstock</span>' : ''}
              <div style="font-family:var(--font-mono);font-weight:600;font-size:0.76rem;margin-top:2px;">${p.stock} units</div>
            </div>
          `;
          listEl.appendChild(item);
        });
      }
    } catch (err) {
      console.warn('Catalog summary fetch error', err);
    }
  }

  // =========================================================================
  // AI Strategy Copilot Execution
  // =========================================================================
  async sendCopilotMessage(userPrompt) {
    if (!this.copilotMessages) return;

    // 1. Render User Message Bubble
    this.appendMessage('user', userPrompt);

    // 2. Render Loading Bubble
    const loadingId = 'loading-' + Date.now();
    this.appendMessage('assistant', '<div style="display:flex;align-items:center;gap:0.5rem;color:var(--text-muted);"><em>Calculando estrategia comercial con Go Guardrails y DeepSeek...</em></div>', loadingId);

    if (this.copilotSendBtn) this.copilotSendBtn.disabled = true;

    try {
      const res = await merchantFetch('/api/merchant/policy/compile', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: userPrompt })
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error?.message || 'Error al procesar la estrategia comercial.');
      }

      const data = await res.json();
      this.currentDraft = data.draft;

      // Remove loading bubble
      const loadingEl = document.getElementById(loadingId);
      if (loadingEl) loadingEl.remove();

      // Render Assistant Response
      this.renderCopilotProposal(data.draft, data.adjustments, data.source);
    } catch (err) {
      const loadingEl = document.getElementById(loadingId);
      if (loadingEl) loadingEl.remove();
      this.appendMessage('assistant', `⚠️ <strong>Error:</strong> ${escapeHtml(err.message || 'No se pudo generar la propuesta.')}`);
    } finally {
      if (this.copilotSendBtn) this.copilotSendBtn.disabled = false;
    }
  }

  appendMessage(role, htmlContent, id = null) {
    const msg = document.createElement('div');
    msg.className = `copilot-msg ${role}`;
    if (id) msg.id = id;

    const avatarHtml = role === 'assistant'
      ? '<div class="copilot-avatar ai">🤖</div>'
      : '<div class="copilot-avatar user-avatar">👤</div>';

    msg.innerHTML = `
      ${avatarHtml}
      <div class="copilot-bubble">${htmlContent}</div>
    `;

    this.copilotMessages.appendChild(msg);
    this.copilotMessages.scrollTop = this.copilotMessages.scrollHeight;
    return msg;
  }

  renderCopilotProposal(draft, adjustments, source) {
    const p = draft.policy;
    const cur = this.currentPolicy || {};

    let adjustmentsHtml = '';
    if (adjustments && adjustments.length > 0) {
      adjustmentsHtml = `
        <div style="background:rgba(46,125,50,0.08);border:1px solid rgba(46,125,50,0.25);border-radius:8px;padding:0.6rem 0.75rem;font-size:0.78rem;color:var(--green);margin-top:0.5rem;">
          <strong>✓ Verificación Go Guardrails:</strong>
          <ul style="margin:0.25rem 0 0 1rem;">
            ${adjustments.map(a => `<li>${escapeHtml(a)}</li>`).join('')}
          </ul>
        </div>
      `;
    }

    const proposalContent = `
      <p><strong>He formulado el siguiente plan de ofertas y packs para tu tienda:</strong></p>
      <p style="margin:0.35rem 0 0.6rem;font-size:0.84rem;color:var(--text);">
        <em>${escapeHtml(p.rationale || 'Estrategia de equilibrio optimizada para inventario y margen.')}</em>
      </p>

      <div class="copilot-proposal-box">
        <div style="font-size:0.75rem;font-weight:700;text-transform:uppercase;color:var(--text-muted);letter-spacing:0.04em;">
          Parámetros Comerciales Propuestos:
        </div>
        <table class="proposal-param-table">
          <tr>
            <th>Margen Mínimo Garantizado:</th>
            <td><strong>${(p.min_margin_pct * 100).toFixed(0)}%</strong> ${cur.min_margin_pct ? `<span style="font-size:0.7rem;color:var(--text-muted);">(actual: ${(cur.min_margin_pct * 100).toFixed(0)}%)</span>` : ''}</td>
          </tr>
          <tr>
            <th>Techo Máximo de Descuento:</th>
            <td><strong>${(p.max_discount_pct * 100).toFixed(0)}%</strong> ${cur.max_discount_pct ? `<span style="font-size:0.7rem;color:var(--text-muted);">(actual: ${(cur.max_discount_pct * 100).toFixed(0)}%)</span>` : ''}</td>
          </tr>
          <tr>
            <th>Accesorio en Pack (Deadstock):</th>
            <td><strong style="color:#b45309;">${escapeHtml(p.bundle_sku || 'Ninguno')}</strong></td>
          </tr>
          <tr>
            <th>Envío Lento (Consolidado):</th>
            <td>${p.allow_slow_shipping ? '<span style="color:var(--green);font-weight:600;">Permitido (ahorro flete)</span>' : 'Desactivado'}</td>
          </tr>
          <tr>
            <th>PayPal Pay in 4:</th>
            <td>${p.pay_later_enabled ? '<span style="color:var(--green);font-weight:600;">Habilitado</span>' : 'Desactivado'}</td>
          </tr>
        </table>

        ${adjustmentsHtml}

        <button type="button" class="btn-activate-copilot" id="btn-activate-draft-${draft.id}">
          <span>⚡ Aplicar Estrategia a la Tienda</span>
        </button>
      </div>
    `;

    this.appendMessage('assistant', proposalContent);

    // Bind activation button
    const btn = document.getElementById(`btn-activate-draft-${draft.id}`);
    if (btn) {
      btn.addEventListener('click', async () => {
        btn.disabled = true;
        btn.textContent = 'Activando en la tienda...';
        try {
          const res = await merchantFetch(`/api/merchant/policy/${draft.id}/activate`, { method: 'POST' });
          if (!res.ok) throw new Error('Error al activar la política.');

          showToast('¡Estrategia activada exitosamente en la tienda!');
          btn.innerHTML = '✓ Estrategia Activa';
          btn.style.background = 'var(--green)';
          btn.style.color = '#ffffff';

          await this.loadPolicy();
          window.dispatchEvent(new Event("fairdeal-ai-policy-updated"));

          this.appendMessage('assistant', `
            <div style="color:var(--green);font-weight:600;">
              ✓ ¡Estrategia en vivo activada!
            </div>
            <div style="font-size:0.82rem;color:var(--text);margin-top:0.25rem;">
              Todas las nuevas negociaciones de compradores usarán automáticamente estos límites y packs.
            </div>
          `);
        } catch (err) {
          showToast(err.message || 'Error de activación');
          btn.disabled = false;
          btn.textContent = 'Reintentar Activación';
        }
      });
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
    if (!this.agreementsTableBody) return;
    this.agreementsTableBody.innerHTML = '';
    if (!list || list.length === 0) {
      this.agreementsTableBody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-muted);padding:1.5rem;">No hay negociaciones registradas todavía. ¡Haz una simulación de compra en la tienda!</td></tr>';
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
      const cell = document.createElement('td');
      const details = document.createElement('details');
      const summary = document.createElement('summary');
      summary.textContent = 'View economics';
      summary.style.cursor = 'pointer';
      summary.style.color = 'var(--accent-contrast)';
      summary.style.fontSize = '0.78rem';
      const content = document.createElement('div');
      summary.addEventListener('click', () => {
        if (!details.open) {
          window.dispatchEvent(new CustomEvent('fairdeal-show-economics', { detail: { id: a.id, target: content } }));
        }
      });
      details.append(summary, content);
      cell.append(details);
      tr.append(cell);
      this.agreementsTableBody.appendChild(tr);
    }
  }
}

window.addEventListener('DOMContentLoaded', () => {
  new MerchantDashboard();
});
