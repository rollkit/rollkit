package node
import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"

	abciclient "github.com/tendermint/tendermint/abci/client"
	llcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/rollmint/block"
	"github.com/celestiaorg/rollmint/config"
	"github.com/celestiaorg/rollmint/da"
	"github.com/celestiaorg/rollmint/da/registry"
	"github.com/celestiaorg/rollmint/mempool"
	mempoolv1 "github.com/celestiaorg/rollmint/mempool/v1"
	"github.com/celestiaorg/rollmint/p2p"
	"github.com/celestiaorg/rollmint/state/indexer"
	"github.com/celestiaorg/rollmint/state/txindex"
	"github.com/celestiaorg/rollmint/store"
)

type FullNode struct {
	service.BaseService
	eventBus  *tmtypes.EventBus
	appClient abciclient.Client

	genesis *tmtypes.GenesisDoc
	// cache of chunked genesis data.
	genChunks []string

	conf config.NodeConfig
	P2P  *p2p.Client

	// TODO(tzdybal): consider extracting "mempool reactor"
	Mempool      mempool.Mempool
	mempoolIDs   *mempoolIDs
	incomingTxCh chan *p2p.GossipMessage

	Store        store.Store
	blockManager *block.Manager
	dalc         da.DataAvailabilityLayerClient

	TxIndexer      txindex.TxIndexer
	BlockIndexer   indexer.BlockIndexer
	IndexerService *txindex.IndexerService

	// keep context here only because of API compatibility
	// - it's used in `OnStart` (defined in service.Service interface)
	ctx context.Context
}

func newFullNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	appClient abciclient.Client,
	genesis *tmtypes.GenesisDoc,
	logger log.Logger,
) (*FullNode, error) {
	eventBus := tmtypes.NewEventBus()
	eventBus.SetLogger(logger.With("module", "events"))
	if err := eventBus.Start(); err != nil {
		return nil, err
	}

	client, err := p2p.NewClient(conf.P2P, p2pKey, genesis.ChainID, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	var baseKV store.KVStore
	if conf.RootDir == "" && conf.DBPath == "" { // this is used for testing
		logger.Info("WARNING: working in in-memory mode")
		baseKV = store.NewDefaultInMemoryKVStore()
	} else {
		baseKV = store.NewDefaultKVStore(conf.RootDir, conf.DBPath, "rollmint")
	}
	mainKV := store.NewPrefixKV(baseKV, mainPrefix)
	dalcKV := store.NewPrefixKV(baseKV, dalcPrefix)
	indexerKV := store.NewPrefixKV(baseKV, indexerPrefix)

	s := store.New(mainKV)

	dalc := registry.GetClient(conf.DALayer)
	if dalc == nil {
		return nil, fmt.Errorf("couldn't get data availability client named '%s'", conf.DALayer)
	}
	err = dalc.Init(conf.NamespaceID, []byte(conf.DAConfig), dalcKV, logger.With("module", "da_client"))
	if err != nil {
		return nil, fmt.Errorf("data availability layer client initialization error: %w", err)
	}

	indexerService, txIndexer, blockIndexer, err := createAndStartIndexerService(conf, indexerKV, eventBus, logger)
	if err != nil {
		return nil, err
	}

	mp := mempoolv1.NewTxMempool(logger, llcfg.DefaultMempoolConfig(), appClient, 0)
	mpIDs := newMempoolIDs()

	blockManager, err := block.NewManager(signingKey, conf.BlockManagerConfig, genesis, s, mp, appClient, dalc, eventBus, logger.With("module", "BlockManager"))
	if err != nil {
		return nil, fmt.Errorf("BlockManager initialization error: %w", err)
	}

	node := &FullNode{
		appClient:      appClient,
		eventBus:       eventBus,
		genesis:        genesis,
		conf:           conf,
		P2P:            client,
		blockManager:   blockManager,
		dalc:           dalc,
		Mempool:        mp,
		mempoolIDs:     mpIDs,
		incomingTxCh:   make(chan *p2p.GossipMessage),
		Store:          s,
		TxIndexer:      txIndexer,
		IndexerService: indexerService,
		BlockIndexer:   blockIndexer,
		ctx:            ctx,
	}

	node.BaseService = *service.NewBaseService(logger, "Node", node)

	node.P2P.SetTxValidator(node.newTxValidator())
	node.P2P.SetHeaderValidator(node.newHeaderValidator())
	node.P2P.SetCommitValidator(node.newCommitValidator())
	node.P2P.SetFraudProofValidator(node.newFraudProofValidator())

	return node, nil

}
