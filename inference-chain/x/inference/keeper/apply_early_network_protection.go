package keeper

import (
	"context"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/productscience/inference/x/inference/types"
)

// ApplyEarlyNetworkProtection applies genesis guardian enhancement to compute results before validator set updates
// This system only applies when network is immature (below maturity threshold)
func (k Keeper) ApplyEarlyNetworkProtection(ctx context.Context, computeResults []stakingkeeper.ComputeResult) []stakingkeeper.ComputeResult {
	// Apply genesis guardian enhancement (only when network immature)
	result := k.ApplyGenesisGuardianEnhancement(ctx, computeResults)

	// Log enhancement application results
	originalTotal := int64(0)
	for _, cr := range computeResults {
		originalTotal += cr.Power
	}

	if result.WasEnhanced {
		genesisGuardianAddresses := k.GetGenesisGuardianAddresses(ctx)

		// Count enhanced guardians and calculate their individual powers
		enhancedGuardians := []string{}
		guardianPowers := []int64{}
		guardianAddressMap := make(map[string]bool)
		for _, address := range genesisGuardianAddresses {
			guardianAddressMap[address] = true
		}

		for _, cr := range result.ComputeResults {
			if guardianAddressMap[cr.OperatorAddress] {
				enhancedGuardians = append(enhancedGuardians, cr.OperatorAddress)
				guardianPowers = append(guardianPowers, cr.Power)
			}
		}

		k.LogInfo("Genesis guardian enhancement applied to staking powers", types.EpochGroup,
			"originalTotalPower", originalTotal,
			"enhancedTotalPower", result.TotalPower,
			"participantCount", len(computeResults),
			"guardianCount", len(enhancedGuardians),
			"enhancedGuardians", enhancedGuardians,
			"guardianPowers", guardianPowers)
	} else {
		genesisGuardianAddresses := k.GetGenesisGuardianAddresses(ctx)
		k.LogInfo("Genesis guardian enhancement evaluated but not applied to staking powers", types.EpochGroup,
			"totalPower", originalTotal,
			"participantCount", len(computeResults),
			"configuredGuardianCount", len(genesisGuardianAddresses),
			"reason", "network mature, insufficient participants, or no genesis guardians found")
	}
	return result.ComputeResults
}
