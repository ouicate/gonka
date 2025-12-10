package public

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"decentralized-api/broker"
	"decentralized-api/chainphase"
	"decentralized-api/payloadstorage"

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

	s.storePayloadsToStorage(context.Background(), "inf-1", promptPayload, responsePayload)

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
	s.storePayloadsToStorage(context.Background(), "inf-1", "prompt", "response")
}

func TestStorePayloadsToStorage_NilPhaseTracker(t *testing.T) {
	storage := newMockPayloadStorage()
	s := &Server{
		payloadStorage: storage,
		phaseTracker:   nil,
	}

	// Should not panic with nil phase tracker
	s.storePayloadsToStorage(context.Background(), "inf-1", "prompt", "response")
	require.Len(t, storage.stored, 0)
}

func TestStorePayloadsToStorage_Retrieval(t *testing.T) {
	storage := newMockPayloadStorage()
	tracker := newTestPhaseTracker(5)

	s := &Server{
		payloadStorage: storage,
		phaseTracker:   tracker,
	}

	promptPayload := `{"model":"test","seed":123}`
	responsePayload := `{"id":"inf-1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}]}`

	s.storePayloadsToStorage(context.Background(), "inf-1", promptPayload, responsePayload)

	// Verify the stored payload can be retrieved
	storedPrompt, storedResponse, err := storage.Retrieve(context.Background(), "inf-1", 5)
	require.NoError(t, err)
	require.Equal(t, promptPayload, storedPrompt)
	require.Equal(t, responsePayload, storedResponse)
}

func TestFileStorageIntegration(t *testing.T) {
	dir := t.TempDir()
	storage := payloadstorage.NewFileStorage(dir)
	tracker := newTestPhaseTracker(5)

	s := &Server{
		payloadStorage: storage,
		phaseTracker:   tracker,
	}

	promptPayload := `{"model":"test","seed":42,"messages":[{"role":"user","content":"test"}]}`
	responsePayload := `{"id":"inf-123","choices":[{"index":0,"message":{"role":"assistant","content":"response"}}]}`

	s.storePayloadsToStorage(context.Background(), "inf-123", promptPayload, responsePayload)

	storedPrompt, storedResponse, err := storage.Retrieve(context.Background(), "inf-123", 5)
	require.NoError(t, err)
	require.Equal(t, promptPayload, storedPrompt)
	require.Equal(t, responsePayload, storedResponse)
}

func TestHTTPRequestWithAuthToken(t *testing.T) {
	tests := []struct {
		name       string
		authToken  string
		expectAuth bool
	}{
		{
			name:       "Empty token",
			authToken:  "",
			expectAuth: false,
		},
		{
			name:       "Valid token",
			authToken:  "valid-token",
			expectAuth: true,
		},
		{
			name:       "Whitespace token",
			authToken:  "   ",
			expectAuth: false,
		},
	}

	endpoints := []string{"tokenize", "completions"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &broker.Node{
				AuthToken: tt.authToken,
			}

			for _, endpoint := range endpoints {
				t.Run(endpoint+"Request", func(t *testing.T) {
					req, err := http.NewRequest("POST", "http://example.com/"+endpoint, bytes.NewReader([]byte("{}")))
					if err != nil {
						t.Fatal(err)
					}

					req.Header.Set("Content-Type", "application/json")
					if strings.TrimSpace(node.AuthToken) != "" {
						req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(node.AuthToken))
					}

					if tt.expectAuth {
						if req.Header.Get("Authorization") == "" {
							t.Error("Expected Authorization header but got none")
						}
					} else {
						if req.Header.Get("Authorization") != "" {
							t.Error("Expected no Authorization header but got one")
						}
					}
				})
			}
		})
	}
}
