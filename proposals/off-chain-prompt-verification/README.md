# Off-Chain Prompt Verification – Overview

This proposal reduces on-chain bloat and preserves verifiability by:
- Moving large prompt payloads off-chain (served by API nodes) while keeping `prompt_hash` on-chain
- Transitioning signatures to commit to the hash of the original body (privacy-preserving)

Phases
- Phase 1: Move `prompt_payload` off-chain, store it in the node’s SQLite, expose a fetch API, and remove it from `MsgStartInference` (keep it in `Inference` as fallback). Validators fetch from executor first, then transfer; if both fail, they submit INVALID and others re-attempt during voting.
  - Details: `PHASE1-offchain-prompt-payload.md`
- Phase 2: Replace on-chain `original_prompt` with `original_prompt_hash` and use it in all signature preimages (off-chain headers and on-chain verification). Enforce for new inferences, allow legacy behavior for already-recorded inferences.
  - Details: `PHASE2-hash-original-prompt.md`

Scope
- Only prompt-side changes (response handling unchanged)
- No new on-chain fallback data mechanism (Phase 3 removed)
- Database pruning: off-chain prompt payloads must be pruned on the same schedule as the chain’s inference data pruning (same retention window).

Why this works
- Integrity: validators verify off-chain payloads against `prompt_hash`
- Privacy: signatures bind to `original_prompt_hash`, not raw bodies
- Availability: two off-chain sources (executor → transfer) with clear validator behavior on failure

Further reading
- Phase 1: `PHASE1-offchain-prompt-payload.md`
- Phase 2: `PHASE2-hash-original-prompt.md`
