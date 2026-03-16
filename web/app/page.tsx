"use client";

import { useEffect, useMemo, useState } from "react";

type RedactedPlaceholder = { resourceId: string; type: string; reasonCode: string; requestAccessToken: string };
type GroupedMatch = {
  resourceType: string;
  matchField: string;
  authorizedCount: number;
  hiddenCount: number;
  items: Record<string, unknown>[];
  redactedPlaceholders: RedactedPlaceholder[];
};
type DebugFlowStep = { stage: string; status: string };
type DebugRewrite = {
  originalMessage?: string;
  rewrittenMessage?: string;
  normalizedInput?: string;
  normalizationApplied?: string[];
  extractedSlots?: Record<string, unknown>;
  preSemanticQuery?: Record<string, unknown>;
  postSemanticQuery?: Record<string, unknown>;
  generatedQuery?: Record<string, unknown>;
  intent?: string;
  intentCategory?: string;
  intentSubcategory?: string;
  resourceType?: string;
};
type DebugInfo = {
  traceId?: string;
  flow?: DebugFlowStep[];
  rewrite?: DebugRewrite;
  filterSource?: string[];
  resolutionMode?: string;
  identifierDetection?: unknown[];
  groupingSummary?: Record<string, unknown>;
};

type SearchResponse = {
  items: Record<string, unknown>[];
  authorizedCount: number;
  hiddenCount: number;
  redactedPlaceholders: RedactedPlaceholder[];
  visibilityMode: string;
  contractVersion: string;
  latencyMs: number;
  nextCursor?: string;
  traceId?: string;
  debug?: DebugInfo;
};

type SeedStats = {
  tenantId: string;
  orders: number;
  customers: number;
  aclGrants: number;
  abacRules: number;
};

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? "http://localhost:8080";

const RESULT_REASON_LABELS: Record<string, string> = {
  VISIBLE_RESULTS: "Visible Results",
  MATCHES_EXIST_BUT_NOT_VISIBLE: "Matches Hidden by Permissions",
  NO_MATCH_IN_TENANT: "No Match in Tenant",
  NO_VISIBLE_RESULTS_FOR_CURRENT_SCOPE: "No Visible Results in Scope",
  CLARIFICATION_REQUIRED: "Clarification Required",
  NO_OP_SHORT_QUERY: "Query Too Short",
  UNSUPPORTED_DOMAIN: "Unsupported Domain"
};

const PERSONAS = [
  { key: "alice-west", label: "Alice (tenant-a, sales_rep, west)", userId: "alice", tenantId: "tenant-a", roles: "sales_rep", region: "west" },
  { key: "bob-east", label: "Bob (tenant-a, sales_rep, east)", userId: "bob", tenantId: "tenant-a", roles: "sales_rep", region: "east" },
  { key: "carol-north", label: "Carol (tenant-a, sales_rep, north)", userId: "carol", tenantId: "tenant-a", roles: "sales_rep", region: "north" },
  { key: "manager-a", label: "Manager (tenant-a)", userId: "mgr-a", tenantId: "tenant-a", roles: "manager", region: "west" },
  { key: "manager-b", label: "Manager (tenant-b)", userId: "mgr-b", tenantId: "tenant-b", roles: "manager", region: "west" }
] as const;

function asDebugInfo(value: unknown): DebugInfo | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const obj = value as Record<string, unknown>;
  return {
    traceId: typeof obj.traceId === "string" ? obj.traceId : undefined,
    flow: Array.isArray(obj.flow)
      ? obj.flow
          .filter((v): v is Record<string, unknown> => Boolean(v) && typeof v === "object")
          .map((v) => ({
            stage: typeof v.stage === "string" ? v.stage : "unknown",
            status: typeof v.status === "string" ? v.status : "unknown"
          }))
      : undefined,
    rewrite: obj.rewrite && typeof obj.rewrite === "object" ? (obj.rewrite as DebugRewrite) : undefined,
    filterSource: Array.isArray(obj.filterSource) ? obj.filterSource.filter((v): v is string => typeof v === "string") : undefined
    ,
    resolutionMode: typeof obj.resolutionMode === "string" ? obj.resolutionMode : undefined,
    identifierDetection: Array.isArray(obj.identifierDetection) ? obj.identifierDetection : undefined,
    groupingSummary: obj.groupingSummary && typeof obj.groupingSummary === "object" ? (obj.groupingSummary as Record<string, unknown>) : undefined
  };
}

function asArrayString(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((v): v is string => typeof v === "string");
}

function timelineClassForStatus(status: string): string {
  if (status === "ok") {
    return "ok";
  }
  if (status === "warning") {
    return "warn";
  }
  if (status === "error") {
    return "err";
  }
  return "unk";
}

function formatStage(stage: string): string {
  const labels: Record<string, string> = {
    ingress: "Ingress",
    auth_subject_built: "Auth Subject Built",
    rewrite_completed: "Rewrite Completed",
    superlinked_gateway_called: "Superlinked Gateway Called",
    superlinked_serving_gate: "Superlinked Serving Gate",
    semantic_refinement_completed: "Semantic Refinement Completed",
    clarification_required: "Clarification Required",
    datastore_search: "Datastore Search",
    policy_filter: "Policy Filter",
    response_composed: "Response Composed"
  };
  if (labels[stage]) {
    return labels[stage];
  }
  if (stage.startsWith("semantic_fallback_reason:")) {
    return `Semantic Fallback (${stage.replace("semantic_fallback_reason:", "")})`;
  }
  return stage;
}

function flowOutcome(flow?: DebugFlowStep[]): string {
  if (!flow || flow.length === 0) {
    return "unknown";
  }
  const fallback = flow.find((s) => s.stage.startsWith("semantic_fallback_reason:"));
  if (fallback) {
    return formatStage(fallback.stage);
  }
  const served = flow.find((s) => s.stage === "superlinked_serving_gate" && s.status === "ok");
  if (served) {
    return "Superlinked Served";
  }
  return "Deterministic/SLM Path";
}

function outcomeClassName(outcome: string): string {
  const lower = outcome.toLowerCase();
  if (lower.includes("served")) {
    return "outcome-badge served";
  }
  if (lower.includes("fallback")) {
    return "outcome-badge fallback";
  }
  return "outcome-badge default";
}

function resultReasonClass(reason: string): string {
  if (reason === "VISIBLE_RESULTS") {
    return "reason-badge ok";
  }
  if (reason === "MATCHES_EXIST_BUT_NOT_VISIBLE" || reason === "NO_VISIBLE_RESULTS_FOR_CURRENT_SCOPE") {
    return "reason-badge warn";
  }
  if (reason === "CLARIFICATION_REQUIRED" || reason === "NO_OP_SHORT_QUERY" || reason === "UNSUPPORTED_DOMAIN" || reason === "NO_MATCH_IN_TENANT") {
    return "reason-badge neutral";
  }
  return "reason-badge neutral";
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

function getPath(item: Record<string, unknown>, path: string[]): unknown {
  let current: unknown = item;
  for (const segment of path) {
    const rec = asRecord(current);
    if (!rec) {
      return undefined;
    }
    current = rec[segment];
  }
  return current;
}

function orderTrackingID(item: Record<string, unknown>): string {
  const fromTop = asString(item.trackingId ?? item.tracking_id);
  if (fromTop) {
    return fromTop;
  }
  const shippingInfo = asRecord(item.shippingInfo);
  if (!shippingInfo || !Array.isArray(shippingInfo.deliveries) || shippingInfo.deliveries.length === 0) {
    return "";
  }
  const delivery = asRecord(shippingInfo.deliveries[0]);
  if (!delivery || !Array.isArray(delivery.parcels) || delivery.parcels.length === 0) {
    return "";
  }
  const parcel = asRecord(delivery.parcels[0]);
  const trackingData = parcel ? asRecord(parcel.trackingData) : null;
  return asString(trackingData?.trackingId);
}

function orderTotal(item: Record<string, unknown>): string {
  const totalPrice = asRecord(item.totalPrice);
  if (!totalPrice) {
    return "";
  }
  const cents = Number(totalPrice.centAmount);
  const currency = asString(totalPrice.currencyCode);
  if (!Number.isFinite(cents) || !currency) {
    return "";
  }
  return `${currency} ${(cents / 100).toFixed(2)}`;
}

function inferResourceFromItem(item: Record<string, unknown>): "order" | "customer" {
  if ("orderNumber" in item || "order_state" in item || "shipment_state" in item) {
    return "order";
  }
  if ("customerNumber" in item || "isEmailVerified" in item || "customer_group" in item) {
    return "customer";
  }
  return "order";
}

function renderResourceRow(item: Record<string, unknown>, resourceType: "order" | "customer", idx: number) {
  if (resourceType === "order") {
    return (
      <div key={`item-${idx}`} className="card">
        <div className="kv-grid">
          <p><strong>ID</strong><br /><code>{asString(item.id)}</code></p>
          <p><strong>Order #</strong><br />{asString(item.orderNumber) || asString(item.order_number)}</p>
          <p><strong>Customer</strong><br />{asString(item.customerEmail) || asString(item.customer_email) || asString(item.customerId) || asString(item.customer_id)}</p>
          <p><strong>State</strong><br />{asString(item.orderState) || asString(item.order_state)}</p>
          <p><strong>Shipment</strong><br />{asString(item.shipmentState) || asString(item.shipment_state)}</p>
          <p><strong>Payment</strong><br />{asString(item.paymentState) || asString(item.payment_state)}</p>
          <p><strong>Total</strong><br />{orderTotal(item) || asString(item.total_amount)}</p>
          <p><strong>Tracking</strong><br />{orderTrackingID(item) || "n/a"}</p>
          <p><strong>Created</strong><br />{asString(item.createdAt) || asString(item.created_at)}</p>
        </div>
        <details>
          <summary>View JSON</summary>
          <pre>{JSON.stringify(item, null, 2)}</pre>
        </details>
      </div>
    );
  }

  return (
    <div key={`item-${idx}`} className="card">
      <div className="kv-grid">
        <p><strong>ID</strong><br /><code>{asString(item.id)}</code></p>
        <p><strong>Customer #</strong><br />{asString(item.customerNumber) || asString(item.customer_number)}</p>
        <p><strong>Name</strong><br />{[asString(item.firstName), asString(item.lastName)].filter(Boolean).join(" ") || asString(item.name)}</p>
        <p><strong>Email</strong><br />{asString(item.email)}</p>
        <p><strong>Group</strong><br />{asString(item.customerGroup) || asString(item.customer_group)}</p>
        <p><strong>VIP Tier</strong><br />{asString(item.vip_tier) || asString(getPath(item, ["custom", "fields", "vipTier"])) || "n/a"}</p>
        <p><strong>Email Verified</strong><br />{asString(item.isEmailVerified) || asString(item.is_email_verified)}</p>
        <p><strong>Created</strong><br />{asString(item.createdAt) || asString(item.created_at)}</p>
      </div>
      <details>
        <summary>View JSON</summary>
        <pre>{JSON.stringify(item, null, 2)}</pre>
      </details>
    </div>
  );
}

export default function Page() {
  const [persona, setPersona] = useState<(typeof PERSONAS)[number]["key"]>("alice-west");
  const [userId, setUserId] = useState("alice");
  const [tenantId, setTenantId] = useState("tenant-a");
  const [roles, setRoles] = useState("sales_rep");
  const [region, setRegion] = useState("west");

  const [resource, setResource] = useState<"order" | "customer">("order");
  const [message, setMessage] = useState("show open orders this week");
  const chatProvider = "slm-superlinked";
  const [debugMode, setDebugMode] = useState<boolean>(true);
  const [result, setResult] = useState<SearchResponse | null>(null);
  const [queryResult, setQueryResult] = useState<Record<string, unknown> | null>(null);
  const [lastTraceId, setLastTraceId] = useState<string>("");
  const [lastDebug, setLastDebug] = useState<DebugInfo | null>(null);
  const [seedStats, setSeedStats] = useState<SeedStats | null>(null);
  const [seedStatsError, setSeedStatsError] = useState<string>("");
  const [queryLoading, setQueryLoading] = useState<boolean>(false);
  const [error, setError] = useState<string>("");

  const headers = useMemo(
    () => ({
      "Content-Type": "application/json",
      "X-User-Id": userId,
      "X-Tenant-Id": tenantId,
      "X-Roles": roles,
      "X-User-Attrs": JSON.stringify({ region })
    }),
    [userId, tenantId, roles, region]
  );

  function applyPersona(key: (typeof PERSONAS)[number]["key"]) {
    const selected = PERSONAS.find((p) => p.key === key);
    if (!selected) {
      return;
    }
    setPersona(selected.key);
    setUserId(selected.userId);
    setTenantId(selected.tenantId);
    setRoles(selected.roles);
    setRegion(selected.region);
  }

  function onResourceChange(next: "order" | "customer") {
    setResource(next);
    setResult(null);
    setError("");
  }

  useEffect(() => {
    async function loadSeedStats() {
      setSeedStatsError("");
      try {
        const res = await fetch(`${API_BASE}/api/admin/seed-stats?tenantId=${encodeURIComponent(tenantId)}`, {
          headers
        });
        const payload = await res.json();
        if (!res.ok) {
          setSeedStatsError(payload.error ?? "seed stats failed");
          return;
        }
        setSeedStats(payload);
      } catch (err) {
        setSeedStatsError(err instanceof Error ? err.message : "seed stats failed");
      }
    }
    loadSeedStats();
  }, [tenantId, headers]);

  async function runStructuredSearch() {
    setError("");
    setQueryResult(null);
    const body = {
      contractVersion: "v2",
      intentCategory: resource === "order" ? "wismo" : "crm_profile",
      filters:
        resource === "order"
          ? [{ field: "order_state", op: "in", value: ["Open", "Confirmed"] }]
          : [{ field: "customer_group", op: "in", value: ["vip", "business"] }],
      sort: { field: "created_at", dir: "desc" },
      page: { limit: 20 },
      debug: debugMode
    };
    const res = await fetch(`${API_BASE}/api/search/${resource === "order" ? "orders" : "customers"}`, {
      method: "POST",
      headers,
      body: JSON.stringify(body)
    });
    const payload = await res.json();
    if (!res.ok) {
      setError(payload.error ?? "search failed");
      return;
    }
    const traceId = (payload.traceId as string | undefined) ?? res.headers.get("X-Trace-Id") ?? "";
    setLastTraceId(traceId);
    setLastDebug(asDebugInfo(payload.debug));
    setResult(payload);
  }

  async function runChat() {
    setError("");
    setResult(null);
    setQueryLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/query/interpret`, {
        method: "POST",
        headers,
        body: JSON.stringify({ message, provider: chatProvider, contractVersion: "v2", debug: debugMode })
      });
      const payload = await res.json();
      if (!res.ok) {
        setError(payload.error ?? "query failed");
        return;
      }
      const traceId = (payload.traceId as string | undefined) ?? res.headers.get("X-Trace-Id") ?? "";
      setLastTraceId(traceId);
      setLastDebug(asDebugInfo(payload.debug));
      setQueryResult(payload);
    } catch (err) {
      setError(err instanceof Error ? err.message : "query failed");
    } finally {
      setQueryLoading(false);
    }
  }

  const queryFilterSource = asArrayString(queryResult?.debug && typeof queryResult.debug === "object" ? (queryResult.debug as Record<string, unknown>).filterSource : undefined);
  const queryItems = Array.isArray(queryResult?.items)
    ? (queryResult?.items as unknown[]).filter((i): i is Record<string, unknown> => Boolean(i) && typeof i === "object" && !Array.isArray(i))
    : [];
  const queryHiddenCount =
    queryResult && typeof queryResult.hiddenCount === "number"
      ? queryResult.hiddenCount
      : queryResult && typeof queryResult.hiddenCount === "string"
        ? Number(queryResult.hiddenCount)
        : 0;
  const queryResourceType: "order" | "customer" = queryItems.length > 0 ? inferResourceFromItem(queryItems[0]) : "order";
  const queryLatencyMs =
    queryResult && typeof queryResult.latencyMs === "number"
      ? queryResult.latencyMs
      : queryResult && typeof queryResult.latencyMs === "string"
        ? Number(queryResult.latencyMs)
        : null;
  const queryResolutionMode = queryResult && typeof queryResult.resolutionMode === "string" ? queryResult.resolutionMode : "intent_semantic_path";
  const queryVisibilityNotice = queryResult && typeof queryResult.visibilityNotice === "string" ? queryResult.visibilityNotice : "";
  const queryResultReasonCode = queryResult && typeof queryResult.resultReasonCode === "string" ? queryResult.resultReasonCode : "";
  const querySuggestedNextActions = queryResult && Array.isArray(queryResult.suggestedNextActions)
    ? (queryResult.suggestedNextActions as unknown[]).filter((v): v is string => typeof v === "string")
    : [];
  const groupedMatches: GroupedMatch[] = Array.isArray(queryResult?.groupedMatches)
    ? (queryResult?.groupedMatches as unknown[])
        .filter((g): g is Record<string, unknown> => Boolean(g) && typeof g === "object" && !Array.isArray(g))
        .map((g) => ({
          resourceType: typeof g.resourceType === "string" ? g.resourceType : "order",
          matchField: typeof g.matchField === "string" ? g.matchField : "id",
          authorizedCount: typeof g.authorizedCount === "number" ? g.authorizedCount : 0,
          hiddenCount: typeof g.hiddenCount === "number" ? g.hiddenCount : 0,
          items: Array.isArray(g.items) ? (g.items as unknown[]).filter((i): i is Record<string, unknown> => Boolean(i) && typeof i === "object" && !Array.isArray(i)) : [],
          redactedPlaceholders: Array.isArray(g.redactedPlaceholders)
            ? (g.redactedPlaceholders as unknown[]).filter((p): p is RedactedPlaceholder => Boolean(p) && typeof p === "object" && !Array.isArray(p))
            : []
        }))
    : [];

  return (
    <main>
      <h1>Permission-Aware Search Demo</h1>
      <p>Auth headers emulate SuperTokens session subject. Unauthorized matches are redacted but counted.</p>

      <section>
        <h2>Identity Context</h2>
        <div className="row">
          <label>
            Persona
            <select value={persona} onChange={(e) => applyPersona(e.target.value as (typeof PERSONAS)[number]["key"])}>
              {PERSONAS.map((p) => (
                <option key={p.key} value={p.key}>
                  {p.label}
                </option>
              ))}
            </select>
          </label>
          <label style={{ display: "inline-flex", alignItems: "center", gap: 8, marginLeft: 16 }}>
            <input type="checkbox" checked={debugMode} onChange={(e) => setDebugMode(e.target.checked)} />
            Debug mode
          </label>
        </div>
        <div className="row">
          <input value={userId} onChange={(e) => setUserId(e.target.value)} placeholder="User ID" />
          <input value={tenantId} onChange={(e) => setTenantId(e.target.value)} placeholder="Tenant" />
          <input value={roles} onChange={(e) => setRoles(e.target.value)} placeholder="Roles (comma separated)" />
          <input value={region} onChange={(e) => setRegion(e.target.value)} placeholder="Region" />
        </div>
        {seedStats && (
          <p>
            Live tenant dataset: <code>{seedStats.tenantId}</code> | customers: <code>{seedStats.customers}</code> | orders:{" "}
            <code>{seedStats.orders}</code> | ACL grants: <code>{seedStats.aclGrants}</code> | ABAC rules: <code>{seedStats.abacRules}</code>
          </p>
        )}
        {seedStatsError && <p className="err">{seedStatsError}</p>}
      </section>

          <section>
            <h2>Structured Search</h2>
            <div className="row">
              <select value={resource} onChange={(e) => onResourceChange(e.target.value as "order" | "customer")}>
                <option value="order">Orders</option>
                <option value="customer">Customers</option>
              </select>
              <button type="button" onClick={runStructuredSearch}>Run Search</button>
            </div>
            {result && (
              <div>
                <p>
                  <strong>Authorized:</strong> {result.authorizedCount} | <strong>Hidden:</strong> {result.hiddenCount} | <strong>Mode:</strong>{" "}
                  <code>{result.visibilityMode}</code> | <strong>Latency:</strong> {result.latencyMs}ms
                </p>
                <div className="list">
                  {result.items.map((item, idx) => renderResourceRow(item, resource, idx))}
                  {result.redactedPlaceholders.map((p, idx) => (
                    <div key={`red-${idx}`} className="card redacted">
                      <p>Hidden {p.type} result</p>
                      <p>
                        Visible ID: <code>{p.resourceId}</code>
                      </p>
                      <p>Reason: {p.reasonCode}</p>
                      <p>
                        Request token: <code>{p.requestAccessToken.slice(0, 26)}...</code>
                      </p>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </section>

      <section>
            <h2>Natural Language Query</h2>
            <div className="row">
              <p>
                Query Provider: <code>{chatProvider}</code> (recommended)
              </p>
            </div>
            <p>
              Resolution mode: <code>{queryResolutionMode}</code>
            </p>
            {queryResultReasonCode && (
              <p>
                Result reason:{" "}
                <span className={resultReasonClass(queryResultReasonCode)}>
                  {RESULT_REASON_LABELS[queryResultReasonCode] ?? queryResultReasonCode}
                </span>
              </p>
            )}
            {queryResult && typeof queryResult.queryShape === "string" && (
              <p>
                Query shape: <code>{queryResult.queryShape as string}</code>
                {typeof queryResult.pathTaken === "string" && (
                  <>
                    {" "}
                    | Path taken: <code>{queryResult.pathTaken as string}</code>
                  </>
                )}
              </p>
            )}
            <div className="row">
              <textarea value={message} onChange={(e) => setMessage(e.target.value)} rows={3} style={{ width: "100%" }} />
              <button type="button" onClick={runChat} disabled={queryLoading}>
                {queryLoading ? "Running..." : "Run Query"}
              </button>
            </div>
            {queryItems.length > 0 && (
              <div className="list">
                {queryItems.map((item, idx) => renderResourceRow(item, queryResourceType, idx))}
              </div>
            )}
            {groupedMatches.length > 0 && (
              <div className="list">
                {groupedMatches.map((group, gIdx) => (
                  <div className="card" key={`group-${gIdx}`}>
                    <p>
                      <strong>{group.resourceType === "customer" ? "Customers" : "Orders"} matched by {group.matchField}</strong>
                    </p>
                    <p>
                      Authorized: <strong>{group.authorizedCount}</strong> | Hidden: <strong>{group.hiddenCount}</strong>
                    </p>
                    <div className="list">
                      {group.items.map((item, idx) =>
                        renderResourceRow(item, group.resourceType === "customer" ? "customer" : "order", idx)
                      )}
                      {group.redactedPlaceholders.map((p, idx) => (
                        <div key={`grp-red-${gIdx}-${idx}`} className="card redacted">
                          <p>Hidden {p.type} result</p>
                          <p>Visible ID: <code>{p.resourceId}</code></p>
                          <p>Reason: {p.reasonCode}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {queryResult && queryItems.length === 0 && !queryLoading && (
              <div className="card redacted">
                <p>{queryVisibilityNotice || "No visible results for this query."}</p>
                {queryResultReasonCode && (
                  <p>
                    Reason code: <code>{queryResultReasonCode}</code>
                  </p>
                )}
                <p>
                  Hidden matches due to permissions: <strong>{Number.isFinite(queryHiddenCount) ? queryHiddenCount : 0}</strong>
                </p>
                {querySuggestedNextActions.length > 0 && (
                  <div>
                    <p><strong>Suggested next actions:</strong></p>
                    <ul>
                      {querySuggestedNextActions.map((a, idx) => (
                        <li key={`next-action-${idx}`}>{a}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}
            {queryResult && (
              <details>
                <summary>View Query JSON</summary>
                <pre>{JSON.stringify(queryResult, null, 2)}</pre>
              </details>
            )}
            <details>
              <summary>Result Reason Legend</summary>
              <div className="reason-legend">
                {Object.entries(RESULT_REASON_LABELS).map(([code, label]) => (
                  <p key={code}>
                    <span className={resultReasonClass(code)}>{label}</span>
                    {" "}
                    <code>{code}</code>
                  </p>
                ))}
              </div>
            </details>
            {queryFilterSource.length > 0 && (
              <p>
                Filter source: <code>{queryFilterSource.join(", ")}</code>
              </p>
            )}
            {queryLatencyMs !== null && Number.isFinite(queryLatencyMs) && (
              <p>
                Latency: <strong>{queryLatencyMs}ms</strong>
              </p>
            )}
      </section>

      {error && <p className="err">{error}</p>}

      {debugMode && (
        <section>
              <h2>Debug Panel</h2>
              <p>
                Trace ID: <code>{lastTraceId || "n/a"}</code>
              </p>
              {queryResult && typeof queryResult.semanticProvider === "string" && (
                <p>
                  Semantic provider: <code>{queryResult.semanticProvider}</code>
                </p>
              )}
              {queryResult && Array.isArray(queryResult.semanticNotes) && (
                <p>
                  Semantic notes: <code>{(queryResult.semanticNotes as unknown[]).filter((n): n is string => typeof n === "string").join(", ") || "n/a"}</code>
                </p>
              )}
              {lastDebug?.rewrite?.intentSubcategory && (
                <p>
                  Intent subcategory: <code>{lastDebug.rewrite.intentSubcategory}</code>
                </p>
              )}
              {lastDebug?.resolutionMode && (
                <p>
                  Debug resolution mode: <code>{lastDebug.resolutionMode}</code>
                </p>
              )}
              {lastDebug?.identifierDetection && lastDebug.identifierDetection.length > 0 && (
                <p>
                  Identifier detection: <code>{JSON.stringify(lastDebug.identifierDetection)}</code>
                </p>
              )}
              {lastDebug?.filterSource && lastDebug.filterSource.length > 0 && (
                <p>
                  Filter source: <code>{lastDebug.filterSource.join(", ")}</code>
                </p>
              )}
              {lastDebug?.flow && lastDebug.flow.length > 0 && (
                <div>
                  <p>
                    Trace Flow Outcome:{" "}
                    <span className={outcomeClassName(flowOutcome(lastDebug.flow))}>
                      {flowOutcome(lastDebug.flow)}
                    </span>
                  </p>
                  <p>Flow</p>
                  <div className="trace-timeline">
                    {lastDebug.flow.map((step, idx) => (
                      <div key={`flow-${idx}`} className="trace-step">
                        <div className={`trace-dot ${timelineClassForStatus(step.status)}`} />
                        <div className="trace-step-body">
                          <p className="trace-stage">
                            <code>{formatStage(step.stage)}</code>
                          </p>
                          <p className="trace-status">{step.status}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {lastDebug?.rewrite && (
                <div>
                  {lastDebug.rewrite.normalizedInput && (
                    <p>
                      Normalized input: <code>{lastDebug.rewrite.normalizedInput}</code>
                    </p>
                  )}
                  {Array.isArray(lastDebug.rewrite.normalizationApplied) && lastDebug.rewrite.normalizationApplied.length > 0 && (
                    <p>
                      Normalization applied: <code>{lastDebug.rewrite.normalizationApplied.join(", ")}</code>
                    </p>
                  )}
                  {lastDebug.rewrite.extractedSlots && (
                    <div>
                      <p>Extracted slots</p>
                      <pre>{JSON.stringify(lastDebug.rewrite.extractedSlots, null, 2)}</pre>
                    </div>
                  )}
                  {lastDebug.rewrite.extractedSlots && (
                    <div>
                      <p>Slot cues</p>
                      <pre>
                        {JSON.stringify(
                          {
                            matchedCues:
                              Array.isArray(lastDebug.rewrite.extractedSlots.matchedCues)
                                ? lastDebug.rewrite.extractedSlots.matchedCues
                                : [],
                            slotConfidence:
                              lastDebug.rewrite.extractedSlots.slotConfidence &&
                              typeof lastDebug.rewrite.extractedSlots.slotConfidence === "object"
                                ? lastDebug.rewrite.extractedSlots.slotConfidence
                                : {}
                          },
                          null,
                          2
                        )}
                      </pre>
                    </div>
                  )}
                  <p>Rewrite</p>
                  <pre>{JSON.stringify(lastDebug.rewrite, null, 2)}</pre>
                </div>
              )}
        </section>
      )}
    </main>
  );
}
