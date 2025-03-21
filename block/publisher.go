package block

import (
	"context"
	"fmt"
	"time"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
)

// Publisher is responsible for publishing blocks to P2P and DA
type Publisher struct {
	eventBus        *events.Bus
	store           store.Store
	dalc            coreda.Client
	headerCache     *HeaderCache
	dataCache       *DataCache
	pendingHeaders  *PendingHeaders
	logger          Logger
	config          config.Config
	daSubmitBackoff time.Duration
	gasPrice        float64
	gasMultiplier   float64
}

// PublisherOptions contains options for creating a new Publisher
type PublisherOptions struct {
	EventBus       *events.Bus
	Store          store.Store
	DALC           coreda.Client
	Logger         Logger
	Config         config.Config
	GasPrice       float64
	GasMultiplier  float64
	PendingHeaders *PendingHeaders
}

// NewPublisher creates a new block publisher
func NewPublisher(opts PublisherOptions) (*Publisher, error) {
	return &Publisher{
		eventBus:        opts.EventBus,
		store:           opts.Store,
		dalc:            opts.DALC,
		headerCache:     NewHeaderCache(),
		dataCache:       NewDataCache(),
		pendingHeaders:  opts.PendingHeaders,
		logger:          opts.Logger,
		config:          opts.Config,
		daSubmitBackoff: initialBackoff,
		gasPrice:        opts.GasPrice,
		gasMultiplier:   opts.GasMultiplier,
	}, nil
}

// Start starts the publisher
func (p *Publisher) Start(ctx context.Context) error {
	// Subscribe to events
	p.eventBus.Subscribe(EventBlockCreated, p.handleBlockCreated)

	return nil
}

// SetDALC sets the DA layer client
func (p *Publisher) SetDALC(dalc coreda.Client) {
	p.dalc = dalc
}

// handleBlockCreated handles BlockCreatedEvent
func (p *Publisher) handleBlockCreated(ctx context.Context, evt events.Event) {
	blockEvent, ok := evt.(BlockCreatedEvent)
	if !ok {
		p.logger.Error("invalid event type", "expected", "BlockCreatedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	header := blockEvent.Header
	data := blockEvent.Data
	height := blockEvent.Height

	// Save block data
	if err := p.store.SaveBlockData(ctx, header, data, &header.Signature); err != nil {
		p.logger.Error("failed to save block data", "error", err)
		return
	}

	// Mark block as seen in cache
	headerHash := header.Hash().String()
	p.headerCache.SetSeen(headerHash)
	p.dataCache.SetSeen(data.Hash().String())

	// Publish to P2P network
	p.eventBus.Publish(BlockPublishedEvent{
		Header: header,
		Data:   data,
		Height: height,
	})

	p.logger.Info("Block published to P2P", "height", height, "hash", headerHash)
}

// HeaderSubmissionLoop is responsible for submitting headers to DA
func (p *Publisher) HeaderSubmissionLoop(ctx context.Context) {
	// Ensure we have a positive duration to avoid panic
	blockTimeDuration := p.config.DA.BlockTime.Duration
	if blockTimeDuration <= 0 {
		blockTimeDuration = time.Second * 5 // Default to 5 seconds if configuration is invalid
		p.logger.Error("Invalid block time duration (must be positive), using default value", "default", blockTimeDuration)
	}
	timer := time.NewTicker(blockTimeDuration)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if p.pendingHeaders.isEmpty() {
			continue
		}

		if err := p.submitHeadersToDA(ctx); err != nil {
			p.logger.Error("error while submitting block to DA", "error", err)
		}
	}
}

// submitHeadersToDA submits headers to the DA layer
func (p *Publisher) submitHeadersToDA(ctx context.Context) error {
	submittedAllHeaders := false
	headersToSubmit, err := p.pendingHeaders.getPendingHeaders(ctx)

	if len(headersToSubmit) == 0 {
		// There are no pending headers; return because there's nothing to do, but:
		// - it might be caused by error, then err != nil
		// - all pending headers are processed, then err == nil
		// whatever the reason, error information is propagated correctly to the caller
		return err
	}

	if err != nil {
		// There are some pending blocks but also an error. It's very unlikely case - probably some error while reading
		// headers from the store.
		// The error is logged and normal processing of pending blocks continues.
		p.logger.Error("error while fetching blocks pending DA", "err", err)
	}

	numSubmittedHeaders := 0
	attempt := 0
	maxBlobSize, err := p.dalc.MaxBlobSize(ctx)
	if err != nil {
		return err
	}
	// allow buffer for the block header and protocol encoding
	maxBlobSize -= blockProtocolOverhead

	initialMaxBlobSize := maxBlobSize
	gasPrice := p.gasPrice
	initialGasPrice := gasPrice

daSubmitRetryLoop:
	for !submittedAllHeaders && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(p.daSubmitBackoff):
		}

		headersBz := make([][]byte, len(headersToSubmit))
		for i, header := range headersToSubmit {
			headerPb, err := header.ToProto()
			if err != nil {
				// do we drop the header from attempting to be submitted?
				return fmt.Errorf("failed to transform header to proto: %w", err)
			}
			headersBz[i], err = headerPb.Marshal()
			if err != nil {
				// do we drop the header from attempting to be submitted?
				return fmt.Errorf("failed to marshal header: %w", err)
			}
		}

		submitCtx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
		defer cancel()

		res := p.dalc.Submit(submitCtx, headersBz, maxBlobSize, gasPrice)

		switch res.Code {
		case coreda.StatusSuccess:
			p.logger.Info("successfully submitted Rollkit headers to DA layer",
				"gasPrice", gasPrice,
				"daHeight", res.Height,
				"headerCount", res.SubmittedCount)

			if res.SubmittedCount == uint64(len(headersToSubmit)) {
				submittedAllHeaders = true
			}

			submittedBlocks, notSubmittedBlocks := headersToSubmit[:res.SubmittedCount], headersToSubmit[res.SubmittedCount:]
			numSubmittedHeaders += len(submittedBlocks)

			for _, block := range submittedBlocks {
				blockHash := block.Hash().String()
				p.headerCache.SetDAIncluded(blockHash)

				// Publish event for DA included block
				p.eventBus.Publish(BlockDAIncludedEvent{
					Header:   block,
					Height:   block.Height(),
					DAHeight: res.Height,
				})
			}

			lastSubmittedHeight := uint64(0)
			if l := len(submittedBlocks); l > 0 {
				lastSubmittedHeight = submittedBlocks[l-1].Height()
			}

			p.pendingHeaders.setLastSubmittedHeight(ctx, lastSubmittedHeight)
			headersToSubmit = notSubmittedBlocks

			// reset submission options when successful
			// scale back gasPrice gradually
			p.daSubmitBackoff = 0
			maxBlobSize = initialMaxBlobSize

			if p.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / p.gasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}

			p.logger.Debug("resetting DA layer submission options",
				"backoff", p.daSubmitBackoff,
				"gasPrice", gasPrice,
				"maxBlobSize", maxBlobSize)

		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			p.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			p.daSubmitBackoff = p.config.DA.BlockTime.Duration * time.Duration(p.config.DA.MempoolTTL)

			if p.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * p.gasMultiplier
			}

			p.logger.Info("retrying DA layer submission with",
				"backoff", p.daSubmitBackoff,
				"gasPrice", gasPrice,
				"maxBlobSize", maxBlobSize)

		case coreda.StatusTooBig:
			maxBlobSize = maxBlobSize / 4
			fallthrough

		default:
			p.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			p.daSubmitBackoff = p.exponentialBackoff(p.daSubmitBackoff)
		}

		attempt += 1
	}

	if !submittedAllHeaders {
		return fmt.Errorf(
			"failed to submit all blocks to DA layer, submitted %d blocks (%d left) after %d attempts",
			numSubmittedHeaders,
			len(headersToSubmit),
			attempt,
		)
	}

	return nil
}

// exponentialBackoff implements exponential backoff strategy
func (p *Publisher) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff == 0 {
		backoff = initialBackoff
	}
	if backoff > p.config.DA.BlockTime.Duration {
		backoff = p.config.DA.BlockTime.Duration
	}
	return backoff
}
