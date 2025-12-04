package keeper

import (
	"context"

	"github.com/productscience/inference/x/inference/types"
)

func (k msgServer) SubmitSeed(ctx context.Context, msg *types.MsgSubmitSeed) (*types.MsgSubmitSeedResponse, error) {
	if err := k.CheckPermission(ctx, msg, ParticipantPermission); err != nil {
		return nil, err
	}
	currentEpoch, found := k.GetEffectiveEpochIndex(ctx)
	if !found {
		return nil, types.ErrEffectiveEpochNotFound
	}

	upcomingEpoch, found := k.GetUpcomingEpochIndex(ctx)

	if msg.EpochIndex != currentEpoch && (msg.EpochIndex != upcomingEpoch || upcomingEpoch == 0) {
		return nil, types.ErrEpochNotFound
	}

	seed := types.RandomSeed{
		Participant: msg.Creator,
		EpochIndex:  msg.EpochIndex,
		Signature:   msg.Signature,
	}

	k.SetRandomSeed(ctx, seed)

	return &types.MsgSubmitSeedResponse{}, nil
}
