package indexer

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a global variable to hold the GORM database connection.

func LabelsTableName(blockchain string) string {
	return fmt.Sprintf(blockchain + "_labels")
}

func BlocksTableName(blockchain string) (string, error) {
	switch blockchain {
	case "arbitrum_one":
		return "arbitrum_one_blocks", nil
	case "arbitrum_sepolia":
		return "arbitrum_sepolia_blocks", nil
	case "b3":
		return "b3_blocks", nil
	case "b3_sepolia":
		return "b3_sepolia_blocks", nil
	case "ethereum":
		return "ethereum_blocks", nil
	case "game7":
		return "game7_blocks", nil
	case "game7_orbit_arbitrum_sepolia":
		return "game7_orbit_arbitrum_sepolia_blocks", nil
	case "game7_testnet":
		return "game7_testnet_blocks", nil
	case "imx_zkevm":
		return "imx_zkevm_blocks", nil
	case "imx_zkevm_sepolia":
		return "imx_zkevm_sepolia_blocks", nil
	case "mantle":
		return "mantle_blocks", nil
	case "mantle_sepolia":
		return "mantle_sepolia_blocks", nil
	case "polygon":
		return "polygon_blocks", nil
	case "ronin":
		return "ronin_blocks", nil
	case "ronin_saigon":
		return "ronin_saigon_blocks", nil
	case "sepolia":
		return "sepolia_blocks", nil
	case "xai":
		return "xai_blocks", nil
	case "xai_sepolia":
		return "xai_sepolia_blocks", nil
	default:
		return "", fmt.Errorf("Unsupported blockchain")
	}
}

func TransactionsTableName(blockchain string) (string, error) {
	switch blockchain {
	case "arbitrum_one":
		return "arbitrum_one_transactions", nil
	case "arbitrum_sepolia":
		return "arbitrum_sepolia_transactions", nil
	case "b3":
		return "b3_transactions", nil
	case "b3_sepolia":
		return "b3_sepolia_transactions", nil
	case "ethereum":
		return "ethereum_transactions", nil
	case "game7":
		return "game7_transactions", nil
	case "game7_orbit_arbitrum_sepolia":
		return "game7_orbit_arbitrum_sepolia_transactions", nil
	case "game7_testnet":
		return "game7_testnet_transactions", nil
	case "imx_zkevm":
		return "imx_zkevm_transactions", nil
	case "imx_zkevm_sepolia":
		return "imx_zkevm_sepolia_transactions", nil
	case "mantle":
		return "mantle_transactions", nil
	case "mantle_sepolia":
		return "mantle_sepolia_transactions", nil
	case "polygon":
		return "polygon_transactions", nil
	case "ronin":
		return "ronin_transactions", nil
	case "ronin_saigon":
		return "ronin_saigon_transactions", nil
	case "sepolia":
		return "sepolia_transactions", nil
	case "xai":
		return "xai_transactions", nil
	case "xai_sepolia":
		return "xai_sepolia_transactions", nil
	default:
		return "", fmt.Errorf("Unsupported blockchain")
	}
}

func CustomerDBTransactionsTableName(blockchain string) string {
	return fmt.Sprintf(blockchain + "_transactions")
}

// Helper function to convert hex string to nullable big.Int
func hexStringToBigInt(hexString string) (*big.Int, error) {
	if hexString == "" {
		return nil, nil
	}

	// Remove the "0x" prefix
	hexString = strings.TrimPrefix(hexString, "0x")

	// Parse hex string to big.Int
	value := new(big.Int)
	value, success := value.SetString(hexString, 16)
	if !success {
		return nil, fmt.Errorf("failed to parse hex string: %s", hexString)
	}

	return value, nil
}

// https://klotzandrew.com/blog/postgres-passing-65535-parameter-limit/ insted of batching
type UnnestInsertValueStruct struct {
	Type   string        `json:"type"`   // e.g. "BIGINT" or "TEXT" or any other PostgreSQL data type
	Values []interface{} `json:"values"` // e.g. [1, 2, 3, 4, 5]
}

func IsBlockchainWithL1Chain(blockchain string) bool {
	switch blockchain {
	case "ethereum":
		return false
	case "polygon":
		return false
	case "arbitrum_one":
		return true
	case "arbitrum_sepolia":
		return true
	case "game7_orbit_arbitrum_sepolia":
		return true
	case "game7_testnet":
		return true
	case "game7":
		return true
	case "xai":
		return true
	case "xai_sepolia":
		return true
	case "mantle":
		return false
	case "mantle_sepolia":
		return false
	case "b3":
		return false
	case "b3_sepolia":
		return false
	case "ronin":
		return false
	case "ronin_saigon":
		return false
	default:
		return false
	}
}

func FilterABIJobs(abiJobs []AbiJob, ids []string) []AbiJob {
	var filteredABIJobs []AbiJob

	for _, abiJob := range abiJobs {
		for _, id := range ids {
			if abiJob.ID == id {
				filteredABIJobs = append(filteredABIJobs, abiJob)
			}
		}
	}

	return filteredABIJobs
}

type PostgreSQLpgx struct {
	pool *pgxpool.Pool
}

func NewPostgreSQLpgx(dbUri string) (*PostgreSQLpgx, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

	defer cancel()

	newURL := fmt.Sprintf("%s?pool_max_conns=10&pool_min_conns=10&pool_max_conn_lifetime=10m&pool_max_conn_idle_time=10m", dbUri)

	config, err := pgxpool.ParseConfig(newURL)

	if err != nil {
		log.Println("Error parsing config", err)
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Println("Error creating pool", err)
		return nil, err
	}

	return &PostgreSQLpgx{
		pool: pool,
	}, nil
}

func NewPostgreSQLpgxWithCustomURI(uri string) (*PostgreSQLpgx, error) {

	//  create a connection to the database

	pool, err := pgxpool.New(context.Background(), uri)
	if err != nil {
		log.Println("Error creating pool", err)
		return nil, err
	}

	return &PostgreSQLpgx{
		pool: pool,
	}, nil
}

func (p *PostgreSQLpgx) Close() {
	p.pool.Close()
}

func (p *PostgreSQLpgx) GetPool() *pgxpool.Pool {
	return p.pool
}

// read from database

func (p *PostgreSQLpgx) ReadBlockIndex(ctx context.Context, startBlock uint64, endBlock uint64) ([]BlockIndex, error) {

	pool := p.GetPool()

	conn, err := pool.Acquire(ctx)

	if err != nil {
		return nil, err
	}

	defer conn.Release()

	rows, err := conn.Query(ctx, "SELECT * FROM products WHERE block_number >= $1 AND block_number <= $2", startBlock, endBlock)

	if err != nil {
		return nil, err
	}

	blocksIndex, err := pgx.CollectRows(rows, pgx.RowToStructByName[BlockIndex])

	if err != nil {
		return nil, err
	}

	return blocksIndex, nil

}

func (p *PostgreSQLpgx) ReadIndexOnRange(tableName string, startBlock uint64, endBlock uint64) ([]interface{}, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return nil, err
	}

	defer conn.Release()

	var indices []interface{}

	rows, err := conn.Query(context.Background(), "SELECT bt.block_number, bt.block_hash, bt.block_timestamp, tt.hash, tt.index, tt.path as transaction_, tt.input as transaction_input, lt.selector, lt.topic1, lt.topic2, lt.transaction_hash, lt.log_index, lt.path as event_path FROM block_index bt LEFT JOIN transaction_index tt ON bt.block_number = tt.block_number LEFT JOIN log_index lt ON tt.hash = lt.transaction_hash WHERE bt.block_number >= $1 AND bt.block_number <= $2", startBlock, endBlock)

	if err != nil {
		return nil, err
	}

	for rows.Next() {

		var index interface{}

		err = rows.Scan(&index)

		if err != nil {
			return nil, err
		}

		indices = append(indices, index)
	}

	return indices, nil
}

func (p *PostgreSQLpgx) ReadLastLabel(blockchain string) (uint64, error) {
	pool := p.GetPool()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	conn, err := pool.Acquire(ctx)

	if err != nil {
		return 0, err
	}

	defer conn.Release()

	var label uint64

	query := fmt.Sprintf("SELECT block_number FROM %s ORDER BY block_number DESC LIMIT 1", blockchain+"_labels")

	err = conn.QueryRow(context.Background(), query).Scan(&label)

	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, err

	}

	return label, nil
}

func decodeAddress(address string) ([]byte, error) {
	if len(address) < 2 {
		return []byte{0x00}, nil
	}
	return hex.DecodeString(address[2:])
}

// updateValues updates the values in the map for a given key
func updateValues(valuesMap map[string]UnnestInsertValueStruct, key string, val interface{}) {
	entry, ok := valuesMap[key]
	if !ok {
		return
	}

	value := determineValue(val)
	entry.Values = append(entry.Values, value)
	valuesMap[key] = entry
}

// determineValue handles type checking and conversion logic
func determineValue(val interface{}) interface{} {
	// Handle nil interface
	if val == nil {
		return nil
	}

	rv := reflect.ValueOf(val)

	// Handle typed nil pointer
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return nil
	}
	// Return value as-is for other types
	return val
}

func (p *PostgreSQLpgx) WriteIndexes(blockchain string, blocksIndexPack []BlockIndex) error {

	ctx := context.Background()
	pool := p.GetPool()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Println("Connection error", err)
		return err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback(ctx)
			panic(r)
		} else if err != nil {
			tx.Rollback(ctx)
		} else {
			err = tx.Commit(ctx)
		}
	}()

	// Write blocks index
	if len(blocksIndexPack) > 0 {
		err = p.writeBlockIndexToDB(tx, blockchain, blocksIndexPack)
		if err != nil {
			return err
		}
	}

	return nil
}

// Batch insert
func (p *PostgreSQLpgx) executeBatchInsert(tx pgx.Tx, ctx context.Context, tableName string, columns []string, values map[string]UnnestInsertValueStruct, conflictClause string) error {

	types := make([]string, 0)

	for index, column := range columns {
		// constract  unnest($1::int[], $2::int[] ...)
		types = append(types, fmt.Sprintf("$%d::%s[]", index+1, values[column].Type))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) SELECT * FROM unnest(%s) %s", tableName, strings.Join(columns, ","), strings.Join(types, ","), conflictClause)

	// create a slices of values
	var valuesSlice []interface{}

	for _, column := range columns {

		valuesSlice = append(valuesSlice, values[column].Values)
	}

	// track execution time

	if _, err := tx.Exec(ctx, query, valuesSlice...); err != nil {
		log.Println("Error executing bulk insert", err)
		return fmt.Errorf("error executing bulk insert for batch: %w", err)
	}

	return nil
}

func (p *PostgreSQLpgx) writeBlockIndexToDB(tx pgx.Tx, blockchain string, indexes []BlockIndex) error {
	tableName, blocksTableErr := BlocksTableName(blockchain)
	if blocksTableErr != nil {
		return blocksTableErr
	}
	isBlockchainWithL1Chain := IsBlockchainWithL1Chain(blockchain)
	columns := []string{"block_number", "block_hash", "block_timestamp", "parent_hash", "row_id", "path", "transactions_indexed_at", "logs_indexed_at"}

	valuesMap := make(map[string]UnnestInsertValueStruct)

	valuesMap["block_number"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: []interface{}{},
	}

	valuesMap["block_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_timestamp"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["parent_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["row_id"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["path"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["transactions_indexed_at"] = UnnestInsertValueStruct{
		Type:   "TIMESTAMP",
		Values: make([]interface{}, 0),
	}

	valuesMap["logs_indexed_at"] = UnnestInsertValueStruct{
		Type:   "TIMESTAMP",
		Values: make([]interface{}, 0),
	}

	if isBlockchainWithL1Chain {
		columns = append(columns, "l1_block_number")
		valuesMap["l1_block_number"] = UnnestInsertValueStruct{
			Type:   "BIGINT",
			Values: make([]interface{}, 0),
		}
	}

	for _, index := range indexes {

		updateValues(valuesMap, "block_number", index.BlockNumber)
		updateValues(valuesMap, "block_hash", index.BlockHash)
		updateValues(valuesMap, "block_timestamp", index.BlockTimestamp)
		updateValues(valuesMap, "parent_hash", index.ParentHash)
		updateValues(valuesMap, "row_id", index.RowID)
		updateValues(valuesMap, "path", index.Path)
		updateValues(valuesMap, "transactions_indexed_at", "now()")
		updateValues(valuesMap, "logs_indexed_at", "now()")

		if isBlockchainWithL1Chain {
			updateValues(valuesMap, "l1_block_number", index.L1BlockNumber)
		}
	}

	ctx := context.Background()
	err = p.executeBatchInsert(tx, ctx, tableName, columns, valuesMap, "ON CONFLICT (block_number) DO NOTHING")

	if err != nil {
		return err
	}

	log.Printf("Add %d records into %s table", len(indexes), tableName)

	return nil
}

// GetEdgeDBBlock fetch first or last block for specified blockchain
func (p *PostgreSQLpgx) GetEdgeDBBlock(ctx context.Context, blockchain, side string) (BlockIndex, error) {
	var blockIndex BlockIndex

	pool := p.GetPool()

	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		return blockIndex, acquireErr
	}

	defer conn.Release()

	tableName, blocksTableErr := BlocksTableName(blockchain)
	if blocksTableErr != nil {
		return blockIndex, blocksTableErr
	}

	query := fmt.Sprintf("SELECT block_number,block_hash,block_timestamp,parent_hash,row_id,path,l1_block_number FROM %s ORDER BY block_number", tableName)

	switch side {
	case "first":
		query = fmt.Sprintf("%s LIMIT 1", query)
	case "last":
		query = fmt.Sprintf("%s DESC LIMIT 1", query)
	default:
		return blockIndex, fmt.Errorf("not supported side, choose 'first' or 'last' block")
	}

	queryErr := conn.QueryRow(context.Background(), query).Scan(
		&blockIndex.BlockNumber,
		&blockIndex.BlockHash,
		&blockIndex.BlockTimestamp,
		&blockIndex.ParentHash,
		&blockIndex.RowID,
		&blockIndex.Path,
		&blockIndex.L1BlockNumber,
	)
	if queryErr != nil {
		return blockIndex, queryErr
	}

	blockIndex.chain = blockchain

	return blockIndex, nil
}

func (p *PostgreSQLpgx) GetLatestDBBlockNumber(blockchain string, reverse ...bool) (uint64, error) {

	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return 0, err

	}

	defer conn.Release()

	var blockNumber uint64

	blocksTableName, blocksTableErr := BlocksTableName(blockchain)
	if blocksTableErr != nil {
		return 0, blocksTableErr
	}
	// Check if reverse is provided, if not, default to false (DESC order)
	orderDirection := "DESC"
	if len(reverse) > 0 && reverse[0] {
		orderDirection = "ASC"
	}

	query := fmt.Sprintf("SELECT block_number FROM %s ORDER BY block_number %s LIMIT 1", blocksTableName, orderDirection)

	err = conn.QueryRow(context.Background(), query).Scan(&blockNumber)
	if err != nil {
		log.Printf("No data found in %s table", blocksTableName)
		return 0, err
	}

	return blockNumber, nil

}

func (p *PostgreSQLpgx) ReadABIJobs(blockchain string) ([]AbiJob, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return nil, err
	}

	defer conn.Release()

	rows, err := conn.Query(context.Background(), "SELECT id, address, user_id, customer_id, abi_selector, chain, abi_name, status, historical_crawl_status, progress, moonworm_task_pickedup, '[' || abi || ']' as abi, (abi::jsonb)->>'type' as abiType, created_at, updated_at, deployment_block_number FROM abi_jobs where chain=$1 and (abi::jsonb)->>'type' is not null", blockchain)

	if err != nil {
		return nil, err
	}

	abiJobs, err := pgx.CollectRows(rows, pgx.RowToStructByName[AbiJob])
	if err != nil {
		log.Println("Error collecting Abi jobs rows", err)
		return nil, err
	}

	// Check if we have at least one job before accessing
	if len(abiJobs) == 0 {
		return nil, nil // or return an appropriate error if this is considered an error state
	}

	//log.Println("Parsed abiJobs:", len(abiJobs), "for blockchain:", blockchain)
	// If you need to process or log the first ABI job separately, do it here

	return abiJobs, nil
}

func (p *PostgreSQLpgx) GetCustomersIDs(blockchain string) ([]string, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return nil, err
	}

	defer conn.Release()

	rows, err := conn.Query(context.Background(), "SELECT DISTINCT customer_id FROM abi_jobs where customer_id is not null and blockchain=$1", blockchain)

	if err != nil {
		return nil, err
	}

	var customerIds []string

	for rows.Next() {

		var customerId string

		err = rows.Scan(&customerId)

		if err != nil {
			return nil, err
		}

		customerIds = append(customerIds, customerId)

	}

	return customerIds, nil
}

func (p *PostgreSQLpgx) ReadUpdates(blockchain string, fromBlock uint64, customerIds []string, minBlocksToSync int) (uint64, uint64, []string, []CustomerUpdates, error) {

	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	var paths []string

	if err != nil {
		return 0, 0, paths, nil, err
	}

	defer conn.Release()

	blocksTableName, blocksTableErr := BlocksTableName(blockchain)
	if blocksTableErr != nil {
		return 0, 0, paths, nil, blocksTableErr
	}

	query := fmt.Sprintf(`WITH path as (
        SELECT
            path,
			block_number
        from
            %s
        WHERE
            block_number >= $1 and block_number <= $1 + $3
    ),
	latest_block_of_path as (
		SELECT
			block_number as latest_block_number
		from
			%s
		WHERE
			path = (SELECT path FROM path ORDER BY block_number DESC LIMIT 1)
		order by block_number desc
		limit 1
	),
    jobs AS (
        SELECT
            address as address,
            '0x' || encode(address, 'hex') as address_str,
            customer_id,
            abi_selector,
            abi_name,
            abi,
			(abi)::jsonb ->> 'type' as abi_type,
        	(abi)::jsonb ->> 'stateMutability' as abi_stateMutability
        FROM
            abi_jobs
        WHERE
            chain = $2
    ),
    address_abis AS (
        SELECT
            address_str,
            customer_id,
            json_object_agg(
                abi_selector,
                json_build_object(
                    'abi', '[' || abi || ']',
                    'abi_name', abi_name,
					'abi_type', abi_type 
                )
            ) AS abis_per_address
        FROM
            jobs
        GROUP BY
            address_str,
            customer_id
    ),
    reformatted_jobs AS (
        SELECT
            customer_id,
            json_object_agg(address_str, abis_per_address) AS abis
        FROM
            address_abis
        GROUP BY
            customer_id
    )
	SELECT
    	latest_block_number,
    	(SELECT array_agg(DISTINCT path) FROM path) as paths,
    	(SELECT json_agg(json_build_object(customer_id, abis)) FROM reformatted_jobs) as jobs
	FROM
    	latest_block_of_path
	`, blocksTableName, blocksTableName)

	rows, err := conn.Query(context.Background(), query, fromBlock, blockchain, minBlocksToSync)

	if err != nil {
		log.Println("Error querying abi jobs from database", err)
		return 0, 0, paths, nil, err
	}

	var customers []map[string]map[string]map[string]*AbiEntry
	var firstBlockNumber, lastBlockNumber uint64

	for rows.Next() {
		err = rows.Scan(&lastBlockNumber, &paths, &customers)
		if err != nil {
			log.Println("Error scanning row:", err)
			return 0, 0, paths, nil, err
		}
	}

	var customerUpdates []CustomerUpdates

	for _, customerUpdate := range customers {

		for customerid, abis := range customerUpdate {

			customerUpdates = append(customerUpdates, CustomerUpdates{
				CustomerID: customerid,
				Abis:       abis,
			})

		}

	}

	return firstBlockNumber, lastBlockNumber, paths, customerUpdates, nil

}

func (p *PostgreSQLpgx) EnsureCorrectSelectors(blockchain string, WriteToDB bool, outputFilePath string, ids []string) error {

	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return err
	}

	defer conn.Release()

	// Get all the ABI jobs for the blockchain

	abiJobs, err := p.ReadABIJobs(blockchain)

	if err != nil {
		return err
	}

	if len(ids) > 0 {
		abiJobs = FilterABIJobs(abiJobs, ids)
	} else {
		log.Println("Found", len(abiJobs), "ABI jobs for blockchain:", blockchain)
	}
	var writer *bufio.Writer
	var f *os.File

	// for each ABI job, check if the selector is correct

	if outputFilePath != "" {

		f, err := os.OpenFile(outputFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)

		if err != nil {
			log.Println("Error opening file:", err)
			return err
		}

		writer := bufio.NewWriter(f)

		writer.WriteString(fmt.Sprintf("ABI jobs for blockchain: %s runned as WriteToDB: %v recorded at %s\n", blockchain, WriteToDB, time.Now().String()))

	}

	for _, abiJob := range abiJobs {

		// Now you can use abiJSONStr as a string
		abiObj, err := abi.JSON(strings.NewReader(abiJob.Abi))
		if err != nil {
			log.Println("Error parsing ABI for ABI job:", abiJob.ID, err)
			return err
		}

		var selector string

		if abiJob.AbiType == "event" {
			selector = abiObj.Events[abiJob.AbiName].ID.String()
		} else {
			selectorRaw := abiObj.Methods[abiJob.AbiName].ID
			selector = fmt.Sprintf("0x%x", selectorRaw)
		}

		if err != nil {
			log.Println("Error getting selector for ABI job:", abiJob.ID, err)
			continue
		}

		// Check if the selector is correct

		if abiJob.AbiSelector != selector {

			if WriteToDB {
				// Update the selector in the database

				_, err := conn.Exec(context.Background(), "UPDATE abi_jobs SET abi_selector = $1 WHERE id = $2", selector, abiJob.ID)

				if err != nil {
					log.Println("Error updating selector for ABI job:", abiJob.ID, err)
					continue
				}

				log.Println("Updated selector:", abiJob.AbiSelector, " for ABI job:", abiJob.ID, " to new selector:", selector)

			}

			if outputFilePath != "" {

				_, err = writer.WriteString(fmt.Sprintf("ABI job ID: %s, Name: %s, Address: %x, Selector: %s, Correct Selector: %s\n", abiJob.ID, abiJob.AbiName, abiJob.Address, abiJob.AbiSelector, selector))
				if err != nil {
					log.Println("Error writing to file:", err)
					continue
				}

			}

		}

	}

	if outputFilePath != "" {
		writer.Flush()

		f.Close()
	}
	return nil
}

func (p *PostgreSQLpgx) WriteDataToCustomerDB(
	blockchain string,
	txCalls []TransactionLabel,
	events []EventLabel,
	rawTransactions []RawTransaction,
) error {

	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return err
	}

	defer conn.Release()

	tx, err := conn.Begin(context.Background())

	if err != nil {
		return err
	}

	defer func() {
		if pErr := recover(); pErr != nil {
			tx.Rollback(context.Background())
			panic(pErr)
		} else if err != nil {
			tx.Rollback(context.Background())
		} else {
			err = tx.Commit(context.Background())
			if err != nil {
				log.Println("Error committing transaction:", err)
				panic(err)
			}
		}
	}()

	if len(txCalls) > 0 {
		err := p.WriteTransactions(tx, blockchain, txCalls)
		if err != nil {
			log.Println("Error writing transactions:", err)
			return err
		}
	}

	if len(events) > 0 {
		err := p.WriteEvents(tx, blockchain, events)
		if err != nil {
			log.Println("Error writing events:", err)
			return err
		}
	}

	if len(rawTransactions) > 0 {
		err := p.WriteRawTransactions(tx, blockchain, rawTransactions)
		if err != nil {
			log.Println("Error writing raw transactions:", err)
			return err
		}
	}

	return err
}

func (p *PostgreSQLpgx) WriteEvents(tx pgx.Tx, blockchain string, events []EventLabel) error {

	tableName := LabelsTableName(blockchain)
	columns := []string{"id", "label", "transaction_hash", "log_index", "block_number", "block_hash", "block_timestamp", "caller_address", "origin_address", "address", "label_name", "label_type", "label_data"}
	var valuesMap = make(map[string]UnnestInsertValueStruct)

	valuesMap["id"] = UnnestInsertValueStruct{
		Type:   "UUID",
		Values: make([]interface{}, 0),
	}

	valuesMap["label"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["transaction_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["log_index"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_number"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_timestamp"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["caller_address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["origin_address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["label_name"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["label_type"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["label_data"] = UnnestInsertValueStruct{
		Type:   "jsonb",
		Values: make([]interface{}, 0),
	}

	for _, event := range events {

		id := uuid.New()

		callerAddressBytes, err := decodeAddress(event.CallerAddress)
		if err != nil {
			fmt.Println("Error decoding caller address:", err, event)
			continue
		}

		originAddressBytes, err := decodeAddress(event.OriginAddress)
		if err != nil {
			fmt.Println("Error decoding origin address:", err, event)
			continue
		}

		addressBytes, err := decodeAddress(event.Address)
		if err != nil {
			fmt.Println("Error decoding address:", err, event)
			continue
		}

		updateValues(valuesMap, "id", id)
		updateValues(valuesMap, "label", event.Label)
		updateValues(valuesMap, "transaction_hash", event.TransactionHash)
		updateValues(valuesMap, "log_index", event.LogIndex)
		updateValues(valuesMap, "block_number", event.BlockNumber)
		updateValues(valuesMap, "block_hash", event.BlockHash)
		updateValues(valuesMap, "block_timestamp", event.BlockTimestamp)
		updateValues(valuesMap, "caller_address", callerAddressBytes)
		updateValues(valuesMap, "origin_address", originAddressBytes)
		updateValues(valuesMap, "address", addressBytes)
		updateValues(valuesMap, "label_name", event.LabelName)
		updateValues(valuesMap, "label_type", event.LabelType)
		updateValues(valuesMap, "label_data", event.LabelData)

	}

	ctx := context.Background()

	err := p.executeBatchInsert(tx, ctx, tableName, columns, valuesMap, "ON CONFLICT DO NOTHING")

	if err != nil {
		return err
	}

	log.Printf("Saved %d events records into %s table", len(events), tableName)

	return nil
}

type Transaction struct {
	BlockNumber uint64   `json:"block_number"`
	FromAddress string   `json:"from_address"`
	ToAddress   string   `json:"to_address"`
	Value       *big.Int `json:"value"`
}

type TransactionsVolume struct {
	MinBlockNumber uint64   `json:"min_block_number"`
	MaxBlockNumber uint64   `json:"max_block_number"`
	Volume         *big.Int `json:"volume"`
	TxsCount       uint64   `json:"txs_count"`
}

// Fetch all unique nodes
// It does not return all transactions between nodes, but only first
func getSelectClause(toAddrDistinct bool) string {
	if toAddrDistinct {
		return "DISTINCT ON (to_address) "
	}
	return ""
}

func getOrderClause(toAddrDistinct bool) string {
	if toAddrDistinct {
		return "to_address, block_number"
	}
	return "block_number"
}

func getAndBlockNumClause(lowestBlockNum uint64) string {
	if lowestBlockNum > 0 {
		return fmt.Sprintf("AND block_number >= %d ", lowestBlockNum)
	}
	return ""
}

func getWhereBidiVolClause(isBidirectional bool) string {
	if isBidirectional {
		return fmt.Sprintf("WHERE from_address IN ($1, $2) AND to_address IN ($1, $2) ")
	}
	return "WHERE from_address = $1 AND to_address = $2 "
}

func (p *PostgreSQLpgx) GetTransactionsVolume(blockchain, fromAddress, toAddress string, limit int, lowestBlockNum uint64, isBidirectional bool) (*TransactionsVolume, error) {
	txTableName, txTableErr := TransactionsTableName(blockchain)
	if txTableErr != nil {
		return nil, txTableErr
	}

	fromAddressBytes, fDecErr := decodeAddress(fromAddress)
	if fDecErr != nil {
		log.Printf("Error decoding address %s, err: %v", fDecErr, fromAddress)
		return nil, fDecErr
	}

	toAddressBytes, tDecErr := decodeAddress(toAddress)
	if tDecErr != nil {
		log.Printf("Error decoding address %s, err: %v", tDecErr, toAddress)
		return nil, tDecErr
	}

	pool := p.GetPool()

	ctx := context.Background()
	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer conn.Release()

	query := fmt.Sprintf(`
		SELECT 
			MIN(block_number) AS min_block_number,
			MAX(block_number) AS max_block_number,
			SUM(value) AS volume,
			COUNT(*) AS txs_count
		FROM (
			SELECT block_number, from_address, to_address, value
			FROM %s
			%s
			%s
			ORDER BY block_number
			LIMIT $3
		) AS limited_transactions;
	`, txTableName, getWhereBidiVolClause(isBidirectional), getAndBlockNumClause(lowestBlockNum))

	row := conn.QueryRow(context.Background(), query, fromAddressBytes, toAddressBytes, limit)

	var minBlockNum, maxBlockNum sql.NullInt64
	var volStr sql.NullString
	var txsCount uint64

	qErr := row.Scan(&minBlockNum, &maxBlockNum, &volStr, &txsCount)
	if qErr != nil {
		return nil, qErr
	}

	if txsCount == 0 {
		return nil, fmt.Errorf("not found")
	}

	if !minBlockNum.Valid || !maxBlockNum.Valid || !volStr.Valid {
		return nil, fmt.Errorf("Not correct results for %s %s address pair: %v %v %v", fromAddress, toAddress, minBlockNum.Valid, maxBlockNum.Valid, volStr.Valid)
	}

	vol := new(big.Int)
	vol.SetString(volStr.String, 10)

	return &TransactionsVolume{
		MinBlockNumber: uint64(minBlockNum.Int64),
		MaxBlockNumber: uint64(maxBlockNum.Int64),
		Volume:         vol,
		TxsCount:       txsCount,
	}, nil
}

func (p *PostgreSQLpgx) GetTransactionsVolumeV2(blockchain, fromAddress, toAddress string, limit int, lowestBlockNum uint64, isBidirectional bool) (*TransactionsVolume, error) {
	txTableName, txTableErr := TransactionsTableName(blockchain)
	if txTableErr != nil {
		return nil, txTableErr
	}

	pool := p.GetPool()

	ctx := context.Background()
	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer conn.Release()

	query := fmt.Sprintf(`
		SELECT 
			MIN(block_number) AS min_block_number,
			MAX(block_number) AS max_block_number,
			SUM(value) AS volume,
			COUNT(*) AS txs_count
		FROM (
			SELECT block_number, from_address, to_address, value
			FROM %s
			%s
			%s
			ORDER BY block_number
			LIMIT $3
		) AS limited_transactions;
	`, txTableName, getWhereBidiVolClause(isBidirectional), getAndBlockNumClause(lowestBlockNum))

	row := conn.QueryRow(context.Background(), query, fromAddress, toAddress, limit)

	var minBlockNum, maxBlockNum sql.NullInt64
	var volStr sql.NullString
	var txsCount uint64

	qErr := row.Scan(&minBlockNum, &maxBlockNum, &volStr, &txsCount)
	if qErr != nil {
		return nil, qErr
	}

	if txsCount == 0 {
		return nil, fmt.Errorf("not found")
	}

	if !minBlockNum.Valid || !maxBlockNum.Valid || !volStr.Valid {
		return nil, fmt.Errorf("Not correct results for %s %s address pair: %v %v %v", fromAddress, toAddress, minBlockNum.Valid, maxBlockNum.Valid, volStr.Valid)
	}

	vol := new(big.Int)
	vol.SetString(volStr.String, 10)

	return &TransactionsVolume{
		MinBlockNumber: uint64(minBlockNum.Int64),
		MaxBlockNumber: uint64(maxBlockNum.Int64),
		Volume:         vol,
		TxsCount:       txsCount,
	}, nil
}

func (p *PostgreSQLpgx) GetTransactionsVolumeBidirectionalV2(blockchain, fromAddress, toAddress string, limit int, lowestBlockNum uint64) (*TransactionsVolume, error) {
	txTableName, txTableErr := TransactionsTableName(blockchain)
	if txTableErr != nil {
		return nil, txTableErr
	}

	pool := p.GetPool()

	ctx := context.Background()
	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer conn.Release()

	query := fmt.Sprintf(`
		SELECT 
			MIN(block_number) AS min_block_number,
			MAX(block_number) AS max_block_number,
			SUM(value) AS volume,
			COUNT(*) AS txs_count
		FROM (
			SELECT block_number, from_address, to_address, value
			FROM %s
			WHERE from_address IN ($1, $2)
			AND to_address IN ($1, $2)
			%s
			ORDER BY block_number
			LIMIT $3
		) AS limited_transactions;
	`, txTableName, getAndBlockNumClause(lowestBlockNum))

	var txsVol TransactionsVolume
	var volStr string

	qErr := conn.QueryRow(context.Background(), query, fromAddress, toAddress, limit).Scan(&txsVol.MinBlockNumber, &txsVol.MaxBlockNumber, &volStr, &txsVol.TxsCount)
	if qErr != nil {
		return nil, qErr
	}

	txsVol.Volume = new(big.Int)
	txsVol.Volume.SetString(volStr, 10)

	return &txsVol, nil
}

func (p *PostgreSQLpgx) GetTransactions(blockchain string, sourceAddress []string, limit int, lowestBlockNum uint64, toAddrDistinct bool) ([]Transaction, error) {
	txTableName, txTableErr := TransactionsTableName(blockchain)
	if txTableErr != nil {
		return nil, txTableErr
	}

	var addressesBytes [][]byte
	for _, address := range sourceAddress {
		addressBytes, err := decodeAddress(address)
		if err != nil {
			log.Printf("Error decoding address %s, err: %v", err, address)
			continue
		}
		addressesBytes = append(addressesBytes, addressBytes)
	}

	pool := p.GetPool()

	ctx := context.Background()
	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer conn.Release()

	query := fmt.Sprintf(`
		SELECT %s
			block_number,
			'0x' || encode(from_address, 'hex'),
			'0x' || encode(to_address, 'hex'),
			value
		FROM %s 
		WHERE from_address = ANY($1)
		%s
		ORDER BY %s
		LIMIT $2`, getSelectClause(toAddrDistinct), txTableName, getAndBlockNumClause(lowestBlockNum), getOrderClause(toAddrDistinct))

	rows, qErr := conn.Query(context.Background(), query, addressesBytes, limit)
	if qErr != nil {
		return nil, qErr
	}

	var txs []Transaction
	for rows.Next() {
		var tx Transaction
		var valueStr string

		err = rows.Scan(&tx.BlockNumber, &tx.FromAddress, &tx.ToAddress, &valueStr)
		if err != nil {
			log.Printf("Unable to scan row, err: %v", err)
		}

		tx.Value = new(big.Int)
		tx.Value.SetString(valueStr, 10)

		// Append the transaction to the slice
		txs = append(txs, tx)
	}

	return txs, nil
}

func (p *PostgreSQLpgx) GetTransactionsV2(blockchain string, sourceAddress []string, limit int, lowestBlockNum uint64, toAddrDistinct bool) ([]Transaction, error) {
	txTableName, txTableErr := TransactionsTableName(blockchain)
	if txTableErr != nil {
		return nil, txTableErr
	}

	pool := p.GetPool()

	ctx := context.Background()
	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer conn.Release()

	query := fmt.Sprintf(`
		SELECT %s
			block_number, 
			from_address, 
			to_address, 
			value
		FROM %s 
		WHERE from_address = ANY($1)
		%s
		ORDER BY %s
		LIMIT $2`, getSelectClause(toAddrDistinct), txTableName, getAndBlockNumClause(lowestBlockNum), getOrderClause(toAddrDistinct))

	rows, qErr := conn.Query(context.Background(), query, sourceAddress, limit)
	if qErr != nil {
		return nil, qErr
	}

	var txs []Transaction
	for rows.Next() {
		var tx Transaction
		var valueStr string

		err = rows.Scan(&tx.BlockNumber, &tx.FromAddress, &tx.ToAddress, &valueStr)
		if err != nil {
			log.Printf("Unable to scan row, err: %v", err)
		}

		tx.Value = new(big.Int)
		tx.Value.SetString(valueStr, 10)

		// Append the transaction to the slice
		txs = append(txs, tx)
	}

	return txs, nil
}

func (p *PostgreSQLpgx) WriteTransactions(tx pgx.Tx, blockchain string, transactions []TransactionLabel) error {
	tableName := LabelsTableName(blockchain)
	columns := []string{"id", "address", "block_number", "block_hash", "caller_address", "label_name", "label_type", "origin_address", "label", "transaction_hash", "label_data", "block_timestamp"}

	var valuesMap = make(map[string]UnnestInsertValueStruct)

	valuesMap["id"] = UnnestInsertValueStruct{
		Type:   "UUID",
		Values: make([]interface{}, 0),
	}

	valuesMap["address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_number"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["caller_address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["label_name"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["label_type"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["origin_address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["label"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["transaction_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["label_data"] = UnnestInsertValueStruct{
		Type:   "jsonb",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_timestamp"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	for _, transaction := range transactions {

		id := uuid.New()

		addressBytes, err := decodeAddress(transaction.Address)
		if err != nil {
			fmt.Println("Error decoding address:", err, transaction)
			continue
		}

		callerAddressBytes, err := decodeAddress(transaction.CallerAddress)
		if err != nil {
			fmt.Println("Error decoding caller address:", err, transaction)
			continue
		}

		originAddressBytes, err := decodeAddress(transaction.OriginAddress)
		if err != nil {
			fmt.Println("Error decoding origin address:", err, transaction)
			continue
		}

		updateValues(valuesMap, "id", id)
		updateValues(valuesMap, "address", addressBytes)
		updateValues(valuesMap, "block_number", transaction.BlockNumber)
		updateValues(valuesMap, "block_hash", transaction.BlockHash)
		updateValues(valuesMap, "caller_address", callerAddressBytes)
		updateValues(valuesMap, "label_name", transaction.LabelName)
		updateValues(valuesMap, "label_type", transaction.LabelType)
		updateValues(valuesMap, "origin_address", originAddressBytes)
		updateValues(valuesMap, "label", transaction.Label)
		updateValues(valuesMap, "transaction_hash", transaction.TransactionHash)
		updateValues(valuesMap, "label_data", transaction.LabelData)
		updateValues(valuesMap, "block_timestamp", transaction.BlockTimestamp)

	}

	ctx := context.Background()

	err := p.executeBatchInsert(tx, ctx, tableName, columns, valuesMap, "ON CONFLICT DO NOTHING")

	if err != nil {
		return err
	}

	log.Printf("Saved %d tx_calls records into %s table", len(transactions), tableName)

	return nil
}

func (p *PostgreSQLpgx) WriteRawTransactions(tx pgx.Tx, blockchain string, rawTransactions []RawTransaction) error {
	tableName := CustomerDBTransactionsTableName(blockchain)
	isBlockchainWithL1Chain := IsBlockchainWithL1Chain(blockchain)

	columns := []string{"hash", "block_hash", "block_timestamp", "block_number",
		"from_address", "to_address", "gas", "gas_price", "input", "nonce",
		"max_fee_per_gas", "max_priority_fee_per_gas", "transaction_index",
		"transaction_type", "value"}

	if isBlockchainWithL1Chain {
		columns = append(columns, "l1_block_number")
	}

	var valuesMap = make(map[string]UnnestInsertValueStruct)

	valuesMap["hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_hash"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_timestamp"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["block_number"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["from_address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["to_address"] = UnnestInsertValueStruct{
		Type:   "BYTEA",
		Values: make([]interface{}, 0),
	}

	valuesMap["gas"] = UnnestInsertValueStruct{
		Type:   "NUMERIC",
		Values: make([]interface{}, 0),
	}

	valuesMap["gas_price"] = UnnestInsertValueStruct{
		Type:   "NUMERIC",
		Values: make([]interface{}, 0),
	}

	valuesMap["input"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["nonce"] = UnnestInsertValueStruct{
		Type:   "TEXT",
		Values: make([]interface{}, 0),
	}

	valuesMap["max_fee_per_gas"] = UnnestInsertValueStruct{
		Type:   "NUMERIC",
		Values: make([]interface{}, 0),
	}

	valuesMap["max_priority_fee_per_gas"] = UnnestInsertValueStruct{
		Type:   "NUMERIC",
		Values: make([]interface{}, 0),
	}

	valuesMap["transaction_index"] = UnnestInsertValueStruct{
		Type:   "BIGINT",
		Values: make([]interface{}, 0),
	}

	valuesMap["transaction_type"] = UnnestInsertValueStruct{
		Type:   "INTEGER",
		Values: make([]interface{}, 0),
	}

	valuesMap["value"] = UnnestInsertValueStruct{
		Type:   "NUMERIC",
		Values: make([]interface{}, 0),
	}

	valuesMap["indexed_at"] = UnnestInsertValueStruct{
		Type:   "TIMESTAMP WITH TIME ZONE",
		Values: make([]interface{}, 0),
	}

	if isBlockchainWithL1Chain {
		valuesMap["l1_block_number"] = UnnestInsertValueStruct{
			Type:   "BIGINT",
			Values: make([]interface{}, 0),
		}
	}

	// Now appending to the Values slice works without errors.
	for _, rawTransaction := range rawTransactions {
		fromAddress, err := decodeAddress(rawTransaction.FromAddress)
		if err != nil {
			return err
		}

		toAddress, err := decodeAddress(rawTransaction.ToAddress)
		if err != nil {
			return err
		}

		gas, err := hexStringToBigInt(rawTransaction.Gas)
		if err != nil {
			log.Printf("error parsing gas for transaction %s: %v", rawTransaction.Hash, err)
			return err
		}
		gasPrice, err := hexStringToBigInt(rawTransaction.GasPrice)
		if err != nil {
			log.Printf("error parsing gas price for transaction %s: %v", rawTransaction.Hash, err)
			return err
		}

		maxFeePerGas, err := hexStringToBigInt(rawTransaction.MaxFeePerGas)
		if err != nil {
			log.Printf("error parsing max fee per gas for transaction %s: %v", rawTransaction.Hash, err)
			return err
		}

		maxPriorityFeePerGas, err := hexStringToBigInt(rawTransaction.MaxPriorityFeePerGas)
		if err != nil {
			log.Printf("error parsing max priority fee per gas for transaction %s: %v", rawTransaction.Hash, err)
			return err
		}

		value, err := hexStringToBigInt(rawTransaction.Value)
		if err != nil {
			log.Printf("error parsing value for transaction %s: %v", rawTransaction.Hash, err)
			return err
		}

		updateValues(valuesMap, "hash", rawTransaction.Hash)
		updateValues(valuesMap, "block_hash", rawTransaction.BlockHash)
		updateValues(valuesMap, "block_timestamp", rawTransaction.BlockTimestamp)
		updateValues(valuesMap, "block_number", rawTransaction.BlockNumber)
		updateValues(valuesMap, "from_address", fromAddress)
		updateValues(valuesMap, "to_address", toAddress)
		updateValues(valuesMap, "gas", gas)
		updateValues(valuesMap, "gas_price", gasPrice)
		updateValues(valuesMap, "input", rawTransaction.Input)
		updateValues(valuesMap, "nonce", rawTransaction.Nonce)
		updateValues(valuesMap, "max_fee_per_gas", maxFeePerGas)
		updateValues(valuesMap, "max_priority_fee_per_gas", maxPriorityFeePerGas)
		updateValues(valuesMap, "transaction_index", rawTransaction.TransactionIndex)
		updateValues(valuesMap, "transaction_type", rawTransaction.TransactionType)
		updateValues(valuesMap, "value", value)
		if isBlockchainWithL1Chain {
			var l1Bn interface{}
			if rawTransaction.L1BlockNumber != nil {
				l1Bn = *rawTransaction.L1BlockNumber
			} else {
				l1Bn = nil
			}
			updateValues(valuesMap, "l1_block_number", l1Bn)
		}
	}

	ctx := context.Background()

	// Insert them in batch
	err := p.executeBatchInsert(tx, ctx, tableName, columns, valuesMap, "ON CONFLICT DO NOTHING")
	if err != nil {
		return err
	}

	log.Printf("Saved %d transactions records into %s table", len(rawTransactions), tableName)
	return nil
}

func (p *PostgreSQLpgx) CleanIndexes(blockchain string, batchLimit uint64, sleepTime int) error {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return err
	}

	defer conn.Release()

	// get max and min block number

	var minBlockNumber uint64
	var maxBlockNumber uint64

	txTableName, txTableErr := TransactionsTableName(blockchain)
	if txTableErr != nil {
		return txTableErr
	}

	query := fmt.Sprintf("SELECT min(block_number), max(block_number) FROM %s", txTableName)

	err = conn.QueryRow(context.Background(), query).Scan(&minBlockNumber, &maxBlockNumber)

	if err != nil {
		return err
	}

	// delete indexes in batches

	log.Printf("Starting deletion of transactions indexes in blocks range from %d to %d number", minBlockNumber, maxBlockNumber)

	for i := minBlockNumber; i <= maxBlockNumber; i += batchLimit {

		commandTag, err := conn.Exec(context.Background(), fmt.Sprintf("DELETE FROM %s WHERE block_number >= $1 AND block_number < $2", txTableName), i, i+batchLimit)
		if err != nil {
			return err
		}

		log.Println("Deleted", commandTag.RowsAffected(), "transactions indexes with corresponding logs")

		// sleep for a while to avoid overloading the database
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	return nil

}

func (p *PostgreSQLpgx) UpdateAbiJobsStatus(blockchain string) error {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()

	query := `
		UPDATE abi_jobs 
		SET historical_crawl_status = 'in_progress', moonworm_task_pickedup = true
		WHERE chain = @chain
		  AND historical_crawl_status = 'pending' 
		  AND status = 'active' 
		  AND deployment_block_number IS NOT NULL
	`

	queryArgs := pgx.NamedArgs{
		"chain": blockchain,
	}

	_, err = conn.Exec(context.Background(), query, queryArgs)
	if err != nil {
		return err
	}

	return nil
}

func (p *PostgreSQLpgx) SelectAbiJobs(blockchain string, addresses []string, customersIds []string, autoJobs, isDeployBlockNotNull bool, abiTypes []string) ([]AbiJob, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	var queryBuilder strings.Builder

	queryArgs := make(pgx.NamedArgs)

	queryBuilder.WriteString(`
		SELECT id, address, user_id, customer_id, abi_selector, chain, abi_name, status, 
		       historical_crawl_status, progress, moonworm_task_pickedup, '[' || abi || ']' as abi, 
		       (abi::jsonb)->>'type' AS abiType, created_at, updated_at, deployment_block_number
		FROM abi_jobs
		WHERE true
	`)

	if len(abiTypes) != 0 {
		var abiConditions []string
		for _, abiType := range abiTypes {
			abiConditions = append(abiConditions, fmt.Sprintf("(abi::jsonb)->>'type' = '%s'", abiType))
		}

		queryBuilder.WriteString(fmt.Sprintf("AND (%s) ", strings.Join(abiConditions, " or ")))
	}

	if isDeployBlockNotNull {
		queryBuilder.WriteString(" AND deployment_block_number IS NOT null")
	}

	if blockchain != "" {
		queryBuilder.WriteString(" AND chain = @chain ")
		queryArgs["chain"] = blockchain
	}

	if autoJobs {
		queryBuilder.WriteString(" AND historical_crawl_status != 'done' ")
	}

	if len(addresses) > 0 {
		queryBuilder.WriteString(" AND address = ANY(@addresses) ")

		// decode addresses
		addressesBytes := make([][]byte, len(addresses))
		for i, address := range addresses {
			addressBytes, err := decodeAddress(address)
			if err != nil {
				return nil, err
			}
			addressesBytes[i] = addressBytes // Assign directly to the index
		}

		queryArgs["addresses"] = addressesBytes
	}

	if len(customersIds) > 0 {
		queryBuilder.WriteString(" AND customer_id = ANY(@customer_ids) ")
		queryArgs["customer_ids"] = customersIds
	}

	rows, err := conn.Query(context.Background(), queryBuilder.String(), queryArgs)
	if err != nil {
		log.Println("Error querying ABI jobs from database", err)
		return nil, err
	}

	abiJobs, err := pgx.CollectRows(rows, pgx.RowToStructByName[AbiJob])
	if err != nil {
		log.Println("Error collecting ABI jobs rows", err)
		return nil, err
	}

	return abiJobs, nil
}

func GetJobIds(abiJobs []AbiJob, isSilent bool) []string {
	var jobIds []string
	abiJobChains := make(map[string]int)
	for _, abiJob := range abiJobs {
		jobIds = append(jobIds, abiJob.ID)
		if _, ok := abiJobChains[abiJob.Chain]; !ok {
			abiJobChains[abiJob.Chain] = 0
		}
		abiJobChains[abiJob.Chain]++
	}

	if !isSilent {
		fmt.Printf("Found %d total:\n", len(jobIds))
		for k, v := range abiJobChains {
			fmt.Printf("- %s - %d jobs\n", k, v)
		}
	}

	return jobIds
}

func (p *PostgreSQLpgx) CopyAbiJobs(sourceCustomerId, destCustomerId string, abiJobs []AbiJob) error {
	pool := p.GetPool()

	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, prepErr := tx.Prepare(ctx, "insertAbiJob", `
        INSERT INTO abi_jobs (id, address, user_id, customer_id, abi_selector, chain, abi_name, status, historical_crawl_status, progress, moonworm_task_pickedup, abi, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), now())
    `)
	if prepErr != nil {
		return err
	}

	for _, abiJob := range abiJobs {
		jobID := uuid.New()

		if len(abiJob.Abi) <= 2 || abiJob.Abi[0] != '[' || abiJob.Abi[len(abiJob.Abi)-1] != ']' {
			log.Printf("Passed ABI job, incorrect format: %s", abiJob.Abi)
			continue
		}
		abi := abiJob.Abi[1 : len(abiJob.Abi)-1]
		abiBytes := []byte(abi)

		_, execErr := tx.Exec(ctx, "insertAbiJob", jobID, abiJob.Address, abiJob.UserID, destCustomerId, abiJob.AbiSelector, abiJob.Chain, abiJob.AbiName, "true", "pending", 0, false, abiBytes)
		if execErr != nil {
			return execErr
		}
	}

	commitErr := tx.Commit(ctx)
	if commitErr != nil {
		return commitErr
	}

	log.Printf("Copied %d ABI jobs from customer %s to %s.", len(abiJobs), sourceCustomerId, destCustomerId)

	return nil
}

func ConvertToCustomerUpdatedAndDeployBlockDicts(abiJobs []AbiJob) ([]CustomerUpdates, map[string]AbiJobsDeployInfo, error) {
	if len(abiJobs) == 0 {
		return []CustomerUpdates{}, map[string]AbiJobsDeployInfo{}, nil
	}

	customerUpdatesDict := make(map[string]CustomerUpdates)
	addressDeployBlockDict := make(map[string]AbiJobsDeployInfo)

	for _, abiJob := range abiJobs {
		address := fmt.Sprintf("0x%x", abiJob.Address)

		if _, exists := customerUpdatesDict[abiJob.CustomerID]; !exists {
			customerUpdatesDict[abiJob.CustomerID] = CustomerUpdates{
				CustomerID: abiJob.CustomerID,
				Abis:       make(map[string]map[string]*AbiEntry),
			}
		}

		if _, exists := customerUpdatesDict[abiJob.CustomerID].Abis[address]; !exists {
			customerUpdatesDict[abiJob.CustomerID].Abis[address] = make(map[string]*AbiEntry)
		}

		customerUpdatesDict[abiJob.CustomerID].Abis[address][abiJob.AbiSelector] = &AbiEntry{
			AbiJSON: abiJob.Abi,
			AbiName: abiJob.AbiName,
			AbiType: abiJob.AbiType,
		}

		if abiJob.DeploymentBlockNumber == nil {
			value := uint64(1)
			abiJob.DeploymentBlockNumber = &value
		}

		// Retrieve the struct from the map
		deployInfo, exists := addressDeployBlockDict[address]

		if !exists {
			// Initialize the struct if it doesn't exist
			deployInfo = AbiJobsDeployInfo{
				DeployedBlockNumber: *abiJob.DeploymentBlockNumber,
				IDs:                 []string{},
			}
		}

		// Modify the struct
		deployInfo.IDs = append(deployInfo.IDs, abiJob.ID)

		// Store the modified struct back in the map
		addressDeployBlockDict[address] = deployInfo
	}

	var customerUpdates []CustomerUpdates
	for _, customerUpdate := range customerUpdatesDict {
		customerUpdates = append(customerUpdates, customerUpdate)
	}

	return customerUpdates, addressDeployBlockDict, nil
}

func (p *PostgreSQLpgx) UpdateAbisAsDone(ids []string) error {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()

	query := `
		UPDATE abi_jobs 
		SET historical_crawl_status = 'done', progress = 100
		WHERE id = ANY($1)
	`

	_, err = conn.Exec(context.Background(), query, ids)
	if err != nil {
		return err
	}

	return nil
}

func (p *PostgreSQLpgx) FindBatchPath(blockchain string, blockNumber uint64) (string, uint64, uint64, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return "", 0, 0, err
	}

	defer conn.Release()

	var path string

	var minBlockNumber uint64

	blocksTableName, blocksTableErr := BlocksTableName(blockchain)
	if blocksTableErr != nil {
		return "", 0, 0, blocksTableErr
	}

	var maxBlockNumber uint64
	query := fmt.Sprintf(`WITH path as (
        SELECT
            path
        from
            %s
        WHERE
            block_number = $1
    ) SELECT path, min(block_number), max(block_number) FROM %s WHERE path = (SELECT path from path) group by path`, blocksTableName, blocksTableName)

	err = conn.QueryRow(context.Background(), query, blockNumber).Scan(&path, &minBlockNumber, &maxBlockNumber)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Blocks not indexed yet
			return "", 0, 0, nil
		}
		return "",
			0,
			0,
			err
	}

	return path, minBlockNumber, maxBlockNumber, nil

}

func (p *PostgreSQLpgx) RetrievePathsAndBlockBounds(blockchain string, blockNumber uint64, minBlocksToSync int) ([]string, uint64, uint64, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return nil, 0, 0, err
	}

	defer conn.Release()

	var paths []string

	var minBlockNumber sql.NullInt64

	var maxBlockNumber sql.NullInt64

	blocksTableName, blocksTableErr := BlocksTableName(blockchain)
	if blocksTableErr != nil {
		return nil, 0, 0, blocksTableErr
	}

	query := fmt.Sprintf(`WITH path as (
        SELECT
            path,
			block_number
        from
            %s
        WHERE
            block_number >= $2 and block_number <= $1
    ), latest_block_of_path as (
		SELECT
			block_number as latest_block_number
		from
			%s
		WHERE
			path = (SELECT path FROM path ORDER BY block_number DESC LIMIT 1)
		order by block_number desc
		limit 1
	), earliest_block_of_path as (
		SELECT
			block_number as first_block_number
		from
			%s
		WHERE
			path = (SELECT path FROM path ORDER BY block_number ASC LIMIT 1)
		order by block_number asc
		limit 1
	)
	select  ARRAY_AGG( DISTINCT path) as paths, (SELECT first_block_number FROM earliest_block_of_path) as min_block_number, (SELECT latest_block_number FROM latest_block_of_path) as max_block_number from path
	`, blocksTableName, blocksTableName, blocksTableName)

	err = conn.QueryRow(context.Background(), query, blockNumber, blockNumber-uint64(minBlocksToSync)).Scan(&paths, &minBlockNumber, &maxBlockNumber)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Blocks not indexed yet
			return nil, 0, 0, nil
		}
		return nil,
			0,
			0,
			err
	}

	return paths, uint64(minBlockNumber.Int64), uint64(maxBlockNumber.Int64), nil

}

func (p *PostgreSQLpgx) GetAbiJobsWithoutDeployBlocks(blockchain string) (map[string]map[string][]string, error) {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return nil, err
	}

	defer conn.Release()

	/// get all addresses that not have deploy block number

	rows, err := conn.Query(context.Background(), `SELECT
		id,
		chain,
		address
	FROM
		abi_jobs
	WHERE
		deployment_block_number is null
		and chain = $1
		and (
			(abi :: jsonb) ->> 'type' = 'event'
			or (
				(abi :: jsonb) ->> 'type' = 'function'
				and (abi :: jsonb) ->> 'stateMutability' != 'view'
			)
		)`, blockchain)
	if err != nil {
		log.Println("Error querying abi jobs from database", err)
		return nil, err
	}

	// chain, address, ids
	chainsAddresses := make(map[string]map[string][]string)

	for rows.Next() {

		var id string
		var chain string
		var raw_address []byte
		var address string

		err = rows.Scan(&id, &chain, &raw_address)

		if err != nil {
			return nil, err
		}

		address = fmt.Sprintf("0x%x", raw_address)

		if _, exists := chainsAddresses[chain]; !exists {
			chainsAddresses[chain] = make(map[string][]string)
		}

		chainsAddresses[chain][address] = append(chainsAddresses[chain][address], id)

	}

	// Run ensure selector for each chain

	for chain, addressIds := range chainsAddresses {

		for address := range addressIds {

			err := p.EnsureCorrectSelectors(chain, true, "", addressIds[address])
			if err != nil {

				log.Println("Error ensuring correct selectors for chain:", chain, err)
				return nil, err
			}
		}

	}

	return chainsAddresses, nil
}

func (p *PostgreSQLpgx) UpdateAbisProgress(ids []string, process int) error {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return err
	}

	defer conn.Release()

	// Transform the ids to a slice of UUIDs
	idsUUID := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		idsUUID[i], err = uuid.Parse(id)
		if err != nil {
			return err
		}
	}

	_, err = conn.Exec(context.Background(), "UPDATE abi_jobs SET progress=$1 WHERE id=ANY($2)", process, idsUUID)

	if err != nil {
		return err
	}

	return nil

}

func (p *PostgreSQLpgx) UpdateAbiJobsDeployBlock(blockNumber uint64, ids []string) error {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())

	if err != nil {
		return err
	}

	defer conn.Release()

	// Transform the ids to a slice of UUIDs
	idsUUID := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		idsUUID[i], err = uuid.Parse(id)
		if err != nil {
			return err
		}
	}

	_, err = conn.Exec(context.Background(), "UPDATE abi_jobs SET deployment_block_number=$1 WHERE id=ANY($2)", blockNumber, idsUUID)

	if err != nil {
		return err
	}

	return nil

}

func (p *PostgreSQLpgx) CreateJobsFromAbi(chain string, address string, abiFile string, customerID string, userID string, deployBlock uint64) error {
	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()

	abiData, err := ioutil.ReadFile(abiFile)
	if err != nil {
		return err
	}

	var abiJson []map[string]interface{}
	err = json.Unmarshal(abiData, &abiJson)
	if err != nil {
		return err
	}

	for _, abiJob := range abiJson {

		// Generate a new UUID for the id column
		jobID := uuid.New()

		abiJobJson, err := json.Marshal(abiJob)
		if err != nil {
			log.Println("Error marshalling ABI job to JSON:", abiJob, err)
			return err
		}

		// Wrap the JSON string in an array
		abiJsonArray := "[" + string(abiJobJson) + "]"

		// Get the correct selector for the ABI
		abiObj, err := abi.JSON(strings.NewReader(abiJsonArray))
		if err != nil {
			log.Println("Error parsing ABI for ABI job:", abiJsonArray, err)
			return err
		}
		var selector string

		if abiJob["type"] == "event" {
			selector = abiObj.Events[abiJob["name"].(string)].ID.String()
		} else if abiJob["type"] == "function" {
			selectorRaw := abiObj.Methods[abiJob["name"].(string)].ID
			selector = fmt.Sprintf("0x%x", selectorRaw)
		} else {
			log.Println("ABI type not supported:", abiJob["type"])
			continue
		}

		addressBytes, err := decodeAddress(address)

		if err != nil {
			log.Println("Error decoding address:", err, address)
			continue
		}

		_, err = conn.Exec(context.Background(), "INSERT INTO abi_jobs (id, address, user_id, customer_id, abi_selector, chain, abi_name, status, historical_crawl_status, progress, moonworm_task_pickedup, abi, deployment_block_number, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now(), now()) ON CONFLICT DO NOTHING", jobID, addressBytes, userID, customerID, selector, chain, abiJob["name"], "true", "pending", 0, false, abiJobJson, deployBlock)

		if err != nil {
			return err
		}

	}

	return nil

}

func (p *PostgreSQLpgx) DeleteJobs(jobIds []string) error {
	if len(jobIds) == 0 {
		log.Println("Nothing to delete")
		return nil
	}

	pool := p.GetPool()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()

	var queryBuilder strings.Builder
	queryBuilder.WriteString("DELETE FROM abi_jobs WHERE id = ANY(@jobIds)")

	queryArgs := make(pgx.NamedArgs)
	queryArgs["jobIds"] = jobIds

	_, delErr := conn.Query(context.Background(), queryBuilder.String(), queryArgs)
	if delErr != nil {
		log.Printf("Error querying ABI jobs from database, err %v", delErr)
		return delErr
	}

	log.Printf("Deleted %d jobs", len(jobIds))

	return nil
}
