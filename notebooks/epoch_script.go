package notebooks

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"time"
)

const DefaultArchiveNodeURL = "http://204.12.168.157:8000"

type epochGroup struct {
	EpochID                int64
	Participants           []*Participant
	PocStartBlockHeight    int64
	CreatedAtBlockHeight   int64
	SetNewValidatorsHeight int64
}

type mismatchParticipant struct {
	OperatorAddress string
	ConsensusWeight int64
	OnChainWeight   int64
}

type missingParticipant struct {
	OperatorAddress string
	Weight          int64
}

func RunEpochScript(baseURL string, out io.Writer) error {
	group, err := loadEpochGroup(baseURL, "current")
	if err != nil {
		return fmt.Errorf("load current epoch: %w", err)
	}

	for {
		notMatching, missing, err := validateEpochGroup(baseURL, group, out)
		if err != nil {
			return fmt.Errorf("validate epoch %d: %w", group.EpochID, err)
		}

		if len(notMatching) > 0 {
			fmt.Fprintln(out, "REPEATING EPOCH GROUP")

			removeSet := collectParticipantsToRemove(notMatching, missing)
			if len(removeSet) > 0 {
				addresses := make([]string, 0, len(removeSet))
				for addr := range removeSet {
					addresses = append(addresses, addr)
				}
				sort.Strings(addresses)
				fmt.Fprintf(out, "Removing %d participants: %v\n", len(addresses), addresses)
			}

			filtered := make([]*Participant, 0, len(group.Participants))
			for _, p := range group.Participants {
				if _, remove := removeSet[p.OperatorAddress]; !remove {
					filtered = append(filtered, p)
				}
			}

			group.Participants = SetConsensusWeight(filtered, false)

			if _, _, err := validateEpochGroup(baseURL, group, out); err != nil {
				return fmt.Errorf("post-filter validation: %w", err)
			}

			fmt.Fprintln(out, "VALIDATED EPOCH GROUP")
		}

		prevID := group.EpochID - 1
		if prevID < 1 {
			return nil
		}

		prevGroup, err := loadEpochGroup(baseURL, fmt.Sprintf("%d", prevID))
		if err != nil {
			return fmt.Errorf("load epoch %d: %w", prevID, err)
		}

		group = prevGroup
	}
}

func collectParticipantsToRemove(notMatching []mismatchParticipant, missing []missingParticipant) map[string]struct{} {
	remove := make(map[string]struct{})
	for _, entry := range notMatching {
		if _, guardian := GenesisGuardianAddresses[entry.OperatorAddress]; guardian {
			continue
		}
		remove[entry.OperatorAddress] = struct{}{}
	}

	for _, entry := range missing {
		remove[entry.OperatorAddress] = struct{}{}
	}

	return remove
}

func validateEpochGroup(baseURL string, group *epochGroup, out io.Writer) ([]mismatchParticipant, []missingParticipant, error) {
	validatorWeights, err := getValidatorsFromChain(baseURL, group.SetNewValidatorsHeight+1)
	if err != nil {
		return nil, nil, err
	}

	var totalOnChain int64
	for _, weight := range validatorWeights {
		totalOnChain += weight
	}

	totalConsensus := group.totalConsensusWeight()

	fmt.Fprintf(
		out,
		"Epoch %3d | %3d participants | %6d | %6d\n",
		group.EpochID,
		len(group.Participants),
		totalConsensus,
		totalOnChain,
	)

	missing := make([]missingParticipant, 0)
	for _, participant := range group.Participants {
		if _, ok := validatorWeights[participant.OperatorAddress]; !ok {
			missing = append(missing, missingParticipant{
				OperatorAddress: participant.OperatorAddress,
				Weight:          participant.Weight,
			})
		}
	}

	mismatching := make([]mismatchParticipant, 0)
	for _, participant := range group.Participants {
		onChain, ok := validatorWeights[participant.OperatorAddress]
		if !ok {
			continue
		}
		if participant.ConsensusWeight != onChain {
			mismatching = append(mismatching, mismatchParticipant{
				OperatorAddress: participant.OperatorAddress,
				ConsensusWeight: participant.ConsensusWeight,
				OnChainWeight:   onChain,
			})
		}
	}

	if len(missing) > 0 {
		fmt.Fprintf(out, "  %d not found on chain:\n", len(missing))
		sort.Slice(missing, func(i, j int) bool { return missing[i].OperatorAddress < missing[j].OperatorAddress })
		for _, entry := range missing {
			fmt.Fprintf(out, "    - %s | %5d\n", entry.OperatorAddress, entry.Weight)
		}
	}

	if len(mismatching) > 0 {
		fmt.Fprintf(out, "  %d not matching:\n", len(mismatching))
		sort.Slice(mismatching, func(i, j int) bool { return mismatching[i].OperatorAddress < mismatching[j].OperatorAddress })

		for _, entry := range mismatching {
			if _, guardian := GenesisGuardianAddresses[entry.OperatorAddress]; guardian {
				fmt.Fprintf(out, "    - guardian %s %d != %d\n", entry.OperatorAddress, entry.ConsensusWeight, entry.OnChainWeight)
			}
		}

		for _, entry := range mismatching {
			if _, guardian := GenesisGuardianAddresses[entry.OperatorAddress]; guardian {
				continue
			}
			fmt.Fprintf(out, "    - %s %d != %d\n", entry.OperatorAddress, entry.ConsensusWeight, entry.OnChainWeight)
		}
	}

	return mismatching, missing, nil
}

func loadEpochGroup(baseURL, epochID string) (*epochGroup, error) {
	active, err := fetchActiveParticipants(baseURL, epochID)
	if err != nil {
		return nil, err
	}

	if active.ActiveParticipants == nil {
		return nil, fmt.Errorf("no active participants returned")
	}

	epID, err := active.ActiveParticipants.parseEpochID()
	if err != nil {
		return nil, fmt.Errorf("invalid epoch id: %w", err)
	}

	createdAt, err := active.ActiveParticipants.parseCreatedAtHeight()
	if err != nil {
		return nil, fmt.Errorf("invalid created_at_block_height: %w", err)
	}

	pocStart, err := active.ActiveParticipants.parsePocStartHeight()
	if err != nil {
		return nil, fmt.Errorf("invalid poc_start_block_height: %w", err)
	}

	participants := make([]*Participant, 0, len(active.ActiveParticipants.Participants))
	for _, raw := range active.ActiveParticipants.Participants {
		if raw.Seed == nil || raw.Seed.Signature == "" {
			continue
		}

		weight := toInt64(raw.Weight)
		participant := NewParticipant(raw.Index, raw.ValidatorKey, weight)
		participants = append(participants, participant)
	}

	SetConsensusWeight(participants, false)

	epochParams, err := fetchEpochParams(baseURL, createdAt-10)
	if err != nil {
		return nil, err
	}

	return &epochGroup{
		EpochID:                epID,
		Participants:           participants,
		PocStartBlockHeight:    pocStart,
		CreatedAtBlockHeight:   createdAt,
		SetNewValidatorsHeight: pocStart + epochParams.totalSetNewValidatorsDelay(),
	}, nil
}

func (g *epochGroup) totalConsensusWeight() int64 {
	var total int64
	for _, p := range g.Participants {
		total += p.ConsensusWeight
	}
	return total
}

type validatorsResponse struct {
	Validators []struct {
		OperatorAddress string `json:"operator_address"`
		Tokens          string `json:"tokens"`
	} `json:"validators"`
}

func getValidatorsFromChain(baseURL string, height int64) (map[string]int64, error) {
	url := fmt.Sprintf("%s/chain-api/cosmos/staking/v1beta1/validators?height=%d", baseURL, height)
	resp := validatorsResponse{}
	if err := fetchJSON(url, map[string]string{"x-cosmos-block-height": strconv.FormatInt(height, 10)}, &resp); err != nil {
		return nil, err
	}

	validators := make(map[string]int64, len(resp.Validators))
	for _, v := range resp.Validators {
		value, ok := new(big.Int).SetString(v.Tokens, 10)
		if !ok {
			return nil, fmt.Errorf("invalid validator tokens: %s", v.Tokens)
		}
		validators[v.OperatorAddress] = value.Int64()
	}

	return validators, nil
}

type epochParamsResponse struct {
	Params struct {
		EpochParams struct {
			PocStageDuration      json.Number `json:"poc_stage_duration"`
			PocValidationDelay    json.Number `json:"poc_validation_delay"`
			PocValidationDuration json.Number `json:"poc_validation_duration"`
			SetNewValidatorsDelay json.Number `json:"set_new_validators_delay"`
		} `json:"epoch_params"`
	} `json:"params"`
}

func (r epochParamsResponse) totalSetNewValidatorsDelay() int64 {
	total := int64(0)
	total += toInt64(r.Params.EpochParams.PocStageDuration)
	total += toInt64(r.Params.EpochParams.PocValidationDelay)
	total += toInt64(r.Params.EpochParams.PocValidationDuration)
	total += toInt64(r.Params.EpochParams.SetNewValidatorsDelay)
	return total
}

type activeParticipantsWrapper struct {
	ActiveParticipants *activeParticipantsBody `json:"active_participants"`
}

type activeParticipantsBody struct {
	EpochID              json.Number                   `json:"epoch_id"`
	CreatedAtBlockHeight json.Number                   `json:"created_at_block_height"`
	PocStartBlockHeight  json.Number                   `json:"poc_start_block_height"`
	Participants         []activeParticipantDescriptor `json:"participants"`
}

type activeParticipantDescriptor struct {
	Index        string      `json:"index"`
	ValidatorKey string      `json:"validator_key"`
	Weight       json.Number `json:"weight"`
	Seed         *struct {
		Signature string `json:"signature"`
	} `json:"seed"`
}

func (b *activeParticipantsBody) parseEpochID() (int64, error) {
	return b.EpochID.Int64()
}

func (b *activeParticipantsBody) parseCreatedAtHeight() (int64, error) {
	return b.CreatedAtBlockHeight.Int64()
}

func (b *activeParticipantsBody) parsePocStartHeight() (int64, error) {
	return b.PocStartBlockHeight.Int64()
}

func fetchActiveParticipants(baseURL, epochID string) (*activeParticipantsWrapper, error) {
	if epochID == "" {
		epochID = "current"
	}

	url := fmt.Sprintf("%s/v1/epochs/%s/participants", baseURL, epochID)
	resp := &activeParticipantsWrapper{}
	if err := fetchJSON(url, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func fetchEpochParams(baseURL string, height int64) (epochParamsResponse, error) {
	url := fmt.Sprintf("%s/chain-api/productscience/inference/inference/params?height=%d", baseURL, height)
	headers := map[string]string{"x-cosmos-block-height": strconv.FormatInt(height, 10)}
	resp := epochParamsResponse{}
	if err := fetchJSON(url, headers, &resp); err != nil {
		return epochParamsResponse{}, err
	}
	return resp, nil
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

func toInt64(number json.Number) int64 {
	value, err := number.Int64()
	if err != nil {
		floatValue, errFloat := strconv.ParseFloat(number.String(), 64)
		if errFloat != nil {
			return 0
		}
		return int64(floatValue)
	}
	return value
}
