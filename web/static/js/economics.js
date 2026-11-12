import { merchantFetch } from './merchant.js';

const amount = cents => new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
const labels = {
  MARGIN_BELOW_MINIMUM: 'Order margin below your minimum', CONTRIBUTION_BELOW_MINIMUM: 'Order profit below your minimum',
  PRODUCT_MARGIN_BELOW_MINIMUM: 'An item fails its own margin rule', PROTECTED_PRODUCT: 'A protected item would be discounted',
  DISCOUNT_LIMIT: 'Discount exceeds your limit', MISSING_COST: 'A product cost is missing or zero',
  SHIPPING_NOT_ALLOWED: 'Slow delivery is disabled', BUNDLE_NOT_ALLOWED: 'Accessory is not authorized',
};
function el(tag, text, parent) { const n = document.createElement(tag); if (text !== undefined) n.textContent = text; if (parent) parent.append(n); return n; }
function breakdown(parent, b) {
  const dl = el('dl', undefined, parent); dl.className = 'economic-breakdown';
  for (const [key, title] of Object.entries({ gross_cents: 'Buyer pays (delivery included)', included_tax_cents: 'Included tax withheld', net_revenue_cents: 'Net revenue', goods_cents: 'All product costs', shipping_cents: 'Merchant delivery cost', handling_cents: 'Preparation', packaging_cents: 'Packaging', payment_fee_cents: 'Payment fee', platform_fee_cents: 'Platform fee', total_cost_cents: 'Total variable costs', contribution_cents: 'Contribution after costs' })) {
    el('dt', title, dl); el('dd', amount(b[key]), dl);
  }
  el('p', `Contribution margin: ${(b.margin_bps / 100).toFixed(2)}% of net revenue.`, parent);
}

async function init() {
  const [response, catalogResponse] = await Promise.all([merchantFetch('/api/merchant/economics'), merchantFetch('/api/merchant/catalog')]);
  if (!response.ok || !catalogResponse.ok) throw new Error('Could not load economic settings. Refresh to retry.');
  let state = await response.json(); const products = (await catalogResponse.json()).products;
  const section = el('section'); section.className = 'economic-panel';
  const container = document.getElementById('economics-container') || document.querySelector('main');
  container.replaceChildren(section);
  el('h2', 'Your rules. Every offer checked.', section);
  el('p', 'Authorize what FairDeal can offer. Every pack and delivery option must meet your order and product limits. Buyer budgets remain private.', section);
  const basis = el('details', undefined, section); el('summary', 'What the margin calculation includes', basis);
  el('p', state.cost_basis, basis); el('p', state.pack_scope, basis);
  const activeNote = el('p', undefined, section); activeNote.setAttribute('role', 'status');
  const form = el('form', undefined, section); const grid = el('div', undefined, form); grid.className = 'economic-grid';
  const fields = {};
  function input(key, title, value, type = 'number', step = '0.01') {
    const label = el('label', title, grid); const field = el('input', undefined, label); field.type = type;
    if (type === 'checkbox') field.checked = value; else { field.value = value; field.required = true; if (type === 'number') { field.min = '0'; field.step = step; } }
    fields[key] = field; return field;
  }
  const m = state.market;
  function fill() {
    grid.replaceChildren();
    const p = state.active.policy, e = p.economics || {};
    activeNote.textContent = `Active policy #${state.active.id}. Changes below remain inactive until you activate a reviewed draft. All amounts are USD.`;
    input('minMargin', 'Minimum order margin (%)', p.min_margin_pct * 100).max = 60;
    fields.minMargin.min = state.active.hard_floor_margin_pct * 100;
    input('maxDiscount', 'Maximum main-item discount (%)', p.max_discount_pct * 100).max = 30;
    input('minimum', 'Minimum contribution per order ($)', (e.min_contribution_cents || 0) / 100);
    input('handling', 'Preparation upper bound / order ($)', (e.handling_cents || 0) / 100);
    input('packaging', 'Packaging upper bound / order ($)', (e.packaging_cents || 0) / 100);
    input('express', 'Express cost upper bound ($), 1–2 days', (e.express_cost_cents ?? m.express_ship_cost_cents) / 100);
    input('ground', 'Ground cost upper bound ($), 5–7 days', (e.ground_cost_cents ?? m.ground_ship_cost_cents) / 100);
    input('paymentBps', 'Payment fee upper bound (%)', (e.payment_fee_bps ?? m.fee_bps) / 100).max = 100;
    input('paymentFixed', 'Payment fixed fee upper bound ($)', (e.payment_fixed_cents ?? m.fee_fixed_cents) / 100);
    input('platformBps', 'Platform fee on net revenue (%)', (e.platform_fee_bps || 0) / 100).max = 100;
    input('platformFixed', 'Platform fixed fee ($)', (e.platform_fixed_cents || 0) / 100);
    input('taxBps', 'Tax rate included in total (%)', (e.included_tax_bps || 0) / 100).max = 100;
    input('slow', 'Allow ground delivery', p.allow_slow_shipping, 'checkbox');
    input('payLater', 'Allow Pay Later (subject to PayPal eligibility)', p.pay_later_enabled, 'checkbox');
    const label = el('label', 'Optional accessory authorized for packs', grid); const select = el('select', undefined, label);
    const none = el('option', 'No accessory', select); none.value = '';
    for (const product of products.filter(p => p.is_deadstock)) { const option = el('option', `${product.name} · available ${product.stock}`, select); option.value = product.sku; }
    select.value = p.bundle_sku || ''; fields.bundle = select;
    productRules.replaceChildren();
    for (const product of products) {
      const rule = (e.product_rules || []).find(r => r.sku === product.sku) || {};
      const row = el('div', undefined, productRules); row.className = 'economic-product';
      el('span', `${product.name} · unit cost ${amount(product.cost_cents)}${product.cost_cents <= 0 ? ' — automatic offers blocked: cost required' : ''}`, row);
      const protectLabel = el('label', 'Protect from discount / free inclusion ', row); const protect = el('input', undefined, protectLabel); protect.type = 'checkbox'; protect.checked = !!rule.protected;
      const marginLabel = el('label', 'Item margin minimum (%) ', row); const margin = el('input', undefined, marginLabel); margin.type = 'number'; margin.min = 0; margin.max = 100; margin.step = '0.01'; margin.value = (rule.min_margin_bps || 0) / 100;
      row.dataset.sku = product.sku; row._ruleFields = { protect, margin };
    }
  }
  el('h3', 'Product protections', form);
  el('p', 'A protected item cannot be discounted or given away. Optional item margins use list-value allocation of revenue and shared costs; rounding is conservative.', form);
  const productRules = el('div', undefined, form);
  const save = el('button', 'Save reviewable draft', form); save.type = 'submit';
  const activate = el('button', 'Activate this draft', section); activate.type = 'button'; activate.hidden = true;
  const status = el('p', '', section); status.setAttribute('role', 'status');
  let draft = null;
  function read() {
    if (!form.reportValidity()) throw new Error('Review the highlighted fields.');
    const n = key => Number(fields[key].value), cents = key => Math.round(n(key) * 100);
    return { ...state.active.policy, min_margin_pct: n('minMargin') / 100, max_discount_pct: n('maxDiscount') / 100, allow_slow_shipping: fields.slow.checked, pay_later_enabled: fields.payLater.checked, bundle_sku: fields.bundle.value || null,
      economics: { min_contribution_cents: cents('minimum'), handling_cents: cents('handling'), packaging_cents: cents('packaging'), express_cost_cents: cents('express'), ground_cost_cents: cents('ground'), payment_fee_bps: cents('paymentBps'), payment_fixed_cents: cents('paymentFixed'), platform_fee_bps: cents('platformBps'), platform_fixed_cents: cents('platformFixed'), included_tax_bps: cents('taxBps'), product_rules: [...productRules.children].map(row => ({ sku: row.dataset.sku, protected: row._ruleFields.protect.checked, min_margin_bps: Math.round(Number(row._ruleFields.margin.value) * 100) })) } };
  }
  async function post(url, body) { const r = await merchantFetch(url, { method: 'POST', body: JSON.stringify(body) }); const data = await r.json(); if (!r.ok) throw new Error(data.error?.message || 'Request failed'); return data; }
  form.addEventListener('input', () => { results.textContent = 'Rules changed. Run the simulation again to refresh these results.'; draft = null; activate.hidden = true; status.textContent = 'Unsaved changes. Simulate and save a new draft before activation.'; });
  form.onsubmit = async event => { event.preventDefault(); save.disabled = true; try { const data = await post('/api/merchant/policy/manual', read()); draft = data.draft; activate.hidden = false; status.textContent = `Draft #${draft.id} saved. Review your rules and simulation; activation expires outstanding offers. Orders already reserved retain their agreed costs.`; } catch (e) { status.textContent = e.message; } finally { save.disabled = false; } };
  activate.onclick = async () => { if (!draft) return; activate.disabled = true; try { const data = await post(`/api/merchant/policy/${draft.id}/activate`, {}); state.active = data.active; draft = null; activate.hidden = true; fill(); status.textContent = 'Rules activated. New offers must pass these limits.'; window.dispatchEvent(new Event('fairdeal-policy-updated')); } catch (e) { status.textContent = e.message; } finally { activate.disabled = false; } };
  fill();
  el('h3', 'Try an offer before authorizing it', section);
  el('p', 'Non-binding simulation using the form above. No inventory is reserved and no payment is created. Checks economics only, not buyer acceptance or delivery eligibility.', section);
  const sim = el('form', undefined, section); sim.className = 'economic-grid';
  const skuLabel = el('label', 'Main product', sim), sku = el('select', undefined, skuLabel);
  for (const p of products) { const o = el('option', p.name, sku); o.value = p.sku; }
  const priceLabel = el('label', 'Complete buyer charge ($); blank tests 100%, 90%, 80% of main list price', sim), price = el('input', undefined, priceLabel); price.type = 'number'; price.min = '0.01'; price.max = 10000; price.step = '0.01';
  const run = el('button', 'Compare permitted combinations', sim); run.type = 'submit';
  const results = el('div', undefined, section); results.setAttribute('aria-live', 'polite');
  sim.onsubmit = async event => { event.preventDefault(); run.disabled = true; try { const data = await post('/api/merchant/economics/simulate', { sku: sku.value, price_cents: price.value ? Math.round(Number(price.value) * 100) : 0, policy: read() }); results.replaceChildren(); for (const row of data.rows) { const details = el('details', undefined, results); const e = row.evaluation; el('summary', `${e.approved ? 'APPROVED' : 'BLOCKED'} · ${amount(row.price_cents)} · ${row.shipping === 'EXPRESS' ? 'Express 1–2 days' : 'Ground 5–7 days'} · ${row.bundle_sku ? 'With ' + row.bundle_sku : 'Single item'} · margin ${(e.breakdown.margin_bps / 100).toFixed(2)}%`, details); if (e.reasons.length) el('p', e.reasons.map(reason => labels[reason] || reason).join('; '), details); breakdown(details, e.breakdown); } } catch (e) { results.textContent = e.message; } finally { run.disabled = false; } };
  window.addEventListener('fairdeal-ai-policy-updated', async () => { const r = await merchantFetch('/api/merchant/economics'); if (r.ok) { state = await r.json(); fill(); draft = null; activate.hidden = true; status.textContent = 'Active policy reloaded. Unsaved configuration was cleared.'; } });
  window.addEventListener('fairdeal-show-economics', async event => { const target = event.detail.target; try { const r = await merchantFetch(`/api/merchant/agreements/${encodeURIComponent(event.detail.id)}/economics`); if (!r.ok) throw new Error('No snapshot for this historical agreement.'); const s = await r.json(); target.replaceChildren(); el('p', `Policy #${s.policy_id} · frozen costs at checkout · ${s.evaluation.approved ? 'Approved by deterministic engine' : 'Blocked'}`, target); breakdown(target, s.evaluation.breakdown); } catch (e) { target.textContent = e.message; } });
}
init().catch(error => { const p = el('p', error.message); p.setAttribute('role', 'alert'); document.querySelector('main').prepend(p); });
