## Phase 1: Move prompt_payload Off-Chain (Executor-first Fetch, Transfer Fallback)

### 1) Goal and Scope

- **Move off-chain only**: Store the full `prompt_payload` off-chain.
- **Redundant storage**: Save at both the transfer node and the executor node.
- **Fetch API with fallback**: Validator fetches from executor first; on failure, fetches from transfer.
- **On-chain fields**: Keep `prompt_hash` and keep `original_prompt` (for signature preimage) in Phase 1.
- **Remove from transactions only**: Remove `prompt_payload` from `MsgStartInference`; keep it in on-chain `Inference` as a legacy/fallback field (will be empty for new inferences).

### 2) On-Chain Changes (proto and state)

- Keep on-chain:
  - `prompt_hash` (string, SHA-256 of canonicalized JSON)
  - `original_prompt` (string, raw body) — retained for signature checks
  - `prompt_payload` in `Inference` (read-only legacy/fallback; empty for new inferences)
- Remove from transactions:
  - `prompt_payload` from `MsgStartInference` (no longer sent)

Implementation notes:
- Update `inference-chain/proto/inference/inference/tx.proto` to remove `prompt_payload` from `MsgStartInference`; leave `Inference` unchanged.
- Regenerate protobufs and clean references accordingly.

Code references (proto/types):
- `inference-chain/proto/inference/inference/tx.proto`
- Generated: `inference-chain/x/inference/types/tx.pb.go`, `inference-chain/api/inference/inference/tx.pulsar.go`
- `inference-chain/x/inference/types/message_start_inference.go` (drop Start validation that requires prompt_payload)
- `inference-chain/x/inference/calculations/inference_state.go` (ensure no PromptPayload writes from Start)

### 3) Off-Chain Data Model (Local DB on nodes)

- Use the same embedded SQLite database already used by the decentralized API (no new DB service). Reuse the existing apiconfig SqliteDb instance and extend its schema with a new table. The prompt payloads for both transfer and executor will live in this same DB file.

Create a new table on both transfer and executor nodes:

```sql
CREATE TABLE inference_prompt_payloads (
  inference_id TEXT PRIMARY KEY,
  prompt_payload TEXT NOT NULL,
  prompt_hash TEXT NOT NULL, -- SHA-256 of canonicalized prompt JSON
  model TEXT,
  request_timestamp INTEGER,
  stored_by TEXT NOT NULL CHECK (stored_by IN ('transfer','executor')),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

- **Invariant**: stored `prompt_hash` must equal on-chain `prompt_hash` once Start is visible on-chain.

Code references (DB):
- `decentralized-api/apiconfig/sqlite_store.go` (extend `EnsureSchema`)
- New: `decentralized-api/internal/storage/inference_payloads.go` (CRUD helpers)

Retention and cleanup
- Prune rows from `inference_prompt_payloads` using the same retention/pruning window as the chain’s inference data pruning (same epochs/blocks). Run periodic cleanup alongside existing pruning routines.

### 4) Write Path Changes

- **Transfer node (Start)**:
  - Canonicalize request body and compute `prompt_hash` as today.
  - Persist `prompt_payload` and `prompt_hash` locally with `stored_by='transfer'`.
  - Submit `MsgStartInference` without `prompt_payload` (includes `prompt_hash`, `original_prompt`).

- **Executor node (Execute/Finish)**:
  - Upon receiving the executor request, persist `prompt_payload` locally with `stored_by='executor'`.
  - If Start not yet visible, mark as tentative and verify stored `prompt_hash` equals on-chain `prompt_hash` asynchronously when Start arrives.
  - Submit `MsgFinishInference` unchanged in Phase 1.

Code references (write path):
- `decentralized-api/internal/server/public/post_chat_handler.go` (remove Start PromptPayload field; add persistence calls)

### 5) Retrieval and Validation Flow (Validator)

0. If `Inference.prompt_payload` is present on-chain, use it directly for replay.
1. Otherwise, query chain for `inference_id`, `prompt_hash`.
2. Attempt fetch from executor node:
   - GET prompt payload by `inference_id`.
   - Canonicalize and verify SHA-256 equals on-chain `prompt_hash`.
3. If executor fetch fails or hash mismatches, fetch from transfer node and verify similarly.
4. If both off-chain fetches fail, submit an INVALID validation immediately (value = 0) noting payload-unavailable as the reason. During voting, other validators re-attempt fetching via the same API flow (executor → transfer) before casting their votes.

Code references (validator):
- `decentralized-api/internal/validation/inference_validation.go` (retrieval order; produce InvalidInferenceResult on double-failure; reporting path)

### 6) Node APIs (/v1/inferences/...)

- **Executor node**
  - `GET /v1/inferences/{inference_id}/prompt`
  - Headers (auth):
    - `X-Validator-Address`: bech32 address of requesting validator/participant
    - `Authorization`: base64 signature over `{payload=inference_id, timestamp=X-Timestamp, transfer_address=X-Validator-Address}` using Developer signature type
    - `X-Timestamp`: unix-nano timestamp for replay protection
  - 200: raw JSON body of the prompt payload; headers include `X-Prompt-Hash`.
  - Errors: 404 (not found), 409 (hash mismatch), 423 (not yet verified against chain), 500.
  - AuthZ: verify `Authorization` against participant pubkey from chain state; apply rate limits per `X-Validator-Address`.

- **Transfer node**
  - `GET /v1/inferences/{inference_id}/prompt`
  - Same headers, status codes, and verification as executor for fallback.

Implementation notes:
- Content-Type: `application/json`; `ETag` set to `prompt_hash`.
- Signature verification can reuse `calculations.ValidateSignature` with Developer type and components `{Payload: inference_id, Timestamp: X-Timestamp, TransferAddress: X-Validator-Address}`.

Code references (API):
- New GET handler under `decentralized-api/internal/server/public/` (e.g., `get_prompt_handler.go`) and route registration in the public server setup
- `decentralized-api/utils/api_headers.go` (add `X-Validator-Address`; reuse `Authorization`, `X-Timestamp`)

### 7) Integrity, Privacy, Availability

- **Integrity**: Always canonicalize fetched JSON and require `SHA-256(canonical)` equals on-chain `prompt_hash`.
- **Privacy**: Restrict endpoints to authenticated validators/network nodes; support at-rest encryption if desired.
- **Availability**: Redundant storage (transfer + executor); future replication optional.

### 8) Backward Compatibility and Rollout

1. Ship builds that persist prompt payload locally at transfer/executor and expose `/v1/inferences/{id}/prompt`.
2. Update proto to remove `prompt_payload` from `MsgStartInference`; regenerate code.
3. Decentralized API: stop attaching `prompt_payload` to Start; validator logic: prefer on-chain `Inference.prompt_payload` if present; else fetch off-chain (executor → transfer) and verify `prompt_hash`.

Legacy handling:
- Validators may continue using on-chain `prompt_payload` when present; off-chain fetch is used only when absent.

### 9) Components to Update (Phase 1)

- Proto/state: remove `prompt_payload` from `MsgStartInference` only; leave `Inference` unchanged.
- Decentralized API (transfer): persist payload; remove attaching it to Start.
- Decentralized API (executor): persist payload; verify hash on Start visibility.
- New internal endpoints on both nodes for prompt retrieval.
- Validator: retrieval flow with executor-first and transfer fallback; hash verification.
- CLI/autocli: remove `prompt_payload` positional arg from `startInference`.

### 10) Errors, Retries, Observability

- **Retries**: Exponential backoff on executor; then fallback to transfer.
- **Distinct errors**: 404 vs 409 vs 423 for clear operator signals.
- **Metrics**: `prompt_fetch_success`, `prompt_fetch_fallback`, `prompt_fetch_mismatch`, `prompt_store_success`.
- **Logs**: Include `inference_id`, source node, and `prompt_hash` (both stored and on-chain).

### 11) Out of Scope (Later Phases)

- Removing `original_prompt` from tx/signature preimages.
- Off-chain response payload migration.
- Replication/gossip for prompt payloads. 
