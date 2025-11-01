from __future__ import annotations
import base64
import hashlib
import requests
from bech32 import bech32_decode, bech32_encode, convertbits
from decimal import Decimal

"""
CRITICAL IMPLEMENTATION NOTE: Guardian Enhancement with Refiltering

This module implements epoch group validation and consensus weight calculation
matching the Go implementation in inference-chain/x/inference/module/genesis_guardian_enhancement.go

KEY DISCREPANCY THAT WAS MISSING:
═══════════════════════════════════════════════════════════════════════════════

The original Python implementation of set_consensus_weight() was incomplete:
1. It called _apply_early_network_protection() on validated participants
2. But it did NOT filter out participants with consensus_weight <= 0 after enhancement
3. This caused discrepancies when these zero-power participants remained in the results

The Go code implicitly handles this filtering in SetComputeValidators:
- cosmos-sdk/x/staking/keeper/compute.go lines 116-118:
  ```go
  if res.Power <= 0 {
      continue  // Skip zero or negative power
  }
  ```

FIX IMPLEMENTED:
═══════════════════════════════════════════════════════════════════════════════

After applying guardian enhancement, the Python code now:
1. Filters to keep only participants with consensus_weight > 0
2. Sets consensus_weight = 0 for all other participants
3. This ensures only valid validators are included in the final set
4. When set_consensus_weight() is called again (with filtered participants),
   the enhancement calculation uses the correct participant count and total weights

WHY THIS MATTERS:
═══════════════════════════════════════════════════════════════════════════════

Example: Participants initially have weights [100k, 50k, 30k]
After power capping: [30k, 30k, 30k]
After enhancement: guardians=[40k, 40k, 40k], non-guardian=[30k]

If a non-guardian has validator_key="" (invalid):
- Before fix: Included as 30k in enhancement calculation
- After fix: Excluded (consensus_weight=0), enhancement recalculates with 3 guardians only

This explains why your test script iterates and removes invalid participants -
the enhancement calculation is affected by WHO is in the participant set.

Reference:
- Go Enhancement: calculateEnhancedPower() in genesis_guardian_enhancement.go lines 98-149
- Go Filtering: SetComputeValidators() in cosmos-sdk/x/staking/keeper/compute.go lines 116-118
- Python: set_consensus_weight() now includes filtering after enhancement
"""

ARCHIVE_NODE_URL = "http://204.12.168.157:8000"
BLOCK_COMMIT_FLAG = 2
GENESIS_GUARDIAN_ADDRESSES = [
    "gonkavaloper1y2a9p56kv044327uycmqdexl7zs82fs5lyang5",
    "gonkavaloper1dkl4mah5erqggvhqkpc8j3qs5tyuetgdc59d0v",
    "gonkavaloper1kx9mca3xm8u8ypzfuhmxey66u0ufxhs70mtf0e"
]
GENESIS_GUARDIAN_MULTIPLIER = 0.52
MATURITY_THRESHOLD = 2000000
MAX_INDIVIDUAL_POWER_PERCENTAGE = 0.30  # 30% max power per participant



# API calls & Utils
def _get_block_for_height(
    base_url: str,
    block_height: int
):
    url = f"{base_url}/chain-rpc/block?height={block_height + 1}"
    headers = {
        "x-cosmos-block-height": str(block_height + 1)
    }
    response = requests.get(url, headers=headers)
    return response.json()

def get_params(
    base_url: str,
    block_height: str | int | None = None
):
    headers = {}
    url = f"{base_url}/chain-api/productscience/inference/inference/params"
    if block_height is not None:
        url = f"{url}?height={block_height}"
        headers = {
            "x-cosmos-block-height": str(block_height)
        }
    response = requests.get(url, headers=headers)
    return response.json()

def get_epoch_params(
    base_url: str,
    block_height: str | int | None = None
):
    params = get_params(base_url, block_height)["params"]["epoch_params"]
    total_set_new_validators_delay = int(params["poc_stage_duration"]) + \
        int(params["poc_validation_delay"]) + \
        int(params["poc_validation_duration"]) + \
        int(params["set_new_validators_delay"])

    return {
        "total_set_new_validators_delay": int(total_set_new_validators_delay),
        "params": params
    }


def get_validator_address_from_pubkey(pubkey_base64: str) -> str:
    pubkey_bytes = base64.b64decode(pubkey_base64)
    pubkey_hash = hashlib.sha256(pubkey_bytes).digest()
    validator_address = pubkey_hash[:20].hex().upper()
    
    return validator_address

def get_operator_address_from_account_address(account_address: str, chain_prefix: str = "gonka") -> str:
    _, data = bech32_decode(account_address)
    if data is None:
        raise ValueError(f"Invalid Bech32 address: {account_address}")
    
    decoded_bytes = convertbits(data, 5, 8, False)
    if decoded_bytes is None:
        raise ValueError(f"Failed to convert bits for address: {account_address}")
    
    valoper_prefix = f"{chain_prefix}valoper"
    
    converted_data = convertbits(decoded_bytes, 8, 5)
    if converted_data is None:
        raise ValueError(f"Failed to convert bits back for address: {account_address}")
    
    valoper_address = bech32_encode(valoper_prefix, converted_data)
    
    return valoper_address


def _get_active_participants_full(
    base_url: str,
    epoch_id: str | int | None = None
):
    if epoch_id is None:
        epoch_id = "current"
    url = f"{base_url}/v1/epochs/{epoch_id}/participants"
    response = requests.get(url)
    return response.json()


def get_active_participants(
    base_url: str,
    epoch_id: str | int | None = None
):
    return _get_active_participants_full(base_url, epoch_id).get("active_participants", [])


def get_group_members(
    base_url: str,
    group_id: int,
    block_height: int | None = None
):
    """Query the x/group module directly for group members.
    This is what GetComputeResults actually uses in the Go code."""
    url = f"{base_url}/chain-api/cosmos/group/v1/groups/{group_id}/members"
    headers = {}
    if block_height is not None:
        headers = {"x-cosmos-block-height": str(block_height)}
    response = requests.get(url, headers=headers)
    return response.json().get("members", [])


def get_block_signers_for_height(
    base_url: str,
    block_height: int
):
    block_data = _get_block_for_height(base_url, block_height)   
    last_commit = block_data["result"]["block"]["last_commit"]
    height = last_commit["height"]
    assert str(height) == str(block_height), f"Height mismatch: {height} != {block_height}"
    signatures = last_commit["signatures"]
    commited_signatures = [x for x in signatures if x["block_id_flag"] == BLOCK_COMMIT_FLAG]
    return [
        x["validator_address"]
        for x in commited_signatures
    ]

def get_validators_from_chain(base_url: str, block_height: int):
    headers = {}
    url = f"{base_url}/chain-api/cosmos/staking/v1beta1/validators"
    if block_height is not None:
        url = f"{url}?height={block_height}"
        headers = {
            "x-cosmos-block-height": str(block_height)
        }

    response = requests.get(url, headers=headers)
    validators = response.json()["validators"]  
    return {
        x["operator_address"]: int(x["tokens"])
        for x in validators
    }

########################################################
# Participant & Block & EpochGroup
########################################################


class Block:
    def __init__(
        self,
        height: int,
        signers_addresses: list[str]
    ):
        self.height = height
        self.signers_addresses = signers_addresses

    @staticmethod
    def from_height(
        base_url: str,
        height: int
    ):
        signers_addresses = get_block_signers_for_height(base_url, height)
        return Block(height, signers_addresses)
    
    def __str__(self):
        return f"Block(height={self.height})"

class Participant:
    def __init__(
        self,
        index: str,
        validator_key: str,
        weight: int,
        consensus_weight: int = 0
    ):
        self.index = index
        self.operator_address = get_operator_address_from_account_address(index)
        self.validator_key = validator_key
        # Handle invalid validator_key gracefully (matching Go's continue on error)
        try:
            self.validator_address = get_validator_address_from_pubkey(validator_key) if validator_key else ""
        except Exception:
            self.validator_address = ""
        self.weight = weight
        self.consensus_weight = consensus_weight


    def __str__(self):
        return f"Participant(index={self.index}, weight={self.weight}, consensus_weight={self.consensus_weight})"

class EpochGroup:
    def __init__(
        self,
        epoch_id: str | int,
        participants: list[Participant],
        poc_start_block_height: int,
        created_at_block_height: int,
        created_at_block: Block,
        total_set_new_validators_delay: int
    ):
        self.epoch_id = int(epoch_id)
        self.participants = participants
        self.poc_start_block_height = poc_start_block_height
        self.created_at_block_height = created_at_block_height
        self.created_at_block = created_at_block

        self.set_new_validators_height = poc_start_block_height + total_set_new_validators_delay

    @staticmethod
    def load(base_url: str, epoch_id: str | int | None = None):
        active_participants = get_active_participants(base_url, epoch_id)
        epoch_id = active_participants["epoch_id"]
        created_at_block_height = active_participants["created_at_block_height"]
        poc_start_block_height = active_participants["poc_start_block_height"]
        epoch_params = get_epoch_params(base_url, created_at_block_height - 10)

        created_at_block = Block.from_height(base_url, created_at_block_height)
        participants = []
        for participant in active_participants["participants"]:
            seed = participant.get("seed")
            if seed is None or not seed.get("signature"):
                # Participants without a seed signature never make it into the
                # epoch group membership (Go inserts members via seed processing).
                # Skip them so our compute results mirror on-chain behavior.
                continue
            participants.append(Participant(
                index=participant["index"],
                validator_key=participant.get("validator_key", ""),
                weight=participant.get("weight", 0)
            ))

        participants = set_consensus_weight(participants)
        return EpochGroup(
            epoch_id,
            participants,
            poc_start_block_height,
            created_at_block_height,
            created_at_block,
            int(epoch_params["total_set_new_validators_delay"])
        )

    def get_total_consensus_weight(
        self
    ):
        return sum(participant.consensus_weight for participant in self.participants)

    def signers_total_consensus_weight(
        self,
        signers_addresses: list[str]
    ):
        return sum(participant.consensus_weight for participant in self.participants if participant.validator_address in signers_addresses)

    def __str__(self):
        return f"EpochGroup(epoch_id={self.epoch_id}, poc_start_block_height={self.poc_start_block_height}, set_new_validators_height={self.set_new_validators_height})"

    def get_validators_from_chain(self):
        return get_validators_from_chain(ARCHIVE_NODE_URL, self.set_new_validators_height + 1)


def _apply_power_capping(participants: list[Participant]) -> None:
    if not participants or len(participants) <= 1:
        return
    
    total_weight = sum(p.weight for p in participants)
    if total_weight == 0:
        return
    
    sorted_participants = sorted(enumerate(participants), key=lambda x: x[1].weight)
    participant_count = len(participants)
    max_percentage = Decimal(str(MAX_INDIVIDUAL_POWER_PERCENTAGE))
    
    cap = -1
    sum_prev = 0
    
    for k in range(participant_count):
        _, p = sorted_participants[k]
        current_power = p.weight
        
        weighted_total = sum_prev + current_power * (participant_count - k)
        
        weighted_total_decimal = Decimal(str(weighted_total))
        threshold = max_percentage * weighted_total_decimal
        current_power_decimal = Decimal(str(current_power))
        
        if current_power_decimal > threshold:
            sum_prev_decimal = Decimal(str(sum_prev))
            numerator = max_percentage * sum_prev_decimal
            
            remaining_participants = Decimal(str(participant_count - k))
            max_percentage_times_remaining = max_percentage * remaining_participants
            denominator = Decimal('1') - max_percentage_times_remaining
            
            if denominator <= 0:
                cap = current_power
                break
            
            cap_decimal = numerator / denominator
            cap = int(cap_decimal)
            break
        
        sum_prev += current_power
    
    if cap == -1:
        return
    
    for p in participants:
        if p.weight > cap:
            p.weight = cap


def _apply_early_network_protection(participants: list[Participant]) -> None:
    """
    Apply genesis guardian enhancement to network participants when network is immature.
    
    This directly mirrors the Go code:
    - inference-chain/x/inference/module/genesis_guardian_enhancement.go::ApplyGenesisGuardianEnhancement
    
    CRITICAL BEHAVIOR:
    1. Only applies when network is immature (total_weight < MATURITY_THRESHOLD)
    2. Requires at least 2 participants
    3. Requires at least one genesis guardian present
    4. Distributes enhancement across ALL genesis guardians equally
    5. Non-guardian participants keep their original weights
    
    IMPORTANT: Participants with weight <= 0 after enhancement will be filtered out
    by set_consensus_weight(), so they won't appear in the final validator set.
    This matches the Go code's behavior in SetComputeValidators (compute.go:116-118).
    """
    if not participants:
        return
    
    total_weight = sum(p.weight for p in participants)

    guardian_map = {addr: True for addr in GENESIS_GUARDIAN_ADDRESSES}
    
    guardian_indices = []
    total_guardian_weight = 0
    has_any_guardian = False
    
    for i, p in enumerate(participants):
        if p.operator_address in guardian_map:
            guardian_indices.append(i)
            total_guardian_weight += p.weight
            has_any_guardian = True
    
    network_immature = total_weight < MATURITY_THRESHOLD
    has_min_participants = len(participants) >= 2
    
    if not (network_immature and has_min_participants and has_any_guardian):
        for p in participants:
            p.consensus_weight = p.weight
        return
    
    other_weight = total_weight - total_guardian_weight
    other_weight_decimal = Decimal(str(other_weight))
    multiplier_decimal = Decimal(str(GENESIS_GUARDIAN_MULTIPLIER))
    total_enhancement_decimal = other_weight_decimal * multiplier_decimal
    
    total_guardian_weight_decimal = Decimal(str(total_guardian_weight))
    if total_enhancement_decimal < total_guardian_weight_decimal:
        for p in participants:
            p.consensus_weight = p.weight
        return
    
    guardian_count = len(guardian_indices)
    guardian_count_decimal = Decimal(str(guardian_count))
    per_guardian_weight_decimal = total_enhancement_decimal / guardian_count_decimal
    per_guardian_weight = int(per_guardian_weight_decimal)  # Truncate to integer (matches Go's IntPart())
    
    for i, p in enumerate(participants):
        if i in guardian_indices:
            p.consensus_weight = per_guardian_weight
        else:
            p.consensus_weight = p.weight

def _validate_participant(p: Participant) -> bool:
    if not p.validator_key:
        return False
    try:
        # Must be valid base64 and exactly 32 bytes for ed25519 pubkey (Go uses ed25519.PubKey{Key: bytes})
        decoded = base64.b64decode(p.validator_key, validate=True)
        if len(decoded) != 32:
            return False
        # Account address must be valid bech32
        _, data = bech32_decode(p.index)
        if data is None:
            return False
        return True
    except Exception:
        return False


def set_consensus_weight(
    participants: list[Participant],
    skip_power_capping: bool = False
):
    # Enhancement and capping operate on every participant with positive weight that
    # is part of the epoch group (seeded members). This matches the Go code which
    # runs ApplyPowerCapping/ApplyGenesisGuardianEnhancement against the full
    # active participant set before SetComputeValidators filters out invalid pubkeys.
    eligible_participants = [p for p in participants if p.weight >= 0]
    
    # Apply power capping unless explicitly skipped
    # NOTE: Weights from the API are ALREADY power-capped if loading from epoch group,
    # but may need capping if calculating from raw participant data
    if not skip_power_capping:
        _apply_power_capping(eligible_participants)

    _apply_early_network_protection(eligible_participants)

    # CRITICAL REFILTERING: After enhancement, only participants with a valid
    # validator pubkey and consensus_weight > 0 remain. Others (missing keys,
    # zero power) must zero out so the validator set mirrors SetComputeValidators.
    valid_map = {
        p.index: p
        for p in eligible_participants
        if _validate_participant(p) and p.consensus_weight > 0
    }
    
    for p in participants:
        if p.index in valid_map:
            p.consensus_weight = valid_map[p.index].consensus_weight
        else:
            p.consensus_weight = 0
    
    return participants
