package keeper_test

import (
	"context"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"

	"github.com/productscience/inference/testutil"
	keepertest "github.com/productscience/inference/testutil/keeper"
	"github.com/productscience/inference/testutil/nullify"
	"github.com/productscience/inference/x/inference/keeper"
	"github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/require"
)

// Prevent strconv unused error
var _ = strconv.IntSize

func createNParticipant(keeper keeper.Keeper, ctx context.Context, n int) []types.Participant {
	items := make([]types.Participant, n)
	for i := range items {
		items[i].Index = testutil.Bech32Addr(i)
		// To test counter
		items[i].Status = types.ParticipantStatus_ACTIVE

		keeper.SetParticipant(ctx, items[i])
	}
	return items
}

func TestParticipantGet(t *testing.T) {
	keeper, ctx := keepertest.InferenceKeeper(t)
	items := createNParticipant(keeper, ctx, 10)
	var expectedCounter uint32 = 0
	for _, item := range items {
		rst, found := keeper.GetParticipant(ctx,
			item.Index,
		)
		require.True(t, found)
		require.Equal(t,
			nullify.Fill(&item),
			nullify.Fill(&rst),
		)
		expectedCounter++
	}
}

func TestIfConsensusKeyAlreadyExist(t *testing.T) {
	keeper, ctx := keepertest.InferenceKeeper(t)
	const (
		developers           = 3
		notEmptyValidatorKey = "some_validator_key"
	)

	developersAddrs := make([]string, developers)
	for i := 0; i < developers; i++ {
		addr := testutil.Bech32Addr(i)
		developersAddrs[i] = addr
		participant := types.Participant{
			Index:   addr,
			Address: addr,
		}
		keeper.SetParticipant(ctx, participant)

		exist := keeper.IfConsensusKeyAlreadyExistByAnotherParticipant(ctx, participant) // developer
		assert.Nil(t, exist)
	}

	participant1 := types.Participant{
		Index:        testutil.Bech32Addr(10),
		Address:      testutil.Bech32Addr(10),
		ValidatorKey: notEmptyValidatorKey,
	}
	keeper.SetParticipant(ctx, participant1)

	exist := keeper.IfConsensusKeyAlreadyExistByAnotherParticipant(ctx, participant1) // same participant
	assert.Nil(t, exist)

	participant2 := types.Participant{
		Index:        testutil.Bech32Addr(11),
		Address:      testutil.Bech32Addr(11),
		ValidatorKey: notEmptyValidatorKey,
	}

	exist = keeper.IfConsensusKeyAlreadyExistByAnotherParticipant(ctx, participant2)
	assert.Error(t, exist)
}

func TestParticipantRemove(t *testing.T) {
	keeper, ctx := keepertest.InferenceKeeper(t)
	items := createNParticipant(keeper, ctx, 10)
	for _, item := range items {
		keeper.RemoveParticipant(ctx,
			item.Index,
		)
		_, found := keeper.GetParticipant(ctx,
			item.Index,
		)
		require.False(t, found)
	}
}

func TestParticipantGetAll(t *testing.T) {
	keeper, ctx := keepertest.InferenceKeeper(t)
	items := createNParticipant(keeper, ctx, 1000)
	require.ElementsMatch(t,
		nullify.Fill(items),
		nullify.Fill(keeper.GetAllParticipant(ctx)),
	)
}
