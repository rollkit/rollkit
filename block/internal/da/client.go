package da

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/celestiaorg/go-square/v3/share"
	"github.com/rs/zerolog"

	"github.com/evstack/ev-node/pkg/blobsize"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
	datypes "github.com/evstack/ev-node/pkg/da/types"
)

// BlobSubmitter turns blobs into a committed DA height.
type BlobSubmitter interface {
	Submit(ctx context.Context, blobs []*blobrpc.Blob, opts *blobrpc.SubmitOptions) (uint64, error)
}

// Config contains configuration for the blob DA client.
type Config struct {
	DA                       *blobrpc.Client
	Submitter                BlobSubmitter // nil submits through DA.Blob
	Logger                   zerolog.Logger
	DefaultTimeout           time.Duration
	Namespace                string
	DataNamespace            string
	ForcedInclusionNamespace string
}

// client wraps the blob RPC with namespace handling and error mapping.
// It is unexported; callers should use the exported Client interface.
type client struct {
	blobAPI            *blobrpc.BlobAPI
	submitter          BlobSubmitter
	headerAPI          *blobrpc.HeaderAPI
	da                 *blobrpc.Client // kept to read live IsWebSocket after transport upgrades
	logger             zerolog.Logger
	defaultTimeout     time.Duration
	namespaceBz        []byte
	dataNamespaceBz    []byte
	forcedNamespaceBz  []byte
	hasForcedNamespace bool
	timestampCache     *blockTimestampCache
}

// Ensure client implements the FullClient interface (Client + BlobGetter + Verifier).
var _ FullClient = (*client)(nil)

const (
	blockTimestampFetchMaxAttempts = 3
	blockTimestampFetchBackoff     = 100 * time.Millisecond
	blockTimestampCacheWindow      = 2048
)

type blockTimestampCache struct {
	mu       sync.RWMutex
	byHeight map[uint64]time.Time
	highest  uint64
	window   uint64
}

func newBlockTimestampCache(window uint64) *blockTimestampCache {
	if window == 0 {
		window = blockTimestampCacheWindow
	}
	return &blockTimestampCache{
		byHeight: make(map[uint64]time.Time),
		window:   window,
	}
}

func (c *blockTimestampCache) get(height uint64) (time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	blockTime, ok := c.byHeight[height]
	return blockTime, ok
}

func (c *blockTimestampCache) put(height uint64, blockTime time.Time) {
	if c == nil || blockTime.IsZero() {
		return
	}

	blockTime = blockTime.UTC()

	c.mu.Lock()
	defer c.mu.Unlock()

	minRetained := c.minRetainedHeightLocked()
	if minRetained > 0 && height < minRetained {
		return
	}

	if height > c.highest {
		c.highest = height
	}
	c.byHeight[height] = blockTime

	minRetained = c.minRetainedHeightLocked()
	if minRetained == 0 {
		return
	}
	for cachedHeight := range c.byHeight {
		if cachedHeight < minRetained {
			delete(c.byHeight, cachedHeight)
		}
	}
}

func (c *blockTimestampCache) minRetainedHeightLocked() uint64 {
	if c.window == 0 || c.highest < c.window-1 {
		return 0
	}
	return c.highest - c.window + 1
}

// NewClient creates a new blob client wrapper with pre-calculated namespace bytes.
func NewClient(cfg Config) FullClient {
	if cfg.DA == nil {
		return nil
	}
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 60 * time.Second
	}

	hasForcedNamespace := cfg.ForcedInclusionNamespace != ""
	var forcedNamespaceBz []byte
	if hasForcedNamespace {
		forcedNamespaceBz = datypes.NamespaceFromString(cfg.ForcedInclusionNamespace).Bytes()
	}

	if cfg.Submitter == nil {
		cfg.Submitter = &cfg.DA.Blob
	}

	return &client{
		blobAPI:            &cfg.DA.Blob,
		submitter:          cfg.Submitter,
		headerAPI:          &cfg.DA.Header,
		da:                 cfg.DA,
		logger:             cfg.Logger.With().Str("component", "da_client").Logger(),
		defaultTimeout:     cfg.DefaultTimeout,
		namespaceBz:        datypes.NamespaceFromString(cfg.Namespace).Bytes(),
		dataNamespaceBz:    datypes.NamespaceFromString(cfg.DataNamespace).Bytes(),
		forcedNamespaceBz:  forcedNamespaceBz,
		hasForcedNamespace: hasForcedNamespace,
		timestampCache:     newBlockTimestampCache(blockTimestampCacheWindow),
	}
}

// Submit submits blobs to the DA layer with the specified options.
func (c *client) Submit(ctx context.Context, data [][]byte, _ float64, namespace []byte, options []byte) datypes.ResultSubmit {
	// calculate blob size
	var blobSize uint64
	for _, b := range data {
		blobSize += uint64(len(b))
	}

	ns, err := share.NewNamespaceFromBytes(namespace)
	if err != nil {
		return datypes.ResultSubmit{
			BaseResult: datypes.BaseResult{
				Code:    datypes.StatusError,
				Message: fmt.Sprintf("invalid namespace: %v", err),
			},
		}
	}

	blobs := make([]*blobrpc.Blob, len(data))
	for i, raw := range data {
		if uint64(len(raw)) > blobsize.DefaultMaxBlobSize {
			return datypes.ResultSubmit{
				BaseResult: datypes.BaseResult{
					Code:    datypes.StatusTooBig,
					Message: datypes.ErrBlobSizeOverLimit.Error(),
				},
			}
		}
		blobs[i], err = blobrpc.NewBlobV0(ns, raw)
		if err != nil {
			return datypes.ResultSubmit{
				BaseResult: datypes.BaseResult{
					Code:    datypes.StatusError,
					Message: fmt.Sprintf("failed to build blob %d: %v", i, err),
				},
			}
		}
	}

	var submitOpts blobrpc.SubmitOptions
	if len(options) > 0 {
		if err := json.Unmarshal(options, &submitOpts); err != nil {
			return datypes.ResultSubmit{
				BaseResult: datypes.BaseResult{
					Code:    datypes.StatusError,
					Message: fmt.Sprintf("failed to parse submit options: %v", err),
				},
			}
		}
	}

	height, err := c.submitter.Submit(ctx, blobs, &submitOpts)
	if err != nil {
		code := datypes.StatusError
		switch {
		case errors.Is(err, context.Canceled):
			code = datypes.StatusContextCanceled
		case strings.Contains(err.Error(), datypes.ErrTxTimedOut.Error()),
			strings.Contains(err.Error(), "eviction poll timeout"):
			code = datypes.StatusNotIncludedInBlock
		case strings.Contains(err.Error(), datypes.ErrTxAlreadyInMempool.Error()):
			code = datypes.StatusAlreadyInMempool
		case strings.Contains(err.Error(), datypes.ErrTxIncorrectAccountSequence.Error()):
			code = datypes.StatusIncorrectAccountSequence
		case strings.Contains(err.Error(), datypes.ErrBlobSizeOverLimit.Error()),
			strings.Contains(strings.ToLower(err.Error()), "too large"),
			strings.Contains(err.Error(), "size exceeds maximum allowed limit"):
			code = datypes.StatusTooBig
		case strings.Contains(err.Error(), datypes.ErrContextDeadline.Error()):
			code = datypes.StatusContextDeadline
		}
		if code == datypes.StatusTooBig {
			c.logger.Debug().Err(err).Uint64("status", uint64(code)).Msg("DA submission failed")
		} else {
			c.logger.Error().Err(err).Uint64("status", uint64(code)).Msg("DA submission failed")
		}
		return datypes.ResultSubmit{
			BaseResult: datypes.BaseResult{
				Code:           code,
				Message:        "failed to submit blobs: " + err.Error(),
				SubmittedCount: 0,
				Height:         0,
				Timestamp:      time.Now(),
				BlobSize:       blobSize,
			},
		}
	}

	if len(blobs) == 0 {
		return datypes.ResultSubmit{
			BaseResult: datypes.BaseResult{
				Code:     datypes.StatusSuccess,
				BlobSize: blobSize,
				Height:   height,
			},
		}
	}

	ids := make([]datypes.ID, len(blobs))
	for i, b := range blobs {
		ids[i] = blobrpc.MakeID(height, b.Commitment)
	}
	c.logger.Debug().Int("num_ids", len(ids)).Msg("DA submission successful")

	return datypes.ResultSubmit{
		BaseResult: datypes.BaseResult{
			Code:           datypes.StatusSuccess,
			IDs:            ids,
			SubmittedCount: uint64(len(ids)),
			Height:         height,
			BlobSize:       blobSize,
			Timestamp:      time.Now(),
		},
	}
}

// getBlockTimestamp fetches the block timestamp from the DA layer header.
func (c *client) getBlockTimestamp(ctx context.Context, height uint64) (time.Time, error) {
	var lastErr error
	backoff := blockTimestampFetchBackoff

	for attempt := 1; attempt <= blockTimestampFetchMaxAttempts; attempt++ {
		headerCtx, cancel := context.WithTimeout(ctx, c.defaultTimeout)
		header, err := c.headerAPI.GetByHeight(headerCtx, height)
		cancel()
		if err == nil {
			blockTime := header.Time().UTC()
			c.storeBlockTimestamp(height, blockTime)
			return blockTime, nil
		}
		lastErr = err

		if attempt == blockTimestampFetchMaxAttempts {
			break
		}

		c.logger.Info().
			Uint64("height", height).
			Int("attempt", attempt).
			Int("max_attempts", blockTimestampFetchMaxAttempts).
			Dur("retry_in", backoff).
			Err(err).
			Msg("failed to get block timestamp, retrying")

		select {
		case <-ctx.Done():
			return time.Time{}, fmt.Errorf("fetching header timestamp for block %d: %w", height, ctx.Err())
		case <-time.After(backoff):
		}

		backoff *= 2
	}

	return time.Time{}, fmt.Errorf("get header timestamp for block %d after %d attempts: %w", height, blockTimestampFetchMaxAttempts, lastErr)
}

func (c *client) cachedBlockTimestamp(height uint64) (time.Time, bool) {
	return c.timestampCache.get(height)
}

func (c *client) storeBlockTimestamp(height uint64, blockTime time.Time) {
	c.timestampCache.put(height, blockTime)
}

func (c *client) resolveBlockTimestamp(ctx context.Context, height uint64, strict bool) (time.Time, error) {
	if !strict {
		if blockTime, ok := c.cachedBlockTimestamp(height); ok {
			return blockTime, nil
		}
		return time.Time{}, nil
	}

	return c.getBlockTimestamp(ctx, height)
}

// RetrieveBlobs retrieves blobs without blocking on DA header timestamps.
func (c *client) RetrieveBlobs(ctx context.Context, height uint64, namespace []byte) datypes.ResultRetrieve {
	return c.retrieve(ctx, height, namespace, false)
}

// Retrieve retrieves blobs from the DA layer at the specified height and namespace.
// It uses GetAll to fetch all blobs at once.
// The timestamp is derived from the DA block header to ensure determinism.
func (c *client) Retrieve(ctx context.Context, height uint64, namespace []byte) datypes.ResultRetrieve {
	return c.retrieve(ctx, height, namespace, true)
}

func (c *client) retrieve(ctx context.Context, height uint64, namespace []byte, strictTimestamp bool) datypes.ResultRetrieve {
	ns, err := share.NewNamespaceFromBytes(namespace)
	if err != nil {
		return datypes.ResultRetrieve{
			BaseResult: datypes.BaseResult{
				Code:      datypes.StatusError,
				Message:   fmt.Sprintf("invalid namespace: %v", err),
				Height:    height,
				Timestamp: time.Now(),
			},
		}
	}

	blobCtx, blobCancel := context.WithTimeout(ctx, c.defaultTimeout)
	defer blobCancel()

	blobs, err := c.blobAPI.GetAll(blobCtx, height, []share.Namespace{ns})
	if err != nil {
		// Handle known errors by substring because RPC may wrap them.
		switch {
		case strings.Contains(err.Error(), datypes.ErrBlobNotFound.Error()):
			c.logger.Debug().Uint64("height", height).Msg("No blobs found at height")
			blockTime, err := c.resolveBlockTimestamp(ctx, height, strictTimestamp)
			if err != nil {
				c.logger.Error().Uint64("height", height).Err(err).Msg("failed to get block timestamp")
				return datypes.ResultRetrieve{
					BaseResult: datypes.BaseResult{
						Code:    datypes.StatusError,
						Message: fmt.Sprintf("failed to get block timestamp: %s", err.Error()),
						Height:  height,
					},
				}
			}

			return datypes.ResultRetrieve{
				BaseResult: datypes.BaseResult{
					Code:      datypes.StatusNotFound,
					Message:   datypes.ErrBlobNotFound.Error(),
					Height:    height,
					Timestamp: blockTime,
				},
			}
		case strings.Contains(err.Error(), datypes.ErrHeightFromFuture.Error()):
			return datypes.ResultRetrieve{
				BaseResult: datypes.BaseResult{
					Code:      datypes.StatusHeightFromFuture,
					Message:   datypes.ErrHeightFromFuture.Error(),
					Height:    height,
					Timestamp: time.Now(),
				},
			}
		default:
			c.logger.Error().Uint64("height", height).Err(err).Msg("failed to get blobs")
			return datypes.ResultRetrieve{
				BaseResult: datypes.BaseResult{
					Code:      datypes.StatusError,
					Message:   fmt.Sprintf("failed to get blobs: %s", err.Error()),
					Height:    height,
					Timestamp: time.Now(),
				},
			}
		}
	}

	blockTime, err := c.resolveBlockTimestamp(ctx, height, strictTimestamp)
	if err != nil {
		c.logger.Error().Uint64("height", height).Err(err).Msg("failed to get block timestamp")
		return datypes.ResultRetrieve{
			BaseResult: datypes.BaseResult{
				Code:    datypes.StatusError,
				Message: fmt.Sprintf("failed to get block timestamp: %s", err.Error()),
				Height:  height,
			},
		}
	}

	if len(blobs) == 0 {
		c.logger.Debug().Uint64("height", height).Msg("No blobs found at height")
		return datypes.ResultRetrieve{
			BaseResult: datypes.BaseResult{
				Code:      datypes.StatusNotFound,
				Message:   datypes.ErrBlobNotFound.Error(),
				Height:    height,
				Timestamp: blockTime,
			},
		}
	}

	// Extract IDs and data from the blobs.
	ids := make([]datypes.ID, len(blobs))
	data := make([]datypes.Blob, len(blobs))
	for i, b := range blobs {
		ids[i] = blobrpc.MakeID(height, b.Commitment)
		data[i] = b.Data()
	}

	c.logger.Debug().Int("num_blobs", len(blobs)).Msg("retrieved blobs")

	return datypes.ResultRetrieve{
		BaseResult: datypes.BaseResult{
			Code:      datypes.StatusSuccess,
			Height:    height,
			IDs:       ids,
			Timestamp: blockTime,
		},
		Data: data,
	}
}

// GetLatestDAHeight returns the latest height available on the DA layer by
// querying the network head.
func (c *client) GetLatestDAHeight(ctx context.Context) (uint64, error) {
	headCtx, cancel := context.WithTimeout(ctx, c.defaultTimeout)
	defer cancel()

	header, err := c.headerAPI.NetworkHead(headCtx)
	if err != nil {
		return 0, fmt.Errorf("failed to get DA network head: %w", err)
	}
	if header == nil {
		return 0, fmt.Errorf("DA network head returned nil header")
	}

	return header.Height, nil
}

// RetrieveForcedInclusion retrieves blobs from the forced inclusion namespace at the specified height.
func (c *client) RetrieveForcedInclusion(ctx context.Context, height uint64) datypes.ResultRetrieve {
	if !c.hasForcedNamespace {
		return datypes.ResultRetrieve{
			BaseResult: datypes.BaseResult{
				Code:    datypes.StatusError,
				Message: "forced inclusion namespace not configured",
				Height:  height,
			},
		}
	}
	return c.Retrieve(ctx, height, c.forcedNamespaceBz)
}

// GetHeaderNamespace returns the header namespace bytes.
func (c *client) GetHeaderNamespace() []byte {
	return c.namespaceBz
}

// GetDataNamespace returns the data namespace bytes.
func (c *client) GetDataNamespace() []byte {
	return c.dataNamespaceBz
}

// GetForcedInclusionNamespace returns the forced inclusion namespace bytes.
func (c *client) GetForcedInclusionNamespace() []byte {
	return c.forcedNamespaceBz
}

// HasForcedInclusionNamespace reports whether forced inclusion namespace is configured.
func (c *client) HasForcedInclusionNamespace() bool {
	return c.hasForcedNamespace
}

// SupportsSubscribe reports whether the underlying transport supports
// channel-based subscriptions (WebSocket). Reads the live IsWebSocket flag
// from the jsonrpc client so transport upgrades are visible immediately.
func (c *client) SupportsSubscribe() bool {
	return c.da != nil && c.da.IsWebSocket.Load()
}

// Subscribe subscribes to blobs in the given namespace via the celestia-node
// Subscribe API. It returns a channel that emits a SubscriptionEvent for every
// DA block containing a matching blob. The channel is closed when ctx is
// cancelled; the underlying jsonrpc subscription channel is drained on exit so
// its delivery goroutine is never left blocked on send.
// Timestamps are included from the header if available (celestia-node v0.29.1+§), otherwise
// fetched via a separate call when includeTimestamp is true. Be aware that fetching timestamps
// separately is an additional call to the celestia node for each event.
func (c *client) Subscribe(ctx context.Context, namespace []byte, includeTimestamp bool) (<-chan datypes.SubscriptionEvent, error) {
	ns, err := share.NewNamespaceFromBytes(namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace: %w", err)
	}

	rawCh, err := c.blobAPI.Subscribe(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("blob subscribe: %w", err)
	}

	out := make(chan datypes.SubscriptionEvent, 16)
	go func() {
		defer close(out)
		// The jsonrpc layer closes rawCh only after its pending send is
		// consumed and the ctx cancellation is processed. Drain it on exit
		// so the delivery goroutine is not leaked on reconnect.
		defer func() {
			for range rawCh { //nolint:revive // draining until producer closes the channel
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-rawCh:
				if !ok {
					return
				}
				if resp == nil {
					continue
				}
				var blockTime time.Time
				// Use header time if available (celestia-node v0.21.0+)
				if resp.Header != nil && !resp.Header.Time.IsZero() {
					blockTime = resp.Header.Time.UTC()
					c.storeBlockTimestamp(resp.Height, blockTime)
				} else if includeTimestamp {
					// Fallback to fetching timestamp for older nodes
					blockTime, err = c.getBlockTimestamp(ctx, resp.Height)
					if err != nil {
						c.logger.Error().Uint64("height", resp.Height).Err(err).Msg("failed to get DA block timestamp for subscription event")
						blockTime = time.Time{}
					}
				}
				select {
				case out <- datypes.SubscriptionEvent{
					Height:    resp.Height,
					Timestamp: blockTime,
					Blobs:     extractBlobData(resp),
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// extractBlobData extracts raw byte slices from a subscription response,
// filtering out nil blobs, empty data, and blobs exceeding DefaultMaxBlobSize.
func extractBlobData(resp *blobrpc.SubscriptionResponse) [][]byte {
	if resp == nil || len(resp.Blobs) == 0 {
		return nil
	}
	blobs := make([][]byte, 0, len(resp.Blobs))
	for _, blob := range resp.Blobs {
		if blob == nil {
			continue
		}
		data := blob.Data()
		if len(data) == 0 || uint64(len(data)) > blobsize.DefaultMaxBlobSize {
			continue
		}
		blobs = append(blobs, data)
	}
	return blobs
}

// Get fetches blobs by their IDs. Used for visualization and fetching specific blobs.
func (c *client) Get(ctx context.Context, ids []datypes.ID, namespace []byte) ([]datypes.Blob, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	ns, err := share.NewNamespaceFromBytes(namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace: %w", err)
	}

	getCtx, cancel := context.WithTimeout(ctx, c.defaultTimeout)
	defer cancel()

	res := make([]datypes.Blob, 0, len(ids))
	for _, id := range ids {
		height, commitment := blobrpc.SplitID(id)
		if commitment == nil {
			return nil, fmt.Errorf("invalid blob id: %x", id)
		}

		b, err := c.blobAPI.Get(getCtx, height, ns, commitment)
		if err != nil {
			return nil, err
		}
		if b == nil {
			continue
		}
		res = append(res, b.Data())
	}

	return res, nil
}

// GetProofs returns inclusion proofs for the provided IDs.
func (c *client) GetProofs(ctx context.Context, ids []datypes.ID, namespace []byte) ([]datypes.Proof, error) {
	if len(ids) == 0 {
		return []datypes.Proof{}, nil
	}

	ns, err := share.NewNamespaceFromBytes(namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace: %w", err)
	}

	getCtx, cancel := context.WithTimeout(ctx, c.defaultTimeout)
	defer cancel()

	proofs := make([]datypes.Proof, len(ids))
	for i, id := range ids {
		height, commitment := blobrpc.SplitID(id)
		if commitment == nil {
			return nil, fmt.Errorf("invalid blob id: %x", id)
		}

		proof, err := c.blobAPI.GetProof(getCtx, height, ns, commitment)
		if err != nil {
			return nil, err
		}

		bz, err := json.Marshal(proof)
		if err != nil {
			return nil, err
		}
		proofs[i] = bz
	}

	return proofs, nil
}

// Validate validates commitments against the corresponding proofs.
func (c *client) Validate(ctx context.Context, ids []datypes.ID, proofs []datypes.Proof, namespace []byte) ([]bool, error) {
	if len(ids) != len(proofs) {
		return nil, errors.New("number of IDs and proofs must match")
	}

	if len(ids) == 0 {
		return []bool{}, nil
	}

	ns, err := share.NewNamespaceFromBytes(namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid namespace: %w", err)
	}

	getCtx, cancel := context.WithTimeout(ctx, c.defaultTimeout)
	defer cancel()

	results := make([]bool, len(ids))
	for i, id := range ids {
		var proof blobrpc.Proof
		if err := json.Unmarshal(proofs[i], &proof); err != nil {
			return nil, err
		}

		height, commitment := blobrpc.SplitID(id)
		if commitment == nil {
			return nil, fmt.Errorf("invalid blob id: %x", id)
		}

		included, err := c.blobAPI.Included(getCtx, height, ns, &proof, commitment)
		if err != nil {
			c.logger.Debug().Err(err).Uint64("height", height).Msg("blob inclusion check returned error")
		}
		results[i] = included
	}

	return results, nil
}
