import sys
sys.path.append(".")

base_url_node_1 = "http://204.12.168.157:8000"

from epochs import (
    EpochGroup,
    GENESIS_GUARDIAN_ADDRESSES,
    set_consensus_weight,
)

epoch_groups_chain = [EpochGroup.load(base_url_node_1, "current")]

def validate_epoch_group(epoch_group: EpochGroup):
    valiators_from_chain = epoch_group.get_validators_from_chain()
    total_consensus_weight = epoch_group.get_total_consensus_weight()
    total_consensus_weight_on_chain = sum(valiators_from_chain.values())
    # assert total_consensus_weight_on_chain <= total_consensus_weight, f"Total consensus weight on chain {total_consensus_weight_on_chain} is greater than total consensus weight {total_consensus_weight}"
    
    print(f"Epoch {epoch_group.epoch_id:3d} | {len(epoch_group.participants):3d} participants | {total_consensus_weight:6d} | {total_consensus_weight_on_chain:6d}")
    missing_participants = []
    not_matching_participants = []
    for participant in epoch_group.participants:
        if participant.operator_address not in valiators_from_chain:
            missing_participants.append((participant.operator_address, participant.weight))
            continue

        validator_voting_power = valiators_from_chain[participant.operator_address]

        if int(validator_voting_power) != participant.consensus_weight:

            not_matching_participants.append((participant.operator_address, participant.consensus_weight, validator_voting_power))
    
    if missing_participants:
        print(f"  {len(missing_participants)} not found on chain:")
        for addr, weight in missing_participants:
            print(f"    - {addr} | {weight:5d}")
    if not_matching_participants:
        print(f"  {len(not_matching_participants)} not matching:")
        for addr, weight, validator_voting_power in not_matching_participants:
            is_guardian = addr in GENESIS_GUARDIAN_ADDRESSES
            if not is_guardian:
                continue
            print(f"    - guardian {addr} {weight} != {validator_voting_power}")

        for addr, weight, validator_voting_power in not_matching_participants:
            is_guardian = addr in GENESIS_GUARDIAN_ADDRESSES
            if is_guardian:
                continue
            print(f"    - {addr} {weight} != {validator_voting_power}")

    return not_matching_participants, missing_participants

epoch_group = epoch_groups_chain[0]
while True:
    not_matching_participants, missing_participants = validate_epoch_group(epoch_group)

    if not_matching_participants:
        print("REPEATING EPOCH GROUP")
        participants_to_remove = [addr for addr, _, _ in not_matching_participants if addr not in GENESIS_GUARDIAN_ADDRESSES] + [addr for addr, _ in missing_participants]
        print(f"Removing {len(participants_to_remove)} participants: {participants_to_remove}")
        epoch_group.participants = [p for p in epoch_group.participants if p.operator_address not in participants_to_remove]
        epoch_group.participants = set_consensus_weight(epoch_group.participants)
        validate_epoch_group(epoch_group)
        print("VALIDATED EPOCH GROUP")

    prev_epoch_group = EpochGroup.load(base_url_node_1, epoch_group.epoch_id - 1)
    epoch_groups_chain.append(prev_epoch_group)
    epoch_group = prev_epoch_group
    if epoch_group.epoch_id == 1:
        break

