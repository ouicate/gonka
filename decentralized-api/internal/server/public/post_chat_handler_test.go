package public

import (
	"context"
	"testing"

	"decentralized-api/chainphase"
	"decentralized-api/payloadstorage"
	"decentralized-api/utils"

	"github.com/productscience/inference/x/inference/types"
	"github.com/stretchr/testify/require"
)

type mockPayloadStorage struct {
	stored         map[string]struct{ prompt, response string }
	storeErr       error
	retrieveErr    error
	retrieveCalled bool
}

func newMockPayloadStorage() *mockPayloadStorage {
	return &mockPayloadStorage{
		stored: make(map[string]struct{ prompt, response string }),
	}
}

func (m *mockPayloadStorage) Store(ctx context.Context, inferenceId string, epochId uint64, promptPayload, responsePayload string) error {
	if m.storeErr != nil {
		return m.storeErr
	}
	m.stored[inferenceId] = struct{ prompt, response string }{promptPayload, responsePayload}
	return nil
}

func (m *mockPayloadStorage) Retrieve(ctx context.Context, inferenceId string, epochId uint64) (string, string, error) {
	m.retrieveCalled = true
	if m.retrieveErr != nil {
		return "", "", m.retrieveErr
	}
	data, ok := m.stored[inferenceId]
	if !ok {
		return "", "", payloadstorage.ErrNotFound
	}
	return data.prompt, data.response, nil
}

func (m *mockPayloadStorage) PruneEpoch(ctx context.Context, epochId uint64) error {
	return nil
}

func newTestPhaseTracker(epochIndex uint64) *chainphase.ChainPhaseTracker {
	tracker := chainphase.NewChainPhaseTracker()
	epoch := types.Epoch{Index: epochIndex}
	params := types.EpochParams{
		EpochLength:      200,
		PocStageDuration: 50,
	}
	tracker.Update(
		chainphase.BlockInfo{Height: 100, Hash: "abc"},
		&epoch,
		&params,
		true,
		nil,
	)
	return tracker
}

func TestStorePayloadsToStorage_Success(t *testing.T) {
	storage := newMockPayloadStorage()
	tracker := newTestPhaseTracker(5)

	s := &Server{
		payloadStorage: storage,
		phaseTracker:   tracker,
	}

	promptPayload := `{"model":"test","seed":123,"messages":[{"role":"user","content":"hello"}]}`
	responsePayload := `{"id":"inf-1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}]}`
	responseHash, err := payloadstorage.ComputeResponseHash(responsePayload)
	require.NoError(t, err)

	s.storePayloadsToStorage(context.Background(), "inf-1", promptPayload, responsePayload, responseHash)

	// Verify storage was called
	require.Len(t, storage.stored, 1)
	stored := storage.stored["inf-1"]
	require.Equal(t, promptPayload, stored.prompt)
	require.Equal(t, responsePayload, stored.response)
}

func TestStorePayloadsToStorage_NilStorage(t *testing.T) {
	s := &Server{
		payloadStorage: nil,
		phaseTracker:   newTestPhaseTracker(5),
	}

	// Should not panic with nil storage
	s.storePayloadsToStorage(context.Background(), "inf-1", "prompt", "response", "hash")
}

func TestStorePayloadsToStorage_NilPhaseTracker(t *testing.T) {
	storage := newMockPayloadStorage()
	s := &Server{
		payloadStorage: storage,
		phaseTracker:   nil,
	}

	// Should not panic with nil phase tracker
	s.storePayloadsToStorage(context.Background(), "inf-1", "prompt", "response", "hash")
	require.Len(t, storage.stored, 0)
}

func TestVerifyStoredPayloads_HashMatch(t *testing.T) {
	storage := newMockPayloadStorage()
	tracker := newTestPhaseTracker(5)

	s := &Server{
		payloadStorage: storage,
		phaseTracker:   tracker,
	}

	promptPayload := `{"model":"test","seed":123}`
	responsePayload := `{"id":"inf-1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}]}`

	// Store the payload
	err := storage.Store(context.Background(), "inf-1", 5, promptPayload, responsePayload)
	require.NoError(t, err)

	// Compute expected hashes
	expectedPromptHash := utils.GenerateSHA256Hash(promptPayload)
	expectedResponseHash, err := payloadstorage.ComputeResponseHash(responsePayload)
	require.NoError(t, err)

	// Verify - should not log warnings (hashes match)
	s.verifyStoredPayloads(context.Background(), "inf-1", 5, promptPayload, expectedResponseHash)
	require.True(t, storage.retrieveCalled)

	// Verify the stored prompt hash matches
	storedPrompt, _, err := storage.Retrieve(context.Background(), "inf-1", 5)
	require.NoError(t, err)
	storedPromptHash := utils.GenerateSHA256Hash(storedPrompt)
	require.Equal(t, expectedPromptHash, storedPromptHash)
}

func TestVerifyStoredPayloads_RetrieveError(t *testing.T) {
	storage := newMockPayloadStorage()
	storage.retrieveErr = payloadstorage.ErrNotFound
	tracker := newTestPhaseTracker(5)

	s := &Server{
		payloadStorage: storage,
		phaseTracker:   tracker,
	}

	// Should handle retrieve error gracefully (just log warning)
	s.verifyStoredPayloads(context.Background(), "inf-1", 5, "prompt", "hash")
	require.True(t, storage.retrieveCalled)
}

func TestDualWriteIntegration(t *testing.T) {
	dir := t.TempDir()
	storage := payloadstorage.NewFileStorage(dir)
	tracker := newTestPhaseTracker(5)

	s := &Server{
		payloadStorage: storage,
		phaseTracker:   tracker,
	}

	promptPayload := `{"model":"test","seed":42,"messages":[{"role":"user","content":"test"}]}`
	responsePayload := `{"id":"inf-123","choices":[{"index":0,"message":{"role":"assistant","content":"response"}}]}`
	responseHash, err := payloadstorage.ComputeResponseHash(responsePayload)
	require.NoError(t, err)

	// Execute dual write
	s.storePayloadsToStorage(context.Background(), "inf-123", promptPayload, responsePayload, responseHash)

	// Verify file was created and can be retrieved
	storedPrompt, storedResponse, err := storage.Retrieve(context.Background(), "inf-123", 5)
	require.NoError(t, err)
	require.Equal(t, promptPayload, storedPrompt)
	require.Equal(t, responsePayload, storedResponse)

	// Verify hashes match
	storedResponseHash, err := payloadstorage.ComputeResponseHash(storedResponse)
	require.NoError(t, err)
	require.Equal(t, responseHash, storedResponseHash)
}

