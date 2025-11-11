package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/productscience/inference/x/inference/types"
)

type SettleParameters struct {
	CurrentSubsidyPercentage float32 `json:"current_subsidy_percentage"`
	TotalSubsidyPaid         int64   `json:"total_subsidy_paid"`
	StageCutoff              float64 `json:"stage_cutoff"`
	StageDecrease            float32 `json:"stage_decrease"`
	TotalSubsidySupply       int64   `json:"total_subsidy_supply"`
}

func (k *Keeper) GetSettleParameters(ctx context.Context) (*SettleParameters, error) {
	params, err := k.GetParamsSafe(ctx)
	if err != nil {
		return nil, err
	}
	tokenomicsData, found := k.GetTokenomicsData(ctx)
	if !found {
		return nil, fmt.Errorf("tokenomics data not found")
	}
	genesisOnlyParams, found := k.GetGenesisOnlyParams(ctx)
	if !found {
		return nil, fmt.Errorf("genesis only params not found")
	}
	normalizedTotalSuply := sdk.NormalizeCoin(sdk.NewInt64Coin(genesisOnlyParams.SupplyDenom, genesisOnlyParams.StandardRewardAmount))
	return &SettleParameters{
		// TODO: Settle Parameters should just use (our) Decimal
		CurrentSubsidyPercentage: params.TokenomicsParams.CurrentSubsidyPercentage.ToFloat32(),
		TotalSubsidyPaid:         int64(tokenomicsData.TotalSubsidies),
		StageCutoff:              params.TokenomicsParams.SubsidyReductionInterval.ToFloat(),
		StageDecrease:            params.TokenomicsParams.SubsidyReductionAmount.ToFloat32(),
		TotalSubsidySupply:       normalizedTotalSuply.Amount.Int64(),
	}, nil
}

func (k *Keeper) SettleAccounts(ctx context.Context, currentEpochIndex uint64, previousEpochIndex uint64) error {
	if currentEpochIndex == 0 {
		k.LogInfo("SettleAccounts Skipped For Epoch 0", types.Settle, "currentEpochIndex", currentEpochIndex, "skipping")
		return nil
	}

	k.LogInfo("SettleAccounts", types.Settle, "currentEpochIndex", currentEpochIndex)
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	blockHeight := sdkCtx.BlockHeight()
	allParticipants := k.GetAllParticipant(ctx)

	k.LogInfo("Block height", types.Settle, "height", blockHeight)
	k.LogInfo("Got all participants", types.Settle, "participants", len(allParticipants))

	data, found := k.GetEpochGroupData(ctx, currentEpochIndex, "")
	k.LogInfo("Settling for block", types.Settle, "height", currentEpochIndex)
	if !found {
		k.LogError("Epoch group data not found", types.Settle, "height", currentEpochIndex)
		return types.ErrCurrentEpochGroupNotFound
	}
	seedSigMap := make(map[string]string)
	for _, seedSig := range data.MemberSeedSignatures {
		seedSigMap[seedSig.MemberAddress] = seedSig.Signature
	}

	// Check governance flag to determine which reward system to use
	params, err := k.GetParamsSafe(ctx)
	if err != nil {
		k.LogError("Error getting params", types.Settle, "error", err)
		return err
	}
	var amounts []*SettleResult
	var rewardAmount int64
	settleParameters, err := k.GetSettleParameters(ctx)
	if err != nil {
		k.LogError("Error getting settle parameters", types.Settle, "error", err)
		return err
	}
	k.LogInfo("Settle parameters", types.Settle, "parameters", settleParameters)

	// Use Bitcoin-style fixed reward system with its own parameters
	k.LogInfo("Using Bitcoin-style reward system", types.Settle)
	amounts, bitcoinResult, err := GetBitcoinSettleAmounts(allParticipants, &data, params.BitcoinRewardParams, settleParameters, k.Logger())
	if err != nil {
		k.LogError("Error getting Bitcoin settle amounts", types.Settle, "error", err)
	}
	if bitcoinResult.Amount < 0 {
		k.LogError("Bitcoin reward amount is negative", types.Settle, "amount", bitcoinResult.Amount)
		return types.ErrNegativeRewardAmount
	}
	k.LogInfo("Bitcoin reward amount", types.Settle, "amount", bitcoinResult.Amount)
	rewardAmount = bitcoinResult.Amount

	err = k.MintRewardCoins(ctx, rewardAmount, "reward_distribution")
	if err != nil {
		k.LogError("Error minting reward coins", types.Settle, "error", err)
		return err
	}
	k.AddTokenomicsData(ctx, &types.TokenomicsData{TotalSubsidies: uint64(rewardAmount)})

	k.LogInfo("Checking downtime for participants", types.Settle, "participants", len(allParticipants))

	for i, participant := range allParticipants {
		// amount should have the same order as participants
		amount := amounts[i]

		if participant.Status == types.ParticipantStatus_ACTIVE {
			participant.EpochsCompleted += 1
		}
		k.SafeLogSubAccountTransaction(ctx, types.ModuleName, participant.Address, "balance", participant.CoinBalance, "settling")
		participant.CoinBalance = 0
		participant.CurrentEpochStats.EarnedCoins = 0
		k.LogInfo("Participant CoinBalance reset", types.Balances, "address", participant.Address)
		epochPerformance := types.EpochPerformanceSummary{
			EpochIndex:            currentEpochIndex,
			ParticipantId:         participant.Address,
			InferenceCount:        participant.CurrentEpochStats.InferenceCount,
			MissedRequests:        participant.CurrentEpochStats.MissedRequests,
			EarnedCoins:           amount.Settle.WorkCoins,
			RewardedCoins:         amount.Settle.RewardCoins,
			ValidatedInferences:   participant.CurrentEpochStats.ValidatedInferences,
			InvalidatedInferences: participant.CurrentEpochStats.InvalidatedInferences,
			Claimed:               false,
		}
		err = k.SetEpochPerformanceSummary(ctx, epochPerformance)
		if err != nil {
			return err
		}
		participant.CurrentEpochStats = types.NewCurrentEpochStats()
		err := k.SetParticipant(ctx, participant)
		if err != nil {
			return err
		}
	}

	for _, amount := range amounts {
		// TODO: Check if we have to store 0 or error settle amount as well, as it store seed signature, which we may use somewhere
		if amount.Error != nil {
			k.LogError("Error calculating settle amounts", types.Settle, "error", amount.Error, "participant", amount.Settle.Participant)
			continue
		}
		totalPayment := amount.Settle.WorkCoins + amount.Settle.RewardCoins
		if totalPayment == 0 {
			k.LogDebug("No payment needed for participant", types.Settle, "address", amount.Settle.Participant)
			continue
		}

		seedSignature, found := seedSigMap[amount.Settle.Participant]
		if found {
			amount.Settle.SeedSignature = seedSignature
		}

		amount.Settle.EpochIndex = currentEpochIndex
		k.LogInfo("Settle for participant", types.Settle, "rewardCoins", amount.Settle.RewardCoins, "workCoins", amount.Settle.WorkCoins, "address", amount.Settle.Participant)
		k.SetSettleAmountWithBurn(ctx, *amount.Settle)
	}

	if previousEpochIndex == 0 {
		return nil
	}

	k.LogInfo("Burning old settle amounts", types.Settle, "previousEpochIndex", previousEpochIndex)
	err = k.BurnOldSettleAmounts(ctx, previousEpochIndex)
	if err != nil {
		k.LogError("Error burning old settle amounts", types.Settle, "error", err)
	}
	return nil
}

type SettleResult struct {
	Settle *types.SettleAmount
	Error  error
}
