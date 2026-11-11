// Fairdeal Content Script: Injects a floating Anti-Surge pill on product pages
(function () {
  if (window.location.host.includes('localhost:8080')) return; // Already on Fairdeal hub
  if (document.getElementById('fairdeal-floating-agent')) return;

  const pill = document.createElement('div');
  pill.id = 'fairdeal-floating-agent';
  pill.style.cssText = `
    position: fixed;
    bottom: 20px;
    right: 20px;
    z-index: 999999;
    background: #222831;
    color: #f5f6f8;
    border: 1px solid rgba(255, 255, 255, 0.12);
    border-radius: 999px;
    padding: 8px 16px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    font-size: 13px;
    font-weight: 500;
    box-shadow: 0 10px 30px rgba(0,0,0,0.25);
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;
  `;
  pill.innerHTML = `
    <span style="background:#d6fb38;color:#131920;width:22px;height:22px;border-radius:6px;display:inline-flex;align-items:center;justify-content:center;font-weight:700;font-size:11px;font-family:monospace;">F</span>
    <span>Fairdeal: Explore a negotiated demo package</span>
  `;
  pill.addEventListener('click', () => {
    window.open('http://localhost:8080', '_blank');
  });
  document.body.appendChild(pill);
})();
