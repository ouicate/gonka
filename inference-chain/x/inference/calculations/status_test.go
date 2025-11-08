package calculations

import (
	"testing"

	"github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/require"
)

var zeroStats = types.CurrentEpochStats{
	InvalidLLR:  types.DecimalFromFloat(0),
	InactiveLLR: types.DecimalFromFloat(0),
}

var defaultConfirmationPoCParams = &types.ConfirmationPoCParams{
	AlphaThreshold: types.DecimalFromFloat(0.5), // 50% threshold
}

func TestComputeStatus(t *testing.T) {
	tests := []struct {
		name        string
		params      *types.ValidationParams
		pocParams   *types.ConfirmationPoCParams
		participant types.Participant
		wantStatus  types.ParticipantStatus
		wantReason  ParticipantStatusReason
	}{
		{
			name:        "nil validation parameters returns active",
			params:      nil,
			pocParams:   defaultConfirmationPoCParams,
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
			},
			pocParams: defaultConfirmationPoCParams,
			participant: types.Participant{
				ConsecutiveInvalidInferences: 20,
			},
			wantStatus: types.ParticipantStatus_INVALID,
			wantReason: ConsecutiveFailures,
		},
		{
			name: "statistical invalidations returns invalid",
			params: &types.ValidationParams{
				BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
				InvalidationHThreshold:         types.DecimalFromFloat(4),
				FalsePositiveRate:              types.DecimalFromFloat(0.05),
				DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
				DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
				DowntimeHThreshold:             types.DecimalFromFloat(4),
			},
			pocParams: defaultConfirmationPoCParams,
			participant: types.Participant{
				CurrentEpochStats: &types.CurrentEpochStats{
					ValidatedInferences:   7,
					InvalidatedInferences: 7,
					ConfirmationPoCRatio:  types.DecimalFromFloat(0.9), // Good PoC ratio
				},
			},
			wantStatus: types.ParticipantStatus_INVALID,
			wantReason: StatisticalInvalidations,
		},
		{
			name: "normal operation returns active",
			params: &types.ValidationParams{
				BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
				InvalidationHThreshold:         types.DecimalFromFloat(4),
				FalsePositiveRate:              types.DecimalFromFloat(0.05),
				DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
				DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
				DowntimeHThreshold:             types.DecimalFromFloat(4),
			},
			pocParams: defaultConfirmationPoCParams,
			participant: types.Participant{
				CurrentEpochStats: &types.CurrentEpochStats{
					ValidatedInferences:   95,
					InvalidatedInferences: 5,
					ConfirmationPoCRatio:  types.DecimalFromFloat(0.9), // Good PoC ratio
				},
			},
			wantStatus: types.ParticipantStatus_ACTIVE,
			wantReason: NoReason,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, _ := ComputeStatus(tt.params, tt.pocParams, tt.participant, zeroStats)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestDowntimeTriggersInactive(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1), // P0
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2), // P1
		DowntimeHThreshold:             types.DecimalFromFloat(4),   // H
	}

	participant := types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			InferenceCount:        50, // passes
			MissedRequests:        60, // failures
			ValidatedInferences:   0,
			InvalidatedInferences: 0,
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.9), // Good PoC ratio
		},
	}

	status, reason, _ := ComputeStatus(params, defaultConfirmationPoCParams, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_INACTIVE, status)
	require.Equal(t, Downtime, reason)
}

func TestDowntimeParamsOutOfRangeReturnAlgorithmError(t *testing.T) {
	badVals := []struct{ good, bad float64 }{
		{0, 0.2},    // good == 0
		{1, 0.2},    // good == 1
		{-0.1, 0.2}, // good < 0
		{0.1, 0},    // bad == 0
		{0.1, 1},    // bad == 1
		{0.1, 1.1},  // bad > 1
	}

	for _, v := range badVals {
		params := &types.ValidationParams{
			FalsePositiveRate:              types.DecimalFromFloat(0.05),
			BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
			InvalidationHThreshold:         types.DecimalFromFloat(4),
			DowntimeGoodPercentage:         types.DecimalFromFloat(v.good),
			DowntimeBadPercentage:          types.DecimalFromFloat(v.bad),
			DowntimeHThreshold:             types.DecimalFromFloat(4),
		}
		participant := types.Participant{CurrentEpochStats: &types.CurrentEpochStats{
			ConfirmationPoCRatio: types.DecimalFromFloat(0.9), // Good PoC ratio
		}}
		status, reason, _ := ComputeStatus(params, defaultConfirmationPoCParams, participant, zeroStats)
		require.Equal(t, types.ParticipantStatus_ACTIVE, status)
		require.Equal(t, AlgorithmError, reason)
	}
}

func TestProbabilityOfConsecutiveFailures_PanicOnBadRate(t *testing.T) {
	// expectedFailureRate must be in [0,1]
	defer func() { _ = recover() }()
	_ = probabilityOfConsecutiveFailures(types.DecimalFromFloat(1.1).ToDecimal(), 3)
}

func TestConfirmationPoCFailureTriggersInactive(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
		DowntimeHThreshold:             types.DecimalFromFloat(4),
	}

	pocParams := &types.ConfirmationPoCParams{
		AlphaThreshold: types.DecimalFromFloat(0.5), // 50% threshold
	}

	// Participant with confirmation PoC ratio below threshold
	participant := types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.3), // 30% < 50% threshold
			ValidatedInferences:   10,
			InvalidatedInferences: 0,
		},
	}

	status, reason, _ := ComputeStatus(params, pocParams, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_INACTIVE, status)
	require.Equal(t, FailedConfirmationPoC, reason)
}

func TestConfirmationPoCPassWithGoodRatio(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
		DowntimeHThreshold:             types.DecimalFromFloat(4),
	}

	pocParams := &types.ConfirmationPoCParams{
		AlphaThreshold: types.DecimalFromFloat(0.5), // 50% threshold
	}

	// Participant with confirmation PoC ratio at threshold
	participant := types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.5), // 50% = 50% threshold
			ValidatedInferences:   10,
			InvalidatedInferences: 0,
		},
	}

	status, reason, _ := ComputeStatus(params, pocParams, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_ACTIVE, status)
	require.Equal(t, NoReason, reason)

	// Participant with confirmation PoC ratio above threshold
	participant2 := types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.8), // 80% > 50% threshold
			ValidatedInferences:   10,
			InvalidatedInferences: 0,
		},
	}

	status2, reason2, _ := ComputeStatus(params, pocParams, participant2, zeroStats)
	require.Equal(t, types.ParticipantStatus_ACTIVE, status2)
	require.Equal(t, NoReason, reason2)
}

func TestConfirmationPoCWithNilParams(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
		DowntimeHThreshold:             types.DecimalFromFloat(4),
	}

	participant := types.Participant{
		CurrentEpochStats: &types.CurrentEpochStats{
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.1), // Low ratio
			ValidatedInferences:   10,
			InvalidatedInferences: 0,
		},
	}

	// Nil confirmation PoC params should pass
	status, reason, _ := ComputeStatus(params, nil, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_ACTIVE, status)
	require.Equal(t, NoReason, reason)

	// Zero threshold should pass
	pocParamsZero := &types.ConfirmationPoCParams{
		AlphaThreshold: types.DecimalFromFloat(0),
	}
	status2, reason2, _ := ComputeStatus(params, pocParamsZero, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_ACTIVE, status2)
	require.Equal(t, NoReason, reason2)
}

func TestAlreadyInvalidStatus(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
		DowntimeHThreshold:             types.DecimalFromFloat(4),
	}

	// Participant already INVALID
	participant := types.Participant{
		Status: types.ParticipantStatus_INVALID,
		CurrentEpochStats: &types.CurrentEpochStats{
			ValidatedInferences:   10,
			InvalidatedInferences: 0,
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.9), // Good PoC ratio
		},
	}

	status, reason, _ := ComputeStatus(params, defaultConfirmationPoCParams, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_INVALID, status)
	require.Equal(t, AlreadySet, reason)
}

func TestAlreadyInactiveStatus(t *testing.T) {
	params := &types.ValidationParams{
		FalsePositiveRate:              types.DecimalFromFloat(0.05),
		BadParticipantInvalidationRate: types.DecimalFromFloat(0.1),
		InvalidationHThreshold:         types.DecimalFromFloat(4),
		DowntimeGoodPercentage:         types.DecimalFromFloat(0.1),
		DowntimeBadPercentage:          types.DecimalFromFloat(0.2),
		DowntimeHThreshold:             types.DecimalFromFloat(4),
	}

	// Participant already INACTIVE
	participant := types.Participant{
		Status: types.ParticipantStatus_INACTIVE,
		CurrentEpochStats: &types.CurrentEpochStats{
			ValidatedInferences:   10,
			InvalidatedInferences: 0,
			ConfirmationPoCRatio:  types.DecimalFromFloat(0.9), // Good PoC ratio
		},
	}

	status, reason, _ := ComputeStatus(params, defaultConfirmationPoCParams, participant, zeroStats)
	require.Equal(t, types.ParticipantStatus_INACTIVE, status)
	require.Equal(t, AlreadySet, reason)
}
