package keeper_test

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/productscience/inference/x/inference/calculations"
	"github.com/productscience/inference/x/inference/types"
)

const (
	benchmarkInferences = 1_000_000
	sampleSize          = 2000
)

// TestClaimRewardsPerformance measures getMustBeValidatedInferences performance
// Run with: go test -v -run TestClaimRewardsPerformance ./x/inference/keeper/
func TestClaimRewardsPerformance(t *testing.T) {
	inferences := make([]types.InferenceValidationDetails, benchmarkInferences)
	for i := 0; i < benchmarkInferences; i++ {
		inferences[i] = types.InferenceValidationDetails{
			InferenceId:        fmt.Sprintf("inf_%d", i),
			Model:              "model_1",
			ExecutorId:         fmt.Sprintf("executor_%d", i%100),
			ExecutorReputation: int32(i % 100),
			TrafficBasis:       10000,
		}
	}

	validatorAddr := "validator_1"
	seed := int64(12345)
	blockHash := []byte("0123456789abcdef0123456789abcdef")

	testParams := &types.ValidationParams{
		MinValidationAverage:        types.DecimalFromFloat(0.1),
		MaxValidationAverage:        types.DecimalFromFloat(1.0),
		FullValidationTrafficCutoff: 10000,
		MinValidationTrafficCutoff:  100,
		MinValidationHalfway:        types.DecimalFromFloat(0.05),
		EpochsToMax:                 10,
	}

	weightMap := map[string]bool{"model_1": true}

	t.Run("Baseline_NoSampling", func(t *testing.T) {
		start := time.Now()
		mustValidate := 0

		for _, inf := range inferences {
			if inf.ExecutorId == validatorAddr {
				continue
			}
			if !weightMap[inf.Model] {
				continue
			}

			shouldValidate, _ := calculations.ShouldValidate(
				seed, &inf, 1000, 100, 50, testParams, false,
			)
			if shouldValidate {
				mustValidate++
			}
		}

		elapsed := time.Since(start)
		t.Logf("No sampling: %d inferences, %d must validate, took %v (%.2f us/inference)",
			benchmarkInferences, mustValidate, elapsed, float64(elapsed.Microseconds())/float64(benchmarkInferences))
	})

	t.Run("Optimized_ReservoirSampling", func(t *testing.T) {
		start := time.Now()

		blockHashSeed := int64(binary.BigEndian.Uint64(blockHash[:8]))
		rng := rand.New(rand.NewSource(blockHashSeed))

		sample := make([]types.InferenceValidationDetails, 0, sampleSize)
		filteredCount := 0
		mustValidate := 0

		for _, inf := range inferences {
			if inf.ExecutorId == validatorAddr {
				continue
			}
			if !weightMap[inf.Model] {
				continue
			}
			filteredCount++

			if len(sample) < sampleSize {
				sample = append(sample, inf)
			} else {
				j := rng.Intn(filteredCount)
				if j < sampleSize {
					sample[j] = inf
				}
			}
		}

		for _, inf := range sample {
			shouldValidate, _ := calculations.ShouldValidate(
				seed, &inf, 1000, 100, 50, testParams, false,
			)
			if shouldValidate {
				mustValidate++
			}
		}

		elapsed := time.Since(start)
		t.Logf("Reservoir sampling: %d inferences, %d sampled, %d must validate, took %v (%.2f us/inference)",
			benchmarkInferences, len(sample), mustValidate, elapsed, float64(elapsed.Microseconds())/float64(benchmarkInferences))
	})
}
