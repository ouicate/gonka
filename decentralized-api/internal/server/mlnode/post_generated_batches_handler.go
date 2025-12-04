package mlnode

import (
	cosmos_client "decentralized-api/cosmosclient"
	"decentralized-api/logging"
	"net/http"

	"decentralized-api/mlnodeclient"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/productscience/inference/api/inference/inference"
	"github.com/productscience/inference/x/inference/types"
)

func (s *Server) postGeneratedBatches(ctx echo.Context) error {
	var body mlnodeclient.ProofBatch

	if err := ctx.Bind(&body); err != nil {
		logging.Error("ProofBatch-callback. Failed to decode request body of type ProofBatch", types.PoC, "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}

	logging.Debug("ProofBatch-callback. Received", types.PoC, "body", body)

	var nodeId string
	node, found := s.broker.GetNodeByNodeNum(body.NodeNum)
	if found {
		nodeId = node.Id
		logging.Info("ProofBatch-callback. Found node by node num", types.PoC,
			"nodeId", nodeId,
			"nodeNum", body.NodeNum)
	} else {
		logging.Warn("ProofBatch-callback. Unknown NodeNum. Sending MsgSubmitPocBatch with empty nodeId",
			types.PoC, "node_num", body.NodeNum)
	}

	go func() {
		// Determine wait target
		targetHeight := body.BlockHeight // Default for Confirmation PoC (starts at trigger)
		epochState := s.broker.GetEpochState()

		// Check if this is a Regular PoC (which requires +1 block delay)
		isConfirmation := false
		if epochState != nil && epochState.ActiveConfirmationPoCEvent != nil {
			if epochState.ActiveConfirmationPoCEvent.TriggerHeight == body.BlockHeight {
				isConfirmation = true
			}
		}

		if !isConfirmation {
			// Regular PoC starts 1 block after the recorded start height
			// We add another +1 block buffer to avoid race conditions on nodes that are slightly lagging
			targetHeight = body.BlockHeight + 2
		}

		// Wait if epochState is nil (unknown state) or if current height is less than target
		shouldWait := true
		if epochState != nil && epochState.CurrentBlock.Height >= targetHeight {
			shouldWait = false
		}

		if shouldWait {
			currentHeight := int64(0)
			if epochState != nil {
				currentHeight = epochState.CurrentBlock.Height
			}
			logging.Info("ProofBatch-callback. Waiting for PoC submission window", types.PoC,
				"currentHeight", currentHeight,
				"targetHeight", targetHeight,
				"isConfirmation", isConfirmation)
			<-s.broker.WaitForHeight(targetHeight)
		}

		msg := &inference.MsgSubmitPocBatch{
			PocStageStartBlockHeight: body.BlockHeight,
			Nonces:                   body.Nonces,
			Dist:                     body.Dist,
			BatchId:                  uuid.New().String(),
			NodeId:                   nodeId,
		}

		if err := s.recorder.SubmitPocBatch(msg); err != nil {
			logging.Error("ProofBatch-callback. Failed to submit MsgSubmitPocBatch", types.PoC, "error", err)
		}
	}()

	return ctx.NoContent(http.StatusOK)
}

func (s *Server) postValidatedBatches(ctx echo.Context) error {
	var body mlnodeclient.ValidatedBatch

	if err := ctx.Bind(&body); err != nil {
		logging.Error("ValidateReceivedBatches-callback. Failed to decode request body of type ValidatedBatch", types.PoC, "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}

	logging.Debug("ValidateReceivedBatches-callback. ValidatedProofBatch received", types.PoC, "body", body)

	address, err := cosmos_client.PubKeyToAddress(body.PublicKey)
	if err != nil {
		logging.Error("ValidateReceivedBatches-callback. Failed to convert public key to address", types.PoC,
			"publicKey", body.PublicKey,
			"NInvalid", body.NInvalid,
			"ProbabilityHonest", body.ProbabilityHonest,
			"FraudDetected", body.FraudDetected,
			"error", err)
		return err
	}

	logging.Info("ValidateReceivedBatches-callback. ValidatedProofBatch received", types.PoC,
		"participant", address,
		"NInvalid", body.NInvalid,
		"ProbabilityHonest", body.ProbabilityHonest,
		"FraudDetected", body.FraudDetected)

	// Move submission to background to avoid blocking HTTP and to handle wait
	go func() {
		// Wait for validation window to open if needed
		epochState := s.broker.GetEpochState()
		if epochState != nil {
			var targetHeight int64

			if epochState.ActiveConfirmationPoCEvent != nil {
				event := epochState.ActiveConfirmationPoCEvent
				if event.TriggerHeight == body.BlockHeight {
					params := epochState.LatestEpoch.EpochParams
					// Confirmation PoC validation starts at ValidationStart.
					// We target ValidationStart + 1 (the second block of the window) to avoid race conditions
					// where dapi sees the block before the node's CheckTx context is updated.
					targetHeight = event.GetValidationStart(&params) + 1
				}
			} else {
				// Regular PoC
				// Check if the submission corresponds to the current epoch's PoC start
				if body.BlockHeight == epochState.LatestEpoch.PocStartBlockHeight {
					// Regular PoC validation window opens at StartOfPoCValidation + 1.
					// We target StartOfPoCValidation + 2 (the second block of the window) to avoid race conditions.
					targetHeight = epochState.LatestEpoch.StartOfPoCValidation() + 2
				}
			}

			if targetHeight > 0 && epochState.CurrentBlock.Height < targetHeight {
				logging.Info("ValidateReceivedBatches-callback. Waiting for validation window", types.PoC,
					"currentHeight", epochState.CurrentBlock.Height,
					"targetHeight", targetHeight)
				// Block this goroutine until height is reached
				<-s.broker.WaitForHeight(targetHeight)
			}
		}

		msg := &inference.MsgSubmitPocValidation{
			ParticipantAddress:       address,
			PocStageStartBlockHeight: body.BlockHeight,
			Nonces:                   body.Nonces,
			Dist:                     body.Dist,
			ReceivedDist:             body.ReceivedDist,
			RTarget:                  body.RTarget,
			FraudThreshold:           body.FraudThreshold,
			NInvalid:                 body.NInvalid,
			ProbabilityHonest:        body.ProbabilityHonest,
			FraudDetected:            body.FraudDetected,
		}

		// FIXME: We empty all arrays to avoid too large chain transactions
		//  We can allow that, because we only use FraudDetected boolean
		//  when making a decision about participant's PoC submissions
		//  Will be fixed in future versions
		emptyArrays(msg)

		if err := s.recorder.SubmitPoCValidation(msg); err != nil {
			logging.Error("ValidateReceivedBatches-callback. Failed to submit MsgSubmitValidatedPocBatch", types.PoC,
				"participant", address,
				"error", err)
		}
	}()

	return ctx.NoContent(http.StatusOK)
}

func emptyArrays(msg *inference.MsgSubmitPocValidation) {
	msg.Dist = make([]float64, 0)
	msg.ReceivedDist = make([]float64, 0)
	msg.Nonces = make([]int64, 0)
}
