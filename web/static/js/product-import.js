// Imported page text is untrusted reference data, never instructions or a SKU match.
const params = new URLSearchParams(location.hash.slice(1));
if (params.has('product')) {
  try {
    const raw = params.get('product');
    if (raw.length > 5000) throw new Error('Reference too large');
    const data = JSON.parse(raw);
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error('Invalid reference');
    const text = (v, n = 200) => ['string', 'number'].includes(typeof v) ? String(v).slice(0, n) : '';
    const product = { name: text(data.name), price: text(data.price, 30), currency: text(data.currency, 8), brand: text(data.brand, 100) };
    if (!product.name) throw new Error('Missing product');
    let source = '';
    try { const u = new URL(data.source); if (['http:', 'https:'].includes(u.protocol) && !u.username && !u.password) source = u.origin + u.pathname; } catch { /* No source link. */ }
    history.replaceState(null, '', location.pathname);
    const el = (tag, value, parent) => { const n = document.createElement(tag); if (value !== undefined) n.textContent = value; if (parent) parent.append(n); return n; };
    const overlay = el('div'); overlay.setAttribute('role', 'dialog'); overlay.setAttribute('aria-modal', 'true'); overlay.setAttribute('aria-label', 'Review product reference');
    overlay.style.cssText = 'position:fixed;inset:0;z-index:99999;background:#0009;display:flex;align-items:center;justify-content:center;padding:20px;overflow:auto';
    const panel = el('section', undefined, overlay); panel.style.cssText = 'padding:28px;max-width:580px;max-height:90vh;overflow:auto;border-radius:20px;background:#faf9f4;color:#222831;box-sizing:border-box';
    el('h2', 'Tu producto como referencia', panel);
    el('p', product.name, panel);
    if (product.price) el('p', 'Precio publicado: ' + product.price + ' ' + product.currency + ' (envío e impuestos externos no verificados).', panel);
    if (source) { const link = el('a', 'Ver producto original', panel); link.href = source; link.target = '_blank'; link.rel = 'noopener noreferrer'; }
    const comparableCategory = /\b(parka|winter jacket|chaqueta de invierno|abrigo de invierno)\b/i.test(product.name);
    el('p', comparableCategory
      ? 'Podemos mostrar alternativas ficticias de parkas. No verificamos que sean el mismo modelo ni que tengan características equivalentes.'
      : 'Todavía no hay un catálogo participante para este producto. Puedes explorar por separado la demostración de parkas de FairDeal.', panel);
    el('p', 'La tienda que visitaste no participa en esta negociación. Las ofertas son de cinco comercios ficticios; importar y comparar no inicia ningún pago. Tu presupuesto máximo no se revela a vendedores ni a sus agentes.', panel);
    const form = el('form', undefined, panel), label = el('label', 'Tu presupuesto máximo para la demo (USD)', form);
    const budget = el('input', undefined, label); budget.type = 'number'; budget.min = '1'; budget.max = '10000'; budget.step = '0.01'; budget.required = true; budget.value = '130'; budget.style.cssText = 'display:block;padding:10px;margin:10px 0;width:180px';
    const confirm = el('button', comparableCategory ? 'Comparar parkas de demostración' : 'Explorar la demo de parkas por separado', form); confirm.type = 'submit';
    const cancel = el('button', 'Cancelar', form); cancel.type = 'button'; cancel.style.marginLeft = '12px';
    const roots = [...document.body.children], previousFocus = document.activeElement;
    const inertState = roots.map(n => n.inert); roots.forEach(n => { n.inert = true; });
    function close() { roots.forEach((n, i) => { n.inert = inertState[i]; }); overlay.remove(); document.removeEventListener('keydown', onKey, true); previousFocus?.focus?.(); }
    function onKey(event) {
      if (event.key === 'Escape') { event.preventDefault(); close(); }
      if (event.key === 'Tab') { const focusable = [...panel.querySelectorAll('a[href],input,button')]; const first = focusable[0], last = focusable.at(-1); if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); } else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); } }
    }
    cancel.onclick = close;
    form.onsubmit = event => {
      event.preventDefault(); if (!form.reportValidity()) return;
      const request = { budget: Number(budget.value), confirmed: true };
      close();
      const note = el('section'); note.className = 'card'; note.style.cssText = 'margin-bottom:20px;padding:16px';
      el('strong', 'Referencia importada: ' + product.name, note);
      el('p', 'Estás comparando el catálogo ficticio de parkas, no ofertas del comercio original. El precio externo no se usa como ahorro verificado ni como presupuesto.', note);
      document.querySelector('main')?.prepend(note);
      window.fairdealImportedRequest = request;
      window.dispatchEvent(new CustomEvent('fairdeal-reference-ready', { detail: request }));
    };
    document.body.append(overlay); document.addEventListener('keydown', onKey, true); budget.focus();
  } catch { history.replaceState(null, '', location.pathname); }
}
