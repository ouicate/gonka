package main

import (
	"fmt"
	"github.com/cometbft/cometbft/rpc/client/http"
	"os"
	"strconv"

	cosmos_client "decentralized-api/cosmosclient"
	"decentralized-api/logging"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"

	"github.com/productscience/inference/x/inference/types"
)

const (
	defaultRPCURL       = "http://node1.gonka.ai:26657/"
	EPOCH_START_DEFAULT = 1
	EPOCH_END_DEFAULT   = 1
)

func getEpochSummary(
	cdc codec.Codec,
	rpcClient *http.HTTP,
	epochStart, epochEnd uint64,
) (map[uint64]int64, error) {
	result := make(map[uint64]int64)
	if epochEnd < epochStart {
		return result, fmt.Errorf("epochEnd (%d) < epochStart (%d)", epochEnd, epochStart)
	}

	logging.Info("Start getting weights for epochs",
		types.Participants,
		"epoch_start", epochStart,
		"epoch_end", epochEnd,
	)

	for epoch := epochStart; epoch <= epochEnd; epoch++ {
		logging.Info("Getting weights for epoch", types.Participants, "epoch", epoch)

		dataKey := types.ActiveParticipantsFullKey(epoch)

		res, err := cosmos_client.QueryByKey(rpcClient, "inference", dataKey)
		if err != nil {
			logging.Error("Failed to query active participants",
				types.Participants,
				"epoch", epoch,
				"error", err,
			)
			return nil, err
		}

		var activeParticipants types.ActiveParticipants
		if err := cdc.Unmarshal(res.Response.Value, &activeParticipants); err != nil {
			logging.Error("Failed to unmarshal active participant",
				types.Participants,
				"epoch", epoch,
				"error", err,
			)
			return nil, err
		}

		var epochWeightTotal int64
		for _, participant := range activeParticipants.Participants {
			epochWeightTotal += participant.Weight
		}
		result[epoch] = epochWeightTotal
	}

	return result, nil
}

func main() {
	epochStart := uint64(EPOCH_START_DEFAULT)
	if v := os.Getenv("EPOCH_START"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
			epochStart = parsed
		} else {
			panic(err)
		}
	}

	epochEnd := uint64(EPOCH_END_DEFAULT)
	if v := os.Getenv("EPOCH_END"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
			epochEnd = parsed
		} else {
			panic(err)
		}
	}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(interfaceRegistry)

	cdc := codec.NewProtoCodec(interfaceRegistry)

	rpcClient, err := cosmos_client.NewRpcClient(defaultRPCURL)
	if err != nil {
		panic(err)
	}

	epochWeights, err := getEpochSummary(cdc, rpcClient, epochStart, epochEnd)
	if err != nil {
		panic(err)
	}

	file, err := os.Create("epoch_weights.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	for epoch := epochStart; epoch <= epochEnd; epoch++ {
		weight := epochWeights[epoch]
		line := fmt.Sprintf("%d | %d\n", epoch, weight)
		if _, err := file.WriteString(line); err != nil {
			panic(err)
		}
	}

	logging.Info("Wrote results to file epoch_weights.txt",
		types.Participants,
		"error", err,
	)
}
