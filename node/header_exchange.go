package node

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/celestiaorg/go-header"
	goheaderp2p "github.com/celestiaorg/go-header/p2p"
	goheaderstore "github.com/celestiaorg/go-header/store"
	"github.com/celestiaorg/go-header/sync"
	goheadersync "github.com/celestiaorg/go-header/sync"
	ds "github.com/ipfs/go-datastore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"go.uber.org/multierr"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/types"
)

type HeaderExchangeService struct {
	conf            config.NodeConfig
	genesis         *tmtypes.GenesisDoc
	p2p             *p2p.Client
	ex              *goheaderp2p.Exchange[*types.Header]
	syncer          *sync.Syncer[*types.Header]
	sub             *goheaderp2p.Subscriber[*types.Header]
	p2pServer       *goheaderp2p.ExchangeServer[*types.Header]
	headerStore     *goheaderstore.Store[*types.Header]
	syncerStarted   bool
	syncedHeadersCh chan *types.SignedHeader

	logger log.Logger
	ctx    context.Context
}

func NewHeaderExchangeService(ctx context.Context, store ds.TxnDatastore, conf config.NodeConfig, genesis *tmtypes.GenesisDoc, p2p *p2p.Client, syncedHeadersCh chan *types.SignedHeader, logger log.Logger) (*HeaderExchangeService, error) {
	// store is TxnDatastore, but we require Batching, hence the type assertion
	// note, the badger datastore impl that is used in the background implements both
	storeBatch, ok := store.(ds.Batching)
	if !ok {
		return nil, errors.New("failed to access the datastore")
	}
	ss, err := goheaderstore.NewStore[*types.Header](storeBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the header store: %w", err)
	}

	return &HeaderExchangeService{
		conf:            conf,
		genesis:         genesis,
		p2p:             p2p,
		ctx:             ctx,
		headerStore:     ss,
		syncedHeadersCh: syncedHeadersCh,
		logger:          logger,
	}, nil
}

func (hExService *HeaderExchangeService) initOrAppendHeaderStore(ctx context.Context, header *types.Header) error {
	var err error

	// Init the header store if first block, else append to store
	if header.Height() == hExService.genesis.InitialHeight {
		err = hExService.headerStore.Init(ctx, header)
	} else {
		_, err = hExService.headerStore.Append(ctx, header)
	}
	return err
}

func (hExService *HeaderExchangeService) initHeaderStoreAndStartSyncer(ctx context.Context, initial *types.Header) error {
	if err := hExService.headerStore.Init(ctx, initial); err != nil {
		return err
	}
	if err := hExService.syncer.Start(hExService.ctx); err != nil {
		return err
	}
	hExService.syncerStarted = true
	return nil
}

func (hExService *HeaderExchangeService) tryInitHeaderStoreAndStartSyncer(ctx context.Context, trustedHeader *types.Header) {
	if trustedHeader != nil {
		if err := hExService.initHeaderStoreAndStartSyncer(ctx, trustedHeader); err != nil {
			hExService.logger.Error("failed to initialize the headerstore and start syncer", "error", err)
		}
	} else {
		signedHeader := <-hExService.syncedHeadersCh
		if signedHeader.Header.Height() == hExService.genesis.InitialHeight {
			if err := hExService.initHeaderStoreAndStartSyncer(ctx, &signedHeader.Header); err != nil {
				hExService.logger.Error("failed to initialize the headerstore and start syncer", "error", err)
			}
		}
	}
}

func (hExService *HeaderExchangeService) writeToHeaderStoreAndBroadcast(ctx context.Context, signedHeader *types.SignedHeader) {
	// Init the header store if first block, else append to store
	if err := hExService.initOrAppendHeaderStore(ctx, &signedHeader.Header); err != nil {
		hExService.logger.Error("failed to write block header to header store", "error", err)
	}

	// Broadcast for subscribers
	if err := hExService.sub.Broadcast(ctx, &signedHeader.Header); err != nil {
		hExService.logger.Error("failed to broadcast block header", "error", err)
	}
}

// OnStart is a part of Service interface.
func (hExService *HeaderExchangeService) Start() error {
	var err error
	// have to do the initializations here to utilize the p2p node which is created on start
	ps := hExService.p2p.PubSub()
	hExService.sub = goheaderp2p.NewSubscriber[*types.Header](ps, pubsub.DefaultMsgIdFn)
	if err = hExService.sub.Start(hExService.ctx); err != nil {
		return fmt.Errorf("error while starting subscriber: %w", err)
	}
	if _, err := hExService.sub.Subscribe(); err != nil {
		return fmt.Errorf("error while subscribing: %w", err)
	}

	if err = hExService.headerStore.Start(hExService.ctx); err != nil {
		return fmt.Errorf("error while starting header store: %w", err)
	}

	_, _, network := hExService.p2p.Info()
	if hExService.p2pServer, err = newP2PServer(hExService.p2p.Host(), hExService.headerStore, network); err != nil {
		return err
	}
	if err = hExService.p2pServer.Start(hExService.ctx); err != nil {
		return fmt.Errorf("error while starting p2p server: %w", err)
	}

	peerIDs := hExService.p2p.PeerIDs()
	if hExService.ex, err = newP2PExchange(hExService.p2p.Host(), peerIDs, network, hExService.p2p.ConnectionGater()); err != nil {
		return err
	}
	if err = hExService.ex.Start(hExService.ctx); err != nil {
		return fmt.Errorf("error while starting exchange: %w", err)
	}

	// for single aggregator configuration, syncer is not needed
	// TODO (ganesh): design syncer flow for multiple aggregator scenario
	if hExService.conf.Aggregator {
		return nil
	}

	if hExService.syncer, err = newSyncer(hExService.ex, hExService.headerStore, hExService.sub, goheadersync.WithBlockTime(hExService.conf.BlockTime)); err != nil {
		return err
	}

	// Check if the headerstore is not initialized and try initializing
	if hExService.headerStore.Height() > 0 {
		if err := hExService.syncer.Start(hExService.ctx); err != nil {
			return fmt.Errorf("error while starting the syncer: %w", err)
		}
		hExService.syncerStarted = true
		return nil
	}

	// Look to see if trusted hash is passed, if not get the genesis header
	var trustedHeader *types.Header
	// Try fetching the trusted header from peers if exists
	if len(peerIDs) > 0 {
		if hExService.conf.TrustedHash != "" {
			trustedHashBytes, err := hex.DecodeString(hExService.conf.TrustedHash)
			if err != nil {
				return fmt.Errorf("failed to parse the trusted hash for initializing the headerstore: %w", err)
			}

			if trustedHeader, err = hExService.ex.Get(hExService.ctx, header.Hash(trustedHashBytes)); err != nil {
				return fmt.Errorf("failed to fetch the trusted header for initializing the headerstore: %w", err)
			}
		} else {
			// Try fetching the genesis header if available, otherwise fallback to signed headers
			if trustedHeader, err = hExService.ex.GetByHeight(hExService.ctx, uint64(hExService.genesis.InitialHeight)); err != nil {
				// Full/light nodes have to wait for aggregator to publish the genesis header
				// if the aggregator is passed as seed while starting the fullnode
				return fmt.Errorf("failed to fetch the genesis header: %w", err)
			}
		}
	}
	go hExService.tryInitHeaderStoreAndStartSyncer(hExService.ctx, trustedHeader)

	return nil
}

// OnStop is a part of Service interface.
func (hExService *HeaderExchangeService) Stop() error {
	err := hExService.headerStore.Stop(hExService.ctx)
	err = multierr.Append(err, hExService.p2pServer.Stop(hExService.ctx))
	err = multierr.Append(err, hExService.ex.Stop(hExService.ctx))
	err = multierr.Append(err, hExService.sub.Stop(hExService.ctx))
	if !hExService.conf.Aggregator && hExService.syncerStarted {
		err = multierr.Append(err, hExService.syncer.Stop(hExService.ctx))
	}
	return err
}

// newP2PServer constructs a new ExchangeServer using the given Network as a protocolID suffix.
func newP2PServer(
	host host.Host,
	store *goheaderstore.Store[*types.Header],
	network string,
	opts ...goheaderp2p.Option[goheaderp2p.ServerParameters],
) (*goheaderp2p.ExchangeServer[*types.Header], error) {
	return goheaderp2p.NewExchangeServer[*types.Header](host, store, network, opts...)
}

func newP2PExchange(
	host host.Host,
	peers []peer.ID,
	network string,
	conngater *conngater.BasicConnectionGater,
	opts ...goheaderp2p.Option[goheaderp2p.ClientParameters],
) (*goheaderp2p.Exchange[*types.Header], error) {
	return goheaderp2p.NewExchange[*types.Header](host, peers, network, conngater, opts...)
}

// newSyncer constructs new Syncer for headers.
func newSyncer(
	ex header.Exchange[*types.Header],
	store header.Store[*types.Header],
	sub header.Subscriber[*types.Header],
	opt goheadersync.Options,
) (*sync.Syncer[*types.Header], error) {
	return sync.NewSyncer(ex, store, sub, opt)
}
