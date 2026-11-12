const sessionResponse = await fetch('/api/merchant/session', { credentials: 'same-origin' });
if (sessionResponse.ok) {
  const session = await sessionResponse.json();
  const identity = document.getElementById("active-merchant-name");
  if (identity) identity.textContent = session.merchant_name || session.email;

  const mountTarget = document.getElementById('catalog-container') || document.body;
  mountTarget.innerHTML = '';

  const card = document.createElement('section');
  card.className = 'catalog-management-card';

  card.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:1rem;margin-bottom:1rem;border-bottom:1px solid var(--border);padding-bottom:1rem;">
      <div>
        <h2 style="font-size:1.3rem;color:var(--heading);margin:0;">Store Inventory &amp; Pack Catalog</h2>
        <p style="font-size:0.84rem;color:var(--text-muted);margin:0.25rem 0 0;">
          Fictional demo inventory. Amounts in dollars and cents; stock reflects available units in warehouse.
        </p>
      </div>
      <button id="btn-show-add-product" type="button" class="btn-action-sm" style="background:var(--island-bg);color:var(--island-text);padding:0.5rem 1rem;font-weight:600;">
        + Add New Product
      </button>
    </div>

    <!-- Product Form (Collapsible) -->
    <div id="catalog-form-drawer" style="display:none;background:var(--bg-subtle);border:1px solid var(--border);border-radius:14px;padding:1.25rem;margin-bottom:1.5rem;">
      <h3 id="form-mode-title" style="font-size:1rem;color:var(--heading);margin-bottom:0.75rem;">Add / Edit Product</h3>
      <form id="product-mgmt-form">
        <div style="display:grid;grid-template-columns:repeat(auto-fit, minmax(200px, 1fr));gap:0.85rem;margin-bottom:1rem;">
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            SKU Code
            <input type="text" id="f-sku" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);font-family:var(--font-mono);" />
          </label>
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            Product Name
            <input type="text" id="f-name" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);" />
          </label>
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            List Price ($)
            <input type="number" id="f-price" step="0.01" min="0" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);font-family:var(--font-mono);" />
          </label>
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            Unit Cost ($)
            <input type="number" id="f-cost" step="0.01" min="0" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);font-family:var(--font-mono);" />
          </label>
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            Retail Perceived Value ($)
            <input type="number" id="f-retail" step="0.01" min="0" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);font-family:var(--font-mono);" />
          </label>
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            Holding Cost / Month ($)
            <input type="number" id="f-holding" step="0.01" min="0" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);font-family:var(--font-mono);" />
          </label>
          <label style="display:flex;flex-direction:column;gap:0.3rem;font-size:0.8rem;font-weight:600;color:var(--text);">
            Warehouse Stock Units
            <input type="number" id="f-stock" min="0" step="1" required style="padding:0.6rem 0.8rem;border:1px solid var(--border);border-radius:8px;background:var(--card);font-family:var(--font-mono);" />
          </label>
        </div>

        <div style="display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:1rem;">
          <label style="display:flex;align-items:center;gap:0.5rem;font-size:0.84rem;font-weight:600;color:var(--heading);cursor:pointer;">
            <input type="checkbox" id="f-deadstock" style="width:18px;height:18px;" />
            Mark as Accessory / Deadstock Candidate (for Bundling)
          </label>
          <div style="display:flex;gap:0.6rem;">
            <button type="button" id="btn-cancel-form" class="btn-action-sm">Cancel</button>
            <button type="submit" id="btn-save-product" class="btn-action-sm" style="background:var(--green);color:white;font-weight:600;">Save Product</button>
          </div>
        </div>
      </form>
    </div>

    <!-- Inventory Table Container -->
    <div class="table-responsive">
      <table class="catalog-table">
        <thead>
          <tr>
            <th>Product &amp; SKU</th>
            <th>List Price</th>
            <th>Unit Cost</th>
            <th>Gross Margin</th>
            <th>Stock</th>
            <th>Role</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody id="catalog-table-body">
          <tr><td colspan="7" style="text-align:center;color:var(--text-muted);padding:1rem;">Loading inventory...</td></tr>
        </tbody>
      </table>
    </div>
    <div id="catalog-status-msg" role="status" style="font-size:0.82rem;margin-top:0.75rem;color:var(--text-muted);"></div>
  `;

  mountTarget.appendChild(card);

  const drawer = document.getElementById('catalog-form-drawer');
  const showBtn = document.getElementById('btn-show-add-product');
  const cancelBtn = document.getElementById('btn-cancel-form');
  const form = document.getElementById('product-mgmt-form');
  const tbody = document.getElementById('catalog-table-body');
  const statusMsg = document.getElementById('catalog-status-msg');

  showBtn.onclick = () => {
    form.reset();
    document.getElementById('f-sku').disabled = false;
    document.getElementById('form-mode-title').textContent = 'Add New Product to Store';
    drawer.style.display = 'block';
  };

  cancelBtn.onclick = () => {
    drawer.style.display = 'none';
  };

  async function loadCatalog() {
    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-muted);padding:1rem;">Loading inventory...</td></tr>';
    try {
      const response = await fetch('/api/merchant/catalog', { credentials: 'same-origin' });
      if (!response.ok) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--red);">Could not load catalog.</td></tr>';
        return;
      }
      const data = await response.json();
      tbody.innerHTML = '';

      if (!data.products || data.products.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-muted);padding:1.5rem;">No products in catalog.</td></tr>';
        return;
      }

      data.products.forEach(p => {
        const tr = document.createElement('tr');
        const listPrice = (p.list_price_cents / 100).toFixed(2);
        const unitCost = (p.cost_cents / 100).toFixed(2);
        const marginPct = p.list_price_cents > 0 ? Math.round(((p.list_price_cents - p.cost_cents) / p.list_price_cents) * 100) : 0;

        tr.innerHTML = `
          <td>
            <div style="font-weight:600;color:var(--heading);">${p.name}</div>
            <div style="font-size:0.75rem;font-family:var(--font-mono);color:var(--text-muted);">${p.sku}</div>
          </td>
          <td style="font-family:var(--font-mono);font-weight:600;">$${listPrice}</td>
          <td style="font-family:var(--font-mono);color:var(--text-muted);">$${unitCost}</td>
          <td>
            <span style="font-family:var(--font-mono);font-weight:600;color:${marginPct >= 30 ? 'var(--green)' : 'var(--yellow)'};">
              ${marginPct}%
            </span>
          </td>
          <td style="font-family:var(--font-mono);font-weight:600;">
            ${p.stock}
            ${p.stock <= 5 ? '<span style="color:var(--red);font-size:0.7rem;margin-left:4px;">Low</span>' : ''}
          </td>
          <td>
            ${p.is_deadstock 
              ? '<span class="deadstock-pill">Deadstock / Pack</span>' 
              : '<span style="font-size:0.72rem;background:rgba(0,0,0,0.06);padding:0.15rem 0.5rem;border-radius:6px;color:var(--text);">Main Catalog</span>'}
          </td>
          <td>
            <div style="display:flex;gap:0.4rem;">
              <button type="button" class="btn-action-sm btn-edit-p" data-sku="${p.sku}">Edit</button>
              <button type="button" class="btn-action-sm danger btn-archive-p" data-sku="${p.sku}">Archive</button>
            </div>
          </td>
        `;

        // Edit
        tr.querySelector('.btn-edit-p').onclick = () => {
          document.getElementById('f-sku').value = p.sku;
          document.getElementById('f-sku').disabled = true;
          document.getElementById('f-name').value = p.name;
          document.getElementById('f-price').value = (p.list_price_cents / 100).toFixed(2);
          document.getElementById('f-cost').value = (p.cost_cents / 100).toFixed(2);
          document.getElementById('f-retail').value = (p.retail_value_cents / 100).toFixed(2);
          document.getElementById('f-holding').value = (p.holding_cost_cents / 100).toFixed(2);
          document.getElementById('f-stock').value = p.stock;
          document.getElementById('f-deadstock').checked = p.is_deadstock;

          document.getElementById('form-mode-title').textContent = `Edit Product: ${p.sku}`;
          drawer.style.display = 'block';
          drawer.scrollIntoView({ behavior: 'smooth' });
        };

        // Archive
        tr.querySelector('.btn-archive-p').onclick = async () => {
          if (!confirm(`Are you sure you want to archive ${p.name}? Availability will be set to zero.`)) return;
          const res = await fetch('/api/merchant/catalog/' + encodeURIComponent(p.sku), {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf_token },
            body: '{}'
          });
          statusMsg.textContent = res.ok ? 'Product archived successfully.' : 'Could not archive: check active reservations.';
          await loadCatalog();
          window.dispatchEvent(new Event('fairdeal-catalog-updated'));
        };

        tbody.appendChild(tr);
      });
    } catch (e) {
      tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--red);">Connection error loading catalog.</td></tr>';
    }
  }

  form.onsubmit = async (e) => {
    e.preventDefault();
    const saveBtn = document.getElementById('btn-save-product');
    saveBtn.disabled = true;
    saveBtn.textContent = 'Saving...';

    const sku = document.getElementById('f-sku').value.trim();
    const product = {
      sku: sku,
      name: document.getElementById('f-name').value.trim(),
      list_price_cents: Math.round(Number(document.getElementById('f-price').value) * 100),
      cost_cents: Math.round(Number(document.getElementById('f-cost').value) * 100),
      retail_value_cents: Math.round(Number(document.getElementById('f-retail').value) * 100),
      holding_cost_cents: Math.round(Number(document.getElementById('f-holding').value) * 100),
      stock: parseInt(document.getElementById('f-stock').value, 10),
      is_deadstock: document.getElementById('f-deadstock').checked
    };

    try {
      const response = await fetch('/api/merchant/catalog/' + encodeURIComponent(sku), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf_token },
        body: JSON.stringify(product)
      });
      if (response.ok) {
        statusMsg.textContent = 'Product saved successfully! Outstanding offers updated.';
        drawer.style.display = 'none';
        await loadCatalog();
        window.dispatchEvent(new Event('fairdeal-catalog-updated'));
      } else {
        statusMsg.textContent = 'Could not save product: please check values.';
      }
    } catch {
      statusMsg.textContent = 'Connection error.';
    } finally {
      saveBtn.disabled = false;
      saveBtn.textContent = 'Save Product';
    }
  };

  await loadCatalog();
}
