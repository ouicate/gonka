import sys
sys.path.append(".")

base_url_node_1 = "http://204.12.168.157:8000"

from epochs import (
    EpochGroup,
    GENESIS_GUARDIAN_ADDRESSES,
    set_consensus_weight,
    validate_epoch_group,
)

epoch_groups_chain = [EpochGroup.load(base_url_node_1, "current")]



epoch_group = epoch_groups_chain[0]
while True:
    not_matching_participants, missing_participants = validate_epoch_group(epoch_group)

    prev_epoch_group = EpochGroup.load(base_url_node_1, epoch_group.epoch_id - 1)
    epoch_groups_chain.append(prev_epoch_group)
    epoch_group = prev_epoch_group
    if epoch_group.epoch_id == 1:
        break

