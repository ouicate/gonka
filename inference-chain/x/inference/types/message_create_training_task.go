package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgCreateTrainingTask{}

func NewMsgCreateTrainingTask(creator string) *MsgCreateTrainingTask {
	return &MsgCreateTrainingTask{
		Creator: creator,
	}
}

func (msg *MsgCreateTrainingTask) ValidateBasic() error {
	_, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid creator address (%s)", err)
	}

	// Validate hardware resources
	if len(msg.HardwareResources) == 0 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "at least one hardware resource must be specified")
	}

	if len(msg.HardwareResources) > 10_000 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "too many hardware resources (max 10000)")
	}

	for i, resource := range msg.HardwareResources {
		if resource.Type == "" {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "hardware resource at index %d has empty type", i)
		}

		if len(resource.Type) > 100 {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "hardware resource type at index %d is too long (max 100 characters)", i)
		}

		if resource.Count == 0 {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "hardware resource at index %d has zero count", i)
		}

		if resource.Count > 1_000_000 {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "hardware resource at index %d has too high count (max one million)", i)
		}
	}

	// Validate training config
	if msg.Config == nil {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "training config must be specified")
	}

	// Validate datasets
	if msg.Config.Datasets == nil {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "training datasets must be specified")
	}

	if msg.Config.Datasets.Train == "" {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "train dataset path cannot be empty")
	}

	if len(msg.Config.Datasets.Train) > 1_000 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "train dataset path is too long (max 1000 characters)")
	}

	if msg.Config.Datasets.Test == "" {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "test dataset path cannot be empty")
	}

	if len(msg.Config.Datasets.Test) > 1_000 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "test dataset path is too long (max 1_000 characters)")
	}

	// Validate num_uoc_estimation_steps
	if msg.Config.NumUocEstimationSteps == 0 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "num_uoc_estimation_steps must be greater than zero")
	}

	if msg.Config.NumUocEstimationSteps > 1_000_000 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "num_uoc_estimation_steps is too high (max 1000000)")
	}

	return nil
}
