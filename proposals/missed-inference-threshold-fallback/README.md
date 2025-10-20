Missed-Inference Threshold Enforcement and Deterministic Executor Fallback

Context

Today a participant’s missed requests are recorded when an inference expires (see `inference-chain/x/inference/module/module.go`, function `handleExpiredInference` where `executor.CurrentEpochStats.MissedRequests++` is applied). During settlement we evaluate missed-request thresholds to reduce rewards via the same statistical test used elsewhere (see `inference-chain/x/inference/keeper/accountsettle.go`, functions `CheckAndPunishForDowntimeForParticipant` and `CheckAndPunishForDowntime`, which call `calculations.MissedStatTest`). We also have slashing logic for epoch downtime percentage in `inference-chain/x/inference/keeper/collateral.go` function `CheckAndSlashForDowntime`, and status-based slashing when a participant transitions to `INVALID` in `CheckAndSlashForInvalidStatus`.

This proposal introduces immediate threshold checks at the point of a missed inference, deterministic executor selection with on-chain attestations of skipped executors, and a transfer-node fallback flow to alternative executors prior to formal invalidation.

Description

1) Immediate missed-inference threshold check with downtime slashing (no double-slash)

- When an inference is missed (expired), immediately evaluate the same missed threshold used during settlement. Use `calculations.MissedStatTest` as wired through `CheckAndPunishForDowntime` in `inference-chain/x/inference/keeper/accountsettle.go`. The integration point is `inference-chain/x/inference/module/module.go` in `handleExpiredInference` right after incrementing `MissedRequests`.
- If the threshold fails (i.e., misses exceed the acceptable bound), perform a DOWNTIME slash immediately using `CheckAndSlashForDowntime` in `inference-chain/x/inference/keeper/collateral.go` (not the INVALID-status slasher). This uses `CollateralParams.SlashFractionDowntime` and `DowntimeMissedPercentageThreshold`.
- Status marking: still make the participant ineligible for execution. Either (A) set participant status to `INVALID` via the existing `calculateStatus` path in `inference-chain/x/inference/keeper/msg_server_validation.go`, or (B) introduce a dedicated downtime-invalid marker (new status/flag) that selection code treats as ineligible. In either case, selection must exclude such participants (see Section 2).
- No double slashing at settlement: add a per-epoch guard so `SettleAccounts` does not re-invoke downtime slashing for the same epoch. Integration points:
  - Write/Read guard in `inference-chain/x/inference/keeper/collateral.go` around `CheckAndSlashForDowntime`.
  - Check guard in `inference-chain/x/inference/keeper/accountsettle.go` inside the `SettleAccounts` loop before calling `CheckAndSlashForDowntime`.
 - Guard with status: only call `CheckAndSlashForDowntime` when `participant.Status != INVALID` to avoid mixing INVALID-status slashing with downtime slashing.
 - Reset guards at epoch switch: clear the per-epoch downtime-slash guard during `onSetNewValidatorsStage` in `inference-chain/x/inference/module/module.go` (where participant statuses are reset to ACTIVE), ensuring the next epoch can evaluate afresh without re-slashing for the prior epoch.
 - Ordering: perform the check immediately after `executor.CurrentEpochStats.MissedRequests++` in `handleExpiredInference` so totals used by `CheckAndPunishForDowntime`/`MissedStatTest` reflect the newly missed request.

- Short-downtime grace window (avoid punishing brief outages):
  - Goal: Don’t punish transient downtime. After the first missed inference, wait a grace window (e.g., 50 blocks) before applying downtime slashing.
  - Record first-miss block: in `handleExpiredInference`, right after `executor.CurrentEpochStats.MissedRequests++`, if `executor.CurrentEpochStats.FirstMissedBlock == 0`, set it to the current block height.
  - Gate immediate slashing: still evaluate the missed threshold using `CheckAndPunishForDowntime`/`MissedStatTest`, but only call `CheckAndSlashForDowntime` if `currentBlockHeight >= FirstMissedBlock + DowntimeGraceBlocks`.
    - Guard with status as above (do not downtime‑slash when `Status == INVALID`).
    - Preserve the existing per‑epoch no‑double‑slash guard.
  - Reset at epoch switch: when we transition epochs, clear `FirstMissedBlock` along with other epoch stats so the next epoch starts clean (in `SettleAccounts` stats reset and/or in `onSetNewValidatorsStage`).
  - Parameterize the grace: add `CollateralParams.DowntimeGraceBlocks` in params; read it when computing the gate.

2) Transfer node must avoid invalid executors

- In the decentralized API, the transfer node should not select executors whose status is `INVALID`.
- Integration point: `decentralized-api/internal/server/public/post_chat_handler.go`, function `getExecutorForRequest`. Filter the returned executor or request only ACTIVE executors from the chain’s `QueryGetRandomExecutor` path (`inference-chain/x/inference/types/query.pb.gw.go` and corresponding keeper/query implementation), ensuring INVALID participants are excluded.

3) Deterministic executor choice and on-chain record of skipped executors

- Executor selection must be deterministic. The transfer node currently seeds requests and selects an executor (see `post_chat_handler.go` where a seed is used and `GetRandomExecutor` is called). We will formalize a deterministic ordering of eligible executors for a given model and context.
- Extend `MsgStartInference` to include a list of `skipped_executors` (addresses and optional reason codes). Update:
  - Proto: `inference-chain/proto/inference/inference/tx.proto` (message `MsgStartInference`).
  - Handler: `inference-chain/x/inference/keeper/msg_server_start_inference.go` to validate that each skipped executor was indeed `INVALID` (or otherwise ineligible) at the time of `StartInference`. The handler should reject starts that claim skipped executors that are not provably invalid on-chain.
- This allows later verification that the assigned executor in `MsgStartInference.AssignedTo` was the first valid executor from the deterministic ordering at that block context, and that skipped ones were legitimately ineligible.

4) Fallback to alternative executor if the primary does not respond (pre-invalidation)

- Before a participant is formally marked `INVALID`, if the transfer node fails to obtain a response from the assigned executor within a defined timeout, it must attempt the next viable executor using the same deterministic ordering. Retry until a response is received or the list is exhausted.
- Integration point: `decentralized-api/internal/server/public/post_chat_handler.go` in the flow that sends the request to the executor and proxies the response. The retry loop should iterate deterministically over eligible executors while appending to the `skipped_executors` that will be included in `MsgStartInference`.
- Alternative finish transaction on-chain: allow the transfer node to submit a finish with an alternative executor if the original missed. Leverage `MsgFinishInference` in `inference-chain/x/inference/keeper/msg_server_finish_inference.go` (and related types in `inference-chain/x/inference/types/tx.pb.go`), enforcing that if `ExecutedBy` differs from `AssignedTo`, it is only accepted (and rewarded) when the original assigned executor has missed (expired) that inference. The original executor must not be prevented from submitting its own `FinishInference`, but rewards should accrue to the alternative executor only when the miss is confirmed.

5) Executor reconciliation with on-chain StartInference and deduped execution

- Executors must track both: (a) all incoming inference HTTP requests they receive, and (b) all on-chain `MsgStartInference` addressed to them (`AssignedTo == executor`).
- If the executor observes an on-chain start for an inference that it never received via HTTP, it must still execute the inference and submit `MsgFinishInference`.
  - The executor reconstructs the effective request from chain data: `MsgStartInference.PromptPayload` contains the canonicalized, seed-modified body created by the transfer node (see `createInferenceStartRequest`).
  - The executor reuses on-chain metadata for `MsgFinishInference`, including `RequestedBy`, `TransferSignature`, and `OriginalPrompt` from `MsgStartInference` when applicable.
- Deduplication contract: execution must be keyed by `inferenceId` so that if an HTTP request subsequently arrives for an already-started on-chain inference, the executor does not start a second run. Instead, it serves the in-progress/already-finished result stream to the requester.
- Integration points:
  - Chain event ingestion: extend the decentralized API event listener (`decentralized-api/internal/event_listener/new_block_dispatcher.go`) to watch new `MsgStartInference` for the local executor address and enqueue missing runs.
  - Execution path: reuse the same model invocation pipeline used by `handleExecutorRequest` (`decentralized-api/internal/server/public/post_chat_handler.go`) but seeded with `PromptPayload` from chain for exact determinism.
  - Finishing: reuse `sendInferenceTransaction` to submit `MsgFinishInference` after completion, ensuring signatures and usage metrics are populated. For transfer-signature dependent fields, read from the corresponding `MsgStartInference` on-chain record.
  - Dedup store: an in-memory (and optionally persistent) map keyed by `inferenceId` to coordinate single execution and multi-client streaming.

References

- Miss recording: `inference-chain/x/inference/module/module.go` (`handleExpiredInference`).
- Threshold test: `inference-chain/x/inference/keeper/accountsettle.go` (`CheckAndPunishForDowntime`, `CheckAndPunishForDowntimeForParticipant`) and `inference-chain/x/inference/calculations/stats.go` (`MissedStatTest`).
- Status invalidation: `inference-chain/x/inference/keeper/msg_server_validation.go` (`calculateStatus`) and `inference-chain/x/inference/keeper/collateral.go` (`CheckAndSlashForInvalidStatus`).
- Start/Finish handlers: `inference-chain/x/inference/keeper/msg_server_start_inference.go`, `inference-chain/x/inference/keeper/msg_server_finish_inference.go`.
- Transfer node selection and transactions: `decentralized-api/internal/server/public/post_chat_handler.go` (`getExecutorForRequest`, request dispatch, `sendInferenceTransaction`).

