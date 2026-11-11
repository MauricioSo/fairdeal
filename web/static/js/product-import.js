const imported = new URLSearchParams(location.hash.slice(1));
if (imported.has('product')) {
  try {
    const raw = imported.get('product');
    if (raw.length > 5000) throw new Error('Reference too large');
    const product = JSON.parse(raw);
    history.replaceState(null, '', location.pathname);
    const overlay = document.createElement('div');
    overlay.setAttribute('role', 'dialog');
    overlay.setAttribute('aria-modal', 'true');
    overlay.setAttribute('aria-label', 'Confirm imported product reference');
    overlay.style.cssText = 'position:fixed;inset:0;z-index:99999;background:rgba(0,0,0,.6);display:flex;align-items:center;justify-content:center;padding:24px';
    const panel = document.createElement('section');
    panel.style.cssText = 'padding:24px;max-width:600px;border-radius:16px;background:#faf9f4;color:#222831';
    const text = document.createElement('p');
    text.textContent = 'Imported reference: ' + String(product.name || 'Unknown product').slice(0, 200) + ' · ' + String(product.price || 'Price unknown').slice(0, 30) + ' ' + String(product.currency || '').slice(0, 8) + '. FairDeal currently compares fictional winter-parka offers. The external store is not participating. Continue only if you want to explore this demo product.';
    const confirm = document.createElement('button');
    confirm.textContent = 'Confirm winter-parka demo';
    const cancel = document.createElement('button');
    cancel.textContent = 'Cancel import';
    const roots = [...document.body.children];
    roots.forEach(element => { element.inert = true; });
    confirm.onclick = () => {
      roots.forEach(element => { element.inert = false; });
      overlay.remove();
    };
    cancel.onclick = () => location.assign(location.pathname);
    panel.append(text, confirm, cancel);
    overlay.append(panel);
    document.body.append(overlay);
    confirm.focus();
  } catch {
    history.replaceState(null, '', location.pathname);
  }
}
