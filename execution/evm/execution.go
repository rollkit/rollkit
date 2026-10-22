package evm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang-jwt/jwt/v5"
	ds "github.com/ipfs/go-datastore"
	"github.com/rs/zerolog"

	"github.com/evstack/ev-node/core/execution"
	"github.com/evstack/ev-node/pkg/telemetry"
)

const (
	// MaxPayloadStatusRetries is the maximum number of retries for SYNCING status.
	// According to the Engine API specification, SYNCING indicates temporary unavailability
	// and should be retried with exponential backoff.
	MaxPayloadStatusRetries = 3
	// InitialRetryBackoff is the initial backoff duration for retries.
	// The backoff doubles on each retry attempt (exponential backoff).
	InitialRetryBackoff = 1 * time.Second
	// SafeBlockLag is the number of blocks the safe block lags behind the head.
	// This provides a buffer for reorg protection - safe blocks won't be reorged
	// under normal operation. A value of 2 means when head is at block N,
	// safe is at block N-2.
	SafeBlockLag = 2
	// FinalizedBlockLag is the number of blocks the finalized block lags behind head.
	// This is a temporary mock value until proper DA-based finalization is wired up.
	// A value of 3 means when head is at block N, finalized is at block N-3.
	FinalizedBlockLag = 3
)

var (
	// ErrInvalidPayloadStatus indicates that the execution engine returned a permanent
	// failure status (INVALID or unknown status). This error should not be retried.
	ErrInvalidPayloadStatus = errors.New("invalid payload status")
	// ErrPayloadSyncing indicates that the execution engine is temporarily syncing.
	// According to the Engine API specification, this is a transient condition that
	// should be handled with retry logic rather than immediate failure.
	ErrPayloadSyncing = errors.New("payload syncing")
)

// Ensure EngineAPIExecutionClient implements the execution.Execute interface
var _ execution.Executor = (*EngineClient)(nil)

// Ensure EngineClient implements the execution.HeightProvider interface
var _ execution.HeightProvider = (*EngineClient)(nil)

// Ensure EngineClient implements the execution.Rollbackable interface
var _ execution.Rollbackable = (*EngineClient)(nil)

// Ensure EngineClient implements optional pruning interface when used with
// ev-node's height-based pruning.
var _ execution.ExecPruner = (*EngineClient)(nil)

// validatePayloadStatus checks the payload status and returns appropriate errors.
// It implements the Engine API specification's status handling:
//   - VALID: Operation succeeded, return nil
//   - SYNCING/ACCEPTED: Temporary unavailability, return ErrPayloadSyncing for retry
//   - INVALID: Permanent failure, return ErrInvalidPayloadStatus (no retry)
//   - Unknown: Treat as permanent failure (no retry)
func validatePayloadStatus(status engine.PayloadStatusV1) error {
	switch status.Status {
	case engine.VALID:
		return nil
	case engine.SYNCING, engine.ACCEPTED:
		// SYNCING and ACCEPTED indicate temporary unavailability - should retry
		return ErrPayloadSyncing
	case engine.INVALID:
		// INVALID is a permanent failure - should not retry
		return ErrInvalidPayloadStatus
	default:
		// Unknown status - treat as invalid
		return ErrInvalidPayloadStatus
	}
}

func latestValidHashHex(latestValidHash *common.Hash) string {
	if latestValidHash == nil {
		return ""
	}
	return latestValidHash.Hex()
}

// retryWithBackoffOnPayloadStatus executes a function with exponential backoff retry logic.
// It implements the Engine API specification's recommendation to retry SYNCING
// status with exponential backoff. The function:
//   - Retries only on ErrPayloadSyncing (transient failures)
//   - Fails immediately on ErrInvalidPayloadStatus (permanent failures)
//   - Respects context cancellation for graceful shutdown
//   - Uses exponential backoff that doubles on each attempt
func retryWithBackoffOnPayloadStatus(ctx context.Context, fn func() error, maxRetries int, initialBackoff time.Duration, operation string) error {
	backoff := initialBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		// Don't retry on invalid status
		if errors.Is(err, ErrInvalidPayloadStatus) {
			return err
		}

		// Only retry on syncing status
		if !errors.Is(err, ErrPayloadSyncing) {
			return err
		}

		// Check if we've exhausted retries
		if attempt >= maxRetries {
			return fmt.Errorf("max retries (%d) exceeded for %s: %w", maxRetries, operation, err)
		}

		// Wait with exponential backoff
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry for %s: %w", operation, ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return fmt.Errorf("max retries (%d) exceeded for %s", maxRetries, operation)
}

// EngineRPCClient abstracts Engine API RPC calls for tracing and testing.
type EngineRPCClient interface {
	// ForkchoiceUpdated updates the forkchoice state and optionally starts payload building.
	ForkchoiceUpdated(ctx context.Context, state engine.ForkchoiceStateV1, args map[string]any) (*engine.ForkChoiceResponse, error)

	// GetPayload retrieves a previously requested execution payload.
	GetPayload(ctx context.Context, payloadID engine.PayloadID) (*EnginePayloadEnvelope, error)

	// NewPayload submits a new execution payload for validation.
	NewPayload(ctx context.Context, payload *EnginePayloadEnvelope, blobHashes []string, parentBeaconBlockRoot string, executionRequests [][]byte) (*engine.PayloadStatusV1, error)
}

// EthRPCClient abstracts Ethereum JSON-RPC calls for tracing and testing.
type EthRPCClient interface {
	// HeaderByNumber retrieves a block header by number (nil = latest).
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)

	// GetTxs retrieves pending transactions from the transaction pool.
	GetTxs(ctx context.Context) ([]string, error)

	// GetNextProposer retrieves the proposer selected by the execution layer.
	GetNextProposer(ctx context.Context, number *big.Int) (common.Hash, error)
}

// EngineClient represents a client that interacts with an Ethereum execution engine
// through the Engine API. It manages connections to both the engine and standard Ethereum
// APIs, and maintains state related to block processing.
type EngineClient struct {
	engineClient  EngineRPCClient // Client for Engine API calls
	ethClient     EthRPCClient    // Client for standard Ethereum API calls
	genesisHash   common.Hash     // Hash of the genesis block
	initialHeight uint64
	feeRecipient  common.Address // Address to receive transaction fees

	// store provides persistence for ExecMeta to enable idempotent execution
	// and crash recovery.
	store *EVMStore

	mu                        sync.Mutex             // Mutex to protect concurrent access to block hashes
	currentHeadBlockHash      common.Hash            // Store last non-finalized HeadBlockHash
	currentHeadHeight         uint64                 // Height of the current head block (for safe lag calculation)
	currentSafeBlockHash      common.Hash            // Store last non-finalized SafeBlockHash
	currentFinalizedBlockHash common.Hash            // Store last finalized block hash
	blockHashCache            map[uint64]common.Hash // height -> hash cache for safe block lookups
	forkchoiceInitialized     bool                   // Whether forkchoice state was restored from the execution layer

	cachedExecutionInfo atomic.Pointer[execution.ExecutionInfo] // Cached execution info (gas limit)

	logger zerolog.Logger
}

// NewEngineExecutionClient creates a new instance of EngineAPIExecutionClient.
// The db parameter is required for ExecMeta tracking which enables idempotent
// execution and crash recovery. The db is wrapped with a prefix to isolate
// EVM execution data from other ev-node data.
// When tracingEnabled is true, the client will inject W3C trace context headers
// and wrap Engine API and Eth API calls with OpenTelemetry spans.
func NewEngineExecutionClient(
	ethURL,
	engineURL string,
	jwtSecret string,
	genesisHash common.Hash,
	feeRecipient common.Address,
	db ds.Batching,
	tracingEnabled bool,
	logger zerolog.Logger,
) (*EngineClient, error) {
	if db == nil {
		return nil, errors.New("db is required for EVM execution client")
	}

	var rpcOpts []rpc.ClientOption
	// If tracing enabled, add W3C header propagation to rpcOpts
	if tracingEnabled {
		rpcOpts = append(rpcOpts, rpc.WithHTTPClient(
			telemetry.NewPropagatingHTTPClient(http.DefaultTransport)))
	}

	// Create ETH RPC client with HTTP options
	ethRPC, err := rpc.DialOptions(context.Background(), ethURL, rpcOpts...)
	if err != nil {
		return nil, err
	}
	rawEthClient := ethclient.NewClient(ethRPC)

	secret, err := decodeSecret(jwtSecret)
	if err != nil {
		return nil, err
	}

	// Create Engine RPC with optional HTTP client and JWT auth
	// Compose engine options: pass-through rpcOpts plus JWT auth
	engineOptions := make([]rpc.ClientOption, len(rpcOpts))
	copy(engineOptions, rpcOpts) // copy to avoid using same backing array from rpcOpts.
	engineOptions = append(engineOptions, rpc.WithHTTPAuth(func(h http.Header) error {
		authToken, err := getAuthToken(secret)
		if err != nil {
			return err
		}
		if authToken != "" {
			h.Set("Authorization", "Bearer "+authToken)
		}
		return nil
	}))
	rawEngineClient, err := rpc.DialOptions(context.Background(), engineURL, engineOptions...)
	if err != nil {
		return nil, err
	}

	// wrap raw clients with interfaces
	engineClient := NewEngineRPCClient(rawEngineClient)
	ethClient := NewEthRPCClient(rawEthClient)

	// if tracing enabled, wrap with traced decorators
	if tracingEnabled {
		engineClient = withTracingEngineRPCClient(engineClient)
		ethClient = withTracingEthRPCClient(ethClient)
	}

	return &EngineClient{
		engineClient:              engineClient,
		ethClient:                 ethClient,
		genesisHash:               genesisHash,
		feeRecipient:              feeRecipient,
		store:                     NewEVMStore(db),
		currentHeadBlockHash:      genesisHash,
		currentSafeBlockHash:      genesisHash,
		currentFinalizedBlockHash: genesisHash,
		blockHashCache:            make(map[uint64]common.Hash),
		logger:                    logger.With().Str("module", "engine_client").Logger(),
	}, nil
}

// InitChain initializes the blockchain with the given genesis parameters
func (c *EngineClient) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, error) {
	if initialHeight != 1 {
		return nil, fmt.Errorf("initialHeight must be 1, got %d", initialHeight)
	}

	// Acknowledge the genesis block with retry logic for SYNCING status
	err := retryWithBackoffOnPayloadStatus(ctx, func() error {
		forkchoiceResult, err := c.engineClient.ForkchoiceUpdated(ctx,
			engine.ForkchoiceStateV1{
				HeadBlockHash:      c.genesisHash,
				SafeBlockHash:      c.genesisHash,
				FinalizedBlockHash: c.genesisHash,
			},
			nil,
		)
		if err != nil {
			return fmt.Errorf("forkchoice update failed: %w", err)
		}

		// Validate payload status
		if err := validatePayloadStatus(forkchoiceResult.PayloadStatus); err != nil {
			c.logger.Warn().
				Str("status", forkchoiceResult.PayloadStatus.Status).
				Str("latestValidHash", latestValidHashHex(forkchoiceResult.PayloadStatus.LatestValidHash)).
				Interface("validationError", forkchoiceResult.PayloadStatus.ValidationError).
				Msg("InitChain: forkchoice update returned non-VALID status")
			return err
		}

		return nil
	}, MaxPayloadStatusRetries, InitialRetryBackoff, "InitChain")
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.currentHeadBlockHash = c.genesisHash
	c.currentHeadHeight = 0
	c.currentSafeBlockHash = c.genesisHash
	c.currentFinalizedBlockHash = c.genesisHash
	if c.blockHashCache == nil {
		c.blockHashCache = make(map[uint64]common.Hash)
	}
	c.blockHashCache[0] = c.genesisHash
	c.forkchoiceInitialized = true
	c.mu.Unlock()

	_, stateRoot, _, _, err := c.getBlockInfo(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get block info: %w", err)
	}

	c.initialHeight = initialHeight

	return stateRoot[:], nil
}

// GetTxs retrieves transactions from the current execution payload
func (c *EngineClient) GetTxs(ctx context.Context) ([][]byte, error) {
	result, err := c.ethClient.GetTxs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tx pool content: %w", err)
	}

	txs := make([][]byte, 0, len(result))
	for _, rlpHex := range result {
		if !strings.HasPrefix(rlpHex, "0x") || len(rlpHex) < 3 {
			return nil, fmt.Errorf("invalid hex format for transaction: %s", rlpHex)
		}
		txBytes := common.FromHex(rlpHex)
		if len(txBytes) == 0 && len(rlpHex) > 2 {
			return nil, fmt.Errorf("failed to decode hex transaction: %s", rlpHex)
		}
		txs = append(txs, txBytes)
	}

	return txs, nil
}

// ensureForkchoiceInitialized restores the forkchoice state persisted by the
// execution layer. Initialization is serialized and remains retryable until all
// required block tags have been read and validated successfully.
func (c *EngineClient) ensureForkchoiceInitialized(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.forkchoiceInitialized {
		return nil
	}

	latest, latestErr := c.headerByBlockTag(ctx, rpc.LatestBlockNumber)
	safe, safeErr := c.headerByBlockTag(ctx, rpc.SafeBlockNumber)
	finalized, finalizedErr := c.headerByBlockTag(ctx, rpc.FinalizedBlockNumber)
	if latestErr != nil {
		return latestErr
	}

	headHeight := latest.Number.Uint64()
	if headHeight == 0 {
		if latest.Hash() != c.genesisHash {
			return fmt.Errorf("latest genesis hash %s does not match configured genesis hash %s", latest.Hash(), c.genesisHash)
		}

		// Some execution clients do not expose safe/finalized tags until their
		// first forkchoice update. A genesis-only chain has no state to recover
		// beyond the configured genesis hash.
		c.currentHeadBlockHash = c.genesisHash
		c.currentHeadHeight = 0
		c.currentSafeBlockHash = c.genesisHash
		c.currentFinalizedBlockHash = c.genesisHash
		if c.blockHashCache == nil {
			c.blockHashCache = make(map[uint64]common.Hash)
		}
		c.blockHashCache[0] = c.genesisHash
		c.forkchoiceInitialized = true
		return nil
	}

	if safeErr != nil {
		return safeErr
	}
	if finalizedErr != nil {
		return finalizedErr
	}

	safeHeight := safe.Number.Uint64()
	finalizedHeight := finalized.Number.Uint64()
	if finalizedHeight > safeHeight || safeHeight > headHeight {
		return fmt.Errorf("inconsistent forkchoice heights: finalized=%d safe=%d head=%d", finalizedHeight, safeHeight, headHeight)
	}
	if safeHeight == headHeight && safe.Hash() != latest.Hash() {
		return fmt.Errorf("inconsistent forkchoice hashes at head height %d", headHeight)
	}
	if finalizedHeight == safeHeight && finalized.Hash() != safe.Hash() {
		return fmt.Errorf("inconsistent forkchoice hashes at safe height %d", safeHeight)
	}

	c.currentHeadBlockHash = latest.Hash()
	c.currentHeadHeight = headHeight
	c.currentSafeBlockHash = safe.Hash()
	c.currentFinalizedBlockHash = finalized.Hash()
	if c.blockHashCache == nil {
		c.blockHashCache = make(map[uint64]common.Hash)
	}
	c.blockHashCache[headHeight] = latest.Hash()
	c.blockHashCache[safeHeight] = safe.Hash()
	c.blockHashCache[finalizedHeight] = finalized.Hash()
	c.forkchoiceInitialized = true

	c.logger.Info().
		Uint64("head_height", headHeight).
		Uint64("safe_height", safeHeight).
		Uint64("finalized_height", finalizedHeight).
		Msg("restored forkchoice state from execution layer")

	return nil
}

func (c *EngineClient) headerByBlockTag(ctx context.Context, tag rpc.BlockNumber) (*types.Header, error) {
	header, err := c.ethClient.HeaderByNumber(ctx, big.NewInt(tag.Int64()))
	if err != nil {
		return nil, fmt.Errorf("failed to get %s block: %w", tag.String(), err)
	}
	if header == nil || header.Number == nil {
		return nil, fmt.Errorf("execution layer returned missing %s block", tag.String())
	}
	return header, nil
}

// ExecuteTxs executes the given transactions at the specified block height and timestamp.
//
// ExecMeta tracking (if store is configured):
// - Checks for already-promoted blocks to enable idempotent execution
// - Saves ExecMeta with the payload ID returned by forkchoiceUpdated for crash recovery
// - Updates ExecMeta to "promoted" after successful execution
func (c *EngineClient) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (execution.ExecuteResult, error) {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return execution.ExecuteResult{}, fmt.Errorf("initialize forkchoice state: %w", err)
	}

	// 1. Check for idempotent execution
	stateRoot, payloadID, found, idempotencyErr := c.reconcileExecutionAtHeight(ctx, blockHeight, timestamp, txs)
	if idempotencyErr != nil {
		c.logger.Warn().Err(idempotencyErr).Uint64("height", blockHeight).Msg("ExecuteTxs: idempotency check failed")
		// Continue execution on error, as it might be transient
	} else if found {
		if stateRoot != nil {
			return c.executionResultAfterBlock(ctx, stateRoot, blockHeight)
		}
		if payloadID != nil {
			// Found in-progress execution, attempt to resume
			stateRoot, err := c.processPayload(ctx, *payloadID, txs)
			if err != nil {
				return execution.ExecuteResult{}, err
			}
			return c.executionResultAfterBlock(ctx, stateRoot, blockHeight)
		}
	}

	prevBlockHash, prevHeaderStateRoot, prevGasLimit, _, err := c.getBlockInfo(ctx, blockHeight-1)
	if err != nil {
		return execution.ExecuteResult{}, fmt.Errorf("failed to get block info: %w", err)
	}
	// It's possible that the prev state root passed in is nil if this is the first block.
	// If so, we can't do a comparison. Otherwise, we compare the roots.
	if len(prevStateRoot) > 0 && !bytes.Equal(prevStateRoot, prevHeaderStateRoot.Bytes()) {
		return execution.ExecuteResult{}, fmt.Errorf("prevStateRoot mismatch at height %d: consensus=%x execution=%x", blockHeight-1, prevStateRoot, prevHeaderStateRoot.Bytes())
	}

	// 2. Prepare payload attributes
	txsPayload := c.filterTransactions(txs)

	// Cache parent block hash for safe-block lookups.
	c.cacheBlockHash(blockHeight-1, prevBlockHash)

	// Use tracked safe/finalized state rather than prevBlockHash to avoid
	// regressing these values. Head must be prevBlockHash to build on top of it.
	c.mu.Lock()
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      prevBlockHash,
		SafeBlockHash:      c.currentSafeBlockHash,
		FinalizedBlockHash: c.currentFinalizedBlockHash,
	}
	c.mu.Unlock()

	// update forkchoice to get the next payload id
	evPayloadAttrs := c.buildPayloadAttributes(txsPayload, blockHeight, timestamp, prevGasLimit)

	c.logger.Debug().
		Uint64("height", blockHeight).
		Int("tx_count", len(txs)).
		Msg("engine_forkchoiceUpdated")

	// 3. Call forkchoice update to get PayloadID
	var newPayloadID *engine.PayloadID
	err = retryWithBackoffOnPayloadStatus(ctx, func() error {
		forkchoiceResult, err := c.engineClient.ForkchoiceUpdated(ctx, args, evPayloadAttrs)
		if err != nil {
			return fmt.Errorf("forkchoice update failed: %w", err)
		}

		// Validate payload status
		if err := validatePayloadStatus(forkchoiceResult.PayloadStatus); err != nil {
			c.logger.Warn().
				Str("status", forkchoiceResult.PayloadStatus.Status).
				Str("latestValidHash", latestValidHashHex(forkchoiceResult.PayloadStatus.LatestValidHash)).
				Interface("validationError", forkchoiceResult.PayloadStatus.ValidationError).
				Uint64("blockHeight", blockHeight).
				Msg("ExecuteTxs: forkchoice update returned non-VALID status")
			return err
		}

		if forkchoiceResult.PayloadID == nil {
			c.logger.Error().
				Str("status", forkchoiceResult.PayloadStatus.Status).
				Str("latestValidHash", latestValidHashHex(forkchoiceResult.PayloadStatus.LatestValidHash)).
				Interface("validationError", forkchoiceResult.PayloadStatus.ValidationError).
				Interface("forkchoiceState", args).
				Interface("payloadAttributes", evPayloadAttrs).
				Uint64("blockHeight", blockHeight).
				Msg("returned nil PayloadID")

			return fmt.Errorf("returned nil PayloadID - (status: %s, latestValidHash: %s)",
				forkchoiceResult.PayloadStatus.Status,
				latestValidHashHex(forkchoiceResult.PayloadStatus.LatestValidHash))
		}

		newPayloadID = forkchoiceResult.PayloadID
		return nil
	}, MaxPayloadStatusRetries, InitialRetryBackoff, "ExecuteTxs forkchoice")
	if err != nil {
		return execution.ExecuteResult{}, err
	}

	// Save ExecMeta with payloadID for crash recovery (Stage="started")
	// This allows resuming the payload build if we crash before completing
	c.saveExecMeta(ctx, blockHeight, timestamp.Unix(), newPayloadID[:], nil, nil, txs, ExecStageStarted)

	// 4. Process the payload (get, submit, finalize)
	stateRoot, err = c.processPayload(ctx, *newPayloadID, txs)
	if err != nil {
		return execution.ExecuteResult{}, err
	}
	return c.executionResultAfterBlock(ctx, stateRoot, blockHeight)
}

// setHead updates the head block hash without changing safe or finalized.
// This is used when reusing an existing block (idempotency check).
func (c *EngineClient) setHead(ctx context.Context, blockHash common.Hash) error {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return fmt.Errorf("initialize forkchoice state: %w", err)
	}

	c.mu.Lock()
	c.currentHeadBlockHash = blockHash
	// Note: safe and finalized are NOT updated - they advance separately via derivation
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      c.currentHeadBlockHash,
		SafeBlockHash:      c.currentSafeBlockHash,
		FinalizedBlockHash: c.currentFinalizedBlockHash,
	}
	c.mu.Unlock()

	return c.doForkchoiceUpdate(ctx, args, "setHead")
}

func (c *EngineClient) setFinal(ctx context.Context, blockHash common.Hash, isFinal bool) error {
	return c.setFinalWithHeight(ctx, blockHash, 0, isFinal)
}

// setFinalWithHeight updates forkchoice state with safe and finalized block lagging.
// When isFinal=false:
//   - Safe block is set to headHeight - SafeBlockLag (when headHeight > SafeBlockLag)
//   - Finalized block is set to headHeight - FinalizedBlockLag (when headHeight > FinalizedBlockLag)
//
// Note: The finalized lag is a temporary mock until proper DA-based finalization is wired up.
func (c *EngineClient) setFinalWithHeight(ctx context.Context, blockHash common.Hash, headHeight uint64, isFinal bool) error {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return fmt.Errorf("initialize forkchoice state: %w", err)
	}

	var safeHash, finalizedHash common.Hash
	updateSafe := !isFinal && headHeight > SafeBlockLag
	updateFinalized := !isFinal && headHeight > FinalizedBlockLag

	// Look up safe block hash
	if updateSafe {
		safeHeight := headHeight - SafeBlockLag

		c.mu.Lock()
		cachedSafeHash, ok := c.blockHashCache[safeHeight]
		c.mu.Unlock()
		if ok {
			safeHash = cachedSafeHash
		} else {
			var err error
			safeHash, _, _, _, err = c.getBlockInfo(ctx, safeHeight)
			if err != nil {
				c.logger.Debug().
					Uint64("safeHeight", safeHeight).
					Err(err).
					Msg("setFinalWithHeight: safe block not found, skipping safe update")
				updateSafe = false
			}
		}
	}

	// Look up finalized block hash
	if updateFinalized {
		finalizedHeight := headHeight - FinalizedBlockLag

		c.mu.Lock()
		cachedFinalizedHash, ok := c.blockHashCache[finalizedHeight]
		c.mu.Unlock()
		if ok {
			finalizedHash = cachedFinalizedHash
		} else {
			var err error
			finalizedHash, _, _, _, err = c.getBlockInfo(ctx, finalizedHeight)
			if err != nil {
				c.logger.Debug().
					Uint64("finalizedHeight", finalizedHeight).
					Err(err).
					Msg("setFinalWithHeight: finalized block not found, skipping finalized update")
				updateFinalized = false
			}
		}
	}

	c.mu.Lock()
	if isFinal {
		c.currentFinalizedBlockHash = blockHash
		c.currentSafeBlockHash = blockHash
	} else {
		c.currentHeadBlockHash = blockHash
		c.currentHeadHeight = headHeight
		if updateSafe {
			c.currentSafeBlockHash = safeHash
		}
		if updateFinalized {
			c.currentFinalizedBlockHash = finalizedHash
		}
	}
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      c.currentHeadBlockHash,
		SafeBlockHash:      c.currentSafeBlockHash,
		FinalizedBlockHash: c.currentFinalizedBlockHash,
	}
	c.mu.Unlock()

	return c.doForkchoiceUpdate(ctx, args, "setFinal")
}

// doForkchoiceUpdate performs the actual forkchoice update RPC call with retry logic.
func (c *EngineClient) doForkchoiceUpdate(ctx context.Context, args engine.ForkchoiceStateV1, operation string) error {

	// Call forkchoice update with retry logic for SYNCING status
	err := retryWithBackoffOnPayloadStatus(ctx, func() error {
		forkchoiceResult, err := c.engineClient.ForkchoiceUpdated(ctx, args, nil)
		if err != nil {
			return fmt.Errorf("forkchoice update failed: %w", err)
		}

		// Validate payload status
		if err := validatePayloadStatus(forkchoiceResult.PayloadStatus); err != nil {
			c.logger.Warn().
				Str("status", forkchoiceResult.PayloadStatus.Status).
				Str("latestValidHash", latestValidHashHex(forkchoiceResult.PayloadStatus.LatestValidHash)).
				Interface("validationError", forkchoiceResult.PayloadStatus.ValidationError).
				Str("operation", operation).
				Msg("forkchoice update returned non-VALID status")
			return err
		}
		return nil
	}, MaxPayloadStatusRetries, InitialRetryBackoff, operation)
	if err != nil {
		return err
	}

	return nil
}

// SetFinal marks the block at the given height as finalized.
//
// When DA inclusion lags far behind the head, the requested height may no
// longer be served by the execution layer (pruned history). If the lookup
// reports the block as pruned or not found and the EL head is at or above the
// height, the update is skipped: a later height will move the finalized label
// forward. Any other lookup failure (e.g. transient RPC errors), lookups above
// the head, or an unreadable head are returned as errors so the caller retries.
func (c *EngineClient) SetFinal(ctx context.Context, blockHeight uint64) error {
	blockHash, _, _, _, err := c.getBlockInfo(ctx, blockHeight)
	if err == nil {
		return c.setFinal(ctx, blockHash, true)
	}
	if !isHistoryUnavailable(err) {
		return fmt.Errorf("failed to get block info: %w", err)
	}

	head, headErr := c.ethClient.HeaderByNumber(ctx, nil)
	if headErr != nil || head == nil || head.Number == nil || !head.Number.IsUint64() || head.Number.Uint64() < blockHeight {
		return fmt.Errorf("failed to get block info: %w", err)
	}

	c.logger.Warn().
		Uint64("height", blockHeight).
		Uint64("el_head", head.Number.Uint64()).
		Err(err).
		Msg("SetFinal: historical block unavailable on execution layer, skipping finalized update")
	return nil
}

// prunedHistoryErrorCode is the JSON-RPC error code execution clients (geth,
// reth) return when a request touches pruned history (EIP-4444).
const prunedHistoryErrorCode = 4444

// isHistoryUnavailable reports whether err means the execution layer no longer
// serves the requested block, as opposed to a transient failure.
func isHistoryUnavailable(err error) bool {
	if errors.Is(err, ethereum.NotFound) {
		return true
	}
	var rpcErr rpc.Error
	return errors.As(err, &rpcErr) && rpcErr.ErrorCode() == prunedHistoryErrorCode
}

// SetSafe explicitly sets the safe block hash.
// This allows the derivation layer to advance the safe block independently of head.
// Safe indicates a block that is unlikely to be reorged (e.g., confirmed by DA).
func (c *EngineClient) SetSafe(ctx context.Context, blockHash common.Hash) error {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return fmt.Errorf("initialize forkchoice state: %w", err)
	}

	c.mu.Lock()
	c.currentSafeBlockHash = blockHash
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      c.currentHeadBlockHash,
		SafeBlockHash:      c.currentSafeBlockHash,
		FinalizedBlockHash: c.currentFinalizedBlockHash,
	}
	c.mu.Unlock()

	return c.doForkchoiceUpdate(ctx, args, "SetSafe")
}

// SetSafeByHeight sets the safe block by looking up the block hash at the given height.
// Uses cached block hashes when available to avoid RPC calls. Falls back to RPC on cache miss
// (e.g., during restart before cache is warmed).
// Returns nil if the height doesn't exist yet (block not produced).
func (c *EngineClient) SetSafeByHeight(ctx context.Context, height uint64) error {
	// Try cache first (avoids RPC call in normal operation)
	c.mu.Lock()
	blockHash, ok := c.blockHashCache[height]
	c.mu.Unlock()

	if !ok {
		// Cache miss - fallback to RPC (happens during restart/recovery)
		var err error
		blockHash, _, _, _, err = c.getBlockInfo(ctx, height)
		if err != nil {
			// Block doesn't exist yet - this is expected during early blocks
			c.logger.Debug().
				Uint64("height", height).
				Err(err).
				Msg("SetSafeByHeight: block not found, skipping safe update")
			return nil
		}
	}
	return c.SetSafe(ctx, blockHash)
}

// cacheBlockHash stores a block hash in the cache for later safe block lookups.
// The cache is bounded to prevent unbounded memory growth - old entries are pruned.
func (c *EngineClient) cacheBlockHash(height uint64, hash common.Hash) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.blockHashCache[height] = hash

	// Prune old entries to keep cache bounded (keep last ~10 blocks)
	// This is sufficient since SafeBlockLag is only 2
	const maxCacheSize = 10
	if len(c.blockHashCache) > maxCacheSize {
		var minHeight uint64
		if height >= maxCacheSize-1 {
			minHeight = height - (maxCacheSize - 1)
		}
		for h := range c.blockHashCache {
			if h < minHeight {
				delete(c.blockHashCache, h)
			}
		}
	}
}

// SetFinalized explicitly sets the finalized block hash.
// This allows the derivation layer to advance finalization independently.
// Finalized indicates a block that will never be reorged (e.g., included in DA with sufficient confirmations).
func (c *EngineClient) SetFinalized(ctx context.Context, blockHash common.Hash) error {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return fmt.Errorf("initialize forkchoice state: %w", err)
	}

	c.mu.Lock()
	c.currentFinalizedBlockHash = blockHash
	// Finalized implies safe.
	c.currentSafeBlockHash = blockHash
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      c.currentHeadBlockHash,
		SafeBlockHash:      c.currentSafeBlockHash,
		FinalizedBlockHash: c.currentFinalizedBlockHash,
	}
	c.mu.Unlock()

	return c.doForkchoiceUpdate(ctx, args, "SetFinalized")
}

// ResumePayload resumes an in-progress payload build using a stored payloadID.
// This is used for crash recovery when we have a payloadID but haven't yet
// retrieved and submitted the payload to the EL.
//
// Returns the state root from the payload, or an error if resumption fails.
// Implements the execution.PayloadResumer interface.
func (c *EngineClient) ResumePayload(ctx context.Context, payloadIDBytes []byte) (stateRoot []byte, err error) {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return nil, fmt.Errorf("initialize forkchoice state: %w", err)
	}

	// Convert bytes to PayloadID
	if len(payloadIDBytes) != 8 {
		return nil, fmt.Errorf("ResumePayload: invalid payloadID length %d, expected 8", len(payloadIDBytes))
	}
	var payloadID engine.PayloadID
	copy(payloadID[:], payloadIDBytes)

	c.logger.Info().
		Str("payloadID", payloadID.String()).
		Msg("ResumePayload: attempting to resume in-progress payload")

	stateRoot, err = c.processPayload(ctx, payloadID, nil) // txs = nil for resume
	return stateRoot, err
}

// reconcileExecutionAtHeight checks if the block at the given height and timestamp has already been executed.
// It returns:
// - stateRoot: non-nil if block is already promoted/finalized (idempotent success)
// - payloadID: non-nil if block execution was started but not finished (resume needed)
// - found: true if either of the above is true
// - err: error during checks
func (c *EngineClient) reconcileExecutionAtHeight(ctx context.Context, height uint64, timestamp time.Time, txs [][]byte) (stateRoot []byte, payloadID *engine.PayloadID, found bool, err error) {
	// 1. Check ExecMeta from store
	execMeta, err := c.store.GetExecMeta(ctx, height)
	if err == nil && execMeta != nil {
		// If we already have a promoted block at this height, verify timestamp matches
		// to catch Dual-Store Conflicts where ExecMeta was saved for an old block
		// that was later replaced via consensus.
		if execMeta.Stage == ExecStagePromoted && len(execMeta.StateRoot) > 0 {
			if execMeta.Timestamp == timestamp.Unix() {
				// Verify the block actually exists in the EL before trusting ExecMeta.
				// ExecMeta could be stale if ev-reth crashed/restarted after we saved it
				// but before the block was persisted on the EL side.
				existingBlockHash, existingStateRoot, _, existingTimestamp, elErr := c.getBlockInfo(ctx, height)
				if elErr == nil && existingBlockHash != (common.Hash{}) && existingTimestamp == uint64(timestamp.Unix()) {
					// Block exists in EL with matching timestamp - safe to reuse
					c.logger.Info().
						Uint64("height", height).
						Str("stage", execMeta.Stage).
						Str("blockHash", existingBlockHash.Hex()).
						Msg("ExecuteTxs: reusing already-promoted execution (idempotent)")

					// Update head to point to this existing block
					if err := c.setHead(ctx, existingBlockHash); err != nil {
						c.logger.Warn().Err(err).Msg("ExecuteTxs: failed to update head to existing block")
					}

					return existingStateRoot.Bytes(), nil, true, nil
				}
				// ExecMeta says promoted but block doesn't exist in EL or timestamp mismatch
				// This can happen if ev-reth crashed before persisting the block
				c.logger.Warn().
					Uint64("height", height).
					Bool("block_exists", existingBlockHash != common.Hash{}).
					Err(elErr).
					Msg("ExecuteTxs: ExecMeta shows promoted but block not found in EL (EL in behind), will re-execute")
				// Fall through to fresh execution
			} else {
				// Timestamp mismatch - ExecMeta is stale from an old block that was replaced.
				// Ignore it and proceed to EL check which will handle rollback if needed.
				c.logger.Warn().
					Uint64("height", height).
					Int64("execmeta_timestamp", execMeta.Timestamp).
					Int64("requested_timestamp", timestamp.Unix()).
					Msg("ExecuteTxs: ExecMeta timestamp mismatch, ignoring stale promoted record")
			}
		}

		// If we have a started execution with a payloadID, validate it still exists before resuming.
		// After node restart, the EL's payload cache is ephemeral and the payloadID may be stale.
		if execMeta.Stage == ExecStageStarted && len(execMeta.PayloadID) == 8 {
			requestedTxHash := hashTxs(txs)
			if execMeta.Timestamp != timestamp.Unix() || !bytes.Equal(execMeta.TxHash, requestedTxHash) {
				c.logger.Debug().
					Uint64("height", height).
					Int64("execmeta_timestamp", execMeta.Timestamp).
					Int64("requested_timestamp", timestamp.Unix()).
					Msg("ExecuteTxs: ignoring stale in-progress execution for different block inputs")
			} else {
				var pid engine.PayloadID
				copy(pid[:], execMeta.PayloadID)

				// Validate payload still exists by attempting to retrieve it
				if _, err = c.engineClient.GetPayload(ctx, pid); err == nil {
					c.logger.Info().
						Uint64("height", height).
						Str("stage", execMeta.Stage).
						Msg("ExecuteTxs: found in-progress execution with payloadID, returning payloadID for resume")
					return nil, &pid, true, nil
				}
				// Payload is stale (expired or node restarted) - proceed with fresh execution
				c.logger.Debug().
					Uint64("height", height).
					Str("payloadID", pid.String()).
					Err(err).
					Msg("ExecuteTxs: stale ExecMeta payloadID no longer valid in EL, will re-execute")
				// Don't return - fall through to fresh execution
			}
		}
	}

	// 2. Check EL for existing block (EL-level idempotency)
	existingBlockHash, existingStateRoot, _, existingTimestamp, err := c.getBlockInfo(ctx, height)
	if err == nil && existingBlockHash != (common.Hash{}) {
		// Block exists at this height - check if timestamp matches
		if existingTimestamp == uint64(timestamp.Unix()) {
			c.logger.Info().
				Uint64("height", height).
				Str("blockHash", existingBlockHash.Hex()).
				Str("stateRoot", existingStateRoot.Hex()).
				Msg("ExecuteTxs: reusing existing block at height (EL idempotency)")

			// Update head to point to this existing block
			if err := c.setHead(ctx, existingBlockHash); err != nil {
				c.logger.Warn().Err(err).Msg("ExecuteTxs: failed to update head to existing block")
				// Continue anyway - the block exists and we can return its state root
			}

			// Update ExecMeta to promoted
			c.saveExecMeta(ctx, height, timestamp.Unix(), nil, existingBlockHash[:], existingStateRoot.Bytes(), txs, ExecStagePromoted)

			return existingStateRoot.Bytes(), nil, true, nil
		}
		// We need to rollback the EL to height-1 so it can re-execute
		c.logger.Warn().
			Uint64("height", height).
			Uint64("existingTimestamp", existingTimestamp).
			Int64("requestedTimestamp", timestamp.Unix()).
			Msg("ExecuteTxs: block exists at height but timestamp differs - rolling back EL to re-sync")

		// Rollback to height-1 to allow re-execution with correct timestamp
		if height > 0 {
			if err := c.Rollback(ctx, height-1); err != nil {
				c.logger.Error().Err(err).
					Uint64("height", height).
					Uint64("rollback_target", height-1).
					Msg("ExecuteTxs: failed to rollback EL for timestamp mismatch")
				return nil, nil, false, fmt.Errorf("failed to rollback EL for timestamp mismatch at height %d: %w", height, err)
			}
			c.logger.Info().
				Uint64("height", height).
				Uint64("rollback_target", height-1).
				Msg("ExecuteTxs: EL rolled back successfully, will re-execute with correct timestamp")
		}
	}

	return nil, nil, false, nil
}

// filterTransactions formats transactions for the payload.
// DA transactions should already be filtered via FilterTxs before reaching here.
// Mempool transactions are already validated when added to mempool.
func (c *EngineClient) filterTransactions(txs [][]byte) []string {
	validTxs := make([]string, 0, len(txs))
	for _, tx := range txs {
		if len(tx) == 0 {
			continue
		}
		validTxs = append(validTxs, "0x"+hex.EncodeToString(tx))
	}
	return validTxs
}

// GetExecutionInfo returns current execution layer parameters.
func (c *EngineClient) GetExecutionInfo(ctx context.Context) (execution.ExecutionInfo, error) {
	var info execution.ExecutionInfo
	if cached := c.cachedExecutionInfo.Load(); cached != nil {
		info.MaxGas = cached.MaxGas
	} else {
		header, err := c.ethClient.HeaderByNumber(ctx, nil) // nil = latest
		if err != nil {
			return execution.ExecutionInfo{}, fmt.Errorf("failed to get latest block: %w", err)
		}

		info.MaxGas = header.GasLimit
		c.cachedExecutionInfo.Store(&execution.ExecutionInfo{MaxGas: info.MaxGas})
	}

	return info, nil
}

func isRPCMethodNotFound(err error) bool {
	var rpcErr rpc.Error
	return errors.As(err, &rpcErr) && rpcErr.ErrorCode() == -32601
}

func (c *EngineClient) executionResultAfterBlock(ctx context.Context, stateRoot []byte, executedBlockHeight uint64) (execution.ExecuteResult, error) {
	nextProposer, err := c.nextProposerChangeAfterBlock(ctx, executedBlockHeight)
	if err != nil {
		return execution.ExecuteResult{}, err
	}
	return execution.ExecuteResult{
		UpdatedStateRoot:    stateRoot,
		NextProposerAddress: nextProposer,
	}, nil
}

func (c *EngineClient) nextProposerChangeAfterBlock(ctx context.Context, executedBlockHeight uint64) ([]byte, error) {
	if executedBlockHeight == 0 {
		return nil, nil
	}

	before, supported, err := c.nextProposerAtBlock(ctx, executedBlockHeight-1)
	if err != nil || !supported {
		return nil, err
	}

	after, supported, err := c.nextProposerAtBlock(ctx, executedBlockHeight)
	if err != nil || !supported {
		return nil, err
	}

	if after == (common.Hash{}) || after == before {
		return nil, nil
	}
	return after.Bytes(), nil
}

func (c *EngineClient) nextProposerAtBlock(ctx context.Context, blockHeight uint64) (common.Hash, bool, error) {
	proposer, err := c.ethClient.GetNextProposer(ctx, new(big.Int).SetUint64(blockHeight))
	if err != nil {
		if isRPCMethodNotFound(err) {
			return common.Hash{}, false, nil
		}
		return common.Hash{}, true, fmt.Errorf("failed to get next proposer at block %d: %w", blockHeight, err)
	}
	return proposer, true, nil
}

// FilterTxs validates force-included transactions and applies gas and size filtering for all passed txs.
// If hasForceIncludedTransaction is false, skip filtering entirely - mempool batch is already filtered.
// Returns a slice of FilterStatus for each transaction.
func (c *EngineClient) FilterTxs(ctx context.Context, txs [][]byte, maxBytes, maxGas uint64, hasForceIncludedTransaction bool) ([]execution.FilterStatus, error) {
	result := make([]execution.FilterStatus, len(txs))

	var cumulativeGas uint64
	var cumulativeBytes uint64
	limitReached := false

	for i, tx := range txs {
		// Skip empty transactions
		if len(tx) == 0 {
			result[i] = execution.FilterRemove
			continue
		}

		txBytes := uint64(len(tx))
		var txGas uint64

		// Only validate and parse tx if force-included txs are present
		// Mempool txs are already validated, so we can skip parsing when not needed
		if hasForceIncludedTransaction {
			var ethTx types.Transaction
			if err := ethTx.UnmarshalBinary(tx); err != nil {
				c.logger.Debug().
					Int("tx_index", i).
					Err(err).
					Str("tx_hex", "0x"+hex.EncodeToString(tx)).
					Msg("filtering out invalid transaction (gibberish)")
				result[i] = execution.FilterRemove
				continue
			}
			txGas = ethTx.Gas()

			// Skip tx that can never make it in a block (too much gas)
			if maxGas > 0 && txGas > maxGas {
				result[i] = execution.FilterRemove
				continue
			}
		}

		// Skip tx that can never make it in a block (too big)
		if maxBytes > 0 && txBytes > maxBytes {
			result[i] = execution.FilterRemove
			continue
		}

		// Once limit is reached, postpone remaining txs
		if limitReached {
			result[i] = execution.FilterPostpone
			continue
		}

		// Check size limit
		if maxBytes > 0 && cumulativeBytes+txBytes > maxBytes {
			limitReached = true
			result[i] = execution.FilterPostpone
			c.logger.Debug().
				Uint64("cumulative_bytes", cumulativeBytes).
				Uint64("tx_bytes", txBytes).
				Uint64("max_bytes", maxBytes).
				Msg("size limit reached, postponing remaining txs")
			continue
		}

		// Check gas limit (only when we have force-included txs and parsed the tx)
		if hasForceIncludedTransaction && maxGas > 0 && cumulativeGas+txGas > maxGas {
			limitReached = true
			result[i] = execution.FilterPostpone
			c.logger.Debug().
				Uint64("cumulative_gas", cumulativeGas).
				Uint64("tx_gas", txGas).
				Uint64("max_gas", maxGas).
				Msg("gas limit reached, postponing remaining txs")
			continue
		}

		cumulativeBytes += txBytes
		cumulativeGas += txGas
		result[i] = execution.FilterOK
	}

	return result, nil
}

// processPayload handles the common logic of getting, submitting, and finalizing a payload.
func (c *EngineClient) processPayload(ctx context.Context, payloadID engine.PayloadID, txs [][]byte) ([]byte, error) {
	// 1. Get Payload
	payloadResult, err := c.engineClient.GetPayload(ctx, payloadID)
	if err != nil {
		return nil, fmt.Errorf("get payload failed: %w", err)
	}

	blockHeight := payloadResult.ExecutionPayload.Number
	blockTimestamp := int64(payloadResult.ExecutionPayload.Timestamp)

	// 2. Submit Payload (newPayload)
	selectedNewPayloadMethod := newPayloadMethod(payloadResult)
	err = retryWithBackoffOnPayloadStatus(ctx, func() error {
		newPayloadResult, err := c.engineClient.NewPayload(ctx,
			payloadResult,
			[]string{},          // No blob hashes
			common.Hash{}.Hex(), // Use zero hash for parentBeaconBlockRoot
			payloadResult.Requests,
		)
		if err != nil {
			return fmt.Errorf("new payload submission failed: %w", err)
		}

		if err := validatePayloadStatus(*newPayloadResult); err != nil {
			c.logger.Warn().
				Str("method", selectedNewPayloadMethod).
				Str("status", newPayloadResult.Status).
				Str("latestValidHash", latestValidHashHex(newPayloadResult.LatestValidHash)).
				Interface("validationError", newPayloadResult.ValidationError).
				Uint64("blockHeight", blockHeight).
				Msg("processPayload: new payload returned non-VALID status")
			return err
		}
		return nil
	}, MaxPayloadStatusRetries, InitialRetryBackoff, "processPayload newPayload")
	if err != nil {
		return nil, err
	}

	// 3. Update Forkchoice
	blockHash := payloadResult.ExecutionPayload.BlockHash
	c.cacheBlockHash(blockHeight, blockHash)

	err = c.setFinalWithHeight(ctx, blockHash, blockHeight, false)
	if err != nil {
		return nil, fmt.Errorf("forkchoice update failed: %w", err)
	}

	// 4. Save ExecMeta (Promoted)
	c.saveExecMeta(ctx, blockHeight, blockTimestamp, payloadID[:], blockHash[:], payloadResult.ExecutionPayload.StateRoot.Bytes(), txs, ExecStagePromoted)

	return payloadResult.ExecutionPayload.StateRoot.Bytes(), nil
}

func (c *EngineClient) derivePrevRandao(blockHeight uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(blockHeight))
}

func (c *EngineClient) buildPayloadAttributes(txsPayload []string, blockHeight uint64, timestamp time.Time, prevGasLimit uint64) map[string]any {
	attrs := map[string]any{
		// Standard Ethereum payload attributes (flattened) - using camelCase as expected by JSON
		"timestamp":             timestamp.Unix(),
		"prevRandao":            c.derivePrevRandao(blockHeight),
		"suggestedFeeRecipient": c.feeRecipient,
		"withdrawals":           []*types.Withdrawal{},
		"parentBeaconBlockRoot": common.Hash{}.Hex(), // Use zero hash for evolve
		// evolve-specific fields
		"transactions": txsPayload,
		"gasLimit":     prevGasLimit,
		// slotNumber is included as a local candidate for Amsterdam. The RPC
		// client strips it from V3 requests and keeps it on V4 retries.
		"slotNumber": hexutil.Uint64(c.deriveSlotNumber(blockHeight)),
	}

	return attrs
}

// deriveSlotNumber returns the slotNumber payload attribute required by
// engine_forkchoiceUpdatedV4 (Amsterdam). ev-node has no beacon slots, so the
// rollup block height serves as the deterministic slot number. This is a wire
// contract with ev-reth's Amsterdam chainspec handling and must not change
// once the fork is enabled on a live chain.
func (c *EngineClient) deriveSlotNumber(blockHeight uint64) uint64 {
	return blockHeight
}

func (c *EngineClient) getBlockInfo(ctx context.Context, height uint64) (common.Hash, common.Hash, uint64, uint64, error) {
	header, err := c.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(height))

	if err != nil {
		return common.Hash{}, common.Hash{}, 0, 0, fmt.Errorf("failed to get block at height %d: %w", height, err)
	}

	return header.Hash(), header.Root, header.GasLimit, header.Time, nil
}

// saveExecMeta persists execution metadata to the store for crash recovery.
// This is a best-effort operation - failures are logged but don't fail the execution.
func (c *EngineClient) saveExecMeta(ctx context.Context, height uint64, timestamp int64, payloadID []byte, blockHash []byte, stateRoot []byte, txs [][]byte, stage string) {
	execMeta := &ExecMeta{
		Height:        height,
		Timestamp:     timestamp,
		PayloadID:     payloadID,
		BlockHash:     blockHash,
		StateRoot:     stateRoot,
		Stage:         stage,
		UpdatedAtUnix: time.Now().Unix(),
	}

	// Compute tx hash for sanity checks on retry
	execMeta.TxHash = hashTxs(txs)

	if err := c.store.SaveExecMeta(ctx, execMeta); err != nil {
		c.logger.Warn().Err(err).Uint64("height", height).Msg("saveExecMeta: failed to save exec meta")
		return
	}

	c.logger.Debug().
		Uint64("height", height).
		Str("stage", stage).
		Msg("saveExecMeta: saved execution metadata")
}

func hashTxs(txs [][]byte) []byte {
	if len(txs) == 0 {
		return nil
	}

	h := sha256.New()
	for _, tx := range txs {
		h.Write(tx)
	}

	return h.Sum(nil)
}

// GetLatestHeight returns the current block height of the execution layer
func (c *EngineClient) GetLatestHeight(ctx context.Context) (uint64, error) {
	header, err := c.ethClient.HeaderByNumber(ctx, nil) // nil = latest block
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block: %w", err)
	}
	return header.Number.Uint64(), nil
}

// Rollback resets the execution layer head to the specified height using forkchoice update.
// This is used for recovery when the EL is ahead of the consensus layer (e.g., during rolling restarts
//
// Implements the execution.Rollbackable interface.
func (c *EngineClient) Rollback(ctx context.Context, targetHeight uint64) error {
	if err := c.ensureForkchoiceInitialized(ctx); err != nil {
		return fmt.Errorf("initialize forkchoice state: %w", err)
	}

	// Get block hash at target height
	blockHash, _, _, _, err := c.getBlockInfo(ctx, targetHeight)
	if err != nil {
		return fmt.Errorf("get block at height %d: %w", targetHeight, err)
	}

	c.logger.Info().
		Uint64("target_height", targetHeight).
		Str("block_hash", blockHash.Hex()).
		Msg("rolling back execution layer via forkchoice update")

	// Reset head, safe, and finalized to target block
	// This forces the EL to reorg its canonical chain to the target height
	c.mu.Lock()
	c.currentHeadBlockHash = blockHash
	c.currentHeadHeight = targetHeight
	c.currentSafeBlockHash = blockHash
	c.currentFinalizedBlockHash = blockHash
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      blockHash,
		SafeBlockHash:      blockHash,
		FinalizedBlockHash: blockHash,
	}
	c.mu.Unlock()

	if err := c.doForkchoiceUpdate(ctx, args, "Rollback"); err != nil {
		return fmt.Errorf("forkchoice update for rollback failed: %w", err)
	}

	c.logger.Info().
		Uint64("target_height", targetHeight).
		Msg("execution layer rollback completed")

	return nil
}

// PruneExec implements execution.ExecPruner by delegating to the
// underlying EVMStore. It is safe to call this multiple times with the same
// or increasing heights; the store tracks its own last-pruned height.
func (c *EngineClient) PruneExec(ctx context.Context, height uint64) error {
	return c.store.PruneExec(ctx, height)
}

// decodeSecret decodes a hex-encoded JWT secret string into a byte slice.
func decodeSecret(jwtSecret string) ([]byte, error) {
	secret, err := hex.DecodeString(strings.TrimPrefix(jwtSecret, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT secret: %w", err)
	}
	return secret, nil
}

// getAuthToken creates a JWT token signed with the provided secret, valid for 1 hour.
func getAuthToken(jwtSecret []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour * 1).Unix(), // Expires in 1 hour
		"iat": time.Now().Unix(),
	})

	// Sign the token with the decoded secret
	authToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}
	return authToken, nil
}
