from __future__ import annotations

import re
from typing import Any

from fastapi import FastAPI
from pydantic import BaseModel


app = FastAPI(title="Superlinked-Compatible Adapter", version="0.1.0")


class AnalyzeRequest(BaseModel):
    tenantId: str | None = None
    message: str
    contractVersion: str | None = "v2"
    resourceHint: str | None = None
    intentCategory: str | None = None
    topK: int | None = 100


def candidate_ids(message: str, top_k: int) -> list[str]:
    out: list[str] = []
    for match in re.findall(r"\b(ord-\d{5}|ORD-\d{6}|cust-\d{5}|CUST-\d{6})\b", message, flags=re.IGNORECASE):
        normalized = match.lower()
        if normalized.startswith("ord-") and len(normalized) == 10:
            out.append(normalized)
        elif normalized.startswith("ord-") and len(normalized) == 11:
            out.append(f"ord-{normalized[-5:]}")
        elif normalized.startswith("cust-") and len(normalized) == 11:
            out.append(f"cust-{normalized[-5:]}")
        elif normalized.startswith("cust-") and len(normalized) == 10:
            out.append(normalized)

    if not out:
        if "customer" in message.lower():
            out = [f"cust-{i:05d}" for i in range(1, min(top_k, 100) + 1)]
        else:
            out = [f"ord-{i:05d}" for i in range(1, min(top_k, 100) + 1)]

    deduped: list[str] = []
    seen: set[str] = set()
    for cid in out:
        if cid in seen:
            continue
        seen.add(cid)
        deduped.append(cid)
        if len(deduped) >= top_k:
            break
    return deduped


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/analyze")
def analyze(req: AnalyzeRequest) -> dict[str, Any]:
    top_k = req.topK or 100
    if top_k <= 0:
        top_k = 100
    ids = candidate_ids(req.message, min(top_k, 300))
    scores = [max(0.2, 1.0 - (i * 0.01)) for i in range(len(ids))]
    return {
        "candidateIds": ids,
        "scores": scores,
        "providerConfidence": 0.82,
        "safeEvidence": [
            "superlinked_adapter",
            f"tenant:{req.tenantId or 'unknown'}",
            f"intent:{req.intentCategory or 'unknown'}",
        ],
        "providerLatencyMs": 12,
        "modelVersion": "adapter-v1",
        "indexVersion": "local-seeded-v1",
    }
