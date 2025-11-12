package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/productscience/inference/testutil"
	keepertest "github.com/productscience/inference/testutil/keeper"
	"github.com/productscience/inference/x/inference/keeper"
	"github.com/productscience/inference/x/inference/types"
)

func TestAssignTrainingTask_AllowListEnforced(t *testing.T) {
	k, ctx := keepertest.InferenceKeeper(t)
	ms := keeper.NewMsgServerImpl(k)
	wctx := sdk.UnwrapSDKContext(ctx)

	// create a queued task
	sdkCtx := sdk.UnwrapSDKContext(wctx)
	require.NoError(t, k.CreateTask(sdkCtx, &types.TrainingTask{Id: 0}))
	assignee := types.Participant{Index: testutil.Validator, Address: testutil.Validator}
	k.SetParticipant(ctx, assignee)

	// not allowed
	_, err := ms.AssignTrainingTask(wctx, &types.MsgAssignTrainingTask{
		Creator:   testutil.Creator,
		TaskId:    1,
		Assignees: nil,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrTrainingNotAllowed)

	// allow
	acc, e := sdk.AccAddressFromBech32(testutil.Creator)
	require.NoError(t, e)
	require.NoError(t, k.TrainingStartAllowListSet.Set(wctx, acc))

	// should succeed now
	_, err = ms.AssignTrainingTask(wctx, &types.MsgAssignTrainingTask{
		Creator:   testutil.Creator,
		TaskId:    1,
		Assignees: []*types.TrainingTaskAssignee{{Participant: testutil.Validator}},
	})
	require.NoError(t, err)
}
