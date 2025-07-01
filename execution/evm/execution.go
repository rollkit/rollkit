package evm

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang-jwt/jwt/v5"

	"github.com/rollkit/rollkit/core/execution"
)

var (
	// ErrNilPayloadStatus indicates that PayloadID returned by EVM was nil
	ErrNilPayloadStatus = errors.New("nil payload status")
	// ErrInvalidPayloadStatus indicates that EVM returned status != VALID
	ErrInvalidPayloadStatus = errors.New("invalid payload status")
)

// Ensure EngineAPIExecutionClient implements the execution.Execute interface
var _ execution.Executor = (*EngineClient)(nil)

// payloadMeta stores execution payload metadata for quick lookup
type payloadMeta struct {
	hash      common.Hash
	stateRoot common.Hash
	gasLimit  uint64
	timestamp uint64
}

// EngineClient represents a client that interacts with an Ethereum execution engine
// through the Engine API. It manages connections to both the engine and standard Ethereum
// APIs, and maintains state related to block processing.
type EngineClient struct {
	engineClient  *rpc.Client       // Client for Engine API calls
	ethClient     *ethclient.Client // Client for standard Ethereum API calls
	genesisHash   common.Hash       // Hash of the genesis block
	initialHeight uint64
	feeRecipient  common.Address // Address to receive transaction fees

	mu                        sync.Mutex  // Mutex to protect concurrent access to block hashes
	currentHeadBlockHash      common.Hash // Store last non-finalized HeadBlockHash
	currentSafeBlockHash      common.Hash // Store last non-finalized SafeBlockHash
	currentFinalizedBlockHash common.Hash // Store last finalized block hash

	indexMu    sync.RWMutex
	blockIndex map[uint64]*payloadMeta
}

// NewEngineExecutionClient creates a new instance of EngineAPIExecutionClient
func NewEngineExecutionClient(
	ethURL,
	engineURL string,
	jwtSecret string,
	genesisHash common.Hash,
	feeRecipient common.Address,
) (*EngineClient, error) {
	ethClient, err := ethclient.Dial(ethURL)
	if err != nil {
		return nil, err
	}

	secret, err := decodeSecret(jwtSecret)
	if err != nil {
		return nil, err
	}

	engineClient, err := rpc.DialOptions(context.Background(), engineURL,
		rpc.WithHTTPAuth(func(h http.Header) error {
			authToken, err := getAuthToken(secret)
			if err != nil {
				return err
			}

			if authToken != "" {
				h.Set("Authorization", "Bearer "+authToken)
			}
			return nil
		}))
	if err != nil {
		return nil, err
	}

	return &EngineClient{
		engineClient:              engineClient,
		ethClient:                 ethClient,
		genesisHash:               genesisHash,
		feeRecipient:              feeRecipient,
		currentHeadBlockHash:      genesisHash,
		currentSafeBlockHash:      genesisHash,
		currentFinalizedBlockHash: genesisHash,
		blockIndex:                make(map[uint64]*payloadMeta),
	}, nil
}

// InitChain initializes the blockchain with the given genesis parameters
func (c *EngineClient) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error) {
	if initialHeight != 1 {
		return nil, 0, fmt.Errorf("initialHeight must be 1, got %d", initialHeight)
	}

	// Acknowledge the genesis block
	var forkchoiceResult engine.ForkChoiceResponse
	err := c.engineClient.CallContext(ctx, &forkchoiceResult, "engine_forkchoiceUpdatedV3",
		engine.ForkchoiceStateV1{
			HeadBlockHash:      c.genesisHash,
			SafeBlockHash:      c.genesisHash,
			FinalizedBlockHash: c.genesisHash,
		},
		nil,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("engine_forkchoiceUpdatedV3 failed: %w", err)
	}

	_, stateRoot, gasLimit, _, err := c.getBlockInfo(ctx, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get block info: %w", err)
	}

	c.initialHeight = initialHeight

	return stateRoot[:], gasLimit, nil
}

// GetTxs retrieves transactions from the current execution payload
func (c *EngineClient) GetTxs(ctx context.Context) ([][]byte, error) {
	var result struct {
		Pending map[string]map[string]*types.Transaction `json:"pending"`
		Queued  map[string]map[string]*types.Transaction `json:"queued"`
	}
	err := c.ethClient.Client().CallContext(ctx, &result, "txpool_content")
	if err != nil {
		return nil, fmt.Errorf("failed to get tx pool content: %w", err)
	}

	var txs [][]byte

	// add pending txs
	for _, accountTxs := range result.Pending {
		// Extract and sort keys to iterate in ordered fashion
		keys := make([]string, 0, len(accountTxs))
		for key := range accountTxs {
			keys = append(keys, key)
		}

		// Sort keys as integers (they represent nonces)
		sort.Slice(keys, func(i, j int) bool {
			// Parse as integers for proper numerical sorting
			a, errA := strconv.Atoi(keys[i])
			b, errB := strconv.Atoi(keys[j])

			// If parsing fails, fall back to string comparison
			if errA != nil || errB != nil {
				return keys[i] < keys[j]
			}

			return a < b
		})

		// Iterate over sorted keys
		for _, key := range keys {
			tx := accountTxs[key]
			txBytes, err := tx.MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("failed to marshal transaction: %w", err)
			}
			txs = append(txs, txBytes)
		}
	}
	return txs, nil
}

// ExecuteTxs executes the given transactions at the specified block height and timestamp
func (c *EngineClient) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (updatedStateRoot []byte, maxBytes uint64, err error) {
	// convert rollkit tx to hex strings for rollkit-reth
	txsPayload := make([]string, len(txs))
	for i, tx := range txs {
		// Use the raw transaction bytes directly instead of re-encoding
		txsPayload[i] = "0x" + hex.EncodeToString(tx)
	}

	prevBlockHash, _, prevGasLimit, prevTimestamp, err := c.getBlockInfo(ctx, blockHeight-1)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get block info: %w", err)
	}

	ts := uint64(timestamp.Unix())
	if ts <= prevTimestamp {
		ts = prevTimestamp + 1
	}

	c.mu.Lock()
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      prevBlockHash,
		SafeBlockHash:      prevBlockHash,
		FinalizedBlockHash: prevBlockHash,
	}
	c.mu.Unlock()

	// update forkchoice to get the next payload id
	var forkchoiceResult engine.ForkChoiceResponse

	// Create rollkit-compatible payload attributes with flattened structure
	rollkitPayloadAttrs := map[string]interface{}{
		// Standard Ethereum payload attributes (flattened) - using camelCase as expected by JSON
		"timestamp":             ts,
		"prevRandao":            c.derivePrevRandao(blockHeight),
		"suggestedFeeRecipient": c.feeRecipient,
		"withdrawals":           []*types.Withdrawal{},
		// V3 requires parentBeaconBlockRoot
		"parentBeaconBlockRoot": common.Hash{}.Hex(), // Use zero hash for rollkit
		// Rollkit-specific fields
		"transactions": txsPayload,
		"gasLimit":     prevGasLimit, // Use camelCase to match JSON conventions
	}

	err = c.engineClient.CallContext(ctx, &forkchoiceResult, "engine_forkchoiceUpdatedV3",
		args,
		rollkitPayloadAttrs,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("forkchoice update failed: %w", err)
	}

	if forkchoiceResult.PayloadID == nil {
		return nil, 0, ErrNilPayloadStatus
	}

	// Small delay to allow payload building to complete
	time.Sleep(10 * time.Millisecond)

	// get payload
	var payloadResult engine.ExecutionPayloadEnvelope
	err = c.engineClient.CallContext(ctx, &payloadResult, "engine_getPayloadV4", *forkchoiceResult.PayloadID)
	if err != nil {
		return nil, 0, fmt.Errorf("get payload failed: %w", err)
	}

	// submit payload
	var newPayloadResult engine.PayloadStatusV1
	err = c.engineClient.CallContext(ctx, &newPayloadResult, "engine_newPayloadV4",
		payloadResult.ExecutionPayload,
		[]string{},          // No blob hashes
		common.Hash{}.Hex(), // Use zero hash for parentBeaconBlockRoot (same as in payload attributes)
		[][]byte{},          // No execution requests
	)
	if err != nil {
		return nil, 0, fmt.Errorf("new payload submission failed: %w", err)
	}

	if newPayloadResult.Status != engine.VALID {
		return nil, 0, ErrInvalidPayloadStatus
	}

	// forkchoice update
	blockHash := payloadResult.ExecutionPayload.BlockHash
	err = c.setFinal(ctx, blockHash, false)
	if err != nil {
		return nil, 0, err
	}

	// Record metadata for quick future retrieval
	c.indexMu.Lock()
	c.blockIndex[blockHeight] = &payloadMeta{
		hash:      payloadResult.ExecutionPayload.BlockHash,
		stateRoot: payloadResult.ExecutionPayload.StateRoot,
		gasLimit:  payloadResult.ExecutionPayload.GasLimit,
		timestamp: payloadResult.ExecutionPayload.Timestamp,
	}
	c.indexMu.Unlock()

	return payloadResult.ExecutionPayload.StateRoot.Bytes(), payloadResult.ExecutionPayload.GasUsed, nil
}

func (c *EngineClient) setFinal(ctx context.Context, blockHash common.Hash, isFinal bool) error {
	c.mu.Lock()
	// Update block hashes based on finalization status
	if isFinal {
		c.currentFinalizedBlockHash = blockHash
	} else {
		c.currentHeadBlockHash = blockHash
		c.currentSafeBlockHash = blockHash
	}

	// Construct forkchoice state
	args := engine.ForkchoiceStateV1{
		HeadBlockHash:      c.currentHeadBlockHash,
		SafeBlockHash:      c.currentSafeBlockHash,
		FinalizedBlockHash: c.currentFinalizedBlockHash,
	}
	c.mu.Unlock()

	var forkchoiceResult engine.ForkChoiceResponse
	err := c.engineClient.CallContext(ctx, &forkchoiceResult, "engine_forkchoiceUpdatedV3",
		args,
		nil,
	)
	if err != nil {
		return fmt.Errorf("forkchoice update failed with error: %w", err)
	}

	if forkchoiceResult.PayloadStatus.Status != engine.VALID {
		return ErrInvalidPayloadStatus
	}

	return nil
}

// SetFinal marks the block at the given height as finalized
func (c *EngineClient) SetFinal(ctx context.Context, blockHeight uint64) error {
	// Try cache first
	c.indexMu.RLock()
	if meta, ok := c.blockIndex[blockHeight]; ok {
		c.indexMu.RUnlock()
		return c.setFinal(ctx, meta.hash, true)
	}
	c.indexMu.RUnlock()

	// Fallback to RPC
	blockHash, _, _, _, err := c.getBlockInfo(ctx, blockHeight)
	if err != nil {
		return fmt.Errorf("failed to get block info: %w", err)
	}
	return c.setFinal(ctx, blockHash, true)
}

func (c *EngineClient) GetExecutionMode() execution.ExecutionMode {
	return execution.ExecutionModeImmediate
}

func (c *EngineClient) derivePrevRandao(blockHeight uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(blockHeight))
}

func (c *EngineClient) getBlockInfo(ctx context.Context, height uint64) (common.Hash, common.Hash, uint64, uint64, error) {
	// Fast path – use in-memory index
	c.indexMu.RLock()
	if meta, ok := c.blockIndex[height]; ok {
		c.indexMu.RUnlock()
		return meta.hash, meta.stateRoot, meta.gasLimit, meta.timestamp, nil
	}
	c.indexMu.RUnlock()

	// Slow path – query RPC with a few quick retries
	const retries = 3
	for i := 0; i < retries; i++ {
		header, err := c.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(height))
		if err == nil {
			return header.Hash(), header.Root, header.GasLimit, header.Time, nil
		}
		// If the node hasn’t indexed the block yet, wait a bit and retry
		if errors.Is(err, ethereum.NotFound) || strings.Contains(err.Error(), "not found") {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return common.Hash{}, common.Hash{}, 0, 0, fmt.Errorf("failed to get block at height %d: %w", height, err)
	}
	return common.Hash{}, common.Hash{}, 0, 0, fmt.Errorf("block %d not yet indexed", height)
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
