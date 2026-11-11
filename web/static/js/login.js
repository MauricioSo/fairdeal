const form = document.getElementById('merchant-login');
const button = document.getElementById('login-submit');
const errorBox = document.getElementById('login-error');
let csrf = '';
async function prepare() {
  const response = await fetch('/api/merchant/auth/csrf', { credentials: 'same-origin', cache: 'no-store' });
  if (!response.ok) throw new Error('Merchant login is unavailable. Contact your administrator.');
  csrf = (await response.json()).csrf_token;
  button.disabled = false;
}
prepare().catch(error => { errorBox.textContent = error.message; });
form.addEventListener('submit', async event => {
  event.preventDefault(); button.disabled = true; errorBox.textContent = '';
  try {
    const response = await fetch('/api/merchant/login', {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
      body: JSON.stringify({ email: form.elements.email.value.trim(), password: form.elements.password.value })
    });
    form.elements.password.value = '';
    if (response.ok) { window.location.replace('/merchant'); return; }
    if (response.status === 429) throw new Error('Too many sign-in attempts. Please try again later.');
    if (response.status === 401) throw new Error('Incorrect email or password.');
    if (response.status === 403) { await prepare(); throw new Error('Your sign-in page expired. Please try again.'); }
    throw new Error('Could not sign in. Please try again.');
  } catch (error) { errorBox.textContent = error.message; }
  finally { button.disabled = false; }
});
