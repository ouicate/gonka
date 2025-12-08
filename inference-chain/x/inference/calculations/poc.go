package calculations

import "github.com/productscience/inference/x/inference/types"

const PoCDefaultRTarget = 1.398077
const PoCDefaultBatchSize = 100
const PoCDefaultFraudThreshold = 1e-7

// getRTarget extracts RTarget from chain params or returns default.
func GetRTarget(chainParams *types.PoCModelParams) float64 {
	if chainParams != nil && chainParams.RTarget != nil {
		return chainParams.RTarget.ToFloat()
	}
	return PoCDefaultRTarget
}
