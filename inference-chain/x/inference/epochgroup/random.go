package epochgroup

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/cosmos/cosmos-sdk/x/group"
	"github.com/productscience/inference/x/inference/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetRandomMemberForModel gets a random member for a specific model
func (eg *EpochGroup) GetDeterministicRandomMemberForModel(
	goCtx context.Context,
	modelId string,
	filterFn func([]*group.GroupMember) []*group.GroupMember,
	seed string,
) (*types.Participant, []string, error) {
	// If modelId is provided and this is the parent group, delegate to the sub-group
	if modelId != "" && eg.GroupData.GetModelId() == "" {
		subGroup, err := eg.GetSubGroup(goCtx, modelId)
		if err != nil {
			return nil, nil, status.Error(codes.Internal, fmt.Sprintf("Error getting sub-group for model %s: %v", modelId, err))
		}
		return subGroup.getDeterministicRandomMember(goCtx, filterFn, seed)
	}

	// Otherwise, get a random member from this group
	return eg.getDeterministicRandomMember(goCtx, filterFn, seed)
}

// GetRandomMemberForModel is a backward-compatible wrapper that preserves the original
// signature used in tests. It delegates to the deterministic selector with an empty seed.
func (eg *EpochGroup) GetRandomMemberForModel(
	goCtx context.Context,
	modelId string,
	filterFn func([]*group.GroupMember) []*group.GroupMember,
) (*types.Participant, error) {
	p, _, err := eg.GetDeterministicRandomMemberForModel(goCtx, modelId, filterFn, "")
	return p, err
}

func (eg *EpochGroup) getDeterministicRandomMember(
	goCtx context.Context,
	filterFn func([]*group.GroupMember) []*group.GroupMember,
	seed string,
) (*types.Participant, []string, error) {
	// Use the context as is, don't try to unwrap it
	// This allows the method to work with both SDK contexts and regular contexts
	ctx := goCtx

	groupMemberResponse, err := eg.GroupKeeper.GroupMembers(ctx, &group.QueryGroupMembersRequest{GroupId: uint64(eg.GroupData.EpochGroupId)})
	if err != nil {
		return nil, nil, status.Error(codes.Internal, err.Error())
	}
	activeParticipants := groupMemberResponse.GetMembers()
	if len(activeParticipants) == 0 {
		return nil, nil, status.Error(codes.Internal, "Active participants found, but length is 0")
	}

	filteredParticipants := filterFn(activeParticipants)
	if len(filteredParticipants) == 0 {
		return nil, nil, status.Error(codes.Internal, "After filtering participants the length is 0")
	}

	// Try randomly selecting until we find an ACTIVE participant; remove non-ACTIVE selections and retry
	candidates := make([]*group.GroupMember, 0, len(filteredParticipants))
	candidates = append(candidates, filteredParticipants...)
	skipped := make([]string, 0)

	// Seeded RNG for determinism
	var rng *rand.Rand
	if seed != "" {
		rng = rand.New(rand.NewSource(seedFromString(seed)))
	}

	for len(candidates) > 0 {
		candidateAddress := selectRandomParticipant(candidates, rng)
		participant, ok := eg.ParticipantKeeper.GetParticipant(ctx, candidateAddress)
		if !ok {
			// Remove this candidate and retry
			candidates = removeGroupMemberByAddress(candidates, candidateAddress)
			skipped = append(skipped, candidateAddress)
			continue
		}
		if participant.Status == types.ParticipantStatus_ACTIVE {
			return &participant, skipped, nil
		}
		// Not ACTIVE; remove and retry
		candidates = removeGroupMemberByAddress(candidates, candidateAddress)
		skipped = append(skipped, candidateAddress)
	}

	return nil, skipped, status.Error(codes.Internal, "No ACTIVE participant available after random selection retries")
}

func selectRandomParticipant(participants []*group.GroupMember, rng *rand.Rand) string {
	cumulativeArray := computeCumulativeArray(participants)

	var randomNumber int64
	if rng != nil {
		randomNumber = rng.Int63n(cumulativeArray[len(cumulativeArray)-1])
	} else {
		randomNumber = rand.Int63n(cumulativeArray[len(cumulativeArray)-1])
	}
	for i, cumulativeWeight := range cumulativeArray {
		if randomNumber < cumulativeWeight {
			return participants[i].Member.Address
		}
	}

	return participants[len(participants)-1].Member.Address
}

// seedFromString derives a stable, non-negative int64 seed from a string.
func seedFromString(s string) int64 {
	sum := sha256.Sum256([]byte(s))
	u := binary.LittleEndian.Uint64(sum[:8])
	return int64(u & 0x7fffffffffffffff)
}

func computeCumulativeArray(participants []*group.GroupMember) []int64 {
	cumulativeArray := make([]int64, len(participants))
	cumulativeArray[0] = int64(getWeight(participants[0]))
	for i := 1; i < len(participants); i++ {
		cumulativeArray[i] = cumulativeArray[i-1] + getWeight(participants[i])
	}
	return cumulativeArray
}

func getWeight(participant *group.GroupMember) int64 {
	weight, err := strconv.Atoi(participant.Member.Weight)
	if err != nil {
		return 0
	}
	return int64(weight)
}

// removeGroupMemberByAddress removes the first occurrence of a member with the given address
// from the provided slice and returns the updated slice.
func removeGroupMemberByAddress(members []*group.GroupMember, address string) []*group.GroupMember {
	for i, m := range members {
		if m != nil && m.Member != nil && m.Member.Address == address {
			// Remove index i without preserving order
			members[i] = members[len(members)-1]
			return members[:len(members)-1]
		}
	}
	return members
}
