CREATE TABLE IF NOT EXISTS merchant_sessions (
  session_hash TEXT PRIMARY KEY,
  csrf_token TEXT NOT NULL,
  credential_version TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  idle_expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS merchant_sessions_expiry ON merchant_sessions(expires_at);
CREATE TABLE IF NOT EXISTS merchant_login_attempts (
  bucket TEXT PRIMARY KEY,
  window_start INTEGER NOT NULL,
  attempts INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
  sku TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  list_price_cents INTEGER NOT NULL,
  cost_cents INTEGER NOT NULL,
  retail_value_cents INTEGER NOT NULL,
  holding_cost_cents INTEGER NOT NULL DEFAULT 0,
  is_deadstock INTEGER NOT NULL DEFAULT 0,
  stock INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS merchant_policies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('DRAFT','ACTIVE','ARCHIVED')),
  source TEXT NOT NULL CHECK (source IN ('seed','llm','heuristic','manual')),
  source_prompt TEXT,
  policy_json TEXT NOT NULL,
  adjustments_json TEXT NOT NULL DEFAULT '[]',
  hard_floor_margin_pct REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS agreements (
  id TEXT PRIMARY KEY,
  negotiation_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  sku TEXT NOT NULL REFERENCES products(sku),
  bundle_sku TEXT REFERENCES products(sku),
  price_cents INTEGER NOT NULL,
  shipping_tier TEXT NOT NULL,
  pay_later INTEGER NOT NULL,
  buyer_budget_cents INTEGER NOT NULL,
  standard_total_cents INTEGER NOT NULL,
  fee_cents INTEGER NOT NULL,
  contribution_cents INTEGER NOT NULL,
  buyer_surplus_cents INTEGER NOT NULL,
  seller_surplus_cents INTEGER NOT NULL,
  policy_id INTEGER NOT NULL REFERENCES merchant_policies(id),
  status TEXT NOT NULL CHECK (status IN ('OFFERED','ORDER_CREATED','CAPTURED','FAILED','EXPIRED')),
  status_reason TEXT,
  paypal_order_id TEXT UNIQUE,
  capture_id TEXT,
  solver_latency_us INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agreements_session ON agreements(session_id, sku, created_at);
CREATE INDEX IF NOT EXISTS idx_agreements_status ON agreements(status);

CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);

CREATE TABLE IF NOT EXISTS inventory_reservations(agreement_id TEXT PRIMARY KEY REFERENCES agreements(id),state TEXT NOT NULL CHECK(state IN ('HELD','CONSUMED','RELEASED')));
CREATE TABLE IF NOT EXISTS payment_reconciliation(agreement_id TEXT PRIMARY KEY REFERENCES agreements(id),created_at INTEGER NOT NULL);

CREATE TABLE IF NOT EXISTS agreement_delivery(agreement_id TEXT PRIMARY KEY REFERENCES agreements(id),country TEXT NOT NULL);

CREATE TABLE IF NOT EXISTS merchant_events(id INTEGER PRIMARY KEY AUTOINCREMENT,kind TEXT NOT NULL,agreement_id TEXT,created_at INTEGER NOT NULL);

CREATE TABLE IF NOT EXISTS agreement_economics(agreement_id TEXT PRIMARY KEY REFERENCES agreements(id), snapshot_json TEXT NOT NULL);
