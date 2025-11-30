# Phase 3: Signature Migration

## Summary

Migrate signatures from full payloads to hashes:

- **Dev (user)**: Signs `original_prompt_hash + timestamp + ta_address` (chain verifies)
- **TA**: Signs `prompt_hash + timestamp + ta_address + executor_address` (chain verifies)

**Terminology:**

- `original_prompt` = raw user request body
- `original_prompt_hash` = SHA256(original_prompt) - what dev signs
- `prompt_payload` = modified request (original_prompt + seed)
- `prompt_hash` = SHA256(prompt_payload) - what TA signs

Breaking change requiring coordinated deployment.

## Key Changes

### 1. Chain Proto - Messages and State

**tx.proto** - Transaction messages:

```protobuf
message MsgStartInference {
  string prompt_hash = 3;
  string prompt_payload = 4 [deprecated = true];
  string original_prompt = 15 [deprecated = true];
  string original_prompt_hash = 16;                 // NEW
}

message MsgFinishInference {
  string response_hash = 3;                         // Already exists
  string response_payload = 4 [deprecated = true];
  string original_prompt = 13 [deprecated = true];
  string prompt_hash = 15;                          // NEW
  string original_prompt_hash = 16;                 // NEW
}
```

**inference.proto** - Stored state:

```protobuf
message Inference {
  string prompt_hash = 3;                           // Already exists
  string prompt_payload = 4 [deprecated = true];
  string response_hash = 5;                         // Already exists
  string response_payload = 6 [deprecated = true];
  string original_prompt = 31 [deprecated = true];
  string original_prompt_hash = 33;                 // NEW
}
```

### 2. Chain - Separate Signature Components

Update `verifyKeys()` and `verifyFinishKeys()` to use:

- Dev signature: `original_prompt_hash`
- TA/Executor signature: `prompt_hash`

### 3. API - Signature Changes

- `validateTransferRequest()`: Validate against `original_prompt_hash`
- `createInferenceStartRequest()`: TA signs `prompt_hash`, populate both hash fields

### 4. Off-chain Hash Validation

Executor validates `prompt_hash` matches `hash(prompt_payload)` before execution.

### 5. Testermint Updates

Add hash fields to data classes, update signature creation.

## Unit Tests

### Chain Tests

- Dev sig verified with original_prompt_hash
- TA sig verified with prompt_hash
- Mismatched hashes rejected

### API Tests

- User sig validated against original_prompt_hash
- TA signs prompt_hash correctly
- Both hash fields populated
- Executor rejects mismatched prompt_hash

## Verification

**All inference-related testermint tests must pass:**

```bash
make run-tests TESTS="InferenceTests"
```

## Documentation

Create `proposals/offchain-payloads/phase-3.md` with:

- Summary of changes
- Key decisions and approaches
- **All new cases found during implementation**
- **Potential issues discovered**
- Test coverage

## Files Modified

| Component | File | Change |

|-----------|------|--------|

| Chain | `proto/.../tx.proto` | Add hash fields, deprecate payloads |

| Chain | `proto/.../inference.proto` | Add original_prompt_hash, deprecate payloads |

| Chain | `keeper/msg_server_start_inference.go` | Separate components for dev vs TA |

| Chain | `keeper/msg_server_finish_inference.go` | Separate components for dev vs TA |

| Chain | `keeper/*_test.go` | Unit tests for signature verification |

| API | `utils.go` | Validate sig with original_prompt_hash |

| API | `utils_test.go` | Unit tests |

| API | `post_chat_handler.go` | TA signs prompt_hash, populate hashes |

| API | `post_chat_handler_test.go` | Unit tests |

| Testermint | `data/inference.kt` | Add hash fields |

| Testermint | `InferenceTests.kt` | Update signatures to use hashes |

| Docs | `phase-3.md` | Implementation docs + issues found |