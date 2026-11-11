import { serverURL } from './server.js';

const opening = new Set();
const trim = (v, n) => typeof v === 'string' || typeof v === 'number' ? String(v).slice(0, n) : '';
function cleanReference(raw) {
  if (!raw || !trim(raw.name, 200)) throw new Error('No se detectó un producto. Abre su ficha y vuelve a intentarlo.');
  const source = new URL(raw.source);
  if (!['http:', 'https:'].includes(source.protocol) || source.username || source.password) throw new Error('Referencia no válida.');
  return { name: trim(raw.name, 200), sku: trim(raw.sku, 80), price: trim(raw.price, 30),
    currency: /^[A-Z]{3}$/.test(raw.currency || '') ? raw.currency : '',
    brand: trim(raw.brand, 100), source: source.origin + source.pathname };
}
async function openProduct(tabId) {
  if (!Number.isInteger(tabId) || opening.has(tabId)) throw new Error('Ya se está abriendo FairDeal.');
  opening.add(tabId);
  try {
    const raw = await chrome.tabs.sendMessage(tabId, { type: 'FAIRDEAL_EXTRACT' });
    const product = cleanReference(raw);
    const url = new URL(await serverURL());
    // A fragment is not included in the initial HTTP request or referrer.
    url.hash = 'product=' + encodeURIComponent(JSON.stringify(product));
    await chrome.tabs.create({ url: url.href });
  } finally { opening.delete(tabId); }
}
chrome.runtime.onInstalled.addListener(() => chrome.contextMenus.removeAll(() => {
  chrome.contextMenus.create({ id: 'fairdeal-product', title: 'Encontrar un FairDeal para este producto', contexts: ['page', 'image'], documentUrlPatterns: ['http://*/*', 'https://*/*'] });
}));
chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId !== 'fairdeal-product' || !tab?.id) return;
  try { await openProduct(tab.id); } catch { await chrome.action.setBadgeText({ tabId: tab.id, text: '!' }); }
});
chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (sender.id !== chrome.runtime.id || message.type !== 'FAIRDEAL_OPEN_PRODUCT') return;
  const fromPage = sender.tab && sender.frameId === 0;
  const fromPopup = !sender.tab && sender.url === chrome.runtime.getURL('popup.html');
  if (!fromPage && !fromPopup) return;
  const tabId = fromPage ? sender.tab.id : message.tabId;
  openProduct(tabId).then(() => respond({ ok: true })).catch(error => respond({ ok: false, error: error.message }));
  return true;
});
