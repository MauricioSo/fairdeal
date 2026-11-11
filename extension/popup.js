const SERVER_URL = 'http://localhost:8080';

document.getElementById('ext-negotiate').addEventListener('click', async () => {
  const btn = document.getElementById('ext-negotiate');
  const results = document.getElementById('ext-results');
  const budget = document.getElementById('ext-budget').value || '105.00';

  btn.disabled = true;
  btn.textContent = 'Calculating a validated package...';
  results.innerHTML = '';

  try {
    const res = await fetch(`${SERVER_URL}/api/fairdeal/match`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        sku: 'CH-WINTER-PARKA-01',
        budget: String(budget),
        shipping_flexibility: 0.85,
        prefers_pay_later: true,
        willing_to_bundle: true
      })
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error?.message || 'Could not match');

    for (const off of data.top_offers) {
      const div = document.createElement('div');
      div.className = 'offer-item';
      const gift = off.concessions.bundle ? ` + Free ${off.concessions.bundle.name}` : '';
      div.innerHTML = `
        <div style="display:flex;justify-content:space-between;font-weight:600;">
          <span>${off.badge}</span>
          <strong style="color:#1a2028;">$${off.final_price}</strong>
        </div>
        <div style="color:#4a5462;margin-top:2px;">${off.merchant_name}${gift}</div>
        <a href="${SERVER_URL}" target="_blank" style="display:inline-block;margin-top:4px;color:#1a2028;font-weight:600;text-decoration:none;">
          Checkout with PayPal →
        </a>
      `;
      results.appendChild(div);
    }
  } catch (err) {
    results.innerHTML = `<p style="color:#dc2626;font-size:12px;">${err.message}</p>`;
  } finally {
    btn.disabled = false;
    btn.textContent = 'Find a Fairdeal offer →';
  }
});
