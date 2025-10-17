package tx_manager

import (
	"context"
	"decentralized-api/apiconfig"
	"decentralized-api/logging"
	"errors"
	"time"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"

	"github.com/nats-io/nats.go"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/productscience/inference/app"
	blstypes "github.com/productscience/inference/x/bls/types"
	collateraltypes "github.com/productscience/inference/x/collateral/types"
	"github.com/productscience/inference/x/inference/types"
	restrictionstypes "github.com/productscience/inference/x/restrictions/types"
)

type TxManager interface {
	SendTransactionAsyncWithRetry(rawTx sdk.Msg) (*sdk.TxResponse, error)
	SendTransactionAsyncNoRetry(rawTx sdk.Msg) (*sdk.TxResponse, error)
	SendTransactionSyncNoRetry(msg proto.Message) (*ctypes.ResultTx, error)
}

type manager struct {
	ctx              context.Context
	client           *cosmosclient.Client
	apiAccount       *apiconfig.ApiAccount
	chainTracker     *ChainTracker
	broadcaster      *Broadcaster
	transactionQueue *TransactionQueue
}

func StartTxManager(
	ctx context.Context,
	client *cosmosclient.Client,
	account *apiconfig.ApiAccount,
	natsConnection *nats.Conn,
	chainTracker *ChainTracker,
	broadcaster *Broadcaster) (*manager, error) {

	// Register all module interfaces to match admin server codec
	app.RegisterLegacyModules(client.Context().InterfaceRegistry)
	types.RegisterInterfaces(client.Context().InterfaceRegistry)
	banktypes.RegisterInterfaces(client.Context().InterfaceRegistry)
	v1.RegisterInterfaces(client.Context().InterfaceRegistry)
	upgradetypes.RegisterInterfaces(client.Context().InterfaceRegistry)
	collateraltypes.RegisterInterfaces(client.Context().InterfaceRegistry)
	restrictionstypes.RegisterInterfaces(client.Context().InterfaceRegistry)
	blstypes.RegisterInterfaces(client.Context().InterfaceRegistry)

	tq := NewTransactionQueue(
		maxAttempts,
		natsConnection,
		client.Context().Codec,
	)
	m := &manager{
		ctx:              ctx,
		client:           client,
		apiAccount:       account,
		chainTracker:     chainTracker,
		broadcaster:      broadcaster,
		transactionQueue: tq,
	}

	err := tq.RegisterSendHandler(m.handleSendStream)
	if err != nil {
		return nil, err
	}
	err = tq.RegisterObserveHandler(m.handleObserveStream)
	if err != nil {
		return nil, err
	}

	return m, nil
}

const maxAttempts = 100

type txToSend struct {
	TxInfo   txInfo
	Sent     bool
	Attempts int
}

type txInfo struct {
	Id       string
	RawTx    []byte
	TxHash   string
	Timeout  time.Time
	Attempts int
}

func (m *manager) SendTransactionAsyncWithRetry(rawTx sdk.Msg) (*sdk.TxResponse, error) {
	id := uuid.New().String()
	logging.Debug("SendTransactionAsyncWithRetry: sending tx", types.Messages, "tx_id", id)

	if halt, err := m.chainTracker.UpdateFromClient(m.ctx); err != nil || halt {
		if err := m.transactionQueue.AddToRetryQueue(id, "", time.Time{}, rawTx, 0, false); err != nil {
			logging.Error("failed to put in queue", types.Messages, "tx_id", id, "resend_err", err)
			return nil, ErrTxFailedToBroadcastAndPutOnRetry
		}
		return &sdk.TxResponse{}, nil
	}

	resp, timeoutTimestamp, broadcastErr := m.broadcaster.Broadcast(m.ctx, id, rawTx)
	if broadcastErr != nil {
		if isTxErrorCritical(broadcastErr) {
			logging.Error("SendTransactionAsyncWithRetry: critical error sending tx", types.Messages, "tx_id", id, "err", broadcastErr)
			return nil, broadcastErr
		}

		err := m.transactionQueue.AddToRetryQueue(id, "", timeoutTimestamp, rawTx, 1, false)
		if err != nil {
			logging.Error("tx failed to broadcast, failed to put in queue", types.Messages, "tx_id", id, "broadcast_err", broadcastErr, "resend_err", err)
			return nil, ErrTxFailedToBroadcastAndRetry
		}
		return nil, ErrTxFailedToBroadcastAndPutOnRetry
	}
	if err := m.transactionQueue.AddToRetryQueue(id, resp.TxHash, timeoutTimestamp, rawTx, 1, true); err != nil {
		logging.Error("tx broadcast, but failed to put in queue", types.Messages, "tx_id", id, "err", err)
	}
	return resp, nil
}

func (m *manager) SendTransactionAsyncNoRetry(rawTx sdk.Msg) (*sdk.TxResponse, error) {
	id := uuid.New().String()
	logging.Debug("SendTransactionAsyncNoRetry: sending tx", types.Messages, "tx_id", id, "originalMsgType", sdk.MsgTypeURL(rawTx))
	_, err := m.chainTracker.UpdateFromClient(m.ctx)
	if err != nil {
		return nil, err
	}
	resp, _, broadcastErr := m.broadcaster.Broadcast(m.ctx, id, rawTx)
	return resp, broadcastErr
}

func (m *manager) SendTransactionSyncNoRetry(msg proto.Message) (*ctypes.ResultTx, error) {
	id := uuid.New().String()
	logging.Debug("SendTransactionSyncNoRetry: sending tx", types.Messages, "tx_id", id)
	_, err := m.chainTracker.UpdateFromClient(m.ctx)
	if err != nil {
		return nil, err
	}
	resp, _, err := m.broadcaster.Broadcast(m.ctx, id, msg)
	if err != nil {
		return nil, err
	}

	logging.Debug("Transaction broadcast successful", types.Messages, "tx_id", id, "tx_hash", resp.TxHash)
	result, err := m.broadcaster.WaitForResponse(m.ctx, resp.TxHash)
	if err != nil {
		logging.Error("Failed to wait for transaction", types.Messages, "tx_id", id, "tx_hash", resp.TxHash, "error", err)
		return nil, err
	}
	return result, nil
}

func (m *manager) handleSendStream(tx txToSend, msg sdk.Msg) (QueueAction, error) {
	if halt, err := m.chainTracker.UpdateFromClient(m.ctx); err != nil || halt {
		// Slow everything down, the chain is halted or slow!
		time.Sleep(3 * time.Second)
		return Redeliver, nil
	}

	if !tx.Sent {
		logging.Debug("start broadcast tx async", types.Messages, "id", tx.TxInfo.Id)
		resp, timeout, err := m.broadcaster.Broadcast(m.ctx, tx.TxInfo.Id, msg)
		if err != nil {
			if isTxErrorCritical(err) {
				logging.Error("got critical error sending tx", types.Messages, "id", tx.TxInfo.Id)
				return Terminate, nil
			}
			return Redeliver, nil
		}
		tx.TxInfo.Timeout = timeout
		tx.TxInfo.TxHash = resp.TxHash
		tx.Sent = true
	}

	logging.Debug("tx broadcast, moving to observe", types.Messages, "id", tx.TxInfo.Id, "tx_hash", tx.TxInfo.TxHash, "timeout", tx.TxInfo.Timeout.String())

	if err := m.transactionQueue.AddToObserveQueue(tx.TxInfo.Id, msg, tx.TxInfo.TxHash, tx.TxInfo.Timeout, tx.Attempts); err != nil {
		logging.Error("error pushing to observe queue", types.Messages, "id", tx.TxInfo.Id, "err", err)
		return Redeliver, nil
	}
	return Acknowledge, nil
}

func (m *manager) handleObserveStream(tx txInfo, msg sdk.Msg) (QueueAction, error) {
	if tx.TxHash == "" {
		logging.Warn("tx hash is empty", types.Messages, "tx_id", tx.Id)
		tx.Attempts++
		if err := m.transactionQueue.AddToRetryQueue(tx.Id, "", time.Time{}, msg, tx.Attempts, false); err != nil {
			return Redeliver, err
		}
		return Acknowledge, nil
	}

	found, err := m.broadcaster.FindTxStatus(m.ctx, tx.TxHash)
	if found {
		logging.Debug("tx found, remove tx from observer queue", types.Messages, "tx_id", tx.Id, "txHash", tx.TxHash)
		return Acknowledge, nil
	}

	if errors.Is(err, ErrDecodingTxHash) {
		return Terminate, nil
	}

	if errors.Is(err, ErrTxNotFound) {
		latestBlockTime := m.chainTracker.GetLatestBlockTime(m.ctx)
		if latestBlockTime.After(tx.Timeout) {
			logging.Debug("tx expired", types.Messages, "tx_id", tx.Id, "tx_hash", tx.TxHash, "tx_timestamp", tx.Timeout, "latest_block_timestamp", latestBlockTime)
			tx.Attempts++
			if err := m.transactionQueue.AddToRetryQueue(tx.Id, "", time.Time{}, msg, tx.Attempts, false); err != nil {
				return Redeliver, err
			}
			return Acknowledge, nil
		}
	}

	return Redeliver, nil
}
