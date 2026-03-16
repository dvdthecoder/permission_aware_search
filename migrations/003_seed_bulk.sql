-- Bulk synthetic seed for realistic commercetools-like payloads.
-- Tenant A minimum: 20000 customers + 20000 orders.
-- Tenant B isolation: 500 customers + 500 orders.

-- Tenant A customers (20000)
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 20000
)
INSERT OR IGNORE INTO customers_docs (id, tenant_id, doc_json)
SELECT
  printf('cust-%05d', n),
  'tenant-a',
  json_object(
    'id', printf('cust-%05d', n),
    'version', (n % 8) + 1,
    'customerNumber', printf('CUST-%06d', n),
    'email', printf('customer%05d@tenant-a.example.com', n),
    'firstName', printf('First%05d', n),
    'lastName', printf('Last%05d', n),
    'companyName', printf('Company %05d', n),
    'isEmailVerified', CASE WHEN n % 5 = 0 THEN 0 ELSE 1 END,
    'dateOfBirth', strftime('%Y-%m-%d', date('1980-01-01', printf('+%d days', n % 9000))),
    'createdAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2024-01-01', printf('+%d minutes', n * 5))),
    'lastModifiedAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2025-12-01', printf('+%d minutes', n * 2))),
    'customerGroup', CASE WHEN n % 10 = 0 THEN 'vip' WHEN n % 3 = 0 THEN 'business' ELSE 'standard' END,
    'stores', json_array(CASE WHEN n % 2 = 0 THEN 'store-west' ELSE 'store-east' END),
    'addresses', json_array(json_object(
      'id', printf('addr-%05d', n),
      'country', CASE (n % 4) WHEN 0 THEN 'east' WHEN 1 THEN 'west' WHEN 2 THEN 'north' ELSE 'south' END,
      'city', printf('City%03d', n % 120)
    )),
    'defaultShippingAddressId', printf('addr-%05d', n),
    'defaultBillingAddressId', printf('addr-%05d', n),
    'shippingAddressIds', json_array(printf('addr-%05d', n)),
    'billingAddressIds', json_array(printf('addr-%05d', n)),
    'custom', json_object('fields', json_object(
      'vipTier', CASE WHEN n % 40 = 0 THEN 'platinum' WHEN n % 10 = 0 THEN 'gold' ELSE 'silver' END,
      'segment', CASE WHEN n % 10 = 0 THEN 'gold' WHEN n % 4 = 0 THEN 'silver' ELSE 'bronze' END,
      'supportHistory', CASE WHEN n % 20 = 0 THEN 'priority' ELSE 'normal' END,
      'accountNotes', CASE WHEN n % 27 = 0 THEN 'Frequent support contact' ELSE '' END
    )),
    'region', CASE (n % 4) WHEN 0 THEN 'east' WHEN 1 THEN 'west' WHEN 2 THEN 'north' ELSE 'south' END
  )
FROM seq;

-- Tenant A orders (20000)
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 20000
)
INSERT OR IGNORE INTO orders_docs (id, tenant_id, doc_json)
SELECT
  printf('ord-%05d', n),
  'tenant-a',
  json_object(
    'id', printf('ord-%05d', n),
    'version', (n % 6) + 1,
    'orderNumber', printf('ORD-%06d', n),
    'createdAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2025-01-01', printf('+%d minutes', n * 15))),
    'lastModifiedAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2026-01-01', printf('+%d minutes', n * 4))),
    'completedAt', CASE WHEN n % 4 = 0 THEN strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2026-02-01', printf('+%d minutes', n))) ELSE NULL END,
    'orderState', CASE WHEN n % 9 = 0 THEN 'Cancelled' WHEN n % 4 = 0 THEN 'Complete' WHEN n % 3 = 0 THEN 'Confirmed' ELSE 'Open' END,
    'shipmentState', CASE WHEN n % 11 = 0 THEN 'Delayed' WHEN n % 5 = 0 THEN 'Shipped' WHEN n % 4 = 0 THEN 'Delivered' ELSE 'Ready' END,
    'paymentState', CASE WHEN n % 13 = 0 THEN 'Failed' WHEN n % 2 = 0 THEN 'Paid' ELSE 'Pending' END,
    'paymentInfo', json_object(
      'paymentReference', printf('PAY-%08d', n)
    ),
    'customerId', printf('cust-%05d', ((n - 1) % 20000) + 1),
    'customerEmail', printf('customer%05d@tenant-a.example.com', ((n - 1) % 20000) + 1),
    'totalPrice', json_object(
      'centAmount', 5000 + (n % 250000),
      'currencyCode', CASE WHEN n % 9 = 0 THEN 'EUR' ELSE 'USD' END,
      'fractionDigits', 2
    ),
    'lineItems', json_array(
      json_object(
        'id', printf('li-%05d-a', n),
        'productId', printf('prod-%04d', n % 700),
        'name', printf('Product %04d', n % 700),
        'variant', json_object('sku', printf('SKU-%06d', n % 20000)),
        'quantity', (n % 4) + 1,
        'totalPrice', json_object('centAmount', 1500 + (n % 80000), 'currencyCode', 'USD')
      )
    ),
    'shippingInfo', json_object(
      'shippingMethodName', CASE WHEN n % 3 = 0 THEN 'Express' ELSE 'Ground' END,
      'price', json_object('centAmount', 500 + (n % 2500), 'currencyCode', 'USD'),
      'deliveries', json_array(json_object(
        'id', printf('del-%05d', n),
        'parcels', json_array(json_object(
          'id', printf('par-%05d', n),
          'trackingData', json_object(
            'trackingId', printf('TRK-%08d', n),
            'carrier', CASE (n % 4) WHEN 0 THEN 'DHL' WHEN 1 THEN 'UPS' WHEN 2 THEN 'FedEx' ELSE 'USPS' END,
            'provider', CASE (n % 4) WHEN 0 THEN 'DHL' WHEN 1 THEN 'UPS' WHEN 2 THEN 'FedEx' ELSE 'USPS' END,
            'providerTransaction', printf('txn-%08d', n)
          )
        ))
      ))
    ),
    'returnInfo', json_array(json_object(
      'eligible', CASE WHEN n % 5 = 0 THEN 'false' ELSE 'true' END,
      'status', CASE WHEN n % 14 = 0 THEN 'Requested' WHEN n % 17 = 0 THEN 'Rejected' ELSE 'NotRequested' END,
      'refundStatus', CASE WHEN n % 14 = 0 THEN 'Pending' WHEN n % 19 = 0 THEN 'Completed' ELSE 'NotInitiated' END
    )),
    'custom', json_object('fields', json_object(
      'delayReason', CASE WHEN n % 11 = 0 THEN 'Weather disruption' WHEN n % 23 = 0 THEN 'Carrier backlog' ELSE '' END,
      'etaNote', CASE WHEN n % 11 = 0 THEN 'Delayed by 2 days' ELSE 'On schedule' END,
      'region', CASE (n % 4) WHEN 0 THEN 'east' WHEN 1 THEN 'west' WHEN 2 THEN 'north' ELSE 'south' END
    )),
    'status', CASE WHEN n % 5 = 0 THEN 'closed' ELSE 'open' END,
    'total_amount', (5000 + (n % 250000)) / 100.0,
    'region', CASE (n % 4) WHEN 0 THEN 'east' WHEN 1 THEN 'west' WHEN 2 THEN 'north' ELSE 'south' END
  )
FROM seq;

-- Tenant B customers (500)
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 500
)
INSERT OR IGNORE INTO customers_docs (id, tenant_id, doc_json)
SELECT
  printf('tb-cust-%04d', n),
  'tenant-b',
  json_object(
    'id', printf('tb-cust-%04d', n),
    'version', 1,
    'customerNumber', printf('TB-CUST-%05d', n),
    'email', printf('customer%04d@tenant-b.example.com', n),
    'firstName', printf('TBFirst%04d', n),
    'lastName', printf('TBLast%04d', n),
    'companyName', printf('TB Company %04d', n),
    'isEmailVerified', 1,
    'createdAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2026-02-01', printf('+%d minutes', n))),
    'lastModifiedAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2026-02-15', printf('+%d minutes', n))),
    'customerGroup', CASE WHEN n % 7 = 0 THEN 'vip' ELSE 'standard' END,
    'stores', json_array('store-b'),
    'addresses', json_array(json_object('id', printf('tb-addr-%04d', n), 'country', CASE WHEN n % 2 = 0 THEN 'west' ELSE 'east' END, 'city', 'TenantBCity')),
    'defaultShippingAddressId', printf('tb-addr-%04d', n),
    'defaultBillingAddressId', printf('tb-addr-%04d', n),
    'shippingAddressIds', json_array(printf('tb-addr-%04d', n)),
    'billingAddressIds', json_array(printf('tb-addr-%04d', n)),
    'custom', json_object('fields', json_object('vipTier', CASE WHEN n % 7 = 0 THEN 'gold' ELSE 'silver' END, 'segment', 'silver', 'supportHistory', 'normal', 'accountNotes', '')),
    'region', CASE WHEN n % 2 = 0 THEN 'west' ELSE 'east' END
  )
FROM seq;

-- Tenant B orders (500)
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 500
)
INSERT OR IGNORE INTO orders_docs (id, tenant_id, doc_json)
SELECT
  printf('tb-ord-%04d', n),
  'tenant-b',
  json_object(
    'id', printf('tb-ord-%04d', n),
    'version', 1,
    'orderNumber', printf('TB-ORD-%05d', n),
    'createdAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2026-02-01', printf('+%d minutes', n * 10))),
    'lastModifiedAt', strftime('%Y-%m-%dT%H:%M:%SZ', datetime('2026-02-20', printf('+%d minutes', n * 4))),
    'completedAt', NULL,
    'orderState', CASE WHEN n % 9 = 0 THEN 'Cancelled' ELSE 'Open' END,
    'shipmentState', CASE WHEN n % 4 = 0 THEN 'Shipped' ELSE 'Ready' END,
    'paymentState', CASE WHEN n % 2 = 0 THEN 'Paid' ELSE 'Pending' END,
    'paymentInfo', json_object(
      'paymentReference', printf('TB-PAY-%06d', n)
    ),
    'customerId', printf('tb-cust-%04d', ((n - 1) % 500) + 1),
    'customerEmail', printf('customer%04d@tenant-b.example.com', ((n - 1) % 500) + 1),
    'totalPrice', json_object('centAmount', 10000 + n * 50, 'currencyCode', 'USD', 'fractionDigits', 2),
    'lineItems', json_array(json_object('id', printf('tb-li-%04d', n), 'productId', printf('tb-prod-%03d', n % 100), 'name', 'TB Product', 'variant', json_object('sku', printf('TB-SKU-%04d', n)), 'quantity', 1, 'totalPrice', json_object('centAmount', 10000 + n * 50, 'currencyCode', 'USD'))),
    'shippingInfo', json_object('shippingMethodName', 'Ground', 'price', json_object('centAmount', 700, 'currencyCode', 'USD'), 'deliveries', json_array(json_object('id', printf('tb-del-%04d', n), 'parcels', json_array(json_object('id', printf('tb-par-%04d', n), 'trackingData', json_object('trackingId', printf('TB-TRK-%06d', n), 'carrier', 'UPS', 'provider', 'UPS', 'providerTransaction', printf('tb-txn-%06d', n))))))),
    'returnInfo', json_array(json_object('eligible', 'true', 'status', 'NotRequested', 'refundStatus', 'NotInitiated')),
    'custom', json_object('fields', json_object('delayReason', '', 'etaNote', 'On schedule', 'region', CASE WHEN n % 2 = 0 THEN 'west' ELSE 'east' END)),
    'status', 'open',
    'total_amount', (10000 + n * 50) / 100.0,
    'region', CASE WHEN n % 2 = 0 THEN 'west' ELSE 'east' END
  )
FROM seq;

-- Additional ACL grants for playground personas.
-- Bob gets every 5th order and every 6th customer in tenant-a.
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 20000
)
INSERT OR IGNORE INTO acl_grants (id, tenant_id, subject_id, resource_type, resource_id, action)
SELECT printf('g-bob-ord-%05d', n), 'tenant-a', 'user:bob', 'order', printf('ord-%05d', n), 'view'
FROM seq WHERE n % 5 = 0;

WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 20000
)
INSERT OR IGNORE INTO acl_grants (id, tenant_id, subject_id, resource_type, resource_id, action)
SELECT printf('g-bob-cust-%05d', n), 'tenant-a', 'user:bob', 'customer', printf('cust-%05d', n), 'view'
FROM seq WHERE n % 6 = 0;

-- Carol gets every 7th order and every 4th customer in tenant-a.
WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 20000
)
INSERT OR IGNORE INTO acl_grants (id, tenant_id, subject_id, resource_type, resource_id, action)
SELECT printf('g-carol-ord-%05d', n), 'tenant-a', 'user:carol', 'order', printf('ord-%05d', n), 'view'
FROM seq WHERE n % 7 = 0;

WITH RECURSIVE seq(n) AS (
  SELECT 1
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 20000
)
INSERT OR IGNORE INTO acl_grants (id, tenant_id, subject_id, resource_type, resource_id, action)
SELECT printf('g-carol-cust-%05d', n), 'tenant-a', 'user:carol', 'customer', printf('cust-%05d', n), 'view'
FROM seq WHERE n % 4 = 0;

-- Tenant B manager wildcard for isolation testing.
INSERT OR IGNORE INTO acl_grants (id, tenant_id, subject_id, resource_type, resource_id, action) VALUES
('g-tb-manager-order-all', 'tenant-b', 'role:manager', 'order', '*', 'view'),
('g-tb-manager-customer-all', 'tenant-b', 'role:manager', 'customer', '*', 'view');
