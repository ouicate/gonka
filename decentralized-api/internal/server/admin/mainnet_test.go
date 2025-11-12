package admin

import (
	"context"
	"decentralized-api/cosmosclient"
	"decentralized-api/internal/utils"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	rpcclient "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/gonka-ai/gonka-utils/go/contracts"
	externalutils "github.com/gonka-ai/gonka-utils/go/utils"
	"github.com/productscience/common"
	keepertest "github.com/productscience/inference/testutil/keeper"
	"github.com/productscience/inference/x/inference/epochgroup"
	"github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/assert"
	"strconv"
	"strings"
	"testing"
)

var activeParticipantsEpoch0Mainnet = types.ActiveParticipants{
	Participants: []*types.ActiveParticipant{
		{
			Index:        "gonka1y2a9p56kv044327uycmqdexl7zs82fs5ryv5le",
			ValidatorKey: "OPwcpfQYOoWDuHKYivRVq5jxrELH0moP5qdznvj3Dps=",
			Weight:       1,
			InferenceUrl: "http://node1.gonka.ai:8000",
		},
		{
			Index:        "gonka1p2lhgng7tcqju7emk989s5fpdr7k2c3ek6h26m",
			ValidatorKey: "LLqBxOz+vD3p7sQsdEhBfrFH2QFMjy3fMasB9yBGSqs=",
			Weight:       1,
			InferenceUrl: "http://47.236.19.22:18000",
		},
		{
			Index:        "gonka1ktl3kkn9l68c9amanu8u4868mcjmtsr5tgzmjk",
			ValidatorKey: "jFC9XywnI7hzIEQ1kmSQf8Q1iuqy861P7vBrqa3LQxs=",
			Weight:       1,
			InferenceUrl: "http://185.216.21.98:8000",
		},
		{
			Index:        "gonka1kx9mca3xm8u8ypzfuhmxey66u0ufxhs7nm6wc5",
			ValidatorKey: "FODVOcIY8RNoGA7WsiNSL6YQ8N4/A5Ox1IyXgc/FmE0=",
			Weight:       1,
			InferenceUrl: "http://node3.gonka.ai:8000",
		},
		{
			Index:        "gonka15p7s7w2hx0y8095lddd4ummm2y0kwpwljk00aq",
			ValidatorKey: "BUWZfCeWI3O+UXcmCbnjacmi0RY0PzX/8aJKdy3rP48=",
			Weight:       1,
			InferenceUrl: "http://36.189.234.197:18026",
		},
		{
			Index:        "gonka1r90m7wlp95zz92eqltys77xyyqkcmz72rc0kv5",
			ValidatorKey: "WDLSFDAjM9OGUER2hmpFivYbaZiXNl8/+2Vq61Z/dDc=",
			Weight:       1,
			InferenceUrl: "http://69.19.136.233:8000",
		},
		{
			Index:        "gonka19fpma3577v3fnk8nxjkvg442ss8hvglxwqgzz6",
			ValidatorKey: "pM9MGrvN6zoLAuA6SKndq2GT/AY8b9tr8PodsnmV4Bk=",
			Weight:       1,
			InferenceUrl: "http://gonka.spv.re:8000",
		},
		{
			Index:        "gonka1dkl4mah5erqggvhqkpc8j3qs5tyuetgdy552cp",
			ValidatorKey: "YHtcky8VaH0qQNhYJkN61RPf83oKWsCPXdaewvDEYLo=",
			Weight:       1,
			InferenceUrl: "http://node2.gonka.ai:8000",
		},
		{
			Index:        "gonka1vhprg9epy683xghp8ddtdlw2y9cycecmm64tje",
			ValidatorKey: "2ykmApZ4pfSMfoREBUDu/vImEYlOym8ymVWOw2wcMQo=",
			Weight:       1,
			InferenceUrl: "http://36.189.234.237:17241",
		},
		{
			Index:        "gonka1d7p03cu2y2yt3vytq9wlfm6tlz0lfhlgv9h82p",
			ValidatorKey: "5QYFI0kdyBPrcld3FfOwoZdynfwN5li0qUbg3zwFK4I=",
			Weight:       1,
			InferenceUrl: "http://47.236.26.199:8000",
		},
		{
			Index:        "gonka1n7njfqnq7z64efe7xma23zu78xex93e04lm52u",
			ValidatorKey: "6BfEgtpNGORi05A9+XTF7yquvV7BKqfOOWcwpD3A8oU=",
			Weight:       1,
			InferenceUrl: "http://93.119.168.58:8000",
		},
	},
	PocStartBlockHeight:  1,
	EffectiveBlockHeight: 1,
	CreatedAtBlockHeight: 1,
	EpochId:              0,
}

func getParticipants(archiveClient *rpcclient.HTTP, epochId uint64) (*types.ActiveParticipants, string, error) {
	var activeParticipants types.ActiveParticipants
	if epochId == 0 {
		return &activeParticipantsEpoch0Mainnet, "", nil
	}

	dataKey := types.ActiveParticipantsFullKey(epochId)
	result, err := cosmosclient.QueryByKey(archiveClient, "inference", dataKey)
	if err != nil {
		return nil, "", err
	}

	activeParticipantsBytes := hex.EncodeToString(result.Response.Value)
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(interfaceRegistry)
	cdc := codec.NewProtoCodec(interfaceRegistry)

	err = cdc.Unmarshal(result.Response.Value, &activeParticipants)
	if err != nil {
		return nil, "", err
	}
	return &activeParticipants, activeParticipantsBytes, nil
}

func getDataFormMainnetFunc(t *testing.T, _ context.Context, _ string) func(ctx context.Context, epoch string) (*contracts.ActiveParticipantWithProof, error) {
	return func(ctx context.Context, epoch string) (*contracts.ActiveParticipantWithProof, error) {

		const (
			archiveNodeEndpoint = "http://204.12.168.157:26657"
		)
		archiveClient, err := rpcclient.New(archiveNodeEndpoint, "/websocket")
		if err != nil {
			return nil, err
		}

		epochId, err := strconv.Atoi(epoch)
		if err != nil {
			return nil, err
		}

		fmt.Printf("epochId %v\n", epochId)
		var (
			prevParticipants *types.ActiveParticipants
		)

		activeParticipants, activeParticipantsBytes, err := getParticipants(archiveClient, uint64(epochId))
		if err != nil {
			return nil, err
		}

		if epochId > 0 {
			prevParticipants, _, err = getParticipants(archiveClient, uint64(epochId-1))
		} else {
			prevParticipants, _, err = getParticipants(archiveClient, uint64(epochId))
		}

		proofBlockHeight := activeParticipants.CreatedAtBlockHeight + 1
		proofBlock, err := archiveClient.Block(ctx, &proofBlockHeight)
		if err != nil {
			return nil, err
		}

		var proofOps *types.ProofOps
		if epochId != 0 {
			proofOps, err = utils.GetParticipantsMerkleProof(archiveClient, uint64(epochId), activeParticipants.CreatedAtBlockHeight)
			if err != nil {
				return nil, err
			}
		}

		proofBlockId := &types.BlockID{
			Hash:               proofBlock.Block.Header.LastBlockID.Hash.String(),
			PartSetHeaderTotal: int64(proofBlock.Block.Header.LastBlockID.PartSetHeader.Total),
			PartSetHeaderHash:  proofBlock.Block.Header.LastBlockID.PartSetHeader.Hash.String(),
		}

		currentValidatorsProof := createValidatorsProofFromBlock(proofBlockId, proofBlock.Block.LastCommit)

		finalParticipants := contracts.ActiveParticipants{
			Participants:         make([]*contracts.ActiveParticipant, len(activeParticipants.Participants)),
			EpochGroupId:         activeParticipants.EpochGroupId,
			PocStartBlockHeight:  activeParticipants.PocStartBlockHeight,
			EffectiveBlockHeight: activeParticipants.EffectiveBlockHeight,
			CreatedAtBlockHeight: activeParticipants.CreatedAtBlockHeight,
			EpochId:              activeParticipants.EpochId,
		}

		for i, participant := range activeParticipants.Participants {
			finalParticipants.Participants[i] = &contracts.ActiveParticipant{
				Index:        participant.Index,
				ValidatorKey: participant.ValidatorKey,
				Weight:       participant.Weight,
				InferenceUrl: participant.InferenceUrl,
				Models:       participant.Models,
			}
		}

		participantsData := make(map[string]string)
		for _, participant := range prevParticipants.Participants {
			// fmt.Printf("participant.ValidatorKey %v \n", participant.ValidatorKey)
			if participant.ValidatorKey == "" {
				continue
			}

			addrHex, err := common.ConsensusKeyToConsensusAddress(participant.ValidatorKey)
			if err != nil {
				return nil, err
			}
			participantsData[strings.ToUpper(addrHex)] = participant.ValidatorKey
		}

		k, ctx, _ := keepertest.InferenceKeeperReturningMocks(t)
		genesisParams := &types.GenesisOnlyParams{
			TotalSupply:                             1000000000000000000,
			OriginatorSupply:                        200000000000000000,
			StandardRewardAmount:                    680000000000000000,
			PreProgrammedSaleAmount:                 120000000000000000,
			TopRewards:                              3,
			SupplyDenom:                             "ngonka",
			TopRewardPeriod:                         31536000,
			TopRewardPayouts:                        12,
			TopRewardPayoutsPerMiner:                4,
			TopRewardMaxDuration:                    126144000,
			GenesisGuardianNetworkMaturityThreshold: 2000000,
			GenesisGuardianMultiplier:               types.DecimalFromFloat(0.52),
			GenesisGuardianEnabled:                  true,
			MaxIndividualPowerPercentage:            types.DecimalFromFloat(0.30),
			GenesisGuardianAddresses: []string{
				"gonkavaloper1y2a9p56kv044327uycmqdexl7zs82fs5lyang5",
				"gonkavaloper1dkl4mah5erqggvhqkpc8j3qs5tyuetgdc59d0v",
				"gonkavaloper1kx9mca3xm8u8ypzfuhmxey66u0ufxhs70mtf0e"},
		}
		k.SetGenesisOnlyParams(ctx, genesisParams)

		members := epochgroup.ParticipantsToMembers(prevParticipants.Participants)
		results := epochgroup.ComputeResultsForMembers(members)
		results = k.ApplyEarlyNetworkProtection(ctx, results)
		commits := make([]*types.CommitInfo, 0)

		prevParticipantsData := make(map[string]*contracts.CommitInfo)
		totalPower := int64(0)
		totalVotedPower := int64(0)

		for _, participant := range results {
			prevParticipantsData[participant.ValidatorPubKey.Address().String()] = &contracts.CommitInfo{
				ValidatorAddress: participant.ValidatorPubKey.Address().String(),
				ValidatorPubKey:  base64.StdEncoding.EncodeToString(participant.ValidatorPubKey.Bytes()),
				VotingPower:      participant.Power,
			}
			totalPower += participant.Power
		}

		for _, sign := range currentValidatorsProof.Signatures {
			data, ok := prevParticipantsData[sign.ValidatorAddressHex]
			if !ok {
				continue
			}
			commits = append(commits, &types.CommitInfo{
				ValidatorAddress: sign.ValidatorAddressHex,
				ValidatorPubKey:  data.ValidatorPubKey,
			})
			totalVotedPower += data.VotingPower
		}

		block := common.ToContractsBlockProof(&types.BlockProof{
			CreatedAtBlockHeight: activeParticipants.CreatedAtBlockHeight,
			AppHashHex:           proofBlock.Block.Header.AppHash.String(),
			EpochIndex:           activeParticipants.EpochId,
			Commits:              commits,
			TotalPower:           totalPower,
		})

		validators := common.ToContractsValidatorsProof(&currentValidatorsProof)
		proofOpsConverted := common.ToCryptoProofOps(proofOps)

		return &contracts.ActiveParticipantWithProof{
			ActiveParticipants:      finalParticipants,
			ProofOps:                proofOpsConverted,
			ActiveParticipantsBytes: activeParticipantsBytes,
			BlockProof:              block,
			ValidatorsProof:         validators,
			ChainId:                 "gonka-mainnet",
		}, nil
	}
}

func Test_Verification(t *testing.T) {
	const expectedAppHash = "9A3FAFD33F4694FD906B41860C6D3AE1DA5DA8F6F6A8C58BE56CFABBD8384E13"
	err := externalutils.VerifyParticipants(context.Background(), expectedAppHash, getDataFormMainnetFunc(t, context.Background(), ""), "75")
	assert.NoError(t, err)
}

func Test_WeightCalculation(t *testing.T) {
	const (
		archiveNodeEndpoint = "http://204.12.168.157:26657"
		epochId             = 36
	)

	archiveClient, err := rpcclient.New(archiveNodeEndpoint, "/websocket")
	assert.NoError(t, err)

	activeParticipants, _, err := getParticipants(archiveClient, epochId)
	assert.NoError(t, err)

	computeResults := make([]keeper.ComputeResult, 0)
	for _, participant := range activeParticipants.Participants {
		pubKeyBytes, err := base64.StdEncoding.DecodeString(participant.ValidatorKey)
		assert.NoError(t, err)
		// The VALIDATOR key (ed25519), never to be confused with the account key (secp256k1 key)
		pubKey := ed25519.PubKey{Key: pubKeyBytes}

		accAddr, err := sdk.AccAddressFromBech32(participant.Index)
		assert.NoError(t, err)

		valOperatorAddr := sdk.ValAddress(accAddr).String()

		computeResults = append(computeResults, keeper.ComputeResult{
			Power:           participant.Weight,
			ValidatorPubKey: &pubKey,
			OperatorAddress: valOperatorAddr,
		})
	}

}

func Test_Weights(t *testing.T) {
	const (
		archiveNodeEndpoint = "http://204.12.168.157:26657"
		epochId             = 38
	)
	archiveClient, err := rpcclient.New(archiveNodeEndpoint, "/websocket")
	assert.NoError(t, err)

	activeParticipants, _, err := getParticipants(archiveClient, uint64(epochId))
	assert.NoError(t, err)

	prevParticipants, _, err := getParticipants(archiveClient, uint64(epochId-1))
	assert.NoError(t, err)

	matched := make([]*types.ActiveParticipant, 0)
	k, ctx, _ := keepertest.InferenceKeeperReturningMocks(t)

	height := activeParticipants.EffectiveBlockHeight
	resp, err := archiveClient.Validators(ctx, &height, nil, nil)
	assert.NoError(t, err)

	for _, val := range resp.Validators {
		for _, participant := range prevParticipants.Participants {
			addr, err := common.ConsensusKeyToConsensusAddress(participant.ValidatorKey)
			assert.NoError(t, err)

			if val.PubKey.Address().String() == addr {
				matched = append(matched, participant)
			}
		}
	}

	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount("gonka", "gonkapub")
	cfg.SetBech32PrefixForValidator("gonkavaloper", "gonkavaloperpub")
	cfg.SetBech32PrefixForConsensusNode("gonkavalcons", "gonkavalconspub")
	cfg.Seal()

	genesisParams := types.GenesisOnlyParams{
		TotalSupply:                             1000000000000000000,
		OriginatorSupply:                        200000000000000000,
		StandardRewardAmount:                    680000000000000000,
		PreProgrammedSaleAmount:                 120000000000000000,
		TopRewards:                              3,
		SupplyDenom:                             "ngonka",
		TopRewardPeriod:                         31536000,
		TopRewardPayouts:                        12,
		TopRewardPayoutsPerMiner:                4,
		TopRewardMaxDuration:                    126144000,
		GenesisGuardianNetworkMaturityThreshold: 2000000,
		GenesisGuardianMultiplier:               types.DecimalFromFloat(0.52),
		GenesisGuardianEnabled:                  true,
		MaxIndividualPowerPercentage:            types.DecimalFromFloat(0.30),
		GenesisGuardianAddresses: []string{
			"gonkavaloper1y2a9p56kv044327uycmqdexl7zs82fs5lyang5",
			"gonkavaloper1dkl4mah5erqggvhqkpc8j3qs5tyuetgdc59d0v",
			"gonkavaloper1kx9mca3xm8u8ypzfuhmxey66u0ufxhs70mtf0e"},
	}
	k.SetGenesisOnlyParams(ctx, &genesisParams)

	members := epochgroup.ParticipantsToMembers(matched)
	results := epochgroup.ComputeResultsForMembers(members)
	results = k.ApplyEarlyNetworkProtection(ctx, results)

	for _, res := range results {
		for _, val := range resp.Validators {
			if res.ValidatorPubKey.Address().String() == val.PubKey.Address().String() {
				if val.VotingPower != res.Power {
					fmt.Printf("valoper addr %v: expected %v vs actual %v\n", res.OperatorAddress, val.VotingPower, res.Power)
				}
				assert.Equal(t, val.VotingPower, res.Power)
			}
		}
	}
}
