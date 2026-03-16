DROP TABLE IF EXISTS acl_grants;
DROP TABLE IF EXISTS policy_rules;
DROP TABLE IF EXISTS orders_docs;
DROP TABLE IF EXISTS customers_docs;

CREATE TABLE IF NOT EXISTS orders_docs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  doc_json TEXT NOT NULL,

  -- v2 (commercetools-like generated fields)
  order_number TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.orderNumber')) STORED,
  customer_id TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerId')) STORED,
  customer_email TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerEmail')) STORED,
  order_state TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.orderState')) STORED,
  shipment_state TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.shipmentState')) STORED,
  payment_state TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.paymentState')) STORED,
  created_at TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.createdAt')) STORED,
  completed_at TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.completedAt')) STORED,
  tracking_id TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.shippingInfo.deliveries[0].parcels[0].trackingData.trackingId')) STORED,
  payment_reference TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.paymentInfo.paymentReference')) STORED,
  currency_code TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.totalPrice.currencyCode')) STORED,
  total_cent_amount REAL GENERATED ALWAYS AS (json_extract(doc_json, '$.totalPrice.centAmount')) STORED,
  return_eligible TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.returnInfo[0].eligible')) STORED,
  return_status TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.returnInfo[0].status')) STORED,
  refund_status TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.returnInfo[0].refundStatus')) STORED,

  -- v1 compatibility generated fields
  status TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.status')) STORED,
  total_amount REAL GENERATED ALWAYS AS (json_extract(doc_json, '$.total_amount')) STORED,
  region TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.custom.fields.region')) STORED
);

CREATE TABLE IF NOT EXISTS customers_docs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  doc_json TEXT NOT NULL,

  -- v2 (commercetools-like generated fields)
  customer_number TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerNumber')) STORED,
  email TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.email')) STORED,
  first_name TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.firstName')) STORED,
  last_name TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.lastName')) STORED,
  is_email_verified TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.isEmailVerified')) STORED,
  created_at TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.createdAt')) STORED,
  customer_group TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerGroup')) STORED,
  vip_tier TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.custom.fields.vipTier')) STORED,

  -- v1 compatibility generated fields
  name TEXT GENERATED ALWAYS AS (
    trim(coalesce(json_extract(doc_json, '$.firstName'), '') || ' ' || coalesce(json_extract(doc_json, '$.lastName'), ''))
  ) STORED,
  tier TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.custom.fields.segment')) STORED,
  region TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.addresses[0].country')) STORED
);

CREATE TABLE IF NOT EXISTS acl_grants (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  subject_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  action TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_rules (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  action TEXT NOT NULL,
  effect TEXT NOT NULL,
  subject_attr TEXT NOT NULL,
  resource_attr TEXT NOT NULL,
  op TEXT NOT NULL,
  value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_orders_wismo ON orders_docs(tenant_id, order_number, tracking_id, shipment_state, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders_docs(tenant_id, customer_id, customer_email, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_returns ON orders_docs(tenant_id, return_eligible, return_status, refund_status, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_state ON orders_docs(tenant_id, order_state, payment_state, shipment_state, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_payment_ref ON orders_docs(tenant_id, payment_reference, created_at);

CREATE INDEX IF NOT EXISTS idx_customers_identity ON customers_docs(tenant_id, customer_number, email);
CREATE INDEX IF NOT EXISTS idx_customers_profile ON customers_docs(tenant_id, customer_group, vip_tier, created_at);

CREATE INDEX IF NOT EXISTS idx_acl_lookup ON acl_grants(tenant_id, subject_id, resource_type, action, resource_id);
CREATE INDEX IF NOT EXISTS idx_policy_rules ON policy_rules(tenant_id, resource_type, action, effect);
