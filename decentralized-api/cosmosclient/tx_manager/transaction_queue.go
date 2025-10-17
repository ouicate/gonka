package tx_manager

import (
	"decentralized-api/internal/nats/server"
	"decentralized-api/logging"
	"encoding/json"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/productscience/inference/x/inference/types"
)

const (
	txSenderConsumer   = "tx-sender"
	txObserverConsumer = "tx-observer"

	defaultSenderNackDelay   = time.Second * 7
	defaultObserverNackDelay = time.Second * 5
)

type QueueAction int

const (
	Terminate QueueAction = iota
	Redeliver
	Acknowledge
)

type TransactionQueue struct {
	MaxAttempts       int
	Codec             codec.Codec
	natsJetStream     nats.JetStreamContext
	senderNackDelay   time.Duration
	observerNackDelay time.Duration
}

func NewTransactionQueue(
	maxAttempts int,
	natsConnection *nats.Conn,
	codec codec.Codec,
) *TransactionQueue {
	js, err := natsConnection.JetStream()
	if err != nil {
		panic(err)
	}
	queue := TransactionQueue{
		MaxAttempts:       maxAttempts,
		Codec:             codec,
		natsJetStream:     js,
		senderNackDelay:   defaultSenderNackDelay,
		observerNackDelay: defaultObserverNackDelay,
	}
	return &queue
}

func terminate(msg *nats.Msg) {
	err := msg.Term()
	if err != nil {
		logging.Error("error terminating message", types.Messages, "err", err)
	}
}

func acknowledge(msg *nats.Msg) {
	err := msg.Ack()
	if err != nil {
		logging.Error("error acknowledging message", types.Messages, "err", err)
	}
}

func redeliver(msg *nats.Msg) {
	err := msg.NakWithDelay(defaultSenderNackDelay)
	if err != nil {
		logging.Error("error redelivering message", types.Messages, "err", err)
	}
}

func (q *TransactionQueue) RegisterSendHandler(handler func(txToSend, sdk.Msg) (QueueAction, error)) error {
	_, err := q.natsJetStream.Subscribe(server.TxsToSendStream, func(msg *nats.Msg) {
		var tx txToSend
		if err := json.Unmarshal(msg.Data, &tx); err != nil {
			logging.Error("error unmarshaling tx_to_send", types.Messages, "err", err)
			terminate(msg)
			return
		}
		logging.Debug("got tx to send", types.Messages, "id", tx.TxInfo.Id)
		rawTx, err := q.unpackTx(tx.TxInfo.RawTx)
		if err != nil {
			logging.Error("error unpacking raw tx", types.Messages, "id", tx.TxInfo.Id, "err", err)
			terminate(msg)
			return
		}

		action, err := handler(tx, rawTx)
		if err != nil {
			logging.Error("error processing tx", types.Messages, "id", tx.TxInfo.Id, "err", err)
			terminate(msg)
			return
		}
		switch action {
		case Acknowledge:
			acknowledge(msg)
			break
		case Redeliver:
			redeliver(msg)
			break
		case Terminate:
			terminate(msg)
		}
	}, nats.Durable(txSenderConsumer), nats.ManualAck())
	return err
}

func (q *TransactionQueue) RegisterObserveHandler(handler func(txInfo, sdk.Msg) (QueueAction, error)) error {
	_, err := q.natsJetStream.Subscribe(server.TxsToObserveStream, func(msg *nats.Msg) {
		var tx txInfo
		if err := json.Unmarshal(msg.Data, &tx); err != nil {
			logging.Error("error unmarshaling tx_to_observe", types.Messages, "err", err)
			terminate(msg)
			return
		}
		logging.Debug("got tx to observe", types.Messages, "id", tx.Id)
		rawTx, err := q.unpackTx(tx.RawTx)
		if err != nil {
			logging.Error("error unpacking raw tx", types.Messages, "id", tx.Id, "err", err)
			terminate(msg)
			return
		}
		action, err := handler(tx, rawTx)
		if err != nil {
			logging.Error("error processing tx", types.Messages, "id", tx.Id, "err", err)
			terminate(msg)
			return
		}
		switch action {
		case Acknowledge:
			acknowledge(msg)
			break
		case Redeliver:
			redeliver(msg)
			break
		case Terminate:
			terminate(msg)
		}
	}, nats.Durable(txObserverConsumer), nats.ManualAck())
	return err
}

func (q *TransactionQueue) AddToRetryQueue(
	id,
	txHash string,
	timeout time.Time,
	rawTx sdk.Msg,
	attempts int,
	sent bool) error {
	logging.Debug("putOnRetry: tx with params", types.Messages,
		"tx_id", id,
		"tx_hash", txHash,
		"timeout", timeout.String(),
		"sent", sent,
	)

	if attempts >= maxAttempts {
		logging.Warn("tx reached max attempts", types.Messages, "tx_id", id)
		return nil
	}

	bz, err := q.Codec.MarshalInterfaceJSON(rawTx)
	if err != nil {
		return err
	}

	if id == "" {
		id = uuid.New().String()
	}

	b, err := json.Marshal(&txToSend{
		TxInfo: txInfo{
			Id:      id,
			RawTx:   bz,
			TxHash:  txHash,
			Timeout: timeout,
		},
		Sent:     sent,
		Attempts: attempts,
	})
	if err != nil {
		return err
	}
	_, err = q.natsJetStream.Publish(server.TxsToSendStream, b)
	return err
}

func (q *TransactionQueue) AddToObserveQueue(id string, rawTx sdk.Msg, txHash string, timeout time.Time, attempts int) error {
	logging.Debug(" putTxToObserve: tx with params", types.Messages,
		"tx_id", id,
		"tx_hash", txHash,
		"timeout", timeout.String(),
	)

	bz, err := q.Codec.MarshalInterfaceJSON(rawTx)
	if err != nil {
		return err
	}

	b, err := json.Marshal(&txInfo{
		Id:       id,
		RawTx:    bz,
		TxHash:   txHash,
		Timeout:  timeout,
		Attempts: attempts,
	})
	if err != nil {
		return err
	}
	_, err = q.natsJetStream.Publish(server.TxsToObserveStream, b)
	return err
}

func (q *TransactionQueue) unpackTx(bz []byte) (sdk.Msg, error) {
	var unpackedAny codectypes.Any
	if err := q.Codec.UnmarshalJSON(bz, &unpackedAny); err != nil {
		return nil, err
	}

	var rawTx sdk.Msg
	if err := q.Codec.UnpackAny(&unpackedAny, &rawTx); err != nil {
		return nil, err
	}
	return rawTx, nil
}
