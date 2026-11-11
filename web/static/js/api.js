export async function fetchConfig() {
  const res = await fetch('/api/config');
  if (!res.ok) throw new Error('Failed to load system config');
  return res.json();
}

export async function fetchProduct(sku = 'CH-WINTER-PARKA-01') {
  const res = await fetch(`/api/products/${encodeURIComponent(sku)}`);
  if (!res.ok) throw new Error('Failed to load product');
  return res.json();
}

export async function parseIntent(text) {
  const res = await fetch('/api/intent/parse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text })
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error?.message || 'Intent parser failed');
  }
  return res.json();
}

export async function createPayPalOrder(agreementId, clearingToken) {
  const res = await fetch('/api/paypal/orders', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      agreement_id: agreementId,
      clearing_token: clearingToken
    })
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error?.message || 'Order creation failed');
  }
  return res.json();
}

export async function capturePayPalOrder(orderId) {
  const res = await fetch(`/api/paypal/orders/${encodeURIComponent(orderId)}/capture`, {
    method: 'POST'
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error?.message || 'Order capture failed');
  }
  return res.json();
}

export async function matchWinWinOffers(payload) {
  const res = await fetch('/api/fairdeal/match', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error?.message || 'Fairdeal matching failed');
  }
  return res.json();
}

export async function fetchBuyerSummary() {
  const res = await fetch('/api/buyer/summary');
  if (!res.ok) throw new Error('Failed to load buyer summary');
  return res.json();
}

export function showToast(message, duration = 3500) {
  const existing = document.querySelector('.toast');
  if (existing) existing.remove();

  const toast = document.createElement('div');
  toast.className = 'toast';
  toast.textContent = message;
  document.body.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transition = 'opacity 0.3s';
    setTimeout(() => toast.remove(), 300);
  }, duration);
}
