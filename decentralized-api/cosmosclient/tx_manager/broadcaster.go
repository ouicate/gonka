package tx_manager

import (
	"context"
	"decentralized-api/apiconfig"
	"decentralized-api/logging"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
	"github.com/productscience/inference/x/inference/types"
)

//type Broadcaster interface {
//	BroadcastTx(is string, rawTx sdk.Msg) (*sdk.TxResponse, time.Time, error)
//}

type TimestampCreator interface {
	GetLatestBlockTime(ctx context.Context) time.Time
}

type Broadcaster struct {
	txFactory        *tx.Factory
	apiAccount       *apiconfig.ApiAccount
	accountRetriever client.AccountRetriever
	client           *cosmosclient.Client
	timestampCreator TimestampCreator
}

func NewBroadcaster(account *apiconfig.ApiAccount, client *cosmosclient.Client, timestampCreator TimestampCreator) *Broadcaster {
	return &Broadcaster{
		apiAccount:       account,
		accountRetriever: authtypes.AccountRetriever{},
		client:           client,
		timestampCreator: timestampCreator,
	}
}

func (b *Broadcaster) getOrCreateFactory() (*tx.Factory, error) {
	// Only need to create the factory once
	if b.txFactory != nil {
		return b.txFactory, nil
	}
	address, err := b.apiAccount.SignerAddress()
	if err != nil {
		logging.Error("Failed to get account address", types.Messages, "error", err)
		return nil, err
	}
	accountNumber, _, err := b.accountRetriever.GetAccountNumberSequence(b.client.Context(), address)
	if err != nil {
		logging.Error("Failed to get account number and sequence", types.Messages, "error", err)
		return nil, err
	}
	factory := b.client.TxFactory.
		WithAccountNumber(accountNumber).
		WithGasAdjustment(10).
		WithFees("").
		WithGasPrices("").
		WithGas(0).
		WithUnordered(true).
		WithKeybase(b.client.AccountRegistry.Keyring)
	b.txFactory = &factory
	return &factory, nil
}

func (b *Broadcaster) Broadcast(ctx context.Context, id string, rawTx sdk.Msg) (*sdk.TxResponse, time.Time, error) {
	factory, err := b.getOrCreateFactory()
	if err != nil {
		return nil, time.Time{}, err
	}

	var finalMsg sdk.Msg = rawTx
	originalMsgType := sdk.MsgTypeURL(rawTx)
	if !b.apiAccount.IsSignerTheMainAccount() {
		granteeAddress, err := b.apiAccount.SignerAddress()
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("failed to get signer address: %w", err)
		}

		execMsg := authztypes.NewMsgExec(granteeAddress, []sdk.Msg{rawTx})
		finalMsg = &execMsg
		logging.Debug("Using authz MsgExec", types.Messages, "grantee", granteeAddress.String(), "originalMsgType", originalMsgType)
	}

	unsignedTx, err := factory.BuildUnsignedTx(finalMsg)
	if err != nil {
		return nil, time.Time{}, err
	}
	txBytes, timeoutTimestamp, err := b.getSignedBytes(ctx, id, unsignedTx)
	if err != nil {
		return nil, time.Time{}, err
	}

	resp, err := b.client.Context().BroadcastTxSync(txBytes)
	if err != nil {
		return nil, time.Time{}, err
	}
	if resp.Code != 0 {
		logging.Error("Broadcast failed immediately", types.Messages, "code", resp.Code, "rawLog", resp.RawLog, "tx_id", id, "originalMsgType", originalMsgType)
	} else {
		logging.Debug("Broadcast successful", types.Messages, "tx_id", id, "originalMsgType", originalMsgType, "resp", resp)
	}
	return resp, timeoutTimestamp, nil
}

func (b *Broadcaster) getSignedBytes(ctx context.Context, id string, unsignedTx client.TxBuilder) ([]byte, time.Time, error) {
	factory, err := b.getOrCreateFactory()
	if err != nil {
		return nil, time.Time{}, err
	}
	timestamp := b.timestampCreator.GetLatestBlockTime(ctx)

	// Gas is not charged, but without a high gas limit the transactions fail
	unsignedTx.SetGasLimit(1000000000)
	unsignedTx.SetFeeAmount(sdk.Coins{})
	unsignedTx.SetUnordered(true)
	unsignedTx.SetTimeoutTimestamp(timestamp)
	name := b.apiAccount.SignerAccount.Name
	logging.Debug("Signing transaction", types.Messages, "tx_id", id, "timeout", timestamp.String(), "name", name)

	err = tx.Sign(ctx, *factory, name, unsignedTx, false)
	if err != nil {
		logging.Error("Failed to sign transaction", types.Messages, "tx_id", id, "error", err)
		return nil, time.Time{}, err
	}
	txBytes, err := b.client.Context().TxConfig.TxEncoder()(unsignedTx.GetTx())
	if err != nil {
		logging.Error("Failed to encode transaction", types.Messages, "tx_id", id, "error", err)
		return nil, time.Time{}, err
	}
	return txBytes, timestamp, nil
}

func (b *Broadcaster) WaitForResponse(ctx context.Context, hash string) (*ctypes.ResultTx, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	transactionAppliedResult, err := b.client.WaitForTx(ctx, hash)
	if err != nil {
		logging.Error("Failed to wait for transaction", types.Messages, "error", err, "result", transactionAppliedResult)
		return nil, err
	}

	txResult := transactionAppliedResult.TxResult
	if txResult.Code != 0 {
		logging.Error("Transaction failed on-chain", types.Messages, "txHash", hash, "code", txResult.Code, "codespace", txResult.Codespace, "rawLog", txResult.Log)
		return nil, NewTransactionErrorFromResult(transactionAppliedResult)
	}
	return transactionAppliedResult, nil

}

func (b *Broadcaster) FindTxStatus(ctx context.Context, hash string) (bool, error) {
	bz, err := hex.DecodeString(hash)
	if err != nil {
		logging.Error("findTxStatus: error decoding tx hash", types.Messages, "err", err)
		return false, ErrDecodingTxHash
	}

	resp, err := b.client.Context().Client.Tx(ctx, bz, false)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, ErrTxNotFound
		}
		return false, err
	}

	if resp.TxResult.Code != 0 {
		logging.Error("findTxStatus: tx failed on-chain", types.Messages, "txHash", hash, "code", resp.TxResult.Code, "codespace", resp.TxResult.Codespace, "rawLog", resp.TxResult.Log)
	}
	logging.Debug("findTxStatus: found tx result", types.Messages, "txHash", hash, "resp", resp)
	return true, nil

}
