RULES. NEVER DELETE

Right now this document is WIP. Assistance should help to:

- Phrase info
- Find info in repo
All found information should be described conciced, clearly and have references to file and function which has this logic

---

# How it works now

The Gonka network implements custom fraud detection and punishment mechanisms in addition to standard Cosmos SDK slashing. This document describes these mechanisms.

## Offenses, Detection, and Penalties

The Gonka network penalizes participants for several offenses to ensure network integrity and performance.

### 1. Invalid Inferences

- **Description**: Submitting incorrect inference results.
- **Detection**: Each inference is validated by some of majorty with probaility based on executor reputation (sampling verifiable). If validator suspects fraud - it calls for consensus. Final decision if inderene is corrent or not is obtained form consensus. History of participant's inference results are checked by stat test and made decision if it was fraud. In that case, participant status is changed to `INVALID`. This happens if their inferences meet either of the following criteria (`x/inference/keeper/msg_server_validation.go:calculateStatus()`):
    - **Consecutive Failures**: The probability of their consecutive validation failures is less than 0.000001 (e.g., 5 consecutive failures with a 5% false positive rate).
    - **Statistical Anomaly**: Their Z-score (actual vs. expected failure rate) is greater than 1.0, after at least 10 samples.
- **Penalties**:
    - **Collateral Slash**: 20% of the participant's collateral is burned (`x/inference/keeper/collateral.go:CheckAndSlashForInvalidStatus()`).
    - **Reward Forfeiture**: All rewards for the epoch are forfeited, as work from `INVALID` participants is not counted during reward calculation (`x/inference/keeper/accountsettle.go:getSettleAmount()`).
    - **Immediate Exclusion**: The participant is immediately removed from the current epoch's active set, revoking their ability to participate in consensus and receive work for the remainder of the epoch (`x/inference/epochgroup/epoch_group.go:UpdateMember()`).

- **Notes**:
    - Q1: is INVALID status reset after PoC? or not continue to be invalid?
    - P1: Limit on who can submit call for concensus and how often is required
    - P2: Don't affect reputation, only EpochsCompleted not incremented (might be okay if INVALID is permanent?)

### 2. Downtime

- **Description**: Inference are scheduled to node but not executed. Detected as TA records `MsgStartInference` TX. Currently participants must be available for at least 95% of transferred request.
- **Detection**: The system tracks missed requests in two ways:
    - **Epoch-End Slashing Check**: At the end of each epoch, checks if a participant missed more than 5% of their assigned inference requests (`x/inference/keeper/collateral.go:CheckAndSlashForDowntime()`). The calculation is `missedPercentage = MissedRequests / (InferenceCount + MissedRequests)`. No penalty is applied if the participant had no work assigned.
    - **Reputation Impact**: Tracks miss percentages per epoch and applies cumulative penalties to reputation score (`x/inference/module/module.go:calculateParticipantReputation()`, `x/inference/calculations/reputation.go:CalculateReputation()`). Penalties apply when missed request rate exceeds 1% (`MissPercentageCutoff` parameter).
- **Penalties**:
    - **Collateral Slash**: 10% of the participant's collateral is burned (applied when downtime threshold of 5% is exceeded) (`x/inference/keeper/collateral.go:CheckAndSlashForDowntime()`).
    - **Reputation Score Reduction**: The participant's reputation score (0-100) is reduced using a complex formula that subtracts "miss cost" from their effective epoch count (`x/inference/calculations/reputation.go:CalculateReputation()`). The calculation applies penalties only for miss rates above the 1% threshold, with penalties scaled by `MissRequestsPenalty` (default 1.0).
    - **Increased Validation Scrutiny**: Lower reputation scores result in more frequent validation checks. The system uses reputation to calculate validation probability (`x/inference/calculations/should_validate.go:ShouldValidate()`): participants with 0% reputation get maximum validation frequency (100% of their work is validated), while those with 100% reputation get minimum validation frequency (as low as 1% of their work is validated, depending on network traffic).

- **Notes**:
    - Q1: The formula for calculating miss percentage differs between slashing and reputation. For slashing, the calculation is `missedPercentage = MissedRequests / (InferenceCount + MissedRequests)`, which considers all assigned work. For reputation, the calculation is `missedPercentage = MissedRequests / InferenceCount`


### 3. Missed Validations

- **Description**: Failing to validate inferences assigned by the network.
- **Detection**: During the reward claim process, the system checks if the participant has submitted validations for all inferences assigned to them in that epoch (`x/inference/keeper/msg_server_claim_rewards.go:hasMissedValidations()`).
- **Penalties**:
    - **Reward Forfeiture**: Prevents the participant from claiming their reward for the epoch until all assigned validations have been submitted (`x/inference/keeper/msg_server_claim_rewards.go:validateClaim()`). This does not change the participant's status to `INVALID`.

- **Notes**:
    - P1: Strict check is used, should be stat test also 
    - P2: The reward claim is declined, not permanently forfeited. A participant can submit missing validations later and then successfully re-claim the rewards for that epoch.
    - P3: Doesn't affect reputation 
    - P4: Doesn't affect collateral 
 
### 4. Failed Proof-of-Compute (PoC) Consensus

- **Description**: Failing to achieve a supermajority consensus on Proof-of-Compute (PoC) submissions.
- **Detection**: The network verifies that the total weight of validators approving the PoC submission **exceeds 50%** of the total network weight. Submissions with 50% or less approval fail (`x/inference/module/chainvalidation.go:pocValidated()`).
- **Penalties**:
    - **Exclusion from Work**: If a participant fails to achieve PoC consensus, they are removed from the active set for the next epoch, effectively excluding them from new work assignments and consensus influence (`x/inference/module/chainvalidation.go:ComputeNewWeights()`). No collateral is slashed for this offense.

- **Notes**:
    - P1: Doesn't affect reputation 
    - P2: Doesn't affect collateral 

### 5. [Not relevant]: Block Generation Downtime

- **Description**: Validator downtime in block production and consensus participation (standard Cosmos SDK slashing).
- **Note**: This is outside the scope of this document as it covers standard Cosmos SDK validator slashing but with reducing weight instead of stake.

## Implementation Details

### How Status Updates Work

A participant's status is re-evaluated each time one of their executed inferences is validated. The process is as follows:

1.  A validator submits a validation for an inference.
2.  - If the validation **passes**, the executor's `ConsecutiveInvalidInferences` counter is reset to zero.
    - If the validation **fails**, a re-validation consensus vote is initiated among the epoch group members. If this vote also fails, the inference is marked `INVALIDATED`, and the executor's failure counters are incremented.
3.  After the executor's performance statistics are updated, the system recalculates their status based on statistical analysis (see Invalid Inference Detection).
4.  If the participant's status changes to `INVALID` as a result of this recalculation, a 20% collateral slash is immediately triggered.

### Validator Assignment

The network assigns validators to inferences using a deterministic algorithm based on:

- The executor's reputation score.
- The validator's network power.
- A cryptographically secure random seed.


# Proposal

