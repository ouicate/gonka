package modelmanager

import (
	"context"
	"decentralized-api/apiconfig"
	"decentralized-api/chainphase"
	"decentralized-api/logging"
	"decentralized-api/mlnodeclient"
	"errors"
	"fmt"
	"time"

	"github.com/productscience/inference/x/inference/types"
)

// ConfigManagerInterface defines the minimal interface needed from ConfigManager
type ConfigManagerInterface interface {
	GetNodes() []apiconfig.InferenceNodeConfig
	GetCurrentNodeVersion() string
}

// PhaseTrackerInterface defines the minimal interface needed from PhaseTracker
type PhaseTrackerInterface interface {
	GetCurrentEpochState() *chainphase.EpochState
}

// ModelWeightManager ensures MLNodes have their configured models downloaded
// before they're needed. Runs independently as a background service.
type ModelWeightManager struct {
	configManager       ConfigManagerInterface
	phaseTracker        PhaseTrackerInterface
	mlNodeClientFactory mlnodeclient.ClientFactory
	checkInterval       time.Duration
}

// NewModelWeightManager creates a new model weight manager
func NewModelWeightManager(
	configManager ConfigManagerInterface,
	phaseTracker PhaseTrackerInterface,
	clientFactory mlnodeclient.ClientFactory,
	checkInterval time.Duration,
) *ModelWeightManager {
	return &ModelWeightManager{
		configManager:       configManager,
		phaseTracker:        phaseTracker,
		mlNodeClientFactory: clientFactory,
		checkInterval:       checkInterval,
	}
}

// Start begins the periodic model checking loop
func (m *ModelWeightManager) Start(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	logging.Info("ModelWeightManager started", types.System, "check_interval", m.checkInterval)

	for {
		select {
		case <-ticker.C:
			m.checkAndDownloadModels()
		case <-ctx.Done():
			logging.Info("ModelWeightManager stopped", types.System)
			return
		}
	}
}

// checkAndDownloadModels performs the periodic check and triggers downloads if needed
func (m *ModelWeightManager) checkAndDownloadModels() {
	epochState := m.phaseTracker.GetCurrentEpochState()
	if !m.isInDownloadWindow(epochState) {
		return
	}

	logging.Info("Starting model pre-download check",
		types.System,
		"block", epochState.CurrentBlock.Height,
		"phase", epochState.CurrentPhase)

	nodes := m.configManager.GetNodes()
	for _, node := range nodes {
		m.checkNodeModels(node)
	}
}

// isInDownloadWindow checks if we're in a safe window to download models
func (m *ModelWeightManager) isInDownloadWindow(epochState *chainphase.EpochState) bool {
	if epochState.IsNilOrNotSynced() {
		return false
	}

	if epochState.CurrentPhase != types.InferencePhase {
		return false
	}

	currentBlock := epochState.CurrentBlock.Height
	setNewValidators := epochState.LatestEpoch.SetNewValidators()
	inferenceValidationCutoff := epochState.LatestEpoch.InferenceValidationCutoff()

	// Window: [SetNewValidators + 30, InferenceValidationCutoff - 200]
	windowStart := setNewValidators + 30
	windowEnd := inferenceValidationCutoff - 200

	if currentBlock < windowStart || currentBlock > windowEnd {
		return false
	}

	return true
}

// checkNodeModels checks and downloads models for a specific node
func (m *ModelWeightManager) checkNodeModels(node apiconfig.InferenceNodeConfig) {
	version := m.configManager.GetCurrentNodeVersion()
	pocUrl := getPoCUrlWithVersion(node, version)
	inferenceUrl := getInferenceUrlWithVersion(node, version)
	client := m.mlNodeClientFactory.CreateClient(pocUrl, inferenceUrl)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpointAvailable := true

	for modelId := range node.Models {
		model := mlnodeclient.Model{
			HfRepo:   modelId,
			HfCommit: nil, // nil = latest
		}

		statusResp, err := client.CheckModelStatus(ctx, model)
		if err != nil {
			var apiNotImplemented *mlnodeclient.ErrAPINotImplemented
			if errors.As(err, &apiNotImplemented) {
				if endpointAvailable {
					logging.Info("Model pre-download endpoint not available",
						types.System,
						"node_id", node.Id)
					endpointAvailable = false
				}
				break
			}

			logging.Warn("Failed to check model status",
				types.System,
				"node_id", node.Id,
				"model", modelId,
				"error", err.Error())
			continue
		}

		switch statusResp.Status {
		case mlnodeclient.ModelStatusNotFound, mlnodeclient.ModelStatusPartial:
			logging.Info("Pre-downloading model",
				types.System,
				"model", modelId,
				"node_id", node.Id)

			_, err := client.DownloadModel(ctx, model)
			if err != nil {
				logging.Warn("Failed to start model download",
					types.System,
					"node_id", node.Id,
					"model", modelId,
					"error", err.Error())
			}

		case mlnodeclient.ModelStatusDownloading:
			logging.Debug("Model already downloading",
				types.System,
				"model", modelId,
				"node_id", node.Id)

		case mlnodeclient.ModelStatusDownloaded:
			logging.Debug("Model already downloaded",
				types.System,
				"model", modelId,
				"node_id", node.Id)
		}
	}
}

func getPoCUrlWithVersion(node apiconfig.InferenceNodeConfig, version string) string {
	if version == "" {
		return getPoCUrl(node)
	}
	return getPoCUrlVersioned(node, version)
}

func getInferenceUrlWithVersion(node apiconfig.InferenceNodeConfig, version string) string {
	if version == "" {
		return getInferenceUrl(node)
	}
	return getInferenceUrlVersioned(node, version)
}

func getPoCUrl(node apiconfig.InferenceNodeConfig) string {
	return formatURL(node.Host, node.PoCPort, node.PoCSegment)
}

func getPoCUrlVersioned(node apiconfig.InferenceNodeConfig, version string) string {
	return formatURLWithVersion(node.Host, node.PoCPort, version, node.PoCSegment)
}

func getInferenceUrl(node apiconfig.InferenceNodeConfig) string {
	return formatURL(node.Host, node.InferencePort, node.InferenceSegment)
}

func getInferenceUrlVersioned(node apiconfig.InferenceNodeConfig, version string) string {
	return formatURLWithVersion(node.Host, node.InferencePort, version, node.InferenceSegment)
}

func formatURL(host string, port int, segment string) string {
	return fmt.Sprintf("http://%s:%d%s", host, port, segment)
}

func formatURLWithVersion(host string, port int, version string, segment string) string {
	return fmt.Sprintf("http://%s:%d/%s%s", host, port, version, segment)
}
