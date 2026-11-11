import { fetchConfig, createPayPalOrder, capturePayPalOrder, showToast } from './api.js';

let systemConfig = null;

export async function renderCheckoutButton(agreementId, clearingToken, agreement, onRequote) {
  if (!systemConfig) {
    try {
      systemConfig = await fetchConfig();
    } catch (err) {
      showToast('Payment configuration unavailable. Please retry.');
      return;
    }
  }

  const container = document.getElementById('paypal-button-container');
  if (!container) return;
  container.innerHTML = '';

  const isMock = systemConfig.paypal_mode === 'mock';
  const params = new URLSearchParams(window.location.search);
  const mockIneligible = params.get('mock_paylater') === 'ineligible';

  if (isMock) {
    if (mockIneligible && agreement.concessions.payment_method === 'PAY_LATER' && onRequote) {
      showToast('Pay Later ineligible — re-calculating equilibrium with standard PayPal...');
      onRequote(agreementId);
      return;
    }
    renderSimulatedButton(container, agreementId, clearingToken, agreement);
  } else {
    renderSandboxSDKButtons(container, agreementId, clearingToken, agreement, onRequote);
  }
}

function renderSimulatedButton(container, agreementId, clearingToken, agreement) {
  const isPayLater = agreement.concessions.payment_method === 'PAY_LATER';
  const btn = document.createElement('button');
  btn.className = 'btn-paypal-simulated';
  btn.innerHTML = `
    <span>${isPayLater ? 'Simulate PayPal checkout' : 'Pay with PayPal'}</span>
    <span class="badge-simulated">SIMULATED</span>
  `;

  btn.addEventListener('click', () => {
    openSimulatedModal(agreementId, clearingToken, agreement);
  });

  container.appendChild(btn);
}

function openSimulatedModal(agreementId, clearingToken, agreement) {
  const existing = document.querySelector('.modal-backdrop');
  if (existing) existing.remove();

  const isPayLater = Boolean(agreement.concessions.installments);
  const planLabel = isPayLater
    ? `Illustrative preference: 4 payments near ${agreement.concessions.installments.amount}`
    : `Single payment of $${agreement.final_price}`;
  const dueToday = isPayLater ? agreement.concessions.installments.amount : agreement.final_price;

  const modal = document.createElement('div');
  modal.className = 'modal-backdrop';
  modal.innerHTML = `
    <div class="modal-card">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.75rem;">
        <h3 style="margin:0;">Fairdeal Payment Simulation</h3>
        <span class="badge-simulated">SIMULATED MODAL</span>
      </div>
      <p style="font-size:0.9rem;color:var(--text-muted);margin-bottom:1rem;">
        Complete checkout for <strong>$${agreement.final_price} USD</strong>.
      </p>

      <div style="background:var(--bg-subtle);border:1px solid var(--border);border-radius:10px;padding:0.9rem;margin-bottom:1rem;font-size:0.88rem;">
        <div style="display:flex;justify-content:space-between;margin-bottom:0.3rem;">
          <span>Item</span>
          <span>Winter Parka Performance 01</span>
        </div>
        <div style="display:flex;justify-content:space-between;margin-bottom:0.3rem;">
          <span>Payment Plan</span>
          <span style="color:var(--accent);font-weight:600;">${planLabel}</span>
        </div>
        <div style="display:flex;justify-content:space-between;margin-bottom:0.3rem;">
          <span>Shipping</span>
          <span style="color:var(--green);font-weight:600;">Free (${agreement.concessions.shipping_days} days)</span>
        </div>
        <div style="display:flex;justify-content:space-between;border-top:1px solid var(--border);padding-top:0.4rem;font-weight:700;">
          <span>Total Today</span>
          <span>$${dueToday} USD</span>
        </div>
      </div>

      <div class="modal-actions">
        <button id="btn-modal-cancel" class="btn-modal-cancel">Cancel</button>
        <button id="btn-modal-approve" class="btn-modal-approve">Approve Payment</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  modal.querySelector('#btn-modal-cancel').addEventListener('click', () => modal.remove());

  const approveBtn = modal.querySelector('#btn-modal-approve');
  approveBtn.addEventListener('click', async () => {
    approveBtn.disabled = true;
    approveBtn.textContent = 'Processing...';

    try {
      const orderRes = await createPayPalOrder(agreementId, clearingToken);
      const orderId = orderRes.order_id;
      await capturePayPalOrder(orderId);

      modal.remove();
      window.location.href = `/checkout/confirmed?agreement=${encodeURIComponent(agreementId)}`;
    } catch (err) {
      showToast(err.message || 'Payment simulation failed');
      approveBtn.disabled = false;
      approveBtn.textContent = 'Approve Payment';
    }
  });
}

function renderSandboxSDKButtons(container, agreementId, clearingToken, agreement, onRequote) {
  const clientId = systemConfig.paypal_client_id;
  if (!clientId) {
    container.textContent = 'PayPal Sandbox is not configured. Payment unavailable.';
    return;
  }

  const sdkId = 'paypal-js-sdk';
  if (!document.getElementById(sdkId)) {
    const script = document.createElement('script');
    script.id = sdkId;
    script.src = `https://www.paypal.com/sdk/js?client-id=${encodeURIComponent(clientId)}&currency=USD&components=buttons,messages&enable-funding=paylater&buyer-country=US`;
    script.onerror = () => { container.textContent = 'Could not load PayPal. Please retry.'; script.remove(); };
    script.onload = () => mountSDKButtons(container, agreementId, clearingToken, agreement, onRequote);
    document.head.appendChild(script);
  } else {
    mountSDKButtons(container, agreementId, clearingToken, agreement, onRequote);
  }
}

function mountSDKButtons(container, agreementId, clearingToken, agreement, onRequote) {
  if (!window.paypal || !window.paypal.Buttons) {
    container.textContent = 'PayPal SDK unavailable. Please retry.';
    return;
  }

  const wantsPayLater = agreement.concessions.payment_method === 'PAY_LATER';
  const funding = wantsPayLater ? window.paypal.FUNDING.PAYLATER : window.paypal.FUNDING.PAYPAL;

  const button = window.paypal.Buttons({
    fundingSource: funding,
    style: {
      layout: 'vertical',
      color: 'gold',
      shape: 'rect',
      label: 'pay'
    },
    createOrder: async () => {
      const res = await createPayPalOrder(agreementId, clearingToken);
      return res.order_id;
    },
    onApprove: async (data) => {
      await capturePayPalOrder(data.orderID);
      window.location.href = `/checkout/confirmed?agreement=${encodeURIComponent(agreementId)}`;
    },
    onError: (err) => {
      showToast('PayPal transaction error: ' + err);
    }
  });

  if (button.isEligible()) {
    button.render(container);
  } else if (wantsPayLater && onRequote) {
    showToast('Pay Later not eligible for this account — re-quoting standard PayPal...');
    onRequote(agreementId);
  } else {
    window.paypal.Buttons({
      fundingSource: window.paypal.FUNDING.PAYPAL,
      createOrder: async () => {
        const res = await createPayPalOrder(agreementId, clearingToken);
        return res.order_id;
      },
      onApprove: async (data) => {
        await capturePayPalOrder(data.orderID);
        window.location.href = `/checkout/confirmed?agreement=${encodeURIComponent(agreementId)}`;
      }
    }).render(container);
  }
}
