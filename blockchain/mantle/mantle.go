package mantle

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"google.golang.org/protobuf/proto"

	seer_common "github.com/G7DAO/seer/blockchain/common"
	"github.com/G7DAO/seer/indexer"
	"github.com/G7DAO/seer/version"
)

func NewClient(url string, timeout int) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	rpcClient, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Client{rpcClient: rpcClient, timeout: time.Duration(timeout) * time.Second}, nil
}

// Client is a wrapper around the Ethereum JSON-RPC client.

type Client struct {
	rpcClient *rpc.Client
	timeout   time.Duration
}

// Client common

// ChainType returns the chain type.
func (c *Client) ChainType() string {
	return "mantle"
}

// Close closes the underlying RPC client.
func (c *Client) Close() {
	c.rpcClient.Close()
}

// GetLatestBlockNumber returns the latest block number.
func (c *Client) GetLatestBlockNumber() (*big.Int, error) {
	var result string

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), c.timeout)

	defer cancel()

	if err := c.rpcClient.CallContext(ctxWithTimeout, &result, "eth_blockNumber"); err != nil {
		return nil, err
	}

	// Convert the hex string to a big.Int
	blockNumber, ok := new(big.Int).SetString(result, 0) // The 0 base lets the function infer the base from the string prefix.
	if !ok {
		return nil, fmt.Errorf("invalid block number format: %s", result)
	}

	return blockNumber, nil
}

// GetBlockByNumber returns the block with the given number.
func (c *Client) GetBlockByNumber(ctx context.Context, number *big.Int, withTransactions bool) (*seer_common.BlockJson, error) {
	var block *seer_common.BlockJson
	err := c.rpcClient.CallContext(ctx, &block, "eth_getBlockByNumber", fmt.Sprintf("0x%x", number), withTransactions)
	if err != nil {
		fmt.Println("Error calling eth_getBlockByNumber:", err)
		return nil, err
	}

	return block, nil
}

// BlockByHash returns the block with the given hash.
func (c *Client) BlockByHash(ctx context.Context, hash common.Hash) (*seer_common.BlockJson, error) {
	var block *seer_common.BlockJson
	err := c.rpcClient.CallContext(ctx, &block, "eth_getBlockByHash", hash, true) // true to include transactions
	return block, err
}

// TransactionReceipt returns the receipt of a transaction by transaction hash.
func (c *Client) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := c.rpcClient.CallContext(ctx, &receipt, "eth_getTransactionReceipt", hash)
	return receipt, err
}

// Get bytecode of a contract by address.
func (c *Client) GetCode(ctx context.Context, address common.Address, blockNumber uint64) ([]byte, error) {
	var code hexutil.Bytes
	if blockNumber == 0 {
		latestBlockNumber, err := c.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}
		blockNumber = latestBlockNumber.Uint64()
	}
	err := c.rpcClient.CallContext(ctx, &code, "eth_getCode", address, "0x"+fmt.Sprintf("%x", blockNumber))
	if err != nil {
		log.Printf("Failed to get code for address %s at block %d: %v", address.Hex(), blockNumber, err)
		return nil, err
	}

	if len(code) == 0 {
		return nil, nil
	}
	return code, nil
}
func (c *Client) ClientFilterLogs(ctx context.Context, q ethereum.FilterQuery, debug bool) ([]*seer_common.EventJson, error) {
	var logs []*seer_common.EventJson
	fromBlock := q.FromBlock
	toBlock := q.ToBlock
	batchStep := new(big.Int).Sub(toBlock, fromBlock) // Calculate initial batch step

	for {
		// Calculate the next "lastBlock" within the batch step or adjust to "toBlock" if exceeding
		nextBlock := new(big.Int).Add(fromBlock, batchStep)
		if nextBlock.Cmp(toBlock) > 0 {
			nextBlock = new(big.Int).Set(toBlock)
		}

		var result []*seer_common.EventJson
		err := c.rpcClient.CallContext(ctx, &result, "eth_getLogs", struct {
			FromBlock string           `json:"fromBlock"`
			ToBlock   string           `json:"toBlock"`
			Addresses []common.Address `json:"addresses"`
			Topics    [][]common.Hash  `json:"topics"`
		}{
			FromBlock: toHex(fromBlock),
			ToBlock:   toHex(nextBlock),
			Addresses: q.Addresses,
			Topics:    q.Topics,
		})

		if err != nil {
			if strings.Contains(err.Error(), "query returned more than 10000 results") {
				// Halve the batch step if too many results and retry
				batchStep.Div(batchStep, big.NewInt(2))
				if batchStep.Cmp(big.NewInt(1)) < 0 {
					// If the batch step is too small we will skip that block
					fromBlock = new(big.Int).Add(nextBlock, big.NewInt(1))
					if fromBlock.Cmp(toBlock) > 0 {
						break
					}
					continue
				}
				continue
			} else {
				// For any other error, return immediately
				return nil, err
			}
		}

		// Append the results and adjust "fromBlock" for the next batch
		logs = append(logs, result...)
		fromBlock = new(big.Int).Add(nextBlock, big.NewInt(1))

		if debug {
			log.Printf("Fetched logs: %d", len(result))
		}

		// Break the loop if we've reached or exceeded "toBlock"
		if fromBlock.Cmp(toBlock) > 0 {
			break
		}
	}

	return logs, nil
}

// Utility function to convert big.Int to its hexadecimal representation.
func toHex(number *big.Int) string {
	return fmt.Sprintf("0x%x", number)
}

func fromHex(hex string) *big.Int {
	number := new(big.Int)
	number.SetString(hex, 0)
	return number
}

// FetchBlocksInRange fetches blocks within a specified range.
// This could be useful for batch processing or analysis.
func (c *Client) FetchBlocksInRange(from, to *big.Int, debug bool) ([]*seer_common.BlockJson, error) {
	var blocks []*seer_common.BlockJson
	ctx := context.Background() // For simplicity, using a background context; consider timeouts for production.

	for i := new(big.Int).Set(from); i.Cmp(to) <= 0; i.Add(i, big.NewInt(1)) {

		ctxWithTimeout, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()

		block, err := c.GetBlockByNumber(ctxWithTimeout, i, true)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
		if debug {
			log.Printf("Fetched block number: %d", i)
		}
	}

	return blocks, nil
}

// FetchBlocksInRangeAsync fetches blocks within a specified range concurrently.
func (c *Client) FetchBlocksInRangeAsync(from, to *big.Int, debug bool, maxRequests int) ([]*seer_common.BlockJson, error) {
	var (
		blocks          []*seer_common.BlockJson
		collectedErrors []error
		mu              sync.Mutex
		wg              sync.WaitGroup
		ctx             = context.Background()
	)

	var blockNumbersRange []*big.Int
	for i := new(big.Int).Set(from); i.Cmp(to) <= 0; i.Add(i, big.NewInt(1)) {
		blockNumbersRange = append(blockNumbersRange, new(big.Int).Set(i))
	}

	sem := make(chan struct{}, maxRequests)             // Semaphore to control concurrency
	errChan := make(chan error, len(blockNumbersRange)) // Channel to collect errors from goroutines

	for _, b := range blockNumbersRange {
		wg.Add(1)
		go func(b *big.Int) {
			defer wg.Done()

			sem <- struct{}{} // Acquire semaphore
			defer func() { <-sem }()

			defer func() {
				if r := recover(); r != nil {
					errChan <- fmt.Errorf("panic in goroutine for block %s: %v", b.String(), r)
				}
			}()

			ctxWithTimeout, cancel := context.WithTimeout(ctx, c.timeout)

			defer cancel()

			block, getErr := c.GetBlockByNumber(ctxWithTimeout, b, true)
			if getErr != nil {
				log.Printf("Failed to fetch block number: %d, error: %v", b, getErr)
				errChan <- getErr
				return
			}

			mu.Lock()
			blocks = append(blocks, block)
			mu.Unlock()

			if debug {
				log.Printf("Fetched block number: %d", b)
			}

		}(b)
	}

	wg.Wait()
	close(sem)
	close(errChan)

	for err := range errChan {
		collectedErrors = append(collectedErrors, err)
	}

	if len(collectedErrors) > 0 {
		var errStrings []string
		for _, err := range collectedErrors {
			errStrings = append(errStrings, err.Error())
		}
		return nil, fmt.Errorf("errors occurred during crawling: %s", strings.Join(errStrings, "; "))
	}
	return blocks, nil
}

// ParseBlocksWithTransactions parses blocks and their transactions into custom data structure.
// This method showcases how to handle and transform detailed block and transaction data.
func (c *Client) ParseBlocksWithTransactions(from, to *big.Int, debug bool, maxRequests int) ([]*MantleBlock, error) {
	var blocksWithTxsJson []*seer_common.BlockJson
	var fetchErr error
	if maxRequests > 1 {
		blocksWithTxsJson, fetchErr = c.FetchBlocksInRangeAsync(from, to, debug, maxRequests)
	} else {
		blocksWithTxsJson, fetchErr = c.FetchBlocksInRange(from, to, debug)
	}
	if fetchErr != nil {
		return nil, fetchErr
	}

	var parsedBlocks []*MantleBlock
	for _, blockAndTxsJson := range blocksWithTxsJson {
		// Convert BlockJson to Block and Transactions as required.
		parsedBlock := ToProtoSingleBlock(blockAndTxsJson)

		for _, txJson := range blockAndTxsJson.Transactions {
			txJson.BlockTimestamp = blockAndTxsJson.Timestamp

			parsedTransaction := ToProtoSingleTransaction(&txJson)
			parsedBlock.Transactions = append(parsedBlock.Transactions, parsedTransaction)
		}

		parsedBlocks = append(parsedBlocks, parsedBlock)
	}

	return parsedBlocks, nil
}

func (c *Client) ParseEvents(from, to *big.Int, blocksCache map[uint64]indexer.BlockCache, debug bool) ([]*MantleEventLog, error) {

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), c.timeout)

	defer cancel()

	logs, err := c.ClientFilterLogs(ctxWithTimeout, ethereum.FilterQuery{
		FromBlock: from,
		ToBlock:   to,
	}, debug)

	if err != nil {
		fmt.Println("Error fetching logs: ", err)
		return nil, err
	}

	var parsedEvents []*MantleEventLog

	for _, log := range logs {
		parsedEvent := ToProtoSingleEventLog(log)
		parsedEvents = append(parsedEvents, parsedEvent)

	}

	return parsedEvents, nil
}

func (c *Client) FetchAsProtoBlocksWithEvents(from, to *big.Int, debug bool, maxRequests int) ([]proto.Message, []indexer.BlockIndex, uint64, error) {
	blocks, err := c.ParseBlocksWithTransactions(from, to, debug, maxRequests)
	if err != nil {
		return nil, nil, 0, err
	}

	var blocksSize uint64

	blocksCache := make(map[uint64]indexer.BlockCache)

	for _, block := range blocks {
		blocksCache[block.BlockNumber] = indexer.BlockCache{
			BlockNumber:    block.BlockNumber,
			BlockHash:      block.Hash,
			BlockTimestamp: block.Timestamp,
		} // Assuming block.BlockNumber is int64 and block.Hash is string
	}

	events, err := c.ParseEvents(from, to, blocksCache, debug)
	if err != nil {
		return nil, nil, 0, err
	}

	var blocksProto []proto.Message
	var blocksIndex []indexer.BlockIndex

	for bI, block := range blocks {
		for _, tx := range block.Transactions {
			for _, event := range events {
				if tx.Hash == event.TransactionHash {
					tx.Logs = append(tx.Logs, event)
				}
			}
		}

		// Prepare blocks to index
		blocksIndex = append(blocksIndex, indexer.NewBlockIndex("mantle",
			block.BlockNumber,
			block.Hash,
			block.Timestamp,
			block.ParentHash,
			uint64(bI),
			"",
			0,
		))

		blocksSize += uint64(proto.Size(block))
		blocksProto = append(blocksProto, block) // Assuming block is already a proto.Message
	}

	return blocksProto, blocksIndex, blocksSize, nil
}

func (c *Client) ProcessBlocksToBatch(msgs []proto.Message) (proto.Message, error) {
	var blocks []*MantleBlock
	for _, msg := range msgs {
		block, ok := msg.(*MantleBlock)
		if !ok {
			return nil, fmt.Errorf("failed to type assert proto.Message to *MantleBlock")
		}
		blocks = append(blocks, block)
	}

	return &MantleBlocksBatch{
		Blocks:      blocks,
		SeerVersion: version.SeerVersion,
	}, nil
}

func ToEntireBlocksBatchFromLogProto(obj *MantleBlocksBatch) *seer_common.BlocksBatchJson {
	blocksBatchJson := seer_common.BlocksBatchJson{
		Blocks:      []seer_common.BlockJson{},
		SeerVersion: obj.SeerVersion,
	}

	for _, b := range obj.Blocks {
		var txs []seer_common.TransactionJson
		for _, tx := range b.Transactions {
			var accessList []seer_common.AccessList
			for _, al := range tx.AccessList {
				accessList = append(accessList, seer_common.AccessList{
					Address:     al.Address,
					StorageKeys: al.StorageKeys,
				})
			}
			var events []seer_common.EventJson
			for _, e := range tx.Logs {
				events = append(events, seer_common.EventJson{
					Address:          e.Address,
					Topics:           e.Topics,
					Data:             e.Data,
					BlockNumber:      fmt.Sprintf("%d", e.BlockNumber),
					TransactionHash:  e.TransactionHash,
					BlockHash:        e.BlockHash,
					Removed:          e.Removed,
					LogIndex:         fmt.Sprintf("%d", e.LogIndex),
					TransactionIndex: fmt.Sprintf("%d", e.TransactionIndex),
				})
			}
			txs = append(txs, seer_common.TransactionJson{
				BlockHash:            tx.BlockHash,
				BlockNumber:          fmt.Sprintf("%d", tx.BlockNumber),
				ChainId:              tx.ChainId,
				FromAddress:          tx.FromAddress,
				Gas:                  tx.Gas,
				GasPrice:             tx.GasPrice,
				Hash:                 tx.Hash,
				Input:                tx.Input,
				MaxFeePerGas:         tx.MaxFeePerGas,
				MaxPriorityFeePerGas: tx.MaxPriorityFeePerGas,
				Nonce:                tx.Nonce,
				V:                    tx.V,
				R:                    tx.R,
				S:                    tx.S,
				ToAddress:            tx.ToAddress,
				TransactionIndex:     fmt.Sprintf("%d", tx.TransactionIndex),
				TransactionType:      fmt.Sprintf("%d", tx.TransactionType),
				Value:                tx.Value,
				IndexedAt:            fmt.Sprintf("%d", tx.IndexedAt),
				BlockTimestamp:       fmt.Sprintf("%d", tx.BlockTimestamp),
				AccessList:           accessList,
				YParity:              tx.YParity,

				Events: events,
			})
		}

		blocksBatchJson.Blocks = append(blocksBatchJson.Blocks, seer_common.BlockJson{
			Difficulty:       fmt.Sprintf("%d", b.Difficulty),
			ExtraData:        b.ExtraData,
			GasLimit:         fmt.Sprintf("%d", b.GasLimit),
			GasUsed:          fmt.Sprintf("%d", b.GasUsed),
			Hash:             b.Hash,
			LogsBloom:        b.LogsBloom,
			Miner:            b.Miner,
			Nonce:            b.Nonce,
			BlockNumber:      fmt.Sprintf("%d", b.BlockNumber),
			ParentHash:       b.ParentHash,
			ReceiptsRoot:     b.ReceiptsRoot,
			Sha3Uncles:       b.Sha3Uncles,
			StateRoot:        b.StateRoot,
			Timestamp:        fmt.Sprintf("%d", b.Timestamp),
			TotalDifficulty:  b.TotalDifficulty,
			TransactionsRoot: b.TransactionsRoot,
			Size:             fmt.Sprintf("%d", b.Size),
			BaseFeePerGas:    b.BaseFeePerGas,
			IndexedAt:        fmt.Sprintf("%d", b.IndexedAt),

			Transactions: txs,
		})
	}

	return &blocksBatchJson
}

func ToProtoSingleBlock(obj *seer_common.BlockJson) *MantleBlock {
	return &MantleBlock{
		BlockNumber:      fromHex(obj.BlockNumber).Uint64(),
		Difficulty:       fromHex(obj.Difficulty).Uint64(),
		ExtraData:        obj.ExtraData,
		GasLimit:         fromHex(obj.GasLimit).Uint64(),
		GasUsed:          fromHex(obj.GasUsed).Uint64(),
		BaseFeePerGas:    obj.BaseFeePerGas,
		Hash:             obj.Hash,
		LogsBloom:        obj.LogsBloom,
		Miner:            obj.Miner,
		Nonce:            obj.Nonce,
		ParentHash:       obj.ParentHash,
		ReceiptsRoot:     obj.ReceiptsRoot,
		Sha3Uncles:       obj.Sha3Uncles,
		Size:             fromHex(obj.Size).Uint64(),
		StateRoot:        obj.StateRoot,
		Timestamp:        fromHex(obj.Timestamp).Uint64(),
		TotalDifficulty:  obj.TotalDifficulty,
		TransactionsRoot: obj.TransactionsRoot,
		IndexedAt:        fromHex(obj.IndexedAt).Uint64(),
	}
}

func ToProtoSingleTransaction(obj *seer_common.TransactionJson) *MantleTransaction {
	var accessList []*MantleTransactionAccessList
	for _, al := range obj.AccessList {
		accessList = append(accessList, &MantleTransactionAccessList{
			Address:     al.Address,
			StorageKeys: al.StorageKeys,
		})
	}

	return &MantleTransaction{
		Hash:                 obj.Hash,
		BlockNumber:          fromHex(obj.BlockNumber).Uint64(),
		BlockHash:            obj.BlockHash,
		FromAddress:          obj.FromAddress,
		ToAddress:            obj.ToAddress,
		Gas:                  obj.Gas,
		GasPrice:             obj.GasPrice,
		MaxFeePerGas:         obj.MaxFeePerGas,
		MaxPriorityFeePerGas: obj.MaxPriorityFeePerGas,
		Input:                obj.Input,
		Nonce:                obj.Nonce,
		TransactionIndex:     fromHex(obj.TransactionIndex).Uint64(),
		TransactionType:      fromHex(obj.TransactionType).Uint64(),
		Value:                obj.Value,
		IndexedAt:            fromHex(obj.IndexedAt).Uint64(),
		BlockTimestamp:       fromHex(obj.BlockTimestamp).Uint64(),

		ChainId: obj.ChainId,
		V:       obj.V,
		R:       obj.R,
		S:       obj.S,

		AccessList: accessList,
		YParity:    obj.YParity,
	}
}

func ToEvenFromLogProto(obj *MantleEventLog) *seer_common.EventJson {
	return &seer_common.EventJson{
		Address:         obj.Address,
		Topics:          obj.Topics,
		Data:            obj.Data,
		BlockNumber:     fmt.Sprintf("%d", obj.BlockNumber),
		TransactionHash: obj.TransactionHash,
		LogIndex:        fmt.Sprintf("%d", obj.LogIndex),
		BlockHash:       obj.BlockHash,
		Removed:         obj.Removed,
	}
}

func ToProtoSingleEventLog(obj *seer_common.EventJson) *MantleEventLog {
	return &MantleEventLog{
		Address:         obj.Address,
		Topics:          obj.Topics,
		Data:            obj.Data,
		BlockNumber:     fromHex(obj.BlockNumber).Uint64(),
		TransactionHash: obj.TransactionHash,
		LogIndex:        fromHex(obj.LogIndex).Uint64(),
		BlockHash:       obj.BlockHash,
		Removed:         obj.Removed,
	}
}

func (c *Client) DecodeProtoEventLogs(data []string) ([]*MantleEventLog, error) {
	var events []*MantleEventLog
	for _, d := range data {
		var event MantleEventLog
		base64Decoded, err := base64.StdEncoding.DecodeString(d)
		if err != nil {
			return nil, err
		}
		if err := proto.Unmarshal(base64Decoded, &event); err != nil {
			return nil, err
		}
		events = append(events, &event)
	}
	return events, nil
}

func (c *Client) DecodeProtoTransactions(data []string) ([]*MantleTransaction, error) {
	var transactions []*MantleTransaction
	for _, d := range data {
		var transaction MantleTransaction
		base64Decoded, err := base64.StdEncoding.DecodeString(d)
		if err != nil {
			return nil, err
		}
		if err := proto.Unmarshal(base64Decoded, &transaction); err != nil {
			return nil, err
		}
		transactions = append(transactions, &transaction)
	}
	return transactions, nil
}

func (c *Client) DecodeProtoBlocks(data []string) ([]*MantleBlock, error) {
	var blocks []*MantleBlock
	for _, d := range data {
		var block MantleBlock
		base64Decoded, err := base64.StdEncoding.DecodeString(d)
		if err != nil {
			return nil, err
		}
		if err := proto.Unmarshal(base64Decoded, &block); err != nil {
			return nil, err
		}
		blocks = append(blocks, &block)
	}
	return blocks, nil
}

func (c *Client) DecodeProtoEntireBlockToJson(rawData *bytes.Buffer) (*seer_common.BlocksBatchJson, error) {
	var protoBlocksBatch MantleBlocksBatch

	dataBytes := rawData.Bytes()

	err := proto.Unmarshal(dataBytes, &protoBlocksBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %v", err)
	}

	blocksBatchJson := ToEntireBlocksBatchFromLogProto(&protoBlocksBatch)

	return blocksBatchJson, nil
}

func (c *Client) DecodeProtoEntireBlockToLabels(rawData *bytes.Buffer, abiMap map[string]map[string]*indexer.AbiEntry, addRawTransactions bool, threads int) ([]indexer.EventLabel, []indexer.TransactionLabel, []indexer.RawTransaction, error) {
	var protoBlocksBatch MantleBlocksBatch

	dataBytes := rawData.Bytes()

	err := proto.Unmarshal(dataBytes, &protoBlocksBatch)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal data: %v", err)
	}

	// Shared slices to collect labels
	var labels []indexer.EventLabel
	var txLabels []indexer.TransactionLabel
	var rawTransactions []indexer.RawTransaction
	var labelsMutex sync.Mutex

	var decodeErr error

	var wg sync.WaitGroup

	// Concurrency limit (e.g., 10 goroutines at a time)
	concurrencyLimit := threads
	semaphoreChan := make(chan struct{}, concurrencyLimit)

	// Channel to collect errors from goroutines
	errorChan := make(chan error, len(protoBlocksBatch.Blocks))

	// Iterate over blocks and launch goroutines
	for _, b := range protoBlocksBatch.Blocks {
		wg.Add(1)
		semaphoreChan <- struct{}{}
		go func(b *MantleBlock) {
			defer wg.Done()
			defer func() { <-semaphoreChan }()
			defer func() {
				if r := recover(); r != nil {
					errorChan <- fmt.Errorf("panic in goroutine for block %d: %v", b.BlockNumber, r)
				}
			}()

			// Local slices to collect labels for this block
			var localEventLabels []indexer.EventLabel
			var localTxLabels []indexer.TransactionLabel
			var localRawTransactions []indexer.RawTransaction
			for _, tx := range b.Transactions {
				var decodedArgsTx map[string]interface{}

				label := indexer.SeerCrawlerLabel

				if addRawTransactions {
					localRawTransactions = append(localRawTransactions, indexer.RawTransaction{
						Hash:                 tx.Hash,
						BlockHash:            tx.BlockHash,
						FromAddress:          tx.FromAddress,
						ToAddress:            tx.ToAddress,
						Input:                tx.Input,
						Gas:                  tx.Gas,
						GasPrice:             tx.GasPrice,
						Nonce:                tx.Nonce,
						Value:                tx.Value,
						MaxFeePerGas:         tx.MaxFeePerGas,
						MaxPriorityFeePerGas: tx.MaxPriorityFeePerGas,
						BlockTimestamp:       b.Timestamp,
						BlockNumber:          b.BlockNumber,
						TransactionIndex:     tx.TransactionIndex,
						TransactionType:      tx.TransactionType,
					})
				}

				if len(tx.Input) < 10 { // If input is less than 3 characters then it direct transfer
					continue
				}

				// Process transaction labels
				selector := tx.Input[:10]

				if abiMap[tx.ToAddress] != nil && abiMap[tx.ToAddress][selector] != nil {

					txAbiEntry := abiMap[tx.ToAddress][selector]

					var initErr error
					txAbiEntry.Once.Do(func() {
						txAbiEntry.Abi, initErr = seer_common.GetABI(txAbiEntry.AbiJSON)
					})

					// Check if an error occurred during ABI parsing
					if initErr != nil || txAbiEntry.Abi == nil {
						errorChan <- fmt.Errorf("error getting ABI for address %s: %v", tx.ToAddress, initErr)
						continue
					}

					inputData, err := hex.DecodeString(tx.Input[2:])
					if err != nil {
						errorChan <- fmt.Errorf("error decoding input data for tx %s: %v", tx.Hash, err)
						continue
					}
					decodedArgsTx, decodeErr = seer_common.DecodeTransactionInputDataToInterface(txAbiEntry.Abi, inputData)
					if decodeErr != nil {
						fmt.Println("Error decoding transaction not decoded data: ", tx.Hash, decodeErr)
						decodedArgsTx = map[string]interface{}{
							"input_raw": tx,
							"abi":       txAbiEntry.AbiJSON,
							"selector":  selector,
							"error":     decodeErr,
						}
						label = indexer.SeerCrawlerRawLabel
					}

					ctxWithTimeout, cancel := context.WithTimeout(context.Background(), c.timeout)

					defer cancel()

					receipt, err := c.TransactionReceipt(ctxWithTimeout, common.HexToHash(tx.Hash))
					if err != nil {
						errorChan <- fmt.Errorf("error getting transaction receipt for tx %s: %v", tx.Hash, err)
						continue
					}

					// check if the transaction was successful
					if receipt.Status == 1 {
						decodedArgsTx["status"] = 1
					} else {
						decodedArgsTx["status"] = 0
					}

					txLabelDataBytes, err := json.Marshal(decodedArgsTx)
					if err != nil {
						errorChan <- fmt.Errorf("error converting decodedArgsTx to JSON for tx %s: %v", tx.Hash, err)
						continue
					}

					// Convert transaction to label
					transactionLabel := indexer.TransactionLabel{
						Address:         tx.ToAddress,
						BlockNumber:     tx.BlockNumber,
						BlockHash:       tx.BlockHash,
						CallerAddress:   tx.FromAddress,
						LabelName:       txAbiEntry.AbiName,
						LabelType:       "tx_call",
						OriginAddress:   tx.FromAddress,
						Label:           label,
						TransactionHash: tx.Hash,
						LabelData:       string(txLabelDataBytes), // Convert JSON byte slice to string
						BlockTimestamp:  b.Timestamp,
					}

					localTxLabels = append(localTxLabels, transactionLabel)
				}

				// Process events
				for _, e := range tx.Logs {
					var decodedArgsLogs map[string]interface{}
					label = indexer.SeerCrawlerLabel

					var topicSelector string

					if len(e.Topics) > 0 {
						topicSelector = e.Topics[0]
					} else {
						// 0x0 is the default topic selector
						topicSelector = "0x0"
					}

					if abiMap[e.Address] == nil || abiMap[e.Address][topicSelector] == nil {
						continue
					}

					abiEntryLog := abiMap[e.Address][topicSelector]

					var initErr error
					abiEntryLog.Once.Do(func() {
						abiEntryLog.Abi, initErr = seer_common.GetABI(abiEntryLog.AbiJSON)
					})

					// Check if an error occurred during ABI parsing
					if initErr != nil || abiEntryLog.Abi == nil {
						errorChan <- fmt.Errorf("error getting ABI for log address %s: %v", e.Address, initErr)
						continue
					}

					// Decode the event data
					decodedArgsLogs, decodeErr = seer_common.DecodeLogArgsToLabelData(abiEntryLog.Abi, e.Topics, e.Data)
					if decodeErr != nil {
						fmt.Println("Error decoding event not decoded data: ", e.TransactionHash, decodeErr)
						decodedArgsLogs = map[string]interface{}{
							"input_raw": e,
							"abi":       abiEntryLog.AbiJSON,
							"selector":  topicSelector,
							"error":     decodeErr,
						}
						label = indexer.SeerCrawlerRawLabel
					}

					// Convert decodedArgsLogs map to JSON
					labelDataBytes, err := json.Marshal(decodedArgsLogs)
					if err != nil {
						errorChan <- fmt.Errorf("error converting decodedArgsLogs to JSON for tx %s: %v", e.TransactionHash, err)
						continue
					}
					// Convert event to label
					eventLabel := indexer.EventLabel{
						Label:           label,
						LabelName:       abiEntryLog.AbiName,
						LabelType:       "event",
						BlockNumber:     e.BlockNumber,
						BlockHash:       e.BlockHash,
						Address:         e.Address,
						OriginAddress:   tx.FromAddress,
						TransactionHash: e.TransactionHash,
						LabelData:       string(labelDataBytes), // Convert JSON byte slice to string
						BlockTimestamp:  b.Timestamp,
						LogIndex:        e.LogIndex,
					}

					localEventLabels = append(localEventLabels, eventLabel)
				}
			}

			// Append local labels to shared slices under mutex
			labelsMutex.Lock()
			labels = append(labels, localEventLabels...)
			txLabels = append(txLabels, localTxLabels...)
			rawTransactions = append(rawTransactions, localRawTransactions...)
			labelsMutex.Unlock()
		}(b)
	}
	// Wait for all block processing goroutines to finish
	wg.Wait()
	close(errorChan)

	// Collect all errors
	var errorMessages []string
	for err := range errorChan {
		errorMessages = append(errorMessages, err.Error())
	}

	// If any errors occurred, return them
	if len(errorMessages) > 0 {
		return nil, nil, nil, fmt.Errorf("errors occurred during processing:\n%s", strings.Join(errorMessages, "\n"))
	}

	return labels, txLabels, rawTransactions, nil
}

func (c *Client) DecodeProtoTransactionsToLabels(transactions []string, blocksCache map[uint64]uint64, abiMap map[string]map[string]*indexer.AbiEntry) ([]indexer.TransactionLabel, error) {

	decodedTransactions, err := c.DecodeProtoTransactions(transactions)

	if err != nil {
		return nil, err
	}

	var labels []indexer.TransactionLabel
	var decodedArgs map[string]interface{}
	var decodeErr error

	for _, transaction := range decodedTransactions {

		label := indexer.SeerCrawlerLabel

		selector := transaction.Input[:10]

		if abiMap[transaction.ToAddress][selector].Abi == nil {
			abiMap[transaction.ToAddress][selector].Abi, err = seer_common.GetABI(abiMap[transaction.ToAddress][selector].AbiJSON)
			if err != nil {
				fmt.Println("Error getting ABI: ", err)
				return nil, err
			}
		}

		inputData, err := hex.DecodeString(transaction.Input[2:])
		if err != nil {
			fmt.Println("Error decoding input data: ", err)
			return nil, err
		}

		decodedArgs, decodeErr = seer_common.DecodeTransactionInputDataToInterface(abiMap[transaction.ToAddress][selector].Abi, inputData)

		if decodeErr != nil {
			fmt.Println("Error decoding transaction not decoded data: ", transaction.Hash, decodeErr)
			decodedArgs = map[string]interface{}{
				"input_raw": transaction,
				"abi":       abiMap[transaction.ToAddress][selector].AbiJSON,
				"selector":  selector,
				"error":     decodeErr,
			}
			label = indexer.SeerCrawlerRawLabel
		}

		labelDataBytes, err := json.Marshal(decodedArgs)
		if err != nil {
			fmt.Println("Error converting decodedArgs to JSON: ", err)
			return nil, err
		}

		// Convert JSON byte slice to string
		labelDataString := string(labelDataBytes)

		// Convert transaction to label
		transactionLabel := indexer.TransactionLabel{
			Address:         transaction.ToAddress,
			BlockNumber:     transaction.BlockNumber,
			BlockHash:       transaction.BlockHash,
			CallerAddress:   transaction.FromAddress,
			LabelName:       abiMap[transaction.ToAddress][selector].AbiName,
			LabelType:       "tx_call",
			OriginAddress:   transaction.FromAddress,
			Label:           label,
			TransactionHash: transaction.Hash,
			LabelData:       labelDataString,
			BlockTimestamp:  blocksCache[transaction.BlockNumber],
		}

		labels = append(labels, transactionLabel)

	}

	return labels, nil
}

func (c *Client) GetTransactionByHash(ctx context.Context, hash string) (*seer_common.TransactionJson, error) {
	var tx *seer_common.TransactionJson
	err := c.rpcClient.CallContext(ctx, &tx, "eth_getTransactionByHash", hash)
	return tx, err
}

func (c *Client) GetTransactionsLabels(startBlock uint64, endBlock uint64, abiMap map[string]map[string]*indexer.AbiEntry, threads int) ([]indexer.TransactionLabel, map[uint64]seer_common.BlockWithTransactions, error) {
	var transactionsLabels []indexer.TransactionLabel

	var blocksCache map[uint64]seer_common.BlockWithTransactions

	// Get blocks in range
	blocks, err := c.FetchBlocksInRangeAsync(big.NewInt(int64(startBlock)), big.NewInt(int64(endBlock)), false, threads)

	if err != nil {
		return nil, nil, err
	}

	// Get transactions in range

	for _, block := range blocks {

		blockNumber, err := strconv.ParseUint(block.BlockNumber, 0, 64)
		if err != nil {
			log.Fatalf("Failed to convert BlockNumber to uint64: %v", err)
		}

		blockTimestamp, err := strconv.ParseUint(block.Timestamp, 0, 64)

		if err != nil {
			log.Fatalf("Failed to convert BlockTimestamp to uint64: %v", err)
		}

		if blocksCache == nil {
			blocksCache = make(map[uint64]seer_common.BlockWithTransactions)
		}

		blocksCache[blockNumber] = seer_common.BlockWithTransactions{
			BlockNumber:    blockNumber,
			BlockHash:      block.Hash,
			BlockTimestamp: blockTimestamp,
			Transactions:   make(map[string]seer_common.TransactionJson),
		}

		for _, tx := range block.Transactions {

			label := indexer.SeerCrawlerLabel

			if len(tx.Input) < 10 { // If input is less than 3 characters then it direct transfer
				continue
			}
			// Fill blocks cache
			blocksCache[blockNumber].Transactions[tx.Hash] = tx

			// Process transaction labels

			selector := tx.Input[:10]

			if abiMap[tx.ToAddress] != nil && abiMap[tx.ToAddress][selector] != nil {

				abiEntryTx := abiMap[tx.ToAddress][selector]

				var err error
				abiEntryTx.Once.Do(func() {
					abiEntryTx.Abi, err = seer_common.GetABI(abiEntryTx.AbiJSON)
					if err != nil {
						fmt.Println("Error getting ABI: ", err)
						return
					}
				})

				// Check if an error occurred during ABI parsing
				if abiEntryTx.Abi == nil {
					fmt.Println("Error getting ABI: ", err)
					return nil, nil, err
				}

				inputData, err := hex.DecodeString(tx.Input[2:])
				if err != nil {
					fmt.Println("Error decoding input data: ", err)
					return nil, nil, err
				}

				decodedArgsTx, decodeErr := seer_common.DecodeTransactionInputDataToInterface(abiEntryTx.Abi, inputData)
				if decodeErr != nil {
					fmt.Println("Error decoding transaction not decoded data: ", tx.Hash, decodeErr)
					decodedArgsTx = map[string]interface{}{
						"input_raw": tx,
						"abi":       abiEntryTx.AbiJSON,
						"selector":  selector,
						"error":     decodeErr,
					}
					label = indexer.SeerCrawlerRawLabel
				}

				ctxWithTimeout, cancel := context.WithTimeout(context.Background(), c.timeout)

				defer cancel()

				receipt, err := c.TransactionReceipt(ctxWithTimeout, common.HexToHash(tx.Hash))

				if err != nil {
					fmt.Println("Error fetching transaction receipt: ", err)
					return nil, nil, err
				}

				// check if the transaction was successful
				if receipt.Status == 1 {
					decodedArgsTx["status"] = 1
				} else {
					decodedArgsTx["status"] = 0
				}

				txLabelDataBytes, err := json.Marshal(decodedArgsTx)
				if err != nil {
					fmt.Println("Error converting decodedArgsTx to JSON: ", err)
					return nil, nil, err
				}

				// Convert transaction to label
				transactionLabel := indexer.TransactionLabel{
					Address:         tx.ToAddress,
					BlockNumber:     blockNumber,
					BlockHash:       tx.BlockHash,
					CallerAddress:   tx.FromAddress,
					LabelName:       abiEntryTx.AbiName,
					LabelType:       "tx_call",
					OriginAddress:   tx.FromAddress,
					Label:           label,
					TransactionHash: tx.Hash,
					LabelData:       string(txLabelDataBytes), // Convert JSON byte slice to string
					BlockTimestamp:  blockTimestamp,
				}

				transactionsLabels = append(transactionsLabels, transactionLabel)
			}

		}

	}

	return transactionsLabels, blocksCache, nil

}

func (c *Client) GetEventsLabels(startBlock uint64, endBlock uint64, abiMap map[string]map[string]*indexer.AbiEntry, blocksCache map[uint64]seer_common.BlockWithTransactions) ([]indexer.EventLabel, error) {
	var eventsLabels []indexer.EventLabel

	if blocksCache == nil {
		blocksCache = make(map[uint64]seer_common.BlockWithTransactions)
	}

	// Get events in range

	var addresses []common.Address
	var topics []common.Hash

	for address, selectorMap := range abiMap {
		for selector, _ := range selectorMap {
			topics = append(topics, common.HexToHash(selector))
		}

		addresses = append(addresses, common.HexToAddress(address))
	}

	// query filter from abiMap
	filter := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(startBlock)),
		ToBlock:   big.NewInt(int64(endBlock)),
		Addresses: addresses,
		Topics:    [][]common.Hash{topics},
	}

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), c.timeout)

	defer cancel()

	logs, err := c.ClientFilterLogs(ctxWithTimeout, filter, false)

	if err != nil {
		return nil, err
	}

	for _, log := range logs {
		var decodedArgsLogs map[string]interface{}
		label := indexer.SeerCrawlerLabel

		var topicSelector string

		if len(log.Topics) > 0 {
			topicSelector = log.Topics[0]
		} else {
			// 0x0 is the default topic selector
			topicSelector = "0x0"
		}

		if abiMap[log.Address] == nil || abiMap[log.Address][topicSelector] == nil {
			continue
		}

		abiEntryLog := abiMap[log.Address][topicSelector]

		var initErr error
		abiEntryLog.Once.Do(func() {
			abiEntryLog.Abi, initErr = seer_common.GetABI(abiEntryLog.AbiJSON)
		})

		// Check if an error occurred during ABI parsing
		if initErr != nil || abiEntryLog.Abi == nil {
			fmt.Println("Error getting ABI: ", initErr)
			return nil, initErr
		}

		// Decode the event data
		decodedArgsLogs, decodeErr := seer_common.DecodeLogArgsToLabelData(abiEntryLog.Abi, log.Topics, log.Data)
		if decodeErr != nil {
			fmt.Println("Error decoding event not decoded data: ", log.TransactionHash, decodeErr)
			decodedArgsLogs = map[string]interface{}{
				"input_raw": log,
				"abi":       abiEntryLog.AbiJSON,
				"selector":  topicSelector,
				"error":     decodeErr,
			}
			label = indexer.SeerCrawlerRawLabel
		}

		// Convert decodedArgsLogs map to JSON
		labelDataBytes, err := json.Marshal(decodedArgsLogs)
		if err != nil {
			fmt.Println("Error converting decodedArgsLogs to JSON: ", err)
			return nil, err
		}

		blockNumber, err := strconv.ParseUint(log.BlockNumber, 0, 64)
		if err != nil {
			return nil, err
		}

		if _, ok := blocksCache[blockNumber]; !ok {

			ctxWithTimeout, cancel := context.WithTimeout(context.Background(), c.timeout)

			defer cancel()

			// get block from rpc
			block, err := c.GetBlockByNumber(ctxWithTimeout, big.NewInt(int64(blockNumber)), true)
			if err != nil {
				return nil, err
			}

			blockTimestamp, err := strconv.ParseUint(block.Timestamp, 0, 64)
			if err != nil {
				return nil, err
			}

			blocksCache[blockNumber] = seer_common.BlockWithTransactions{
				BlockNumber:    blockNumber,
				BlockHash:      block.Hash,
				BlockTimestamp: blockTimestamp,
				Transactions:   make(map[string]seer_common.TransactionJson),
			}

			for _, tx := range block.Transactions {
				blocksCache[blockNumber].Transactions[tx.Hash] = tx
			}

		}

		transaction := blocksCache[blockNumber].Transactions[log.TransactionHash]

		logIndex, err := strconv.ParseUint(log.LogIndex, 0, 64)
		if err != nil {
			return nil, err
		}

		// Convert event to label
		eventLabel := indexer.EventLabel{
			Label:           label,
			LabelName:       abiEntryLog.AbiName,
			LabelType:       "event",
			BlockNumber:     blockNumber,
			BlockHash:       log.BlockHash,
			Address:         log.Address,
			OriginAddress:   transaction.FromAddress,
			TransactionHash: log.TransactionHash,
			LabelData:       string(labelDataBytes), // Convert JSON byte slice to string
			BlockTimestamp:  blocksCache[blockNumber].BlockTimestamp,
			LogIndex:        logIndex,
		}

		eventsLabels = append(eventsLabels, eventLabel)

	}

	return eventsLabels, nil

}
