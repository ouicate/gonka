package modelmanager

import (
	"context"
	"decentralized-api/apiconfig"
	"decentralized-api/chainphase"
	"decentralized-api/mlnodeclient"
	"errors"
	"testing"
	"time"

	"github.com/productscience/inference/x/inference/types"
)

// configManagerInterface defines the minimal interface needed by ModelWeightManager
type configManagerInterface interface {
	GetNodes() []apiconfig.InferenceNodeConfig
	GetCurrentNodeVersion() string
}

// Mock ConfigManager
type mockConfigManager struct {
	nodes              []apiconfig.InferenceNodeConfig
	currentNodeVersion string
}

func (m *mockConfigManager) GetNodes() []apiconfig.InferenceNodeConfig {
	return m.nodes
}

func (m *mockConfigManager) GetCurrentNodeVersion() string {
	return m.currentNodeVersion
}

// Mock PhaseTracker
type mockPhaseTracker struct {
	epochState *chainphase.EpochState
}

func (m *mockPhaseTracker) GetCurrentEpochState() *chainphase.EpochState {
	return m.epochState
}

// Mock ClientFactory
type mockClientFactory struct {
	client mlnodeclient.MLNodeClient
}

func (m *mockClientFactory) CreateClient(pocUrl, inferenceUrl string) mlnodeclient.MLNodeClient {
	return m.client
}

// Custom mock client for testing error handling
type customMockClient struct {
	*mlnodeclient.MockClient
	callCount int
}

func (c *customMockClient) CheckModelStatus(ctx context.Context, model mlnodeclient.Model) (*mlnodeclient.ModelStatusResponse, error) {
	c.callCount++
	if c.callCount == 1 {
		return nil, errors.New("network error")
	}
	return c.MockClient.CheckModelStatus(ctx, model)
}

// Test isInDownloadWindow
func TestIsInDownloadWindow(t *testing.T) {
	manager := &ModelWeightManager{}

	t.Run("nil epoch state", func(t *testing.T) {
		if manager.isInDownloadWindow(nil) {
			t.Error("expected false for nil epoch state")
		}
	})

	t.Run("not synced", func(t *testing.T) {
		epochState := &chainphase.EpochState{
			IsSynced: false,
		}
		if manager.isInDownloadWindow(epochState) {
			t.Error("expected false for not synced state")
		}
	})

	t.Run("not in inference phase", func(t *testing.T) {
		epochState := &chainphase.EpochState{
			IsSynced:     true,
			CurrentPhase: types.PoCGeneratePhase,
			LatestEpoch: types.EpochContext{
				EpochIndex:          1,
				PocStartBlockHeight: 1000,
				EpochParams: types.EpochParams{
					PocStageDuration:          100,
					PocValidationDuration:     100,
					InferenceValidationCutoff: 200,
					SetNewValidatorsDelay:     50,
				},
			},
			CurrentBlock: chainphase.BlockInfo{Height: 1100},
		}
		if manager.isInDownloadWindow(epochState) {
			t.Error("expected false for non-inference phase")
		}
	})

	t.Run("before window start", func(t *testing.T) {
		epochParams := types.EpochParams{
			PocStageDuration:          1000,
			PocValidationDuration:     1000,
			InferenceValidationCutoff: 200,
			SetNewValidatorsDelay:     100,
		}
		epochState := &chainphase.EpochState{
			IsSynced:     true,
			CurrentPhase: types.InferencePhase,
			LatestEpoch: types.NewEpochContext(
				types.Epoch{
					Index:               1,
					PocStartBlockHeight: 10000,
				},
				epochParams,
			),
			CurrentBlock: chainphase.BlockInfo{Height: 11100}, // Before windowStart (11130)
		}
		if manager.isInDownloadWindow(epochState) {
			t.Error("expected false for block before window start")
		}
	})

	t.Run("after window end", func(t *testing.T) {
		epochParams := types.EpochParams{
			PocStageDuration:          1000,
			PocValidationDuration:     1000,
			InferenceValidationCutoff: 200,
			SetNewValidatorsDelay:     100,
		}
		ec := types.NewEpochContext(
			types.Epoch{
				Index:               1,
				PocStartBlockHeight: 10000,
			},
			epochParams,
		)
		// NextPoCStart = 10000 + 1000 + 1000 = 12000
		// InferenceValidationCutoff = 12000 - 200 = 11800
		// windowEnd = 11800 - 200 = 11600
		epochState := &chainphase.EpochState{
			IsSynced:     true,
			CurrentPhase: types.InferencePhase,
			LatestEpoch:  ec,
			CurrentBlock: chainphase.BlockInfo{Height: 11700}, // After windowEnd (11600)
		}
		if manager.isInDownloadWindow(epochState) {
			t.Error("expected false for block after window end")
		}
	})

	t.Run("inside window", func(t *testing.T) {
		epochParams := types.EpochParams{
			EpochLength:               10000,
			PocStageDuration:          1000,
			PocExchangeDuration:       100,
			PocValidationDelay:        100,
			PocValidationDuration:     1000,
			SetNewValidatorsDelay:     100,
			InferenceValidationCutoff: 500,
		}
		ec := types.NewEpochContext(
			types.Epoch{
				Index:               1,
				PocStartBlockHeight: 10000,
			},
			epochParams,
		)
		// Calculation:
		// getPocAnchor = 10000
		// GetEndOfPoCStage = 0 + 1000 = 1000
		// GetStartOfPoCValidationStage = 1000 + 100 = 1100
		// GetEndOfPoCValidationStage = 1100 + 1000 = 2100
		// GetSetNewValidatorsStage = 2100 + 100 = 2200
		// SetNewValidators = 10000 + 2200 = 12200
		// windowStart = 12200 + 30 = 12230
		// NextPoCStart = 10000 + 10000 = 20000
		// InferenceValidationCutoff = 20000 - 500 = 19500
		// windowEnd = 19500 - 200 = 19300
		epochState := &chainphase.EpochState{
			IsSynced:     true,
			CurrentPhase: types.InferencePhase,
			LatestEpoch:  ec,
			CurrentBlock: chainphase.BlockInfo{Height: 15000}, // Inside window [12230, 19300]
		}
		if !manager.isInDownloadWindow(epochState) {
			t.Error("expected true for block inside window")
		}
	})
}

// Test checkNodeModels
func TestCheckNodeModels(t *testing.T) {
	t.Run("model not found triggers download", func(t *testing.T) {
		mockClient := mlnodeclient.NewMockClient()
		// Don't add to CachedModels - it will return NOT_FOUND by default

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"test-model": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		if mockClient.CheckModelStatusCalled != 1 {
			t.Errorf("expected CheckModelStatus to be called once, got %d", mockClient.CheckModelStatusCalled)
		}

		if mockClient.DownloadModelCalled != 1 {
			t.Errorf("expected DownloadModel to be called once, got %d", mockClient.DownloadModelCalled)
		}
	})

	t.Run("partial model triggers download", func(t *testing.T) {
		mockClient := mlnodeclient.NewMockClient()
		mockClient.CachedModels["test-model:latest"] = mlnodeclient.ModelListItem{
			Model: mlnodeclient.Model{
				HfRepo:   "test-model",
				HfCommit: nil,
			},
			Status: mlnodeclient.ModelStatusPartial,
		}

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"test-model": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		if mockClient.DownloadModelCalled != 1 {
			t.Errorf("expected DownloadModel to be called once, got %d", mockClient.DownloadModelCalled)
		}
	})

	t.Run("already downloading skips download", func(t *testing.T) {
		mockClient := mlnodeclient.NewMockClient()
		// Add to DownloadingModels to simulate downloading state
		mockClient.DownloadingModels["test-model:latest"] = &mlnodeclient.DownloadProgress{
			StartTime:      1234567890,
			ElapsedSeconds: 100,
		}

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"test-model": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		if mockClient.CheckModelStatusCalled != 1 {
			t.Errorf("expected CheckModelStatus to be called once, got %d", mockClient.CheckModelStatusCalled)
		}

		if mockClient.DownloadModelCalled != 0 {
			t.Errorf("expected DownloadModel not to be called, got %d", mockClient.DownloadModelCalled)
		}
	})

	t.Run("already downloaded skips download", func(t *testing.T) {
		mockClient := mlnodeclient.NewMockClient()
		mockClient.CachedModels["test-model:latest"] = mlnodeclient.ModelListItem{
			Model: mlnodeclient.Model{
				HfRepo:   "test-model",
				HfCommit: nil,
			},
			Status: mlnodeclient.ModelStatusDownloaded,
		}

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"test-model": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		if mockClient.CheckModelStatusCalled != 1 {
			t.Errorf("expected CheckModelStatus to be called once, got %d", mockClient.CheckModelStatusCalled)
		}

		if mockClient.DownloadModelCalled != 0 {
			t.Errorf("expected DownloadModel not to be called, got %d", mockClient.DownloadModelCalled)
		}
	})

	t.Run("endpoint not implemented stops checking node", func(t *testing.T) {
		mockClient := mlnodeclient.NewMockClient()
		mockClient.CheckModelStatusError = &mlnodeclient.ErrAPINotImplemented{
			Endpoint:   "/api/v1/models/status",
			StatusCode: 404,
		}

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"model1": {Args: []string{}},
						"model2": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		// Should only check once and then stop
		if mockClient.CheckModelStatusCalled != 1 {
			t.Errorf("expected CheckModelStatus to be called once, got %d", mockClient.CheckModelStatusCalled)
		}

		if mockClient.DownloadModelCalled != 0 {
			t.Errorf("expected DownloadModel not to be called, got %d", mockClient.DownloadModelCalled)
		}
	})

	t.Run("network error continues to next model", func(t *testing.T) {
		// Create a custom mock that will fail only on first call
		mockClient := &customMockClient{
			MockClient: mlnodeclient.NewMockClient(),
			callCount:  0,
		}

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"model1": {Args: []string{}},
						"model2": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		// Should try checking both models despite first error
		if mockClient.callCount != 2 {
			t.Errorf("expected CheckModelStatus to be called twice, got %d", mockClient.callCount)
		}
	})

	t.Run("multiple models in config", func(t *testing.T) {
		mockClient := mlnodeclient.NewMockClient()
		// model1: NOT_FOUND (not in CachedModels)
		// model2: DOWNLOADED
		mockClient.CachedModels["model2:latest"] = mlnodeclient.ModelListItem{
			Model: mlnodeclient.Model{
				HfRepo:   "model2",
				HfCommit: nil,
			},
			Status: mlnodeclient.ModelStatusDownloaded,
		}
		// model3: NOT_FOUND (not in CachedModels)

		configMgr := &mockConfigManager{
			nodes: []apiconfig.InferenceNodeConfig{
				{
					Id:               "node1",
					Host:             "localhost",
					PoCPort:          8080,
					PoCSegment:       "/api",
					InferencePort:    8081,
					InferenceSegment: "/inference",
					Models: map[string]apiconfig.ModelConfig{
						"model1": {Args: []string{}},
						"model2": {Args: []string{}},
						"model3": {Args: []string{}},
					},
				},
			},
			currentNodeVersion: "",
		}

		factory := &mockClientFactory{client: mockClient}

		manager := NewModelWeightManager(
			configMgr,
			nil,
			factory,
			30*time.Minute,
		)

		manager.checkNodeModels(configMgr.nodes[0])

		if mockClient.CheckModelStatusCalled != 3 {
			t.Errorf("expected CheckModelStatus to be called 3 times, got %d", mockClient.CheckModelStatusCalled)
		}

		// Should download 2 models (model1 and model3 are NOT_FOUND)
		if mockClient.DownloadModelCalled != 2 {
			t.Errorf("expected DownloadModel to be called twice, got %d", mockClient.DownloadModelCalled)
		}
	})
}

// Test URL formatting
func TestURLFormatting(t *testing.T) {
	node := apiconfig.InferenceNodeConfig{
		Host:             "localhost",
		PoCPort:          8080,
		PoCSegment:       "/api/v1",
		InferencePort:    8081,
		InferenceSegment: "/inference",
	}

	t.Run("PoC URL without version", func(t *testing.T) {
		url := getPoCUrl(node)
		expected := "http://localhost:8080/api/v1"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})

	t.Run("PoC URL with version", func(t *testing.T) {
		url := getPoCUrlVersioned(node, "v2")
		expected := "http://localhost:8080/v2/api/v1"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})

	t.Run("Inference URL without version", func(t *testing.T) {
		url := getInferenceUrl(node)
		expected := "http://localhost:8081/inference"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})

	t.Run("Inference URL with version", func(t *testing.T) {
		url := getInferenceUrlVersioned(node, "v2")
		expected := "http://localhost:8081/v2/inference"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})

	t.Run("URL with version helper", func(t *testing.T) {
		url := getPoCUrlWithVersion(node, "v2")
		expected := "http://localhost:8080/v2/api/v1"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})

	t.Run("URL without version helper (empty string)", func(t *testing.T) {
		url := getPoCUrlWithVersion(node, "")
		expected := "http://localhost:8080/api/v1"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})
}
