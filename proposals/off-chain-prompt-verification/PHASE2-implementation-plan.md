# Hash-Based Original Prompt (Phase 2) - Task Plan

## Prerequisite Reading

Before starting implementation, please read the following documents to understand the full context of the changes:
- Main overview: `proposals/off-chain-prompt-verification/README.md`
- Phase spec: `proposals/off-chain-prompt-verification/PHASE2-hash-original-prompt.md`
- Signature logic and validators: `inference-chain/x/inference/calculations/signature_validate.go`

## System Overview

This implementation replaces on-chain `original_prompt` with `original_prompt_hash` and updates all signature preimages (off-chain request headers and on-chain verification) to use the hash instead of the raw body. For new inferences, hash-based signatures are required; for existing legacy inferences (already recorded with `original_prompt`), we skip new enforcement for subsequent messages.

## How to Use This Task List

### Workflow
- Focus on a single task at a time. Avoid implementing parts of future tasks.
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

### Section 1: Proto and Types

#### 1.1 Add original_prompt_hash to Start/Finish/Inference
- **Task**: [ ] Add `original_prompt_hash` fields and deprecate `original_prompt`
- **What**: Update proto definitions; regenerate Go code; mark legacy fields as deprecated
- **Where**:
  - `inference-chain/proto/inference/inference/tx.proto`
  - `inference-chain/proto/inference/inference/inference.proto`
  - Generated: `inference-chain/x/inference/types/tx.pb.go`, `inference-chain/api/inference/inference/tx.pulsar.go`, `inference-chain/x/inference/types/inference.pb.go`, `inference-chain/api/inference/inference/inference.pulsar.go`
- **Why**: Commit to hash on-chain for privacy and consistent signatures
- **Dependencies**: None

#### 1.2 ValidateBasic updates
- **Task**: [ ] Require `original_prompt_hash` for new messages
- **What**: Add hex/length validation; prefer hash when both legacy and new present
- **Where**:
  - `inference-chain/x/inference/types/message_start_inference.go`
  - `inference-chain/x/inference/types/message_finish_inference.go`
- **Why**: Enforce new preimage source
- **Dependencies**: 1.1

### Section 2: Keeper Signature Enforcement

#### 2.1 Update signature components payload
- **Task**: [ ] Switch payload to `msg.OriginalPromptHash`
- **What**: Update `getSignatureComponents` and `getFinishSignatureComponents` to use hash
- **Where**:
  - `inference-chain/x/inference/keeper/msg_server_start_inference.go`
  - `inference-chain/x/inference/keeper/msg_server_finish_inference.go`
- **Why**: Ensure on-chain signature verification matches off-chain signing
- **Dependencies**: 1.1

#### 2.2 Conditional compatibility logic
- **Task**: [ ] Enforce for new inferences; skip for legacy
- **What**: If no `Inference` exists yet → require hash-based signatures; if existing `Inference` has `original_prompt` (legacy) → allow subsequent messages to pass without new enforcement
- **Where**: same as 2.1 (start/finish msg servers)
- **Why**: Backward compatibility for in-flight legacy inferences
- **Dependencies**: 2.1

### Section 3: Decentralized API Changes

#### 3.1 Compute and propagate original_prompt_hash
- **Task**: [ ] Hash body and use it in all signatures
- **What**: Compute `original_prompt_hash = SHA-256(raw request body)`; use it as payload in `calculateSignature` calls; populate `MsgStartInference`/`MsgFinishInference` with hash, omit legacy `original_prompt`
- **Where**: `decentralized-api/internal/server/public/post_chat_handler.go`
- **Why**: Align off-chain headers and on-chain messages with hash-based preimages
- **Dependencies**: 1.1, 2.1

### Section 4: Tests

#### 4.1 Hash preimage signing tests
- **Task**: [ ] Update/extend tests to cover hash-based preimages
- **What**: Golden tests for signature byte construction using hash payload; mixed-mode tests for compatibility
- **Where**: chain keeper tests and API tests (`*_test.go` files)
- **Why**: Ensure correctness and compatibility during transition
- **Dependencies**: 2.1, 3.1

### Section 5: Documentation and Rollout

#### 5.1 Update docs and examples
- **Task**: [ ] Update proposal docs and any operator guides
- **What**: Reflect `original_prompt_hash` usage and compatibility rules
- **Where**: `proposals/off-chain-prompt-verification/PHASE2-hash-original-prompt.md` and any relevant README
- **Why**: Keep operator/developer guidance accurate
- **Dependencies**: 1–4


