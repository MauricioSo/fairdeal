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
