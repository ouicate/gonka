package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgRemoveUserFromTrainingAllowList{}

func NewMsgRemoveUserFromTrainingAllowList(creator string, authority string, address string) *MsgRemoveUserFromTrainingAllowList {
	return &MsgRemoveUserFromTrainingAllowList{
		Creator:   creator,
		Authority: authority,
		Address:   address,
	}
}

func (msg *MsgRemoveUserFromTrainingAllowList) ValidateBasic() error {
	_, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid creator address (%s)", err)
	}
	return nil
}
