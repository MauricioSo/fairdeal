import { serverURL } from './server.js';
const select = document.getElementById('ext-server'); select.value = await serverURL();
select.onchange = () => chrome.storage.local.set({ serverURL: select.value });
const toggle = document.getElementById('ext-photo-button');
const settings = await chrome.storage.local.get('showProductButton'); toggle.checked = settings.showProductButton !== false;
toggle.onchange = () => chrome.storage.local.set({ showProductButton: toggle.checked });
document.getElementById('ext-open').onclick = async () => chrome.tabs.create({ url: await serverURL() });
const status = document.getElementById('ext-status'), button = document.getElementById('ext-find');
const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
try {
  const product = await chrome.tabs.sendMessage(tab.id, { type: 'FAIRDEAL_EXTRACT' });
  if (!product?.name) throw new Error('No product');
  status.textContent = product.name + (product.price ? ' · ' + product.price + ' ' + product.currency : ''); button.disabled = false;
} catch { status.textContent = 'Abre o recarga una ficha de producto compatible. También puedes usar el botón sobre la foto.'; }
button.onclick = async () => {
  button.disabled = true;
  try { const result = await chrome.runtime.sendMessage({ type: 'FAIRDEAL_OPEN_PRODUCT', tabId: tab.id }); if (!result?.ok) throw new Error(result?.error || 'No se pudo abrir FairDeal.'); window.close(); }
  catch (error) { status.textContent = error.message; button.disabled = false; }
};
