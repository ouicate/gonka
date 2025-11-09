package calculations

import (
	"testing"

	"github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeStatus(t *testing.T) {
	tests := []struct {
		name        string
		params      *types.ValidationParams
		participant types.Participant
		wantStatus  types.ParticipantStatus
		wantReason  ParticipantStatusReason
	}{
		{
			name:        "nil validation parameters returns active",
			params:      nil,
			participant: types.Participant{},
			wantStatus:  types.ParticipantStatus_ACTIVE,
			wantReason:  NoReason,
		},
		{
			name: "consecutive failures returns invalid",
			params: &types.ValidationParams{
				FalsePositiveRate:              types.DecimalFromFloat(0.05),
				BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
				InvalidationHThreshold:         types.DecimalFromFloat(4),
				DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
				DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
				DowntimeHThreshold:             types.DecimalFromFloat(4),
				QuickFailureThreshold:          types.DecimalFromFloat(0.000001),
			},
			participant: types.Participant{
				ConsecutiveInvalidInferences: 20,
			},
			wantStatus: types.ParticipantStatus_INVALID,
			wantReason: ConsecutiveFailures,
		},
		{
			name: "statistical invalidations returns invalid",
			params: &types.ValidationParams{
				FalsePositiveRate:              types.DecimalFromFloat(0.05),
				BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
				InvalidationHThreshold:         types.DecimalFromFloat(4),
				DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
				DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
				DowntimeHThreshold:             types.DecimalFromFloat(4),
				QuickFailureThreshold:          types.DecimalFromFloat(0.000001),
			},
			participant: types.Participant{
				ConsecutiveInvalidInferences: 5,
				CurrentEpochStats: &types.CurrentEpochStats{
					ValidatedInferences:   0,
					InvalidatedInferences: 100,
					InvalidLLR:            types.DecimalFromFloat(10),
				},
			},
			wantStatus: types.ParticipantStatus_INVALID,
			wantReason: StatisticalInvalidations,
		},
		{
			name: "returns active when invalid LLR is below threshold",
			params: &types.ValidationParams{
				FalsePositiveRate:              types.DecimalFromFloat(0.05),
				BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
				InvalidationHThreshold:         types.DecimalFromFloat(4),
				DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
				DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
				DowntimeHThreshold:             types.DecimalFromFloat(4),
				QuickFailureThreshold:          types.DecimalFromFloat(0.000001),
			},
			participant: types.Participant{
				CurrentEpochStats: &types.CurrentEpochStats{
					ValidatedInferences:   95,
					InvalidatedInferences: 5,
				},
			},
			wantStatus: types.ParticipantStatus_ACTIVE,
			wantReason: NoReason,
		},
	}

	zeroStats := types.CurrentEpochStats{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, _ := ComputeStatus(tt.params, nil, tt.participant, zeroStats)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestComputeStatus_DowntimeFail(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
		DowntimeHThreshold:             types.DecimalFromFloat(4),
		QuickFailureThreshold:          types.DecimalFromFloat(0.000001),
	}
	zeroStats := types.CurrentEpochStats{}
	participant := types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			InferenceCount:        50, // passes
			MissedRequests:        60, // failures
			ValidatedInferences:   0,
			InvalidatedInferences: 0,
		},
	}

	status, reason, _ := ComputeStatus(params, nil, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_INACTIVE, status)
	require.Equal(t, Downtime, reason)
}

func TestDowntimeParamsOutOfRangeReturnAlgorithmError(t *testing.T) {
	badVals := []struct{ good, bad float64 }{
		{0.8, 0.1},
		{0.3, 0.2},
		{0.5, 0.6}, // bad > good
	}

	zeroStats := types.CurrentEpochStats{}
	for _, v := range badVals {
		params := &types.ValidationParams{
			FalsePositiveRate:              types.DecimalFromFloat(0.05),
			BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
			InvalidationHThreshold:         types.DecimalFromFloat(4),
			DowntimeGoodPercentage:         types.DecimalFromFloat(v.good),
			DowntimeBadPercentage:          types.DecimalFromFloat(v.bad),
			DowntimeHThreshold:             types.DecimalFromFloat(4),
			QuickFailureThreshold:          types.DecimalFromFloat(0.000001),
		}
		participant := types.Participant{CurrentEpochStats: &types.CurrentEpochStats{}}
		status, reason, _ := ComputeStatus(params, nil, participant, zeroStats)
		require.Equal(t, types.ParticipantStatus_ACTIVE, status)
		require.Equal(t, AlgorithmError, reason)
	}
}

func TestProbabilityOfConsecutiveFailures_PanicOnBadRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic on invalid expectedFailureRate")
		}
	}()

	_ = probabilityOfConsecutiveFailures(types.DecimalFromFloat(1.5).ToDecimal(), 1)
}

func TestGetStats(t *testing.T) {
	part := &types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			InvalidLLR:  types.DecimalFromFloat(1.5),
			InactiveLLR: types.DecimalFromFloat(2.0),
		},
	}

	result := getStats(part)
	assert.NotNil(t, result.InvalidLLR)
	assert.NotNil(t, result.InactiveLLR)

	// Test with nil participant
	part2 := &types.Participant{}
	result2 := getStats(part2)
	assert.NotNil(t, result2.InvalidLLR)
	assert.NotNil(t, result2.InactiveLLR)
	assert.Equal(t, int64(0), result2.InvalidLLR.Value)
	assert.Equal(t, int64(0), result2.InactiveLLR.Value)
}
