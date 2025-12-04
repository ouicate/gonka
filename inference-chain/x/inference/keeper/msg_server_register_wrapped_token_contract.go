package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/productscience/inference/x/inference/types"
)

// RegisterWrappedTokenContract sets the code id used for new wrapped-token instantiations.
func (k msgServer) RegisterWrappedTokenContract(goCtx context.Context, req *types.MsgRegisterWrappedTokenContract) (*types.MsgRegisterWrappedTokenContractResponse, error) {
	if err := k.CheckPermission(goCtx, req, GovernancePermission); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	k.SetWrappedTokenCodeID(ctx, req.CodeId)
	return &types.MsgRegisterWrappedTokenContractResponse{}, nil
}
