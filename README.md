# Fairdeal

AI-assisted commerce that helps a buyer and a merchant find an affordable package without violating the merchant's configured contribution margin.

## Demo scope

One demo merchant, Alpine Gear Co., sells a Winter Parka and can include a stocked Cashmere Scarf. The catalog, inventory, costs and shipping assumptions are seeded demonstration data. There is no external merchant network or live retail-price audit.

1. Describe a budget and delivery preference. The configured LLM translates your request into editable constraints; an explicitly labeled heuristic fallback works offline.
2. Review your maximum budget, shipping flexibility, accessory acceptance and Pay Later preference.
3. A deterministic solver searches eligible shipping, payment and bundle combinations. Product cost, accessory cost, shipping and configured payment fees are deducted before checking the contribution margin. Estimated holding-cost savings are a separate utility term, not margin revenue.
4. Fairdeal presents one feasible offer, copied directly from its persisted agreement. Its price and concessions are bound to an expiring HMAC token. If no agreement fits, the application reports that result without raising the buyer's budget.
5. The buyer approves payment through PayPal. Orders are created from the stored agreement, never a client-supplied price.

AI interprets buyer mandates and compiles merchant policy drafts. It does not conduct autonomous multi-agent bargaining. The offer's decision trace describes the actual solver result. Product discovery is limited to the seeded catalog.

## Run locally

Requires Go as specified in go.mod. Copy .env.example to .env. The local default is mock payment mode, which is explicitly simulated and transfers no money.

~~~powershell
go test -p 1 ./...
go build -p 1 -o fairdeal.exe .
./fairdeal.exe
~~~

Open http://localhost:8080 for the buyer and /merchant for the seller dashboard. On Windows, use -p 1 to limit compilation parallelism.

## PayPal Sandbox

Set PAYPAL_MODE=sandbox, PAYPAL_CLIENT_ID, PAYPAL_CLIENT_SECRET and NASHPAY_HMAC_SECRET (at least 32 random bytes). Use Sandbox merchant and buyer accounts. Sandbox startup fails when credentials are missing; checkout never silently substitutes a mock payment button.

Pay Later is a preference, not a guaranteed loan or installment schedule. Eligibility and terms are determined by PayPal. If unavailable, the application requotes without Pay Later. All configured payment fees are demo estimates. Sandbox payments do not transfer live funds.

PayPal docs: https://developer.paypal.com/docs/checkout/standard/integrate/ and https://developer.paypal.com/docs/checkout/pay-later/

## AI

Configure LLM_BASE_URL, LLM_API_KEY and LLM_MODEL for an OpenAI-compatible provider. The example uses DeepSeek's deepseek-flash; set the model to an identifier available in your account. Failed or unconfigured requests fall back to labeled heuristic parsing. The merchant policy compiler applies deterministic bounds to proposed parameters.

## Data and metrics

Browsing history is not collected or requested by the extension. Explicitly submitted deal constraints are processed on the server; submitted natural-language requests go to the configured AI provider when enabled. This is not local-only or zero-leakage processing.

Merchant metrics reflect stored agreements and captures, including simulations in mock mode. Captures below the catalog checkout total are not proof of recovered abandonment or conversion lift. Buyer summary is limited to the current browser session among the latest 100 agreements. No fabricated purchase history is added.

Kernel market verification is unavailable until a real product source, navigation, extraction and evidence are implemented. Configuring a key does not mark prices verified and does not create billable browser sessions. Savings compare this demo catalog's standard checkout, not a verified market MSRP.

## Render

The render.yaml Blueprint defines a persistent Go service and SQLite disk. It requires PayPal Sandbox credentials and AI credentials, generates HMAC and merchant access tokens, and disables demo reset. Enter MERCHANT_TOKEN using the seller dashboard's access button. The extension currently targets localhost; update its server URL and allowed host for a hosted demo.

## Architecture

Single Go binary with embedded HTML/CSS/JavaScript, pure Go SQLite, deterministic clearing solver, HMAC agreements, WebSocket previews, PayPal Orders v2 client, AI intent/policy parsing, and a Manifest V3 browser extension. The canonical matching endpoint is POST /api/fairdeal/match; legacy aliases remain for compatibility.

## Limitations

This is a hackathon proof of concept, not a production marketplace. There is no inventory reservation/fulfillment integration, live retail audit, verified real-world conversion study, or webhook reconciliation for uncertain payment outcomes. Contribution margin is based on configured costs and is not accounting net profit. Do not present simulated payments or data as live commerce.

MIT license. Built for the PayPal AI Hackathon.

## Repository contents

This repository includes the runnable application source, tests, assets and deployment configuration. Internal specifications and planning documents are omitted. Credentials must be configured separately; they are not included.

