# Upgrade Proposal: v0.2.5

This document outlines the proposed changes for on-chain software upgrade v0.2.5. The `Changes` section details the major modifications, and the `Upgrade Plan` section describes the process for applying these changes.

## Upgrade Plan

This PR updates the code for the `api` and `node` services and introduced new service `bridge` for native bridge with etherium. The PR modifies the container versions in `deploy/join/docker-compose.yml`.

The binary versions will be updated via an on-chain upgrade proposal. For more information on the upgrade process, refer to `/docs/upgrades.md`.

Existing participants are **not** required to upgrade their `api` and `node` containers. The updated container versions are intended for new participants who join after the on-chain upgrade is complete.

### Proposed Process
1. Active participants review this proposal on GitHub.
2. Once the PR is approved by a majority, a `v0.2.5` release will be created from this branch, and an on-chain upgrade proposal for this version will be submitted.
3. If the on-chain proposal is approved, this PR will be merged immediately after the upgrade is executed on-chain.

Creating the release from this branch (instead of `main`) minimizes the time that the `/deploy/join/` directory on the `main` branch contains container versions that do not match the on-chain binary versions, ensuring a smoother onboarding experience for new participants.

The `bridge` container can be started any time after upgrade using by:

1. Pulling last changes from `main` branch (after `upgrade-v0.2.5` merged)
```
git pull
```

2. Start
```
source config.env && docker compose up bridge -d
```

It'll take some time to syncronize.

New MLNode container `v3.0.11` is fully compartible with `v3.0.10` and can be updated asyncronously at any time.
Additionally, the version `v3.0.11-blackwell` is introduced for blackwell GPUs (CUDA 12.8+ required).

### Further Proposals

The PR introduces 3 contracts:
- [liquidity pool](inference-chain/contracts/liquidity-pool/)
- [wrapped token](inference-chain/contracts/wrapped-token/)
- [etherium contract](proposals/ethereum-bridge-contact/BridgeContract.sol)

All contract should be proposed after voters by separate proposals.


## Testing

### Testnet

The on-chain upgrade from version `v0.2.4` to `v0.2.5`  has been successfully deployed and verified on the testnet.

We encourage all reviewers to request access to our testnet environment to validate the upgrade. Alternatively, reviewers can test the on-chain upgrade process on their own private testnets.

## Changes
---
### Native Bridge 
Commit: [f7470c1eab3ebdda30dda90b0d81131b7b472a64](https://github.com/gonka-ai/gonka/pull/404/commits/168f7a8652260528c56acb25d918e7be5a19beca).

That commit introduces native bridge for Etherium blockchain and contracts for it's integration. Details can be found [here](bridge.md).


---

### BLS Signature fix
Commit: [f7470c1eab3ebdda30dda90b0d81131b7b472a64](https://github.com/gonka-ai/gonka/pull/404/commits/f7470c1eab3ebdda30dda90b0d81131b7b472a64).



---

### Participant Status Update

---

### Confirmation PoC


---

### New Schedule for `POC_SLOT=true` (noes who serves inference during PoC)

---


### Support Blackwell as MLNode, fixes

---

### Accoung transfer fix

---

### Paginator fix for `GetMembers`

---

### MLNode status check fixes, retry meachanism
