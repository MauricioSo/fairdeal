# FairDeal

AI-assisted bilateral commerce with five fictional merchants, private buyer budgets and PayPal Sandbox checkout. This is a hackathon prototype with seeded products, inventory, shipping assumptions and estimated fees.

## What works

- Five persisted merchant scenarios: Alpine, Nordic, Glacier, Sierra and Polaris. Each uses a separate SQLite database, catalog, policies, login sessions and sales history.
- Buyer intent parsing and merchant policy compilation through a configurable OpenAI-compatible LLM, with labeled deterministic fallbacks.
- Five isolated seller-agent contexts. Each receives only public product/preferences and its own policy. Up to two concession revisions run under a shared concurrency limit and a bounded deadline. All monetary decisions are validated by Go.
- Seller quotations use public catalog ceilings and a fixed demo valuation reference. The private budget only filters feasible quotes in the coordinator; it never anchors a seller's price. Buyer budgets and derived private surplus are not persisted into new merchant records or exposed in merchant APIs/metrics/prompts.
- Catalog editing and archival behind authenticated, CSRF-protected merchant APIs. Catalog changes invalidate outstanding quotes; active reservations prevent inconsistent edits.
- Atomic product/accessory reservations at checkout. Duplicate captures consume inventory and record a sale once. Uncertain payments retain their reservation for reconciliation instead of being treated as confirmed failures.
- Versioned HMAC contracts bind merchant, product, policy, amount, currency, delivery, payment preference, accessory and expiration. Existing captured agreements remain intact during migration.
- Sandbox Orders v2 amount/reference checks; provider reconciliation and a verified webhook listener. No live funds are transferred.
- Manifest V3 extension with explicit context-menu extraction from JSON-LD/metadata, confirmation in the web app and a selectable hosted/local destination. External pages do not become participating merchants.

Offers are sorted by total price. Compare delivery and accessories before selecting. Pay Later is a preference; eligibility and terms are determined by PayPal. Demo delivery currently supports US and CL: all five scenarios serve US, while Alpine, Nordic and Sierra also serve CL. There is no physical fulfillment or tax engine.

## Merchant profitability engine

The seller console includes explicit cost/rule configuration, draft review and activation, a non-binding pack/delivery simulator, and private economic audit for each new agreement. Monetary amounts are integer USD cents and configured rates use basis points. AI policy compilation preserves the merchant's economic protections.

A pure economics evaluator is the sole authority for offer profitability. It subtracts product costs, shipping, preparation, packaging, payment fees and platform fees from net revenue after included tax. Both minimum contribution and minimum order margin must hold. Optional per-product protections prevent discount/free inclusion and enforce allocated item margins. Holding-cost relief can rank candidates but never increases accounting contribution.

Costs are merchant-configured values or conservative upper bounds, not verified carrier rates. Legacy optional costs default to zero and must be reviewed before real use; zero or missing product costs block automatic offers. Payment fees apply to the complete charge; platform percentage applies to net revenue. Tax is an explicitly configured included rate, not a jurisdiction-aware tax engine.

Each new offer stores a private economic snapshot and economic-input revision. Checkout checks that revision and re-evaluates the snapshot inside the same transaction that reserves inventory. Changed conditions require a new offer; reserved orders retain their frozen agreement. Payment completion still requires the exact amount, currency and provider reference. Merchant snapshots contain no buyer budget or private utility.

The current pack scope is one main item plus one merchant-selected accessory, one unit each, with the two existing demo delivery tiers. The simulator shows why combinations pass or fail and creates neither agreements nor payments. It does not predict customer acceptance. Arbitrary multi-item carts, carrier fulfillment and tax determination remain outside this prototype.

Architecture stays within the Go monolith: pure economic value objects/rules, the clearing candidate search, an application service coordinating agreements, and transactional SQLite repositories. Strategy boundaries are limited to actual external providers; no service mesh or speculative class hierarchy is required.

Before the first economics migration, an existing file-backed database gets a private consistent SQLite pre-economics backup. Historical captures and reserved orders are preserved; outstanding offers without snapshots expire and need a fresh quote.

## Run and test

Use the Go version specified in go.mod. Copy .env.example to .env and configure the settings you need. Mock mode is explicitly simulated and transfers no funds.

~~~powershell
go test -p 1 ./...
go vet ./...
go build -p 1 -o fairdeal.exe .
./fairdeal.exe
~~~

Open http://localhost:8080 and /merchant/login. The database directory must exist. Keep DB_PATH and all five sibling database files on persistent storage.

## Provision five private merchant logins

Generate credentials outside the checkout:

~~~powershell
go run ./cmd/marketplace-access -out-dir ../private-merchant-accounts
~~~

The command refuses to overwrite files and creates two private files: merchant-access-private.json (emails and passwords) and merchant-verifiers-private.json (email/password-hash objects). Set MERCHANTS_JSON to the complete verifier JSON only. Never publish either file or place plaintext passwords in server configuration. The legacy MERCHANT_EMAIL/MERCHANT_PASSWORD_HASH pair remains available for Alpine when no Alpine object is provided.

Passwords use PBKDF2-HMAC-SHA256 with 600,000 iterations and unique salts. Sessions are revocable, have idle/absolute expiration, and use Secure/HttpOnly/SameSite Strict cookies on HTTPS. Mutations require same-origin JSON and CSRF tokens. Missing login configuration fails closed. Registration, recovery and MFA are outside this demo.

## PayPal Sandbox and webhooks

Set PAYPAL_MODE=sandbox, PAYPAL_CLIENT_ID, PAYPAL_CLIENT_SECRET and a stable NASHPAY_HMAC_SECRET of at least 32 random bytes. The historical environment/module namespace remains for compatibility; the app brand is FairDeal. Sandbox startup rejects missing credentials and never substitutes mock buttons on failure.

Register /api/paypal/webhook for PAYMENT.CAPTURE.COMPLETED in the same Sandbox application and set PAYPAL_WEBHOOK_ID. Until configured, the endpoint returns unavailable. Signatures are verified through PayPal before any event can schedule reconciliation; authoritative order/capture data is checked again. Repeated events do not duplicate payments. A periodic observer also reconciles uncertain captures without initiating a new capture.

Five logical stores sharing a Sandbox application do not constitute marketplace payouts to five real merchant accounts.

[PayPal webhook verification](https://developer.paypal.com/api/webhooks/v1/verify-webhook-signature-post)

## Chrome extension

Load the extension directory as an unpacked Manifest V3 extension from chrome://extensions (Developer mode), then reload the product page. For updates use Reload on the extension card and refresh the shop tab. The ZIP release contains the same runtime files; unzip it before loading. Installation and permission approval remain under the user's control.

Version 1.1 detects a selected product from JSON-LD, schema microdata or explicit product OpenGraph tags and places a dismissible FairDeal button at the top of its matching product photo. It handles lazy-loaded images, layout changes and SPA metadata updates. Ambiguous listings, ordinary articles, checkout/account pages and unmatched images do not get a button. Sites without supported metadata may not be detected.

The popup can disable automatic photo buttons, select hosted/local FairDeal, or open the current product reference. The image/page context menu is also available. Automatic detection requires content-script access to HTTP/HTTPS pages, but detection is local: no scraping service, browsing history, automatic server requests or account access. On explicit search the extension sends only name, brand, SKU, displayed price/currency and source URL without query/fragment; the price never becomes the user's budget. No external images are loaded by the import dialog.

FairDeal asks the buyer to review the reference and enter a private USD budget before comparing. The current marketplace still contains fictional parkas: no external store joins the negotiation, no exact product equivalence or external savings are claimed, and unsupported categories offer a separate, explicitly labeled parka demo. Import does not initiate payment.

Run extension unit tests with Node: Get-Content extension/product.test.cjs -Raw | node - . Browser harness tests use the actual product/content scripts with simulated Chrome APIs; installation on a user's Chrome profile requires a separate manual load/approval.

## AI and privacy

Configure AI_MODE=llm, LLM_BASE_URL, LLM_MODEL and LLM_API_KEY. Use a model actually available in your account. The example uses deepseek-flash. Requests are bounded and failed/unconfigured calls fall back to explicitly labeled deterministic behavior.

Submitted natural-language buyer requests may go to the configured buyer AI provider. Seller agents get separately constructed allowlisted contexts without the buyer's original prompt, budget or private utility. We do not collect browsing history. This is server-mediated processing, not a claim of local-only or inference-proof privacy: accepting a price necessarily reveals that accepted amount.

Kernel market verification remains unavailable for the fictional catalog: no external equivalent product source/evidence is configured. No key triggers billable browser sessions or marks prices verified. Savings compare each merchant's seeded standard checkout, not an audited market MSRP.

## Storage and recovery

Alpine retains DB_PATH. Other tenants use fairdeal-nordic.db, fairdeal-glacier.db, fairdeal-sierra.db and fairdeal-polaris.db in the same directory. Tables are migrated additively. Before the first merchant-identity migration, a consistent SQLite pre-marketplace backup is written beside each file. The migration expires outstanding old-version quotes and removes legacy private budget/surplus values; capture amounts and IDs are preserved. Backups are private operational data and must never be served as public assets.

Uncertain provider operations intentionally retain inventory. Review unresolved orders before releasing stock; a timeout is not proof of failure. Reconciliation observes up to ten oldest-attempted uncertain records per merchant on each pass and is not a complete accounting, refund or fulfillment system. Monitor it for long-lived pending orders.

## Deployment

render.yaml defines a Go service and persistent disk. Configure PayPal/AI credentials, MERCHANTS_JSON and the optional webhook ID privately in Render. Back up the database before deployment and verify each login and Sandbox checkout after startup. Never publish internal specifications, .env files, databases, backups, account files or provider credentials.

MIT license. Built for the PayPal AI Hackathon.
