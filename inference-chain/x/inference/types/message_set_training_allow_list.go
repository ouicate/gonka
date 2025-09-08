package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgSetTrainingAllowList{}

func NewMsgSetTrainingAllowList(creator string, authority string, addresses []string) *MsgSetTrainingAllowList {
	return &MsgSetTrainingAllowList{
		Creator:   creator,
		Authority: authority,
		Addresses: addresses,
	}
}

func (msg *MsgSetTrainingAllowList) ValidateBasic() error {
	_, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid creator address (%s)", err)
	}
	return nil
}
