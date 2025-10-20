# Missed-Inference Threshold Fallback - Task Plan

## Prerequisite Reading

Before starting implementation, please read the following documents to understand the full context of the changes:
- The main proposal: `proposals/missed-inference-threshold-fallback/README.md`

## How to Use This Task List

### Workflow
- **Focus on a single task**: Please work on only one task at a time to ensure clarity and quality. Avoid implementing parts of future tasks.
- **Request a review**: Once a task's implementation is complete, change its status to `[?] - Review` and wait for my confirmation.
- **Update all usages**: If a function or variable is renamed, find and update all its references throughout the codebase.
- **Build after each task**: After each task is completed, build the project to ensure there are no compilation errors.
- **Test after each section**: After completing all tasks in a section, run the corresponding tests to verify the functionality.
- **Wait for completion**: After I confirm the review, mark the task as `[x] - Finished`, add a **Result** section summarizing the changes, and then move on to the next one.

### Build & Test Commands
- **Build Inference Chain**: From the project root, run `make node-local-build`
- **Build Decentralized API**: From the project root, run `make api-local-build`
- **Run Inference Chain Unit Tests**: From the project root, run `make node-test`
- **Run API Node Unit Tests**: From the project root, run `make api-test`

### Code Generation
- **Proto generation**: After any protobuf change, run `ignite generate proto-go` inside `inference-chain/`.
- **Scaffold usage**: When adding new messages/handlers/queries, prefer using Ignite scaffold commands (e.g., `ignite scaffold message ...`) to generate boilerplate and wiring, then refine implementation.

### Status Indicators
- `[ ]` **Not Started** - Task has not been initiated
- `[~]` **In Progress** - Task is currently being worked on
- `[?]` **Review** - Task completed, requires review/testing
- `[x]` **Finished** - Task completed and verified

### Task Organization
Tasks are organized by implementation area and numbered for easy reference. Dependencies are noted where critical. Complete tasks in order.

### Task Format
Each task includes:
- **What**: Clear description of work to be done
- **Where**: Specific files/locations to modify
- **Why**: Brief context of purpose when not obvious

## Task List

### Section 1: Immediate Missed-Inference Threshold & Grace Window

#### 1.1 Add DowntimeGraceBlocks parameter
- **Task**: [x] Add `DowntimeGraceBlocks` to collateral params
- **What**: Add field, defaults, and plumbing for grace window blocks
- **Where**: `inference-chain/proto/inference/inference/params.proto`, `inference-chain/x/inference/types/params.go`
- **Dependencies**: None
 - **Result**:
   - Added `downtime_grace_blocks` to `CollateralParams` in `inference-chain/proto/inference/inference/params.proto`.
   - Wired defaults and validation in `inference-chain/x/inference/types/params.go` (`KeyDowntimeGraceBlocks`, `DefaultCollateralParams=50`, `ParamSetPairs`, `Validate`).
   - Regenerated protobufs (`ignite generate proto-go`) and built the inference chain successfully.

#### 1.2 Track first missed block in epoch stats
- **Task**: [x] Add `FirstMissedBlock` to executor epoch stats
- **What**: Extend stats struct and reset on epoch switch
- **Where**: `inference-chain/x/inference/module/module.go` (stats struct/init/reset)
- **Dependencies**: 1.1
 - **Result**:
   - Added `first_missed_block` to `CurrentEpochStats` in `inference-chain/proto/inference/inference/participant.proto` and regenerated protobufs.
   - Updated `handleExpiredInference` in `inference-chain/x/inference/module/module.go` to set `FirstMissedBlock` on the first miss and increment `MissedRequests`.
   - Build passed successfully.

#### 1.3 Gate immediate slashing by grace window
- **Task**: [x] Evaluate missed threshold and gate slashing by grace
- **What**: In `handleExpiredInference`, set `FirstMissedBlock` and only call downtime slashing if `currentHeight >= FirstMissedBlock + DowntimeGraceBlocks`
- **Where**: `inference-chain/x/inference/module/module.go` (`handleExpiredInference`)
- **Dependencies**: 1.1, 1.2
 - **Result**:
  - Implemented gating in `handleExpiredInference`: only slashes when `Status == ACTIVE` and `currentHeight >= FirstMissedBlock + DowntimeGraceBlocks`.
  - Uses the same statistical test as rewards (`MissedStatTest`) to decide slashing; when the test fails, performs `collateralKeeper.Slash` with `SlashFractionDowntime`.
  - Marks executor `Status = INACTIVE` for the rest of the epoch; resets to ACTIVE on epoch rollover via stats reset.
   - Build passed successfully.

#### 1.4 Add per-epoch no-double-slash guard
- **Task**: [x] Prevent repeated downtime slashing in same epoch
- **What**: Remove settlement-time downtime slashing (handled immediately) to avoid double slashing
- **Where**: `inference-chain/x/inference/keeper/accountsettle.go`
- **Dependencies**: 1.3
- **Result**:
  - Removed call to `CheckAndSlashForDowntime` during `SettleAccounts` and replaced with a log noting it is handled immediately.
  - Double-slash risk eliminated; collateral slashing now occurs only on the immediate path after grace.

#### 1.5 Reset guards on epoch transition
- **Task**: [x] Clear per-epoch guard and `FirstMissedBlock` at epoch switch
- **What**: Reset state in epoch transition handler
- **Where**: `inference-chain/x/inference/keeper/accountsettle.go` (stats reset in `SettleAccounts`), module stage already calls it
- **Dependencies**: 1.4
- **Result**:
  - `SettleAccounts` resets `participant.CurrentEpochStats = &types.CurrentEpochStats{}` which clears `FirstMissedBlock` each epoch.
  - Per-epoch guard no longer needed after removing settlement-time downtime slashing; immediate-only path avoids double slash.

### Section 2: Transfer Node Avoids Invalid Executors

#### 2.1 Filter INVALID executors from selection
- **Task**: [x] Exclude non-`ACTIVE` executors in selection
- **What**: Filter candidates or request only ACTIVE from chain query
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go` (`getExecutorForRequest`)
- **Dependencies**: None
 - **Result**:
  - On-chain selection filters updated to include only `ACTIVE` executors during inference phase and require `ACTIVE` with available PoC node during PoC phase (`inference-chain/x/inference/keeper/query_get_random_executor.go`).
  - Ensures downstream consumers get an `ACTIVE` executor or an empty result when none are available.

#### 2.2 Random selection resilience for changing activity
- **Task**: [x] Skip non-`ACTIVE` picks during random selection
- **What**: If a random pick is not `ACTIVE`, remove it and redraw until an `ACTIVE` member is found or none remain
- **Where**: `inference-chain/x/inference/epochgroup/random.go`
- **Dependencies**: 2.1
- **Result**:
  - Implemented retry logic in `GetRandomMember` using `selectRandomParticipant` + removal loop to skip non-`ACTIVE` picks.
  - Allows future deterministic seeding while ensuring current selection is verifiable against the ACTIVE set at pick time.

### Section 3: Deterministic Selection & Skipped Executors (Chain Query)

#### 3.1 Deterministic GetRandomExecutor seeding
- **Task**: [x] Add deterministic seed to selection
- **What**: Add `inference_id` to `QueryGetRandomExecutorRequest` and use it to seed selection
- **Where**: `inference-chain/proto/inference/inference/query.proto`, `inference-chain/x/inference/epochgroup/random.go`
- **Dependencies**: 2.1, 2.2
- **Result**:
  - `inference_id` added to request; protobuf regenerated successfully.
  - Selection uses a local RNG seeded via `sha256(inference_id)` + little-endian to derive a stable `int64`.
  - Backward-compatible wrapper `GetRandomMemberForModel` maintained for tests.

#### 3.2 Return skipped executors from query
- **Task**: [x] Include skipped executors in query response
- **What**: Add `skipped_executors` to `QueryGetRandomExecutorResponse` and populate it when non-ACTIVE picks are skipped
- **Where**: `inference-chain/proto/inference/inference/query.proto`, `inference-chain/x/inference/keep er/query_get_random_executor.go`, `inference-chain/x/inference/epochgroup/random.go`
- **Dependencies**: 3.1, 2.2
- **Result**:
  - Response now contains `skipped_executors` reflecting excluded picks during selection retries.
  - `GetRandomExecutor` returns both the chosen `executor` and the `skipped_executors` array.

#### 3.3 Deterministic selection behavior (future-proof)
- **Task**: [x] Use seeded RNG and ACTIVE-only retry loop
- **What**: Retry selection, removing non-ACTIVE until an ACTIVE is found or none remain
- **Where**: `inference-chain/x/inference/epochgroup/random.go`
- **Dependencies**: 2.2
- **Result**:
  - Implemented seeded RNG + skip-and-retry; selection is reproducible and verifiable with the same inputs.

#### 3.4 Extend MsgStartInference with skipped_executors
- **Task**: [x] Add `skipped_executors` to `MsgStartInference`
- **What**: Proto changes and generated code update so the transfer node can attest skipped executors in the tx
- **Where**: `inference-chain/proto/inference/inference/tx.proto`
- **Dependencies**: 3.2
 - **Result**:
  - Added `skipped_executors` field to `MsgStartInference` and regenerated protobufs.
  - Build succeeded; field is now available for transfer agents.

#### 3.5 Validate skipped executors in handler
- **Task**: [x] Ensure skipped are provably ineligible at start time
- **What**: In `msg_server_start_inference.go`, verify each skipped executor was not eligible at the selection block (status/PoC availability)
- **Where**: `inference-chain/x/inference/keeper/msg_server_start_inference.go`
- **Dependencies**: 3.4
- **Result**:
  - Implemented `validateSkippedExecutorsNotActive` to ensure all skipped executor addresses are not `ACTIVE` at the start block; StartInference now fails with `ErrSkippedExecutorActive` if any skipped address is ACTIVE.

#### 3.6 Deterministic ordering in transfer node
- **Task**: [x] Implement deterministic executor ordering in dAPI
- **What**: Compute a deterministic order from (inference_id, model_id, epoch info) and iterate; submit skipped list to chain
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go`
- **Dependencies**: 3.1, 3.2, 3.4, 3.5
- **Result**:
  - dAPI now passes `inference_id` to `GetRandomExecutor`, which uses a seeded, deterministic selection and returns `skipped_executors` reflecting the deterministic trail.
  - dAPI includes the returned `skipped_executors` in `MsgStartInference`, ensuring consistent, auditable ordering across nodes without redundant local re-ordering.

### Section 4: Fallback to Alternative Executor

#### 4.1 Retry next executor on timeout
- **Task**: [ ] Implement deterministic retry loop with accumulation of skipped
- **What**: Iterate ordered list until response or exhaustion
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go`
- **Dependencies**: 3.3

#### 4.2 Accept alternative finisher if original missed
- **Task**: [ ] Reward alternative executor when original expired
- **What**: Enforce acceptance rules and reward attribution in finish handler
- **Where**: `inference-chain/x/inference/keeper/msg_server_finish_inference.go`
- **Dependencies**: 4.1

### Section 5: Executor Reconciliation (SQLite marker, no hard dedup)

#### 5.1 Persist HTTP-first marker in SQLite KV
- **Task**: [x] Record transfer-path requests in DB
- **What**: On HTTP request start, write `kv_config` key `inference_seen/<inferenceId>` with timestamp (or "1").
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go` (transfer path) using `config.SqlDb().GetDb()` + `apiconfig.KVSetString`.
- **Dependencies**: None
 - **Result**:
   - Wrote marker on transfer path before executor dispatch: key `inference_seen/<inferenceId>` with request timestamp via `apiconfig.KVSetString`.
   - Code: `post_chat_handler.go` inside `handleTransferRequest` after computing `inferenceUUID`.

#### 5.2 Reconcile StartInference against marker
- **Task**: [x] Process on-chain-first with WARN; skip if HTTP-first
- **What**: On `StartInference` observation, read `kv_config` key `inference_seen/<inferenceId>`.
  - If missing → on-chain-first: log WARN and handle executor-start path.
  - If present → HTTP-first: skip executor-start path; no WARN.
- **Where**: `decentralized-api/internal/event_listener/new_block_dispatcher.go` (or the specific handler reacting to `StartInference`), using `apiconfig.KVGetString`.
- **Dependencies**: 5.1
 - **Result**:
   - Added `inference_started` event emission on-chain with `inference_id` in `inference-chain/x/inference/keeper/msg_server_start_inference.go` to enable extraction by listeners.
  - Registered `StartInferenceEventHandler` in `decentralized-api/internal/event_listener/event_listener.go` to detect `inference_started.inference_id` and reconcile against SQLite KV.
  - Handler behavior: if KV missing → WARN "StartInference arrived before HTTP" and invoke the shared execution path; if KV present → no WARN.
  - Consolidated execution into a reusable function `ExecuteProxyAndFinish` inside `decentralized-api/internal/server/public/post_chat_handler.go` that performs request mutation, executor POST, streaming proxy, completion parsing, and delegates to `sendInferenceTransaction` for finishing.

#### 5.3 Allow duplicate FinishInference submissions
- **Task**: [x] Tolerate two `FinishInference` sends in rare races
- **What**: No separate dedup store. Accept potential double submissions; chain-side logic remains authoritative.
- **Where**: No-op unless handlers need minor tolerance checks; document behavior.
- **Dependencies**: 5.2
 - **Result**:
   - Left Finish flow unchanged; duplicate submissions tolerated. Chain-level handling remains authoritative.
  - Verified build after wiring events and shared execution path; no compilation errors.

### Section 6: Testing

#### 6.1 Unit tests for missed threshold & grace
- **Task**: [ ] Test threshold gating and guard behavior
- **What**: Add targeted unit tests
- **Where**: Chain module/keeper tests
- **Dependencies**: 1.*

#### 6.2 Integration tests for deterministic selection & fallback
- **Task**: [ ] Validate ordering, retries, and skipped recording
- **What**: End-to-end API tests
- **Where**: Decentralized API tests
- **Dependencies**: 3.*, 4.*

#### 6.3 Tests for reconciliation path
- **Task**: [ ] Ensure on-chain start triggers single execution
- **What**: Event ingestion and dedup behavior
- **Where**: API event listener tests
- **Dependencies**: 5.*

### Section 7: Documentation & Migration

#### 7.1 Update API documentation
- **Task**: [ ] Document deterministic selection and skipped executors
- **What**: Update API docs and examples
- **Where**: API documentation files
- **Dependencies**: 3.*, 4.*

#### 7.2 Add migration notes
- **Task**: [ ] Document new param and behavior changes
- **What**: `DowntimeGraceBlocks`, executor selection, fallback
- **Where**: Migration documentation
- **Dependencies**: 1.*, 3.*, 4.*


