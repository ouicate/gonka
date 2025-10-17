package tx_manager

import (
	"context"
	"decentralized-api/chainphase"
	"decentralized-api/logging"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/productscience/inference/x/inference/types"
)

type ChainTracker struct {
	latestBlockTime   atomic.Value
	latestBlockHeight int64
	lastUpdatedAt     time.Time
	maxBlockTimeout   time.Duration
	chainHalt         bool
	mtx               sync.Mutex
	client            *cosmosclient.Client
}

func NewChainTracker(client *cosmosclient.Client, maxBlockTimeout time.Duration) *ChainTracker {
	value := atomic.Value{}
	value.Store(time.Time{})
	return &ChainTracker{
		latestBlockTime:   value,
		latestBlockHeight: 0,
		lastUpdatedAt:     time.Now(),
		maxBlockTimeout:   maxBlockTimeout,
		chainHalt:         false,
		client:            client,
	}
}

func (ct *ChainTracker) GetLatestBlockTime(ctx context.Context) time.Time {
	blockTs := ct.latestBlockTime.Load().(time.Time)
	if blockTs.IsZero() {
		_, err := ct.UpdateFromClient(ctx)
		if err != nil {
			return time.Time{}
		}
	}
	return blockTs
}

func (ct *ChainTracker) UpdateFromEvent(blockInfo *chainphase.BlockInfo) {
	ct.mtx.Lock()
	logging.Info("UpdateFromEvent", types.Messages, "block_height", blockInfo.Height, "block_time", blockInfo.Time.String())
	defer ct.mtx.Unlock()
	if blockInfo.Time.After(ct.latestBlockTime.Load().(time.Time)) &&
		blockInfo.Height > ct.latestBlockHeight {
		ct.latestBlockHeight = blockInfo.Height
		ct.latestBlockTime.Store(blockInfo.Time)
		ct.chainHalt = false
	}

	ct.lastUpdatedAt = time.Now()
	return
}

func (ct *ChainTracker) UpdateFromClient(ctx context.Context) (bool, error) {
	now := time.Now()
	// Under operation, this should rarely be called, as the new block event will update proactively
	if now.Sub(ct.lastUpdatedAt) < time.Second*6 {
		return ct.chainHalt, nil
	}

	status, err := ct.client.Status(ctx)
	if err != nil {
		logging.Error("error getting blockchain status", types.Messages, "err", err)
		return false, err
	}

	ct.mtx.Lock()
	defer ct.mtx.Unlock()

	if status.SyncInfo.LatestBlockTime.Equal(ct.latestBlockTime.Load().(time.Time)) &&
		status.SyncInfo.LatestBlockHeight == ct.latestBlockHeight &&
		!ct.lastUpdatedAt.IsZero() && now.Sub(ct.lastUpdatedAt) > ct.maxBlockTimeout {
		// same block, and we sow it more than N seconds ago -> chain halt
		ct.chainHalt = true
	}

	if status.SyncInfo.LatestBlockTime.After(ct.latestBlockTime.Load().(time.Time)) &&
		status.SyncInfo.LatestBlockHeight > ct.latestBlockHeight {
		ct.latestBlockHeight = status.SyncInfo.LatestBlockHeight
		ct.latestBlockTime.Store(status.SyncInfo.LatestBlockTime)
		ct.chainHalt = false
	}

	ct.lastUpdatedAt = now
	if ct.chainHalt {
		logging.Error("Chain halt or slowdown", types.Messages, "latest_block_time", ct.latestBlockTime.Load().(time.Time), "latest_block_height", ct.latestBlockHeight)
	}
	return ct.chainHalt, nil
}
