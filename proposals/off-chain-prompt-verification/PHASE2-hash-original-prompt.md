## Phase 2: Replace on-chain original_prompt with original_prompt_hash and use in signatures

### 1) Goal

- Remove raw prompt body from transaction messages and state fields used for signatures.
- Store and transmit only the hash of the original request body: `original_prompt_hash`.
- Use `original_prompt_hash` in all signature preimages (off-chain request auth and on-chain key verification).

Result: privacy-preserving signatures and smaller transactions while keeping verifiability via hashes.

### 2) Definitions

- original_prompt (transport body): the exact raw HTTP request body bytes that the transfer node forwards to the executor (before any seed injection).
- original_prompt_hash: `SHA-256(raw_body_bytes)` encoded as lowercase hex string.
- prompt_hash (existing): `SHA-256(canonicalized_json)`; retained for validation replay coherence with off-chain prompt_payload.

### 3) Proto changes (add hash, stop sending raw body)

- MsgStartInference
  - Add: `original_prompt_hash` (string)
  - Keep: `prompt_hash`, `requested_by`, `assigned_to`, etc.
  - Stop requiring: `original_prompt` (deprecated)

- MsgFinishInference
  - Add: `original_prompt_hash` (string)
  - Keep: response fields, model, signatures
  - Stop requiring: `original_prompt` (deprecated)

- Inference
  - Add: `original_prompt_hash` (string)
  - Keep: `prompt_hash`
  - Keep: `prompt_payload` (as Phase 1 fallback reservoir, may be empty for new inferences)
  - Mark: `original_prompt` as deprecated (field may remain for wire compatibility but not set for new tx)

Validation/encoding
- `ValidateBasic` should require `original_prompt_hash` non-empty (hex, 64 chars) for new messages.
- If both `original_prompt` and `original_prompt_hash` are present (during transition), prefer the hash.

Code references (proto/types):
- `inference-chain/proto/inference/inference/tx.proto` (add `original_prompt_hash` to Start/Finish; deprecate `original_prompt`)
- `inference-chain/proto/inference/inference/inference.proto` (add `original_prompt_hash`; keep `prompt_hash`, `prompt_payload`)
- Generated: `inference-chain/x/inference/types/tx.pb.go`, `inference-chain/api/inference/inference/tx.pulsar.go`, `inference-chain/x/inference/types/inference.pb.go`, `inference-chain/api/inference/inference/inference.pulsar.go`
- `inference-chain/x/inference/types/message_start_inference.go` and `message_finish_inference.go` (ValidateBasic updates)

### 4) Signature preimages: use hash instead of raw body

Off-chain (request headers, transfer→executor)
- Replace developer/transfer/executor signature components to use `original_prompt_hash` as payload instead of raw body:
  - Developer header (Authorization): sign bytes composed from `{payload=original_prompt_hash, timestamp, transfer_address}`
  - Transfer (X-TA-Signature): sign `{payload=original_prompt_hash, timestamp, transfer_address, executor_address}`
  - Executor (Finish signature): sign `{payload=original_prompt_hash, timestamp, transfer_address, executor_address}`

On-chain (keeper verification)
- `getSignatureComponents` (Start) → `Payload = msg.OriginalPromptHash`
- `getFinishSignatureComponents` (Finish) → `Payload = msg.OriginalPromptHash`
- No other concatenation rule changes (timestamp, transfer, executor remain as today).

Compatibility
- New inferences (no existing legacy record): require hash-based preimages. Legacy raw-body preimages are rejected.
- Existing legacy inferences (already recorded with `original_prompt`): for subsequent messages (e.g., Finish), skip new hash-based signature enforcement (process with existing rules). This ensures backward compatibility for in-flight legacy inferences.

Code references (keeper/signatures):
- `inference-chain/x/inference/keeper/msg_server_start_inference.go` (`getSignatureComponents` payload)
- `inference-chain/x/inference/keeper/msg_server_finish_inference.go` (`getFinishSignatureComponents` payload)
- `inference-chain/x/inference/calculations/signature_validate.go` (no byte layout change besides payload source)

### 5) Decentralized API changes

- Start path (transfer node):
  - Compute `original_prompt_hash = SHA-256(request.Body)` immediately after reading the body.
  - Use `original_prompt_hash` in all header signature calculations.
  - Submit `MsgStartInference` with `original_prompt_hash` (omit `original_prompt`).

- Executor path:
  - On receipt, recompute `original_prompt_hash` from the same raw body it receives.
  - Validate incoming headers against the hash-based preimages.
  - Submit `MsgFinishInference` with `original_prompt_hash` (omit `original_prompt`).

- Validator replay:
  - Unchanged wrt prompt replay (still uses off-chain `prompt_payload` with `prompt_hash`).
  - For signature verification in any tooling, rely on `original_prompt_hash` from chain instead of raw body.

Code references (API/headers):
- `decentralized-api/internal/server/public/post_chat_handler.go` (compute hash from request body; replace calls to `calculateSignature` payload argument)
- `decentralized-api/internal/server/public/post_chat_handler.go` (populate `MsgStartInference`/`MsgFinishInference` fields)
- `decentralized-api/utils/api_headers.go` (no new constants needed if reusing `Authorization`, `X-Timestamp`, etc.)

### 6) Backward compatibility and rollout plan

Single-step activation with conditional enforcement:

- Proto: add `original_prompt_hash` to Start/Finish and `Inference`; keep `original_prompt` for legacy records.
- On-chain:
  - If `Inference` does not exist (new inference): enforce hash-based preimages and `original_prompt_hash` in msgs.
  - If `Inference` exists and has `original_prompt` (legacy): allow subsequent messages (e.g., Finish) without requiring hash-based signatures.
- Off-chain: begin sending hash-based headers immediately; old-format headers for new inferences will be rejected.

Code references (params/feature gating):
- `inference-chain/x/inference/keeper/params.go` (if a param is used to gate behavior; else direct conditional by inference existence)

Notes
- No changes to `prompt_hash` or Phase 1 off-chain storage APIs.
- Inference may keep `prompt_payload` (Phase 1 fallback) and add `original_prompt_hash` for audit.

### 7) Security and correctness

- Privacy: raw bodies never stored or signed on-chain post Phase 2; signatures commit to the hash.
- Integrity: the same raw body that is executed must produce the hash that is committed in Start/Finish and used in signatures.
- Replay: timestamp/nonce rules unchanged; preimages switch from body→hash.

### 8) Required changes (high level)

- Proto
  - Edit tx.proto: add `original_prompt_hash` to Start/Finish; mark `original_prompt` deprecated.
  - Edit inference.proto: add `original_prompt_hash`; keep existing fields for compatibility.

- On-chain code
  - Update ValidateBasic checks.
  - Update keeper signature component builders to use hash.

- Decentralized API
  - Compute and propagate `original_prompt_hash`.
  - Switch header signature calculations to hash payload.
  - Populate Start/Finish with `original_prompt_hash`.

- Config/params
  - Add module param to gate acceptance of legacy body-based signatures during Step A.

### 9) Testing

- Golden tests for signature byte construction using hash payload.
- Dual-mode tests: both legacy and hash preimages accepted in Step A.
- Cross-node interop tests (mixed versions).
- End-to-end: transfer→executor→finish→validator replay; ensure hashes match and signatures validate.


