package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/productscience/inference/x/bls/types"
	"golang.org/x/crypto/sha3"
)

func (k Keeper) LogInfo(msg string, keyvals ...interface{}) {
	k.Logger().Info(msg, append(keyvals, "subsystem", "BLS")...)
}

func (k Keeper) LogError(msg string, keyvals ...interface{}) {
	k.Logger().Error(msg, append(keyvals, "subsystem", "BLS")...)
}

func (k Keeper) LogWarn(msg string, keyvals ...interface{}) {
	k.Logger().Warn(msg, append(keyvals, "subsystem", "BLS")...)
}

func (k Keeper) LogDebug(msg string, keyVals ...interface{}) {
	k.Logger().Debug(msg, append(keyVals, "subsystem", "BLS")...)
}

// SubmitGroupKeyValidationSignature handles the submission of partial signatures for group key validation
func (ms msgServer) SubmitGroupKeyValidationSignature(goCtx context.Context, msg *types.MsgSubmitGroupKeyValidationSignature) (*types.MsgSubmitGroupKeyValidationSignatureResponse, error) {
	ms.Keeper.LogInfo("Processing group key validation signature", "new_epoch_id", msg.NewEpochId, "creator", msg.Creator)
	ctx := sdk.UnwrapSDKContext(goCtx)

	// Genesis case: Epoch 1 doesn't need validation (no previous epoch)
	if msg.NewEpochId == 1 {
		ms.Keeper.LogInfo("Rejecting group key validation for genesis epoch", "new_epoch_id", msg.NewEpochId)
		return nil, fmt.Errorf("epoch 1 does not require group key validation (genesis case)")
	}

	previousEpochId := msg.NewEpochId - 1

	// Get the new epoch's BLS data to get the group public key being validated
	newEpochBLSData, found := ms.GetEpochBLSData(ctx, msg.NewEpochId)
	if !found {
		ms.Keeper.LogError("New epoch not found", "new_epoch_id", msg.NewEpochId)
		return nil, fmt.Errorf("new epoch %d not found", msg.NewEpochId)
	}

	// Ensure the new epoch has completed DKG
	if newEpochBLSData.DkgPhase != types.DKGPhase_DKG_PHASE_COMPLETED && newEpochBLSData.DkgPhase != types.DKGPhase_DKG_PHASE_SIGNED {
		ms.Keeper.LogError("Invalid DKG phase for group key validation", "new_epoch_id", msg.NewEpochId, "current_phase", newEpochBLSData.DkgPhase.String())
		return nil, fmt.Errorf("new epoch %d DKG is not completed (current phase: %s)", msg.NewEpochId, newEpochBLSData.DkgPhase.String())
	}

	// If already signed, silently ignore the submission
	if newEpochBLSData.DkgPhase == types.DKGPhase_DKG_PHASE_SIGNED {
		ms.Keeper.LogInfo("Group key validation already completed", "new_epoch_id", msg.NewEpochId)
		return &types.MsgSubmitGroupKeyValidationSignatureResponse{}, nil
	}

	// Get the previous epoch's BLS data for slot validation and signature verification
	previousEpochBLSData, found := ms.GetEpochBLSData(ctx, previousEpochId)
	if !found {
		// Emit a searchable event and continue using current epoch data as fallback
		ms.Keeper.LogWarn("Previous epoch not found - using current epoch for validation", "previous_epoch_id", previousEpochId, "new_epoch_id", msg.NewEpochId)
		ctx.EventManager().EmitTypedEvent(&types.EventGroupKeyValidationFailed{
			NewEpochId: msg.NewEpochId,
			Reason:     fmt.Sprintf("previous_epoch_missing_fallback:%d", previousEpochId),
		})

		previousEpochBLSData = newEpochBLSData
		previousEpochId = msg.NewEpochId
	}

	// Find the participant in the previous epoch
	participantIndex := -1
	var participantInfo *types.BLSParticipantInfo
	for i, participant := range previousEpochBLSData.Participants {
		if participant.Address == msg.Creator {
			participantIndex = i
			participantInfo = &participant
			break
		}
	}

	if participantIndex == -1 {
		ms.Keeper.LogError("Participant not found in previous epoch", "creator", msg.Creator, "previous_epoch_id", previousEpochId)
		return nil, fmt.Errorf("participant %s not found in previous epoch %d", msg.Creator, previousEpochId)
	}

	// Validate slot ownership - ensure submitted slot indices match participant's assigned range
	expectedSlots := make([]uint32, 0)
	for i := participantInfo.SlotStartIndex; i <= participantInfo.SlotEndIndex; i++ {
		expectedSlots = append(expectedSlots, i)
	}

	// Check if submitted slot indices exactly match expected slots
	if len(msg.SlotIndices) != len(expectedSlots) {
		ms.Keeper.LogError("Slot indices count mismatch", "expected_slots_count", len(expectedSlots), "submitted_slots_count", len(msg.SlotIndices))
		return nil, fmt.Errorf("slot indices count mismatch: expected %d, got %d", len(expectedSlots), len(msg.SlotIndices))
	}

	for i, slotIndex := range msg.SlotIndices {
		if slotIndex != expectedSlots[i] {
			ms.Keeper.LogError("Invalid slot index", "expected_slot_index", expectedSlots[i], "submitted_slot_index", slotIndex)
			return nil, fmt.Errorf("invalid slot index at position %d: expected %d, got %d", i, expectedSlots[i], slotIndex)
		}
	}

	// Check or create GroupKeyValidationState
	var validationState *types.GroupKeyValidationState
	validationStateKey := fmt.Sprintf("group_validation_%d", msg.NewEpochId)

	// Try to get existing validation state
	store := ms.storeService.OpenKVStore(ctx)
	bz, err := store.Get([]byte(validationStateKey))
	if err != nil {
		ms.Keeper.LogError("Failed to get validation state", "new_epoch_id", msg.NewEpochId, "error", err.Error())
		return nil, fmt.Errorf("failed to get validation state: %w", err)
	}

	if bz == nil {
		// First signature for this epoch - create validation state
		validationState = &types.GroupKeyValidationState{
			NewEpochId:      msg.NewEpochId,
			PreviousEpochId: previousEpochId,
			Status:          types.GroupKeyValidationStatus_GROUP_KEY_VALIDATION_STATUS_COLLECTING_SIGNATURES,
			SlotsCovered:    0,
		}
		ms.Keeper.LogInfo("Created new validation state", "new_epoch_id", msg.NewEpochId, "previous_epoch_id", previousEpochId)

		// Prepare validation data for message hash
		messageHash, err := ms.computeValidationMessageHash(ctx, newEpochBLSData.GroupPublicKey, previousEpochId, msg.NewEpochId)
		if err != nil {
			ms.Keeper.LogError("Failed to compute message hash", "error", err.Error())
			return nil, fmt.Errorf("failed to compute message hash: %w", err)
		}
		validationState.MessageHash = messageHash
	} else {
		// Existing validation state
		validationState = &types.GroupKeyValidationState{}
		ms.cdc.MustUnmarshal(bz, validationState)

		// Check if participant already submitted
		for _, partialSig := range validationState.PartialSignatures {
			if partialSig.ParticipantAddress == msg.Creator {
				ms.Keeper.LogError("Participant already submitted group key validation signature", "creator", msg.Creator)
				return nil, fmt.Errorf("participant %s already submitted group key validation signature", msg.Creator)
			}
		}
	}

	// Verify BLS partial signature against participant's computed individual public key
	if !ms.verifyBLSPartialSignature(msg.PartialSignature, validationState.MessageHash, &previousEpochBLSData, msg.SlotIndices) {
		ms.Keeper.LogError("Invalid BLS signature verification", "creator", msg.Creator)
		return nil, fmt.Errorf("invalid BLS signature verification failed for participant %s", msg.Creator)
	}
	ms.Keeper.LogInfo("Valid signature received", "creator", msg.Creator, "slots_count", len(msg.SlotIndices))

	// Add the partial signature
	partialSignature := &types.PartialSignature{
		ParticipantAddress: msg.Creator,
		SlotIndices:        msg.SlotIndices,
		Signature:          msg.PartialSignature,
	}
	validationState.PartialSignatures = append(validationState.PartialSignatures, *partialSignature)

	// Update slots covered
	validationState.SlotsCovered += uint32(len(msg.SlotIndices))

	// Check if we have sufficient participation (>50% of previous epoch slots)
	requiredSlots := previousEpochBLSData.ITotalSlots/2 + 1
	ms.Keeper.LogInfo("Checking for signature readiness", "required_slots", requiredSlots, "slots_covered", validationState.SlotsCovered)
	if validationState.SlotsCovered >= requiredSlots {
		ms.Keeper.LogInfo("Enough signatures collected, validating group key")
		// Aggregate signatures and finalize validation
		finalSignature, aggErr := ms.aggregateBLSPartialSignatures(validationState.PartialSignatures)
		if aggErr != nil {
			ms.Keeper.LogError("Failed to aggregate partial signatures", "error", aggErr.Error())
			return nil, fmt.Errorf("failed to aggregate partial signatures: %w", aggErr)
		}
		validationState.FinalSignature = finalSignature
		validationState.Status = types.GroupKeyValidationStatus_GROUP_KEY_VALIDATION_STATUS_VALIDATED

		// Store the final signature in the new epoch's EpochBLSData and transition to SIGNED phase
		newEpochBLSData.ValidationSignature = validationState.FinalSignature
		newEpochBLSData.DkgPhase = types.DKGPhase_DKG_PHASE_SIGNED
		ms.SetEpochBLSData(ctx, newEpochBLSData)
		ms.Keeper.LogInfo("Group key validation completed", "new_epoch_id", msg.NewEpochId, "slots_covered", validationState.SlotsCovered)

		// Emit success event
		err := ctx.EventManager().EmitTypedEvent(&types.EventGroupKeyValidated{
			NewEpochId:     msg.NewEpochId,
			FinalSignature: validationState.FinalSignature,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to emit EventGroupKeyValidated: %w", err)
		}
	}

	// Store updated validation state
	bz = ms.cdc.MustMarshal(validationState)
	if err := store.Set([]byte(validationStateKey), bz); err != nil {
		return nil, fmt.Errorf("failed to store validation state: %w", err)
	}

	return &types.MsgSubmitGroupKeyValidationSignatureResponse{}, nil
}

// computeValidationMessageHash computes the message hash for group key validation.
// Uses Ethereum-compatible abi.encodePacked(previous_epoch_id [8], chain_id [32], new_group_key_uncompressed [256]).
func (ms msgServer) computeValidationMessageHash(ctx sdk.Context, groupPublicKey []byte, previousEpochId, newEpochId uint64) ([]byte, error) {
	// Expect 96-byte compressed G2 key; decompress deterministically.
	if len(groupPublicKey) != 96 {
		return nil, fmt.Errorf("invalid group public key length: expected 96 bytes, got %d", len(groupPublicKey))
	}
	var g2 bls12381.G2Affine
	if err := g2.Unmarshal(groupPublicKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal compressed G2 key: %w", err)
	}

	// Use GONKA_CHAIN_ID bytes32 (hash of chain-id string), consistent with bridge signing logic
	gonkaIdHash := sha256.Sum256([]byte(ctx.ChainID()))
	chainIdBytes := gonkaIdHash[:]

	// Implement Ethereum-compatible abi.encodePacked with uncompressed G2 in 64-byte limbs:
	// Order: X.c0, X.c1, Y.c0, Y.c1, each 64-byte big-endian (16 zero bytes + 48-byte fp).
	var encodedData []byte

	// Add previous_epoch_id (uint64 -> 8 bytes big endian)
	previousEpochBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(previousEpochBytes, previousEpochId)
	encodedData = append(encodedData, previousEpochBytes...)

	// Add chain_id (32 bytes)
	encodedData = append(encodedData, chainIdBytes...)

	// Build 256-byte uncompressed encoding: 4 field elements, each 64 bytes
	appendFp64 := func(e fp.Element) {
		// 48-byte big-endian field element
		be48 := e.Bytes()
		// Left-pad to 64 bytes
		var limb [64]byte
		copy(limb[64-48:], be48[:])
		encodedData = append(encodedData, limb[:]...)
	}

	// Note: gnark-crypto stores E2 as (A0, A1). We need c0 then c1.
	// g2.X.A0 = c0, g2.X.A1 = c1; same for Y.
	appendFp64(g2.X.A0)
	appendFp64(g2.X.A1)
	appendFp64(g2.Y.A0)
	appendFp64(g2.Y.A1)

	// Compute keccak256 hash (Ethereum-compatible)
	hash := sha3.NewLegacyKeccak256()
	hash.Write(encodedData)
	return hash.Sum(nil), nil
}
