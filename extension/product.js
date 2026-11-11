// Local-only product detection. No network requests or persisted history.
(() => {
  const text = (v, n = 200) => ['string', 'number'].includes(typeof v) ? String(v).replace(/[\u0000-\u001f]/g, ' ').trim().slice(0, n) : '';
  function safeURL(value, base) {
    if (typeof value !== 'string' || !value) return '';
    try { const u = new URL(value, base); return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password ? u.origin + u.pathname : ''; } catch { return ''; }
  }
  const isType = (node, type) => [].concat(node?.['@type'] || []).some(v => typeof v === 'string' && v.split(/[\/#]/).pop() === type);
  function images(value, base) { return [].concat(value || []).slice(0, 12).map(v => safeURL(typeof v === 'object' ? v?.url || v?.contentUrl : v, base)).filter(Boolean); }
  function detect(doc, href) {
    const source = safeURL(href, href); if (!source || source.length > 2048) return null;
    if (/\/(checkout|cart|basket|account|orders?|login|signin|payment)(?:\/|$)/i.test(new URL(href).pathname)) return null;
    const meta = key => text(doc.querySelector(`meta[property="${key}"],meta[name="${key}"]`)?.content, 500);
    const products = [], primary = []; let visited = 0, listing = false;
    function visit(node, depth = 0, main = false) {
      if (!node || depth > 9 || ++visited > 1500) return;
      if (Array.isArray(node)) { node.slice(0, 100).forEach(v => visit(v, depth + 1, main)); return; }
      if (typeof node !== 'object') return;
      if (isType(node, 'ItemList') || isType(node, 'CollectionPage')) listing = true;
      if (isType(node, 'Product') || isType(node, 'ProductGroup')) { products.push(node); if (main) primary.push(node); return; }
      for (const [key, value] of Object.entries(node).slice(0, 100)) visit(value, depth + 1, key === 'mainEntity');
    }
    for (const tag of [...doc.querySelectorAll('script[type="application/ld+json"]')].slice(0, 20)) {
      if (tag.textContent.length > 200000) continue;
      try { visit(JSON.parse(tag.textContent)); } catch { /* Ignore malformed metadata. */ }
    }
    const unique = [...new Map(products.map(p => [text(p.sku || p.name || p['@id']), p])).values()];
    const product = primary[0] || unique.find(p => safeURL(p.url || p.mainEntityOfPage?.['@id'] || p.mainEntityOfPage, href) === source) || (!listing && unique.length === 1 ? unique[0] : null);
    if (!product && listing) return null;
    const microSelector = '[itemscope][itemtype$="/Product"],[itemscope][itemtype$="#Product"]';
    const micro = !product ? doc.querySelector(microSelector) : null;
    if (micro && doc.querySelectorAll(microSelector).length > 1) return null;
    const prop = key => { const n = micro?.querySelector(`[itemprop="${key}"]`); return n?.getAttribute('content') || n?.getAttribute('src') || n?.textContent || ''; };
    if (!product && !micro && !/^(product|product\.item|og:product)$/i.test(meta('og:type'))) return null;
    const name = text(product?.name || prop('name') || meta('og:title')); if (!name) return null;
    let offers = [].concat(product?.offers || []).filter(v => v && typeof v === 'object');
    if (offers[0]?.offers) offers = [].concat(offers[0].offers);
    const offer = offers[0] || {};
    const rawPrice = offer.price ?? offer.lowPrice ?? prop('price');
    const price = text(rawPrice === '' ? meta('product:price:amount') : rawPrice, 30);
    if (!offers.length && !price) return null;
    const currency = text(offer.priceCurrency || prop('priceCurrency') || meta('product:price:currency'), 8).toUpperCase();
    return { name, sku: text(product?.sku || prop('sku'), 80), price, currency: /^[A-Z]{3}$/.test(currency) ? currency : '', source,
      brand: text(typeof product?.brand === 'object' ? product.brand?.name : product?.brand, 100),
      images: images(product?.image || prop('image') || meta('og:image'), href), detection: product ? 'jsonld' : micro ? 'microdata' : 'opengraph' };
  }
  function selectImage(doc, product, href) {
    const words = product.name.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(v => v.length > 3);
    let selected = null, best = 0;
    for (const img of [...doc.querySelectorAll('img')].slice(0, 300)) {
      const r = img.getBoundingClientRect();
      if (r.width < 180 || r.height < 160 || img.closest('[aria-hidden="true"],nav,header,footer')) continue;
      const css = doc.defaultView.getComputedStyle(img);
      if (css.display === 'none' || css.visibility === 'hidden' || Number(css.opacity) === 0) continue;
      const urls = [img.currentSrc, img.src, img.getAttribute('data-src')].map(v => safeURL(v, href));
      const exact = product.images.some(v => urls.includes(v));
      const similarity = words.filter(v => text(img.alt, 500).toLowerCase().includes(v)).length;
      if (!exact && similarity < Math.min(2, words.length || 2) && img.getAttribute('itemprop') !== 'image') continue;
      const score = (exact ? 10000000 : similarity * 100000) + Math.min(r.width * r.height, 2000000);
      if (score > best) { selected = img; best = score; }
    }
    return selected;
  }
  const reference = p => p ? { name: text(p.name), sku: text(p.sku, 80), price: text(p.price, 30), currency: /^[A-Z]{3}$/.test(p.currency || '') ? p.currency : '', source: safeURL(p.source, p.source), brand: text(p.brand, 100) } : null;
  globalThis.FairDealProduct = Object.freeze({ detect, selectImage, reference, safeURL });
})();
