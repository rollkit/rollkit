package block

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/celestiaorg/go-header"
	goheaderp2p "github.com/celestiaorg/go-header/p2p"
	goheaderstore "github.com/celestiaorg/go-header/store"
	goheadersync "github.com/celestiaorg/go-header/sync"
	"github.com/cometbft/cometbft/libs/log"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/types"
)

// HeaderSyncService is the P2P Sync Service for header that implements the
// go-header interface.
// Contains a header store where synced headers are stored.
// Uses the go-header library for handling all P2P logic.
type HeaderSyncService struct {
	conf        config.NodeConfig
	genesis     *cmtypes.GenesisDoc
	p2p         *p2p.Client
	ex          *goheaderp2p.Exchange[*types.SignedHeader]
	sub         *goheaderp2p.Subscriber[*types.SignedHeader]
	p2pServer   *goheaderp2p.ExchangeServer[*types.SignedHeader]
	headerStore *goheaderstore.Store[*types.SignedHeader]

	syncer       *goheadersync.Syncer[*types.SignedHeader]
	syncerStatus *SyncerStatus

	logger log.Logger
	ctx    context.Context
}

// NewHeaderSyncService returns a new HeaderSyncService.
func NewHeaderSyncService(ctx context.Context, store ds.TxnDatastore, conf config.NodeConfig, genesis *cmtypes.GenesisDoc, p2p *p2p.Client, logger log.Logger) (*HeaderSyncService, error) {
	if genesis == nil {
		return nil, errors.New("genesis doc cannot be nil")
	}
	if p2p == nil {
		return nil, errors.New("p2p client cannot be nil")
	}
	// store is TxnDatastore, but we require Batching, hence the type assertion
	// note, the badger datastore impl that is used in the background implements both
	storeBatch, ok := store.(ds.Batching)
	if !ok {
		return nil, errors.New("failed to access the datastore")
	}
	ss, err := goheaderstore.NewStore[*types.SignedHeader](
		storeBatch,
		goheaderstore.WithStorePrefix("headerSync"),
		goheaderstore.WithMetrics(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the header store: %w", err)
	}

	return &HeaderSyncService{
		conf:         conf,
		genesis:      genesis,
		p2p:          p2p,
		ctx:          ctx,
		headerStore:  ss,
		logger:       logger,
		syncerStatus: new(SyncerStatus),
	}, nil
}

// HeaderStore returns the headerstore of the HeaderSynceService
func (hSyncService *HeaderSyncService) HeaderStore() *goheaderstore.Store[*types.SignedHeader] {
	return hSyncService.headerStore
}

func (hSyncService *HeaderSyncService) initHeaderStoreAndStartSyncer(ctx context.Context, initial *types.SignedHeader) error {
	if initial == nil {
		return fmt.Errorf("failed to initialize the headerstore and start syncer")
	}
	if err := hSyncService.headerStore.Init(ctx, initial); err != nil {
		return err
	}
	if err := hSyncService.StartSyncer(); err != nil {
		return err
	}
	return nil
}

// WriteToHeaderStoreAndBroadcast initializes header store if needed and broadcasts provided header.
// Note: Only returns an error in case header store can't be initialized. Logs error if there's one while broadcasting.
func (hSyncService *HeaderSyncService) WriteToHeaderStoreAndBroadcast(ctx context.Context, signedHeader *types.SignedHeader) error {
	if hSyncService.genesis.InitialHeight < 0 {
		return fmt.Errorf("invalid initial height; cannot be negative")
	}
	isGenesis := signedHeader.Height() == uint64(hSyncService.genesis.InitialHeight)
	// For genesis header initialize the store and start the syncer
	if isGenesis {
		if err := hSyncService.headerStore.Init(ctx, signedHeader); err != nil {
			return fmt.Errorf("failed to initialize header store")
		}

		if err := hSyncService.StartSyncer(); err != nil {
			return fmt.Errorf("failed to start syncer after initializing header store: %w", err)
		}
	}

	// Broadcast for subscribers
	if err := hSyncService.sub.Broadcast(ctx, signedHeader); err != nil {
		// for the genesis header, broadcast error is expected as we have already initialized the store
		// for starting the syncer. Hence, we ignore the error.
		// exact reason: validation failed, err header verification failed: known header: '1' <= current '1'
		if isGenesis && errors.Is(err, pubsub.ValidationError{Reason: pubsub.RejectValidationFailed}) {
			return nil
		}
		return fmt.Errorf("failed to broadcast block header: %w", err)
	}
	return nil
}

func (hSyncService *HeaderSyncService) isInitialized() bool {
	return hSyncService.headerStore.Height() > 0
}

// Start is a part of Service interface.
func (hSyncService *HeaderSyncService) Start() error {
	// have to do the initializations here to utilize the p2p node which is created on start
	ps := hSyncService.p2p.PubSub()

	var err error
	hSyncService.sub, err = goheaderp2p.NewSubscriber[*types.SignedHeader](
		ps,
		pubsub.DefaultMsgIdFn,
		goheaderp2p.WithSubscriberNetworkID(hSyncService.genesis.ChainID),
		goheaderp2p.WithSubscriberMetrics(),
	)
	if err != nil {
		return err
	}

	if err := hSyncService.sub.Start(hSyncService.ctx); err != nil {
		return fmt.Errorf("error while starting subscriber: %w", err)
	}
	if _, err := hSyncService.sub.Subscribe(); err != nil {
		return fmt.Errorf("error while subscribing: %w", err)
	}

	if err := hSyncService.headerStore.Start(hSyncService.ctx); err != nil {
		return fmt.Errorf("error while starting header store: %w", err)
	}

	_, _, network, err := hSyncService.p2p.Info()
	if err != nil {
		return fmt.Errorf("error while fetching the network: %w", err)
	}
	if hSyncService.p2pServer, err = newP2PServer(hSyncService.p2p.Host(), hSyncService.headerStore, network); err != nil {
		return fmt.Errorf("error while creating p2p server: %w", err)
	}
	if err := hSyncService.p2pServer.Start(hSyncService.ctx); err != nil {
		return fmt.Errorf("error while starting p2p server: %w", err)
	}

	peerIDs := hSyncService.p2p.PeerIDs()
	if hSyncService.ex, err = newP2PExchange(hSyncService.p2p.Host(), peerIDs, network, hSyncService.genesis.ChainID, hSyncService.p2p.ConnectionGater()); err != nil {
		return fmt.Errorf("error while creating exchange: %w", err)
	}
	if err := hSyncService.ex.Start(hSyncService.ctx); err != nil {
		return fmt.Errorf("error while starting exchange: %w", err)
	}

	if hSyncService.syncer, err = newSyncer(
		hSyncService.ex,
		hSyncService.headerStore,
		hSyncService.sub,
		[]goheadersync.Option{goheadersync.WithBlockTime(hSyncService.conf.BlockTime)},
	); err != nil {
		return fmt.Errorf("error while creating syncer: %w", err)
	}

	if hSyncService.isInitialized() {
		if err := hSyncService.StartSyncer(); err != nil {
			return fmt.Errorf("error while starting the syncer: %w", err)
		}
		return nil
	}

	// Look to see if trusted hash is passed, if not get the genesis header
	var trustedHeader *types.SignedHeader
	// Try fetching the trusted header from peers if exists
	if len(peerIDs) > 0 {
		if hSyncService.conf.TrustedHash != "" {
			trustedHashBytes, err := hex.DecodeString(hSyncService.conf.TrustedHash)
			if err != nil {
				return fmt.Errorf("failed to parse the trusted hash for initializing the headerstore: %w", err)
			}

			if trustedHeader, err = hSyncService.ex.Get(hSyncService.ctx, header.Hash(trustedHashBytes)); err != nil {
				return fmt.Errorf("failed to fetch the trusted header for initializing the headerstore: %w", err)
			}
		} else {
			// Try fetching the genesis header if available, otherwise fallback to signed headers
			if trustedHeader, err = hSyncService.ex.GetByHeight(hSyncService.ctx, uint64(hSyncService.genesis.InitialHeight)); err != nil {
				// Full/light nodes have to wait for aggregator to publish the genesis header
				// proposing aggregator can init the store and start the syncer when the first header is published
				return fmt.Errorf("failed to fetch the genesis header: %w", err)
			}
		}
		return hSyncService.initHeaderStoreAndStartSyncer(hSyncService.ctx, trustedHeader)

	}
	return nil
}

// Stop is a part of Service interface.
//
// `headerStore` is closed last because it's used by other services.
func (hSyncService *HeaderSyncService) Stop() error {
	err := errors.Join(
		hSyncService.p2pServer.Stop(hSyncService.ctx),
		hSyncService.ex.Stop(hSyncService.ctx),
		hSyncService.sub.Stop(hSyncService.ctx),
	)
	if hSyncService.syncerStatus.isStarted() {
		err = errors.Join(err, hSyncService.syncer.Stop(hSyncService.ctx))
	}
	err = errors.Join(err, hSyncService.headerStore.Stop(hSyncService.ctx))
	return err
}

// newP2PServer constructs a new ExchangeServer using the given Network as a protocolID suffix.
func newP2PServer(
	host host.Host,
	store *goheaderstore.Store[*types.SignedHeader],
	network string,
	opts ...goheaderp2p.Option[goheaderp2p.ServerParameters],
) (*goheaderp2p.ExchangeServer[*types.SignedHeader], error) {
	opts = append(opts,
		goheaderp2p.WithNetworkID[goheaderp2p.ServerParameters](network),
		goheaderp2p.WithMetrics[goheaderp2p.ServerParameters](),
	)
	return goheaderp2p.NewExchangeServer[*types.SignedHeader](host, store, opts...)
}

func newP2PExchange(
	host host.Host,
	peers []peer.ID,
	network, chainID string,
	conngater *conngater.BasicConnectionGater,
	opts ...goheaderp2p.Option[goheaderp2p.ClientParameters],
) (*goheaderp2p.Exchange[*types.SignedHeader], error) {
	opts = append(opts,
		goheaderp2p.WithNetworkID[goheaderp2p.ClientParameters](network),
		goheaderp2p.WithChainID(chainID),
		goheaderp2p.WithMetrics[goheaderp2p.ClientParameters](),
	)

	return goheaderp2p.NewExchange[*types.SignedHeader](host, peers, conngater, opts...)
}

// newSyncer constructs new Syncer for headers.
func newSyncer(
	ex header.Exchange[*types.SignedHeader],
	store header.Store[*types.SignedHeader],
	sub header.Subscriber[*types.SignedHeader],
	opts []goheadersync.Option,
) (*goheadersync.Syncer[*types.SignedHeader], error) {
	opts = append(opts,
		goheadersync.WithMetrics(),
	)
	return goheadersync.NewSyncer[*types.SignedHeader](ex, store, sub, opts...)
}

// StartSyncer starts the HeaderSyncService's syncer
func (hSyncService *HeaderSyncService) StartSyncer() error {
	if hSyncService.syncerStatus.isStarted() {
		return nil
	}
	err := hSyncService.syncer.Start(hSyncService.ctx)
	if err != nil {
		return err
	}
	hSyncService.syncerStatus.started.Store(true)
	return nil
}
