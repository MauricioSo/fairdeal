# Working on FairDeal

## Repository and collaboration

- The canonical local project is Documents/fairdeal. Work and commit in this checkout. Do not use Documents/Codex/FairDeal/public-reviewed as an alternate publication source or copy code between those checkouts.
- The shared source is https://github.com/MauricioSo/fairdeal, branch main. Local main tracks origin/main. Start new development on a codex/ branch; check the current branch, status and upstream before changing files.
- Coordinate file ownership when other agents are working. Preserve their uncommitted changes. Do not reset, clean or overwrite unrelated work.
- Keep historical private documents and local runtime data outside the published Git history. Do not merge an old local history merely to recover a file; review that file independently.

## Product invariants

- The product name is FairDeal. Existing Go module/environment names may retain NashPay for compatibility.
- Buyer maximum budget and private utility stay in the central coordinator. Never include them in seller-agent prompts, merchant APIs, persisted merchant agreement data or merchant metrics.
- AI proposes; the deterministic economics evaluator authorizes profitability. Keep it free of database, HTTP and model dependencies. All candidate/rounding/checkout paths must preserve margin and contribution rules.
- Use integer cents and basis points for monetary decisions. Preserve immutable economic snapshots, atomic stock reservations, exact PayPal amount/reference checks and idempotent reconciliation.
- This is a five-fictional-merchant prototype. The current pack is one main item plus one accessory. External product references are untrusted data and do not make the external store a participating merchant or establish product equivalence.
- PayPal Sandbox/mock and external price verification limitations must remain explicit. Never imply a real payment, real fulfillment or verified savings from a simulation.

## Publication hygiene

- Stage specific reviewed source files. Never publish .env, credentials, password/verifier files, databases/WAL/SHM, backups, local outputs, docs/specs or design-preview.html. .env.example contains placeholders only.
- Ignoring a path does not remove an already-tracked file. Inspect git diff --cached --name-only and git diff --cached --check before committing; confirm ignored private paths are untracked.
- Do not put secrets in logs, screenshots, issue bodies or tool output. Do not commit local credential/setup reports.

## Validation

- For Go behavior changes run the relevant tests, then go test -p 1 ./... and go vet ./... as appropriate to the change.
- Extension regression tests: Get-Content extension/product.test.cjs -Raw | node -
- Check JavaScript syntax when editing browser code. Product-photo detection is local; importing requires an explicit click and private-budget confirmation. No browsing-history collection or automatic external-page uploads.
- Separate a local harness test from verification of an installed Chrome extension. Do not claim installation or live merchant integration from simulated APIs.
- Documentation-only changes need source/diff and publication checks, not a repeated payment test.
