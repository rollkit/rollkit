package evm

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

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
		engineClient: engineClient,
		ethClient:    ethClient,
		genesisHash:  genesisHash,
		feeRecipient: feeRecipient,
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
		for _, tx := range accountTxs {
			txBytes, err := tx.MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("failed to marshal transaction: %w", err)
			}
			txs = append(txs, txBytes)
		}
	}

	// add queued txs
	for _, accountTxs := range result.Queued {
		for _, tx := range accountTxs {
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
	// convert rollkit tx to eth tx
	ethTxs := make([]*types.Transaction, len(txs))
	for i, tx := range txs {
		ethTxs[i] = new(types.Transaction)
		err := ethTxs[i].UnmarshalBinary(tx)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal transaction: %w", err)
		}
		txHash := ethTxs[i].Hash()
		_, isPending, err := c.ethClient.TransactionByHash(context.Background(), txHash)
		if err == nil && isPending {
			continue // skip SendTransaction
		}
		err = c.ethClient.SendTransaction(context.Background(), ethTxs[i])
		if err != nil {
			// Ignore the error if the transaction fails to be sent since transactions can be invalid and an invalid transaction sent by a client should not crash the node.
			continue
		}
	}

	// encode
	txsPayload := make([][]byte, len(txs))
	for i, tx := range ethTxs {
		buf := bytes.Buffer{}
		err := tx.EncodeRLP(&buf)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to RLP encode tx: %w", err)
		}
		txsPayload[i] = buf.Bytes()
	}

	var (
		prevBlockHash common.Hash
		prevTimestamp uint64
	)

	// fetch previous block hash to update forkchoice for the next payload id
	// if blockHeight == c.initialHeight {
	// 	prevBlockHash = c.genesisHash
	// } else {
	prevBlockHash, _, _, prevTimestamp, err = c.getBlockInfo(ctx, blockHeight-1)
	if err != nil {
		return nil, 0, err
	}
	// }

	// make sure that the timestamp is increasing
	ts := uint64(timestamp.Unix())
	if ts <= prevTimestamp {
		ts = prevTimestamp + 1 // Subsequent blocks must have a higher timestamp.
	}

    parentStateRoot := common.BytesToHash(prevStateRoot)
	// update forkchoice to get the next payload id
	var forkchoiceResult engine.ForkChoiceResponse
	err = c.engineClient.CallContext(ctx, &forkchoiceResult, "engine_forkchoiceUpdatedV3",
		engine.ForkchoiceStateV1{
			HeadBlockHash:      prevBlockHash,
			SafeBlockHash:      prevBlockHash,
			FinalizedBlockHash: prevBlockHash,
		},
		&engine.PayloadAttributes{
			Timestamp:             ts,
			Random:                prevBlockHash, //c.derivePrevRandao(height),
			SuggestedFeeRecipient: c.feeRecipient,
			Withdrawals:           []*types.Withdrawal{},
			BeaconRoot:            &parentStateRoot,
			Transactions:          txsPayload,
			NoTxPool:              true,
		},
	)
	if err != nil {
		return nil, 0, fmt.Errorf("forkchoice update failed: %w", err)
	}

	if forkchoiceResult.PayloadID == nil {
		return nil, 0, ErrNilPayloadStatus
	}

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
		[]string{}, // No blob hashes
		c.genesisHash.Hex(),
		[][]byte{}, // No execution requests
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
	blockHash, _, _, _, err := c.getBlockInfo(ctx, blockHeight)
	if err != nil {
		return fmt.Errorf("failed to get block info: %w", err)
	}
	return c.setFinal(ctx, blockHash, true)
}

func (c *EngineClient) derivePrevRandao(blockHeight uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(blockHeight))
}

func (c *EngineClient) getBlockInfo(ctx context.Context, height uint64) (common.Hash, common.Hash, uint64, uint64, error) {
	header, err := c.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(height))

	if err != nil {
		return common.Hash{}, common.Hash{}, 0, 0, fmt.Errorf("failed to get block at height %d: %w", height, err)
	}

	return header.Hash(), header.Root, header.GasLimit, header.Time, nil
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
