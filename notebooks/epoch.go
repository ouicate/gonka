package notebooks

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"sort"
	"strings"

	"github.com/btcsuite/btcutil/bech32"
)

const (
	maturityThreshold = int64(2_000_000)
)

var (
	GenesisGuardianAddresses = map[string]struct{}{
		"gonkavaloper1y2a9p56kv044327uycmqdexl7zs82fs5lyang5": {},
		"gonkavaloper1dkl4mah5erqggvhqkpc8j3qs5tyuetgdc59d0v": {},
		"gonkavaloper1kx9mca3xm8u8ypzfuhmxey66u0ufxhs70mtf0e": {},
	}
	genesisGuardianMultiplier    = big.NewRat(52, 100)
	maxIndividualPowerPercentage = big.NewRat(30, 100)
)

// Participant mirrors the minimal fields needed by the enhancement logic.
type Participant struct {
	Index            string
	OperatorAddress  string
	ValidatorKey     string
	ValidatorAddress string
	Weight           int64
	ConsensusWeight  int64
}

func NewParticipant(index, validatorKey string, weight int64) *Participant {
	operatorAddress, err := getOperatorAddressFromAccountAddress(index, "gonka")
	if err != nil {
		operatorAddress = ""
	}

	validatorAddress, err := getValidatorAddressFromPubKey(validatorKey)
	if err != nil {
		validatorAddress = ""
	}

	return &Participant{
		Index:            index,
		OperatorAddress:  operatorAddress,
		ValidatorKey:     validatorKey,
		ValidatorAddress: validatorAddress,
		Weight:           weight,
	}
}

// ApplyEarlyNetworkProtection reproduces the out-of-chain guardian enhancement.
func ApplyEarlyNetworkProtection(participants []*Participant) {
	if len(participants) == 0 {
		return
	}

	var totalWeight int64
	for _, p := range participants {
		totalWeight += p.Weight
	}

	guardianIndices := make([]int, 0)
	var totalGuardianWeight int64
	for i, p := range participants {
		if _, ok := GenesisGuardianAddresses[p.OperatorAddress]; ok {
			guardianIndices = append(guardianIndices, i)
			totalGuardianWeight += p.Weight
		}
	}

	if totalWeight >= maturityThreshold || len(participants) < 2 || len(guardianIndices) == 0 {
		mirrorWeights(participants)
		return
	}

	otherWeight := totalWeight - totalGuardianWeight
	totalEnhancement := new(big.Rat).Mul(big.NewRat(otherWeight, 1), new(big.Rat).Set(genesisGuardianMultiplier))

	if totalEnhancement.Cmp(big.NewRat(totalGuardianWeight, 1)) < 0 {
		mirrorWeights(participants)
		return
	}

	perGuardian := new(big.Rat).Quo(totalEnhancement, big.NewRat(int64(len(guardianIndices)), 1))
	perGuardianWeight := ratFloor(perGuardian)

	guardianSet := make(map[int]struct{}, len(guardianIndices))
	for _, idx := range guardianIndices {
		guardianSet[idx] = struct{}{}
	}

	for i, p := range participants {
		if _, ok := guardianSet[i]; ok {
			p.ConsensusWeight = perGuardianWeight
			continue
		}
		p.ConsensusWeight = p.Weight
	}
}

func applyPowerCapping(participants []*Participant) {
	if len(participants) <= 1 {
		return
	}

	var totalWeight int64
	for _, p := range participants {
		totalWeight += p.Weight
	}

	if totalWeight == 0 {
		return
	}

	sortedParticipants := make([]struct {
		Index  int
		Weight int64
	}, len(participants))
	for i, p := range participants {
		sortedParticipants[i] = struct {
			Index  int
			Weight int64
		}{Index: i, Weight: p.Weight}
	}

	sort.Slice(sortedParticipants, func(i, j int) bool {
		return sortedParticipants[i].Weight < sortedParticipants[j].Weight
	})

	participantCount := len(participants)
	maxPercentage := new(big.Rat).Set(maxIndividualPowerPercentage)

	capValue := int64(-1)
	var sumPrev int64

	for k, entry := range sortedParticipants {
		currentPower := entry.Weight
		weightedTotal := sumPrev + currentPower*int64(participantCount-k)

		weightedTotalRat := big.NewRat(weightedTotal, 1)
		threshold := new(big.Rat).Mul(maxPercentage, weightedTotalRat)
		currentPowerRat := big.NewRat(currentPower, 1)

		if currentPowerRat.Cmp(threshold) > 0 {
			numerator := new(big.Rat).Mul(maxPercentage, big.NewRat(sumPrev, 1))

			remaining := int64(participantCount - k)
			denominator := new(big.Rat).Sub(big.NewRat(1, 1), new(big.Rat).Mul(maxPercentage, big.NewRat(remaining, 1)))

			if denominator.Sign() <= 0 {
				capValue = currentPower
				break
			}

			capRat := new(big.Rat).Quo(numerator, denominator)
			capValue = ratFloor(capRat)
			break
		}

		sumPrev += currentPower
	}

	if capValue == -1 {
		return
	}

	for _, p := range participants {
		if p.Weight > capValue {
			p.Weight = capValue
		}
	}
}

func ValidateParticipant(p *Participant) bool {
	if p == nil || len(p.ValidatorKey) == 0 {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(p.ValidatorKey)
	if err != nil || len(decoded) != 32 {
		return false
	}

	if _, _, err := bech32.Decode(p.Index); err != nil {
		return false
	}

	return true
}

// SetConsensusWeight mirrors the Python implementation in epochs.py.
func SetConsensusWeight(participants []*Participant, skipPowerCapping bool) []*Participant {
	eligible := make([]*Participant, 0, len(participants))
	for _, p := range participants {
		if p.Weight >= 0 {
			eligible = append(eligible, p)
		}
	}

	if !skipPowerCapping {
		applyPowerCapping(eligible)
	}

	ApplyEarlyNetworkProtection(eligible)

	validMap := make(map[string]*Participant)
	for _, p := range eligible {
		if ValidateParticipant(p) && p.ConsensusWeight > 0 {
			validMap[p.Index] = p
		}
	}

	for _, p := range participants {
		if vp, ok := validMap[p.Index]; ok {
			p.ConsensusWeight = vp.ConsensusWeight
			continue
		}
		p.ConsensusWeight = 0
	}

	return participants
}

func mirrorWeights(participants []*Participant) {
	for _, p := range participants {
		p.ConsensusWeight = p.Weight
	}
}

func ratFloor(r *big.Rat) int64 {
	if r == nil {
		return 0
	}

	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	if den.Sign() == 0 {
		return 0
	}

	return new(big.Int).Quo(num, den).Int64()
}

func getValidatorAddressFromPubKey(pubkey string) (string, error) {
	if pubkey == "" {
		return "", errors.New("empty pubkey")
	}

	decoded, err := base64.StdEncoding.DecodeString(pubkey)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(decoded)
	return strings.ToUpper(hex.EncodeToString(hash[:20])), nil
}

func getOperatorAddressFromAccountAddress(accountAddress, chainPrefix string) (string, error) {
	if chainPrefix == "" {
		chainPrefix = "gonka"
	}

	hrP, data, err := bech32.Decode(accountAddress)
	if err != nil {
		return "", err
	}

	if len(data) == 0 {
		return "", errors.New("invalid bech32 data")
	}

	_ = hrP // hrp is unused but validate

	decoded, err := convertBits(data, 5, 8, false)
	if err != nil {
		return "", err
	}

	converted, err := convertBits(decoded, 8, 5, true)
	if err != nil {
		return "", err
	}

	return bech32.Encode(chainPrefix+"valoper", converted)
}

func convertBits(data []byte, fromBits, toBits uint, pad bool) ([]byte, error) {
	var acc int
	var bits uint
	maxv := (1 << toBits) - 1
	maxAcc := (1 << (fromBits + toBits - 1)) - 1

	result := make([]byte, 0, len(data)*int(fromBits)/int(toBits))
	for _, value := range data {
		if (value >> fromBits) > 0 {
			return nil, errors.New("invalid bech32 data value")
		}
		acc = ((acc << fromBits) | int(value)) & maxAcc
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			result = append(result, byte((acc>>bits)&maxv))
		}
	}

	if pad {
		if bits > 0 {
			result = append(result, byte((acc<<(toBits-bits))&maxv))
		}
	} else if bits >= fromBits || ((acc<<(toBits-bits))&maxv) != 0 {
		return nil, errors.New("invalid padding in bech32 data")
	}

	return result, nil
}
