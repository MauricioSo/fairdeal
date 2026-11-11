(() => {
  if (globalThis.__fairdealButtonLoaded) return;
  globalThis.__fairdealButtonLoaded = true;
  if (window.top !== window) return;
  const ownSite = location.hostname === 'fairdeal-5h0w.onrender.com' || (['localhost','127.0.0.1'].includes(location.hostname) && location.port === '8080');
  if (ownSite) return;
  const { detect, selectImage, reference } = globalThis.FairDealProduct;
  let host, image, product, button, timer, frame, dismissed = '', enabled = true, currentURL = location.href, mountedName = '';
  function remove() { host?.remove(); host = image = button = null; }
  function position() {
    frame = null; if (!host || !image?.isConnected) return;
    const r = image.getBoundingClientRect();
    const modal = [...document.querySelectorAll('[aria-modal="true"],dialog[open]')].some(n => {const box=n.getBoundingClientRect();return box.width>0 && box.height>0;});
    const visible = !modal && r.bottom > 65 && r.top < innerHeight - 35 && r.right > 150 && r.left < innerWidth - 100;
    host.style.setProperty('display', visible ? 'block' : 'none', 'important'); if (!visible) return;
    host.style.setProperty('top', `${Math.max(10, r.top + 10)}px`, 'important');
    host.style.setProperty('left', `${Math.max(8, Math.min(r.left + 10, innerWidth - 190))}px`, 'important');
  }
  function schedulePosition() { if (!frame) frame = requestAnimationFrame(position); }
  function mount(target) {
    remove(); image = target; mountedName = product.name; host = document.createElement('div'); host.dataset.fairdealButton = 'true';
    host.style.cssText = 'all:initial!important;position:fixed!important;z-index:2147483646!important;pointer-events:none!important;';
    const root = host.attachShadow({mode:'open'}), style = document.createElement('style');
    style.textContent = `:host{font-family:system-ui,sans-serif}.bar{display:flex;gap:2px;align-items:center;padding:4px;border-radius:24px;background:#222831;box-shadow:0 3px 16px #0005;pointer-events:auto}button{font:600 13px/20px system-ui,sans-serif;border:0;cursor:pointer;padding:8px 12px;border-radius:20px;background:#d6fb38;color:#172027}button:hover{background:#e5ff80}button:focus-visible{outline:3px solid #0085ff;outline-offset:3px}button:disabled{opacity:.7;cursor:wait}.close{background:transparent;color:white;padding:6px 8px}.close:hover{background:#ffffff20}`;
    const bar = document.createElement('div'); bar.className = 'bar';
    button = document.createElement('button'); button.type = 'button'; button.textContent = 'Encontrar FairDeal';
    button.title = `Usar ${product.name} como referencia. Solo se envían datos al hacer clic.`;
    button.setAttribute('aria-label', `Encontrar un FairDeal para ${product.name}`);
    button.onclick = async event => {
      if (!event.isTrusted) return;
      event.preventDefault(); event.stopPropagation();
      const clicked = button; clicked.disabled = true; clicked.textContent = 'Abriendo…';
      try { const result = await chrome.runtime.sendMessage({type:'FAIRDEAL_OPEN_PRODUCT'}); if (!result?.ok) throw new Error('Open failed'); }
      catch { clicked.textContent = 'Reintentar FairDeal'; clicked.title = 'No se pudo abrir. Recarga la página si actualizaste la extensión.'; }
      finally { clicked.disabled = false; if (clicked.textContent === 'Abriendo…') clicked.textContent = 'Encontrar FairDeal'; }
    };
    const close = document.createElement('button'); close.type = 'button'; close.className = 'close'; close.textContent = '×'; close.setAttribute('aria-label','Ocultar FairDeal en este producto');
    close.onclick = event => { event.preventDefault(); event.stopPropagation(); dismissed = product.source; remove(); };
    bar.append(button,close); root.append(style,bar); document.documentElement.append(host); position();
  }
  function scan() {
    timer = null;
    if (location.href !== currentURL) { currentURL = location.href; dismissed = ''; }
    product = enabled ? detect(document,location.href) : null;
    if (!product || dismissed === product.source) { remove(); return; }
    const target = selectImage(document,product,location.href);
    if (!target) { remove(); return; }
    if (image !== target || !host?.isConnected || mountedName !== product.name) mount(target); else position();
  }
  function scheduleScan() { if (!timer) timer = setTimeout(scan,350); }
  chrome.runtime.onMessage.addListener((message,sender,respond) => {
    if (sender.id !== chrome.runtime.id) return;
    if (message.type === 'FAIRDEAL_EXTRACT') respond(reference(detect(document,location.href)));
  });
  chrome.storage.local.get('showProductButton').then(settings => { enabled = settings.showProductButton !== false; scan(); }).catch(scan);
  chrome.storage.onChanged.addListener((changes,area) => { if (area === 'local' && changes.showProductButton) { enabled = changes.showProductButton.newValue !== false; scheduleScan(); } });
  const observer = new MutationObserver(records => {
    if (records.some(r => r.type === 'attributes' ? r.target !== host : [...r.addedNodes,...r.removedNodes].some(n => n !== host))) scheduleScan();
  });
  observer.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['src','srcset','content','aria-hidden','aria-modal','open']});
  window.addEventListener('scroll',schedulePosition,{passive:true});
  window.addEventListener('resize',scheduleScan,{passive:true}); window.addEventListener('popstate',scheduleScan);
  document.addEventListener('load',scheduleScan,true);
  window.addEventListener('pageshow',scheduleScan);
  window.addEventListener('pagehide',() => { clearTimeout(timer); if (frame) cancelAnimationFrame(frame); remove(); });
})();
