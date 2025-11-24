package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"decentralized-api/logging"

	"github.com/productscience/inference/x/inference/types"
)

const (
	defaultRPCURL = "http://node1.gonka.ai:26657/"
	baseURL       = "http://node1.gonka.ai:8000"

	EPOCH_START_DEFAULT = 1
	EPOCH_END_DEFAULT   = 1
)

func loadEpochGroup(baseURL string, epoch uint64) (*epochGroupData, error) {
	resp, err := fetchEpochGroupData(baseURL, epoch)
	if err != nil {
		return nil, err
	}

	eg := resp.EpochGroupData
	return &eg, nil
}

type epochGroupDataResponse struct {
	EpochGroupData epochGroupData `json:"epoch_group_data"`
}

type epochGroupData struct {
	PocStartBlockHeight uint64 `json:"poc_start_block_height,string"`
	EpochGroupID        uint64 `json:"epoch_group_id,string"`

	EpochPolicy          string   `json:"epoch_policy"`
	EffectiveBlockHeight int64    `json:"effective_block_height,string"`
	LastBlockHeight      int64    `json:"last_block_height,string"`
	TotalWeight          int64    `json:"total_weight,string"`
	ModelID              string   `json:"model_id"`
	SubGroupModels       []string `json:"sub_group_models"`
	EpochIndex           uint64   `json:"epoch_index,string"`
	TotalThroughput      int64    `json:"total_throughput,string"`
}

func fetchEpochGroupData(baseURL string, epoch uint64) (epochGroupDataResponse, error) {
	url := fmt.Sprintf("%s/chain-api/productscience/inference/inference/epoch_group_data/%d", baseURL, epoch)
	headers := map[string]string{}

	resp := epochGroupDataResponse{}
	if err := fetchJSON(url, headers, &resp); err != nil {
		return epochGroupDataResponse{}, err
	}
	return resp, nil
}

func getEpochSummary(epochStart, epochEnd uint64) (map[uint64]int64, error) {
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

		epochGroup, err := loadEpochGroup(baseURL, epoch)
		if err != nil {
			return result, err
		}
		result[epoch] = epochGroup.TotalWeight
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

	epochWeights, err := getEpochSummary(epochStart, epochEnd)
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

func fetchJSON(url string, headers map[string]string, dst interface{}) error {
	client := http.Client{Timeout: 20 * time.Second}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", response.Status)
	}

	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	return decoder.Decode(dst)
}
