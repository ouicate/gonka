package event_listener

import (
	"context"
	"sync"
	"testing"
	"time"

	"decentralized-api/apiconfig"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

// mockTmHTTPClient implements TmHTTPClient for tests.
type mockTmHTTPClient struct {
	mu          sync.Mutex
	calls       []int64
	txsPerBlock int
}

func newMockTmHTTPClient(txsPerBlock int) *mockTmHTTPClient {
	return &mockTmHTTPClient{txsPerBlock: txsPerBlock}
}

func (m *mockTmHTTPClient) BlockResults(ctx context.Context, height *int64) (*coretypes.ResultBlockResults, error) {
	m.mu.Lock()
	m.calls = append(m.calls, *height)
	m.mu.Unlock()

	// Build deterministic tx results for the requested height
	txs := make([]*abcitypes.ExecTxResult, m.txsPerBlock)
	for i := 0; i < m.txsPerBlock; i++ {
		txs[i] = &abcitypes.ExecTxResult{
			Events: []abcitypes.Event{
				{
					Type: "inference_finished",
					Attributes: []abcitypes.EventAttribute{
						{Key: "inference_id", Value: "id-", Index: true},
					},
				},
			},
		}
	}
	return &coretypes.ResultBlockResults{TxsResults: txs}, nil
}

// Test that BlockObserver handles a large backlog without deadlocking when the consumer is slow.
func TestBlockObserver_StressBackpressure(t *testing.T) {
	// Arrange
	manager := &apiconfig.ConfigManager{}
	bo := NewBlockObserverWithClient(manager, newMockTmHTTPClient(10))
	// Inject mock RPC client
	const (
		totalBlocks = 200
		txsPerBlock = 10
		totalEvents = totalBlocks * txsPerBlock
	)
	bo.tmClient = newMockTmHTTPClient(txsPerBlock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bo.Process(ctx)

	// Act: set caughtUp and jump height forward to create a backlog
	bo.updateStatus(totalBlocks, true)

	// Simulate slow consumer: delay before starting reads
	time.Sleep(100 * time.Millisecond)

	// Consume events slowly but ensure we eventually read them all
	received := 0
	deadline := time.After(5 * time.Second)
	for received < totalEvents {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for events: got %d, want %d", received, totalEvents)
		case ev, ok := <-bo.Queue.Out:
			if !ok {
				t.Fatalf("queue closed prematurely after %d events", received)
			}
			if ev == nil {
				t.Fatalf("nil event received at count=%d", received)
			}
			received++
			// Slow down the consumer a bit to exercise backpressure
			if received%200 == 0 {
				time.Sleep(5 * time.Millisecond)
			}
		}
	}

	// Assert: processed all events and advanced last processed height
	if got := bo.lastProcessedBlockHeight.Load(); got != totalBlocks {
		t.Fatalf("lastProcessedHeight=%d, want %d", got, totalBlocks)
	}
}

// Test that repeated status updates without changes do not re-trigger processing.
func TestBlockObserver_NoSpuriousWakeups(t *testing.T) {
	manager := &apiconfig.ConfigManager{}
	bo := NewBlockObserverWithClient(manager, newMockTmHTTPClient(1))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bo.Process(ctx)

	// First update triggers processing of height 1
	bo.updateStatus(1, true)

	// Wait for exactly 1 event
	select {
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for first event")
	case <-bo.Queue.Out:
	}

	// Extra duplicate updates should not produce more events
	for i := 0; i < 5; i++ {
		bo.updateStatus(1, true)
	}

	select {
	case <-time.After(200 * time.Millisecond):
		// ok, no new events
	case <-bo.Queue.Out:
		t.Fatalf("received unexpected extra event after duplicate updates")
	}
}

// Note: we rely on zero-value apiconfig.ConfigManager methods that read/write
// in-memory fields and no-op writes when WriterProvider is nil.
