DELETE FROM acl_grants;
DELETE FROM policy_rules;
DELETE FROM orders_docs;
DELETE FROM customers_docs;

INSERT INTO customers_docs (id, tenant_id, doc_json) VALUES
('cust-1', 'tenant-a', json_object(
  'id','cust-1','version',1,'customerNumber','CUST-1001','email','aster@example.com','firstName','Aster','lastName','Retail',
  'companyName','Aster Retail LLC','isEmailVerified',1,'dateOfBirth','1990-01-10','createdAt','2026-02-20T10:00:00Z','lastModifiedAt','2026-03-01T10:00:00Z',
  'customerGroup','vip','stores',json_array('store-west'),
  'addresses',json_array(json_object('id','addr-1','country','west','city','San Francisco')),
  'defaultShippingAddressId','addr-1','defaultBillingAddressId','addr-1',
  'shippingAddressIds',json_array('addr-1'),'billingAddressIds',json_array('addr-1'),
  'custom',json_object('fields', json_object('vipTier','gold','segment','gold','supportHistory','priority','accountNotes','High value account')),
  'region','west'
)),
('cust-2', 'tenant-a', json_object(
  'id','cust-2','version',1,'customerNumber','CUST-1002','email','beacon@example.com','firstName','Beacon','lastName','Foods',
  'companyName','Beacon Foods','isEmailVerified',1,'dateOfBirth','1988-09-19','createdAt','2026-01-16T11:00:00Z','lastModifiedAt','2026-03-02T10:00:00Z',
  'customerGroup','standard','stores',json_array('store-east'),
  'addresses',json_array(json_object('id','addr-2','country','east','city','Boston')),
  'defaultShippingAddressId','addr-2','defaultBillingAddressId','addr-2',
  'shippingAddressIds',json_array('addr-2'),'billingAddressIds',json_array('addr-2'),
  'custom',json_object('fields', json_object('vipTier','silver','segment','silver','supportHistory','normal','accountNotes','')),
  'region','east'
)),
('cust-3', 'tenant-a', json_object(
  'id','cust-3','version',1,'customerNumber','CUST-1003','email','comet@example.com','firstName','Comet','lastName','Supply',
  'companyName','Comet Supply','isEmailVerified',0,'dateOfBirth','1992-05-03','createdAt','2026-03-01T09:30:00Z','lastModifiedAt','2026-03-02T12:10:00Z',
  'customerGroup','vip','stores',json_array('store-west'),
  'addresses',json_array(json_object('id','addr-3','country','west','city','Seattle')),
  'defaultShippingAddressId','addr-3','defaultBillingAddressId','addr-3',
  'shippingAddressIds',json_array('addr-3'),'billingAddressIds',json_array('addr-3'),
  'custom',json_object('fields', json_object('vipTier','platinum','segment','gold','supportHistory','priority','accountNotes','Escalation history')),
  'region','west'
));

INSERT INTO orders_docs (id, tenant_id, doc_json) VALUES
('ord-1', 'tenant-a', json_object(
  'id','ord-1','version',5,'orderNumber','ORD-1001','createdAt','2026-03-06T10:00:00Z','lastModifiedAt','2026-03-07T12:00:00Z','completedAt','2026-03-08T12:00:00Z',
  'orderState','Confirmed','shipmentState','Shipped','paymentState','Paid',
  'paymentInfo',json_object('paymentReference','PAY-00001001'),
  'customerId','cust-1','customerEmail','aster@example.com',
  'totalPrice',json_object('centAmount',125000,'currencyCode','USD','fractionDigits',2),
  'lineItems',json_array(json_object('id','li-1','productId','p-1','name','Widget A','variant',json_object('sku','SKU-1'),'quantity',1,'totalPrice',json_object('centAmount',125000,'currencyCode','USD'))),
  'shippingInfo',json_object('shippingMethodName','Ground','price',json_object('centAmount',1200,'currencyCode','USD'),'deliveries',json_array(json_object('id','del-1','parcels',json_array(json_object('id','par-1','trackingData',json_object('trackingId','TRK-1001','carrier','DHL','provider','DHL','providerTransaction','txn-1')))))),
  'returnInfo',json_array(json_object('eligible','true','status','NotRequested','refundStatus','NotInitiated')),
  'custom',json_object('fields',json_object('delayReason','','etaNote','On schedule','region','west')),
  'status','open','total_amount',1250.0
)),
('ord-2', 'tenant-a', json_object(
  'id','ord-2','version',3,'orderNumber','ORD-1002','createdAt','2026-03-05T10:00:00Z','lastModifiedAt','2026-03-07T12:00:00Z','completedAt','2026-03-09T12:00:00Z',
  'orderState','Open','shipmentState','Delayed','paymentState','Paid',
  'paymentInfo',json_object('paymentReference','PAY-00001002'),
  'customerId','cust-2','customerEmail','beacon@example.com',
  'totalPrice',json_object('centAmount',430000,'currencyCode','USD','fractionDigits',2),
  'lineItems',json_array(json_object('id','li-2','productId','p-2','name','Widget B','variant',json_object('sku','SKU-2'),'quantity',2,'totalPrice',json_object('centAmount',430000,'currencyCode','USD'))),
  'shippingInfo',json_object('shippingMethodName','Express','price',json_object('centAmount',2500,'currencyCode','USD'),'deliveries',json_array(json_object('id','del-2','parcels',json_array(json_object('id','par-2','trackingData',json_object('trackingId','TRK-1002','carrier','UPS','provider','UPS','providerTransaction','txn-2')))))),
  'returnInfo',json_array(json_object('eligible','false','status','NotEligible','refundStatus','NotInitiated')),
  'custom',json_object('fields',json_object('delayReason','Weather disruption','etaNote','Delayed by 2 days','region','east')),
  'status','open','total_amount',4300.0
)),
('ord-3', 'tenant-a', json_object(
  'id','ord-3','version',2,'orderNumber','ORD-1003','createdAt','2026-03-04T10:00:00Z','lastModifiedAt','2026-03-05T10:00:00Z','completedAt','2026-03-05T12:00:00Z',
  'orderState','Complete','shipmentState','Delivered','paymentState','Paid',
  'paymentInfo',json_object('paymentReference','PAY-00001003'),
  'customerId','cust-3','customerEmail','comet@example.com',
  'totalPrice',json_object('centAmount',21000,'currencyCode','USD','fractionDigits',2),
  'lineItems',json_array(json_object('id','li-3','productId','p-3','name','Widget C','variant',json_object('sku','SKU-3'),'quantity',1,'totalPrice',json_object('centAmount',21000,'currencyCode','USD'))),
  'shippingInfo',json_object('shippingMethodName','Ground','price',json_object('centAmount',800,'currencyCode','USD'),'deliveries',json_array(json_object('id','del-3','parcels',json_array(json_object('id','par-3','trackingData',json_object('trackingId','TRK-1003','carrier','FedEx','provider','FedEx','providerTransaction','txn-3')))))),
  'returnInfo',json_array(json_object('eligible','true','status','Requested','refundStatus','Pending')),
  'custom',json_object('fields',json_object('delayReason','','etaNote','Delivered','region','west')),
  'status','closed','total_amount',210.0
)),
('ord-4', 'tenant-a', json_object(
  'id','ord-4','version',1,'orderNumber','ORD-1004','createdAt','2026-03-03T10:00:00Z','lastModifiedAt','2026-03-03T13:00:00Z','completedAt',NULL,
  'orderState','Open','shipmentState','Ready','paymentState','Pending',
  'paymentInfo',json_object('paymentReference','PAY-00001004'),
  'customerId','cust-3','customerEmail','comet@example.com',
  'totalPrice',json_object('centAmount',99900,'currencyCode','USD','fractionDigits',2),
  'lineItems',json_array(json_object('id','li-4','productId','p-4','name','Widget D','variant',json_object('sku','SKU-4'),'quantity',3,'totalPrice',json_object('centAmount',99900,'currencyCode','USD'))),
  'shippingInfo',json_object('shippingMethodName','Ground','price',json_object('centAmount',1000,'currencyCode','USD'),'deliveries',json_array(json_object('id','del-4','parcels',json_array(json_object('id','par-4','trackingData',json_object('trackingId','TRK-1004','carrier','USPS','provider','USPS','providerTransaction','txn-4')))))),
  'returnInfo',json_array(json_object('eligible','true','status','NotRequested','refundStatus','NotInitiated')),
  'custom',json_object('fields',json_object('delayReason','','etaNote','Preparing for shipment','region','west')),
  'status','open','total_amount',999.0
));

-- Direct and role ACLs.
INSERT INTO acl_grants (id, tenant_id, subject_id, resource_type, resource_id, action) VALUES
('g1', 'tenant-a', 'user:alice', 'order', 'ord-1', 'view'),
('g2', 'tenant-a', 'user:alice', 'order', 'ord-4', 'view'),
('g3', 'tenant-a', 'user:alice', 'customer', 'cust-1', 'view'),
('g4', 'tenant-a', 'user:alice', 'customer', 'cust-3', 'view'),
('g5', 'tenant-a', 'role:manager', 'order', '*', 'view'),
('g6', 'tenant-a', 'role:manager', 'customer', '*', 'view');

-- ABAC allow by region attribute match.
INSERT INTO policy_rules (id, tenant_id, resource_type, action, effect, subject_attr, resource_attr, op, value) VALUES
('p1', 'tenant-a', 'order', 'view', 'allow', 'region', 'region', 'equals', '__MATCH_RESOURCE__'),
('p2', 'tenant-a', 'customer', 'view', 'allow', 'region', 'region', 'equals', '__MATCH_RESOURCE__');

-- ABAC deny for east region to demonstrate precedence.
INSERT INTO policy_rules (id, tenant_id, resource_type, action, effect, subject_attr, resource_attr, op, value) VALUES
('p3', 'tenant-a', 'order', 'view', 'deny', 'region', 'region', 'equals', 'east');
