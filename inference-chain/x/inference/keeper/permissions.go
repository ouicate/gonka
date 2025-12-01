package keeper

import (
	"context"
	"reflect"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/productscience/inference/x/inference/types"
)

type Permission string

const (
	GovernancePermission               Permission = "governance"
	TrainingExecPermission             Permission = "training_execution"
	TrainingStartPermission            Permission = "training_start"
	ParticipantPermission              Permission = "participant"
	ActiveParticipantPermission        Permission = "active_participant"
	AccountPermission                  Permission = "account"
	CurrentActiveParticipantPermission Permission = "current_active_participant"
)

var MessagePermissions = map[reflect.Type][]Permission{
	reflect.TypeOf((*types.MsgUpdateParams)(nil)):                    {GovernancePermission},
	reflect.TypeOf((*types.MsgSetTrainingAllowList)(nil)):            {GovernancePermission},
	reflect.TypeOf((*types.MsgAddUserToTrainingAllowList)(nil)):      {GovernancePermission},
	reflect.TypeOf((*types.MsgRemoveUserFromTrainingAllowList)(nil)): {GovernancePermission},
	reflect.TypeOf((*types.MsgApproveBridgeTokenForTrading)(nil)):    {GovernancePermission},
	reflect.TypeOf((*types.MsgCreatePartialUpgrade)(nil)):            {GovernancePermission},
	reflect.TypeOf((*types.MsgMigrateAllWrappedTokens)(nil)):         {GovernancePermission},
	reflect.TypeOf((*types.MsgRegisterBridgeAddresses)(nil)):         {GovernancePermission},
	reflect.TypeOf((*types.MsgRegisterLiquidityPool)(nil)):           {GovernancePermission},
	reflect.TypeOf((*types.MsgRegisterModel)(nil)):                   {GovernancePermission},
	reflect.TypeOf((*types.MsgRegisterTokenMetadata)(nil)):           {GovernancePermission},
	reflect.TypeOf((*types.MsgSetTrainingAllowList)(nil)):            {GovernancePermission},

	reflect.TypeOf((*types.MsgBridgeExchange)(nil)):          {AccountPermission},
	reflect.TypeOf((*types.MsgRequestBridgeMint)(nil)):       {AccountPermission},
	reflect.TypeOf((*types.MsgRequestBridgeWithdrawal)(nil)): {AccountPermission},

	reflect.TypeOf((*types.MsgSubmitNewParticipant)(nil)):         {},
	reflect.TypeOf((*types.MsgSubmitNewUnfundedParticipant)(nil)): {},

	reflect.TypeOf((*types.MsgClaimRewards)(nil)):                     {ParticipantPermission},
	reflect.TypeOf((*types.MsgSubmitHardwareDiff)(nil)):               {ParticipantPermission},
	reflect.TypeOf((*types.MsgSubmitPocBatch)(nil)):                   {ParticipantPermission},
	reflect.TypeOf((*types.MsgSubmitPocValidation)(nil)):              {ParticipantPermission},
	reflect.TypeOf((*types.MsgSubmitSeed)(nil)):                       {ParticipantPermission},
	reflect.TypeOf((*types.MsgSubmitUnitOfComputePriceProposal)(nil)): {ParticipantPermission},

	reflect.TypeOf((*types.MsgSubmitTrainingKvRecord)(nil)):         {TrainingExecPermission},
	reflect.TypeOf((*types.MsgJoinTraining)(nil)):                   {TrainingExecPermission},
	reflect.TypeOf((*types.MsgJoinTrainingStatus)(nil)):             {TrainingExecPermission},
	reflect.TypeOf((*types.MsgSetBarrier)(nil)):                     {TrainingExecPermission},
	reflect.TypeOf((*types.MsgTrainingHeartbeat)(nil)):              {TrainingExecPermission},
	reflect.TypeOf((*types.MsgAssignTrainingTask)(nil)):             {TrainingStartPermission},
	reflect.TypeOf((*types.MsgClaimTrainingTaskForAssignment)(nil)): {TrainingStartPermission},
	reflect.TypeOf((*types.MsgCreateDummyTrainingTask)(nil)):        {TrainingStartPermission},
	reflect.TypeOf((*types.MsgCreateTrainingTask)(nil)):             {TrainingStartPermission},

	reflect.TypeOf((*types.MsgFinishInference)(nil)):     {ActiveParticipantPermission},
	reflect.TypeOf((*types.MsgInvalidateInference)(nil)): {ActiveParticipantPermission},
	reflect.TypeOf((*types.MsgRevalidateInference)(nil)): {ActiveParticipantPermission},
	reflect.TypeOf((*types.MsgStartInference)(nil)):      {ActiveParticipantPermission},

	reflect.TypeOf((*types.MsgValidation)(nil)): {ActiveParticipantPermission},
}

func (k msgServer) CheckPermission(ctx context.Context, msg sdk.Msg, actor string) error {
	actorAddr, err := sdk.AccAddressFromBech32(actor)
	if err != nil {
		return err
	}
	permission, ok := MessagePermissions[reflect.TypeOf(msg)]
	if !ok {
		k.LogError("Permission not found for message type", types.Messages, "message type", reflect.TypeOf(msg))
		return types.ErrInvalidPermission
	}

	for _, perm := range permission {
		switch perm {
		case GovernancePermission:
			if err := k.checkGovernancePermission(ctx, actorAddr); err != nil {
				return err
			}
		case AccountPermission:
			if err := k.checkAccountPermission(ctx, actorAddr); err != nil {
				return err
			}
		case ParticipantPermission:
			if err := k.checkParticipantPermission(ctx, actorAddr); err != nil {
				return err
			}
		case ActiveParticipantPermission:
			if err := k.checkActiveParticipantPermission(ctx, actorAddr); err != nil {
				return err
			}
		case TrainingExecPermission:
			if err := k.checkTrainingExecPermission(ctx, actorAddr); err != nil {
				return err
			}
		case TrainingStartPermission:
			if err := k.checkTrainingStartPermission(ctx, actorAddr); err != nil {
				return err
			}
		case CurrentActiveParticipantPermission:
			if err := k.checkCurrentActiveParticipantPermission(ctx, actorAddr); err != nil {
				return err
			}
		default:
			return types.ErrInvalidPermission
		}
	}
	return nil

}

func (k msgServer) checkAccountPermission(ctx context.Context, actor sdk.AccAddress) error {
	acc := k.AccountKeeper.GetAccount(ctx, actor)
	if acc == nil {
		return types.ErrAccountNotFound
	}
	return nil
}

func (k msgServer) checkParticipantPermission(ctx context.Context, actor sdk.AccAddress) error {
	_, err := k.Participants.Get(ctx, actor)
	return err
}

func (k msgServer) checkActiveParticipantPermission(ctx context.Context, actor sdk.AccAddress) error {
	currentEpoch, err := k.EffectiveEpochIndex.Get(ctx)
	if err != nil {
		return err
	}
	p, found := k.GetActiveParticipants(ctx, currentEpoch)
	if !found {
		return types.ErrParticipantNotFound
	}
	for _, participant := range p.Participants {
		if participant.Index == actor.String() {
			return nil
		}
	}
	return types.ErrParticipantNotFound
}

func (k msgServer) checkCurrentActiveParticipantPermission(ctx context.Context, actor sdk.AccAddress) error {
	err := k.checkActiveParticipantPermission(ctx, actor)
	if err != nil {
		return err
	}
	currentEpoch, err := k.EffectiveEpochIndex.Get(ctx)
	if err != nil {
		return err
	}
	has, err := k.ExcludedParticipantsMap.Has(ctx, collections.Join(currentEpoch, actor))
	if err != nil {
		return err
	}
	if has {
		return types.ErrParticipantNotFound
	}
	return nil
}

func (k msgServer) checkTrainingExecPermission(ctx context.Context, actor sdk.AccAddress) error {
	allowed, err := k.TrainingExecAllowListSet.Has(ctx, actor)
	if err != nil {
		return err
	}
	if !allowed {
		return types.ErrTrainingNotAllowed
	}
	return nil
}

func (k msgServer) checkTrainingStartPermission(ctx context.Context, actor sdk.AccAddress) error {
	allowed, err := k.TrainingStartAllowListSet.Has(ctx, actor)
	if err != nil {
		return err
	}
	if !allowed {
		return types.ErrTrainingNotAllowed
	}
	return nil
}

func (k msgServer) checkGovernancePermission(ctx context.Context, actor sdk.AccAddress) error {
	if k.GetAuthority() != actor.String() {
		return types.ErrInvalidSigner
	}
	return nil
}
