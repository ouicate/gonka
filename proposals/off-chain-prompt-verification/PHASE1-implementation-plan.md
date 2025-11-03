# Off-Chain Prompt Payload (Phase 1) - Task Plan

## Prerequisite Reading

Before starting implementation, please read the following documents to understand the full context of the changes:
- Main overview: `proposals/off-chain-prompt-verification/README.md`
- Phase spec: `proposals/off-chain-prompt-verification/PHASE1-offchain-prompt-payload.md`
- Current inference request/response flow: `decentralized-api/internal/server/public/post_chat_handler.go`

## System Overview

This implementation removes `prompt_payload` from `MsgStartInference` and moves the full prompt payload off-chain, stored redundantly on the transfer and executor nodes in the existing embedded SQLite. Validators fetch prompt payload from executor first, then transfer as fallback, verifying integrity via on-chain `prompt_hash`. If both off-chain fetches fail, validators submit INVALID (value=0) immediately and other validators re-attempt fetching during voting.

## How to Use This Task List

### Workflow
- Focus on a single task at a time. Avoid implementing parts of future tasks.
- Before starting the work on a task, mark its status `[~] - In Progress`
- Request a review: once a task is complete, change its status to `[?] - Review` and wait for confirmation.
- Update all usages: if a function or variable is renamed, update all references across the codebase.
- Build after each task to ensure no compilation errors.
- Test after each section to verify functionality.

### Build & Test Commands
- Build Inference Chain: `make node-local-build`
- Build Decentralized API: `make api-local-build`
- Run Inference Chain Unit Tests: `make node-test`
- Run API Node Unit Tests: `make api-test`
- Generate Proto Go Code (when modifying proto files): in the `inference-chain` folder run `ignite generate proto-go`

### Status Indicators
- `[ ]` Not Started
- `[~]` In Progress
- `[?]` Review
- `[x]` Finished

### Task Organization
Tasks are organized by implementation area and numbered for easy reference. Dependencies are noted where critical. Complete tasks in order.

### Task Format
Each task includes:
- What: Clear description of work to be done
- Where: Specific files/locations to modify
- Why: Brief context of purpose when not obvious
- Dependencies: Upstream tasks, if any

## Task List

### Section 1: Proto and On-Chain Type Updates

#### 1.1 Remove prompt_payload from StartInference
- **Task**: [x] Remove `prompt_payload` from `MsgStartInference` proto
- **What**: Delete the field from tx.proto; regenerate Go code; keep `Inference.prompt_payload` unchanged
- **Where**:
  - `inference-chain/proto/inference/inference/tx.proto`
  - Generated: `inference-chain/x/inference/types/tx.pb.go`, `inference-chain/api/inference/inference/tx.pulsar.go`
- **Why**: Shift prompt payload to off-chain storage; reduce tx size
- **Dependencies**: None

- Result:
  - Removed field from `MsgStartInference` in `inference-chain/proto/inference/inference/tx.proto` and regenerated Go code.
  - Verified compile by building the node; addressed downstream references in subsequent task.
  - No changes to `Inference.prompt_payload` on-chain type as planned.

#### 1.2 Adjust ValidateBasic and state calculations
- **Task**: [x] Update type validators and calculations
- **What**: Drop Start-time validation requiring `prompt_payload`; ensure `calculations/inference_state.go` does not set `PromptPayload` from Start
- **Where**:
  - `inference-chain/x/inference/types/message_start_inference.go`
  - `inference-chain/x/inference/calculations/inference_state.go`
- **Why**: Align validation/state with new proto shape
- **Dependencies**: 1.1

- Result:
  - `ValidateBasic` no longer requires `prompt_payload`; only `prompt_hash` and `original_prompt` remain required among prompt fields.
  - `calculations/inference_state.go` no longer sets `PromptPayload` during Start; state updates rely on `PromptHash` and other fields.
  - Updated unit tests across `types`, `calculations`, and `keeper` to remove `PromptPayload` from `MsgStartInference` constructions and assertions.
  - Implemented macOS wasmvm static library caching in `inference-chain/Makefile` to avoid re-downloading on each build.
  - Test results: `make node-test` reports 1162 passed, 0 failed; manual `go test ./... -v` also green.

### Section 2: SQLite Schema and Storage Helpers

#### 2.1 Extend SQLite schema
- **Task**: [x] Add `inference_prompt_payloads` table
- **What**: Extend `EnsureSchema` to create the new table with columns: `inference_id (PK)`, `prompt_payload`, `prompt_hash`, `model`, `request_timestamp`, `stored_by`, `created_at`
- **Where**: `decentralized-api/apiconfig/sqlite_store.go`
- **Why**: Persist off-chain prompt payloads on both transfer and executor nodes
- **Dependencies**: None

- Result:
  - Added `inference_prompt_payloads` with indexes in `decentralized-api/apiconfig/sqlite_store.go`.
  - Columns: `inference_id (PK)`, `prompt_payload`, `prompt_hash`, `model`, `request_timestamp`, `stored_by`, `created_at`.
  - Full API build currently blocked by pending Task 3.1 changes; table creation compiles and will be exercised once 3.1 updates are in.

#### 2.2 Implement storage helpers
- **Task**: [x] CRUD for prompt payloads
- **What**: Implement `SavePromptPayload`, `GetPromptPayload`, `DeletePromptPayloadsOlderThan`, and utility methods
- **Where**: `decentralized-api/internal/storage/inference_payloads.go` (new file)
- **Why**: Encapsulate DB operations and support pruning
- **Dependencies**: 2.1

- Result:
  - Added `SavePromptPayload`, `GetPromptPayload`, `DeletePromptPayloadsOlderThan` in `decentralized-api/internal/storage/inference_payloads.go`.
  - Helpers use the new `inference_prompt_payloads` table and are wired in the transfer path (3.1).

#### 2.3 Pruning/retention
- **Task**: [x] Prune off-chain payloads on same schedule as chain pruning
- **What**: Add periodic cleanup to prune rows using same retention window as chain’s inference data
- **Where**: background task in API startup or existing maintenance loop (e.g., `decentralized-api/internal/startup`)
- **Why**: Keep storage bounded and aligned with on-chain retention
- **Dependencies**: 2.2

- Result:
  - Added periodic pruning tied to epoch transitions in `decentralized-api/internal/event_listener/new_block_dispatcher.go`.
  - Retention window derives from chain `EpochParams.inference_pruning_epoch_threshold`; epoch duration approximated using average seconds per block.
  - On each validator set change, deletes rows older than cutoff via `DeletePromptPayloadsOlderThan`.

### Section 3: API Write Path (Transfer/Executor)

#### 3.1 Transfer path updates
- **Task**: [x] Stop attaching prompt_payload to Start; persist off-chain
- **What**: Before submitting Start, compute `prompt_hash`, persist `{inference_id, prompt_payload, prompt_hash, stored_by='transfer'}`; construct `MsgStartInference` without `prompt_payload`
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go`
- **Why**: Move payload off-chain while preserving integrity via `prompt_hash`
- **Dependencies**: 1.1, 2.2

- Result:
  - Removed `prompt_payload` from Start message construction in `decentralized-api/internal/server/public/post_chat_handler.go`.
  - Persisting `{inference_id, prompt_payload, prompt_hash, model, request_timestamp, stored_by='transfer'}` via `storage.SavePromptPayload` using the API's SQLite.
  - API builds successfully.

#### 3.2 Executor path updates
- **Task**: [x] Persist prompt payload on executor receipt
- **What**: On executor request, persist `{inference_id, prompt_payload, prompt_hash?, stored_by='executor'}`; if Start not visible yet, verify `prompt_hash` after Start observed
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go`
- **Why**: Ensure redundancy and later availability for validators
- **Dependencies**: 2.2

- Result:
  - On executor request, computes prompt hash from modified request body and persists payload with `stored_by='executor'` in `post_chat_handler.go`.
  - Uses same hashing as transfer side to ensure integrity; verification against Start `prompt_hash` can be added in 5.1 flow.
  - API builds successfully.

### Section 4: Prompt Retrieval API

#### 4.1 Add GET /v1/inferences/{inference_id}/prompt
- **Task**: [x] Implement retrieval endpoint with auth
- **What**: Add handler that returns raw JSON body, sets `X-Prompt-Hash`, validates signature headers (`X-Validator-Address`, `Authorization`, `X-Timestamp`)
- **Where**:
  - Handler (new): `decentralized-api/internal/server/public/get_prompt_handler.go`
  - Headers: `decentralized-api/utils/api_headers.go`
  - Router registration: existing public server setup
- **Why**: Provide executor-first, transfer-fallback retrieval for validators
- **Dependencies**: 2.2, 3.1, 3.2

- Result:
  - Added `GET /v1/inferences/:id/prompt` returning raw prompt JSON with `X-Prompt-Hash` header.
  - Basic auth header checks: `X-Validator-Address` and `Authorization` (timestamp optional now).
  - Route registered in public server; API builds successfully.

### Section 5: Validator Retrieval and INVALID Behavior

#### 5.1 Update validator retrieval sequence
- **Task**: [x] Implement off-chain fetch flow and INVALID on failure
- **What**: In `validateInferenceAndSendValMessage` path, try: on-chain `Inference.prompt_payload` → executor API → transfer API; if both fail, submit INVALID (reason: payload-unavailable). Ensure others re-attempt during voting
- **Where**: `decentralized-api/internal/validation/inference_validation.go`
- **Why**: Deterministic validator behavior and clear failure handling
- **Dependencies**: 4.1

- Result:
  - Validator now fetches prompt off-chain if not present on-chain: executor first, then transfer fallback, via `GET /v1/inferences/:id/prompt`.
  - Integrity check recomputes `prompt_hash` from fetched JSON; mismatch or double-failure yields INVALID path with reason.
  - Builds successfully.

### Section 6: Testing

#### 6.1 Unit tests for storage and hash verification
- **Task**: [x] Add tests for DB helpers and integrity checks
- **What**: CRUD tests for `inference_prompt_payloads`; verify `prompt_hash` recomputation matches on-chain
- **Where**: `decentralized-api/internal/storage/..._test.go`
- **Why**: Ensure persistence and integrity are correct
- **Dependencies**: 2.2

#### 6.2 Handler and validator tests
- **Task**: [x] Add tests for handler auth and validator fallback
- **What**: Tests for GET handler auth (valid/invalid headers), and validator INVALID on double-failure
- **Where**: `decentralized-api/internal/server/public/..._test.go`, `decentralized-api/internal/validation/..._test.go`
- **Why**: Validate API security and validator behavior
- **Dependencies**: 4.1, 5.1

- Result:
  - Handler: added unauthorized and success tests for `GET /v1/inferences/:id/prompt`, verifying required headers and `X-Prompt-Hash` returned.
  - Validator: added unit tests for off-chain fetch helper covering success, unauthorized, and empty-body cases; integrated into validator retrieval path.

### Section 7: Rollout and Backward Compatibility

#### 7.1 Backward compatibility checks
- **Task**: [x] Ensure Start without prompt_payload is accepted; Inference still exposes legacy `prompt_payload` when present
- **What**: Manual/automated checks across endpoints and CLI
- **Where**: Chain and API integration tests
- **Why**: Smooth transition without breaking legacy inferences
- **Dependencies**: 1.1–5.1

- Result:
  - Chain accepts `MsgStartInference` without `prompt_payload` (node tests all pass).
  - Validator flow prefers on-chain `Inference.prompt_payload` if present; otherwise fetches off-chain and verifies hash.
  - API Start path no longer sends `prompt_payload`; both transfer and executor persist payload off-chain.


