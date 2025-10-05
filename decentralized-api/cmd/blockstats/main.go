package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"decentralized-api/cosmosclient"
	"decentralized-api/logging"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	inferencetypes "github.com/productscience/inference/x/inference/types"
)

type tmHTTPClient interface {
	BlockResults(ctx context.Context, height *int64) (*coretypes.ResultBlockResults, error)
}

// Example command:
// ./blockstats --start 658088 --end 658188 --rpc-url http://node1.gonka.ai:26657 --out blockstats.csv
func main() {
	var startHeightFlag int64
	var endHeightFlag int64
	var outPath string
	var rpcURL string

	flag.Int64Var(&startHeightFlag, "start", 0, "start block height (inclusive)")
	flag.Int64Var(&endHeightFlag, "end", 0, "end block height (inclusive)")
	flag.StringVar(&outPath, "out", "blockstats.csv", "output CSV file path")
	flag.StringVar(&rpcURL, "rpc-url", "", "override Tendermint RPC URL (optional)")
	flag.Parse()

	if startHeightFlag <= 0 || endHeightFlag <= 0 || endHeightFlag < startHeightFlag {
		fmt.Fprintf(os.Stderr, "invalid range: start=%d end=%d\n", startHeightFlag, endHeightFlag)
		os.Exit(2)
	}

	if rpcURL == "" {
		fmt.Fprintln(os.Stderr, "--rpc-url is required")
		os.Exit(2)
	}

	httpClient, err := cosmosclient.NewRpcClient(rpcURL)
	if err != nil {
		logging.Error("Failed to create Tendermint RPC client", inferencetypes.EventProcessing, "error", err)
		os.Exit(1)
	}

	file, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logging.Error("Failed to open output file", inferencetypes.EventProcessing, "path", outPath, "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = file.Sync()
		_ = file.Close()
	}()

	writer := bufio.NewWriterSize(file, 64*1024)

	// Process each block in range
	for height := startHeightFlag; height <= endHeightFlag; height++ {
		startTime := time.Now()
		linesWritten, err := processSingleBlock(context.Background(), httpClient, writer, file, height)
		duration := time.Since(startTime)
		if err != nil {
			logging.Warn("Block processing failed", inferencetypes.EventProcessing,
				"height", height, "duration_ms", duration.Milliseconds(), "error", err)
			continue
		}
		logging.Info("Processed block", inferencetypes.EventProcessing,
			"height", height, "tx_or_msgs", linesWritten, "duration_ms", duration.Milliseconds())
	}
}

// processSingleBlock queries block results and writes one CSV line per message action found.
// CSV line format: <block_height>,<msg_type> (no header). Flush+fsync at the end of a block.
func processSingleBlock(ctx context.Context, client tmHTTPClient, writer *bufio.Writer, file *os.File, height int64) (int, error) {
	res, err := client.BlockResults(ctx, &height)
	if err != nil || res == nil {
		if err == nil {
			err = errors.New("nil BlockResults")
		}
		return 0, err
	}
	return writeEventsForBlock(writer, file, height, res.TxsResults)
}

// writeEventsForBlock extracts message.action values and writes CSV lines.
func writeEventsForBlock(writer *bufio.Writer, file *os.File, height int64, txs []*abcitypes.ExecTxResult) (int, error) {
	lines := 0
	for _, tx := range txs {
		for _, ev := range tx.Events {
			for _, attr := range ev.Attributes {
				if _, err := writer.WriteString(strconv.FormatInt(height, 10) + "," + ev.Type + "," + attr.Key + "," + attr.Value + "\n"); err != nil {
					return lines, err
				}
				lines++
				/*				if attr.Key == "action" && attr.Value != "" {
								// One line per message action
								if _, err := writer.WriteString(strconv.FormatInt(height, 10) + "," + ev.Type + "," + attr.Key + "," + attr.Value + "\n"); err != nil {
									return lines, err
								}
								lines++
							}*/
			}
		}
	}
	if err := writer.Flush(); err != nil {
		return lines, err
	}
	if err := file.Sync(); err != nil {
		return lines, err
	}
	return lines, nil
}
