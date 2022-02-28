package node

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p-core/crypto"
	"go.uber.org/multierr"

	abci "github.com/tendermint/tendermint/abci/types"
	llcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	corep2p "github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/block"
	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/da/registry"
	"github.com/celestiaorg/optimint/mempool"
	"github.com/celestiaorg/optimint/p2p"
	"github.com/celestiaorg/optimint/state/indexer"
	blockidxkv "github.com/celestiaorg/optimint/state/indexer/block/kv"
	"github.com/celestiaorg/optimint/state/txindex"
	"github.com/celestiaorg/optimint/state/txindex/kv"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

// prefixes used in KV store to separate main node data from DALC data
var (
	mainPrefix    = []byte{0}
	dalcPrefix    = []byte{1}
	indexerPrefix = []byte{2}
)

const (
	// genesisChunkSize is the maximum size, in bytes, of each
	// chunk in the genesis structure for the chunked API
	genesisChunkSize = 16 * 1024 * 1024 // 16 MiB
)

// Node represents a client node in Optimint network.
// It connects all the components and orchestrates their work.
type Node struct {
	service.BaseService
	eventBus *tmtypes.EventBus
	proxyApp proxy.AppConns

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

// NewNode creates new Optimint node.
func NewNode(ctx context.Context, conf config.NodeConfig, nodeKey crypto.PrivKey, clientCreator proxy.ClientCreator, genesis *tmtypes.GenesisDoc, logger log.Logger) (*Node, error) {
	proxyApp := proxy.NewAppConns(clientCreator)
	proxyApp.SetLogger(logger.With("module", "proxy"))
	if err := proxyApp.Start(); err != nil {
		return nil, fmt.Errorf("error starting proxy app connections: %w", err)
	}

	eventBus := tmtypes.NewEventBus()
	eventBus.SetLogger(logger.With("module", "events"))
	if err := eventBus.Start(); err != nil {
		return nil, err
	}

	client, err := p2p.NewClient(conf.P2P, nodeKey, genesis.ChainID, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	var baseKV store.KVStore
	if conf.RootDir == "" && conf.DBPath == "" { // this is used for testing
		logger.Info("WARNING: working in in-memory mode")
		baseKV = store.NewDefaultInMemoryKVStore()
	} else {
		baseKV = store.NewDefaultKVStore(conf.RootDir, conf.DBPath, "optimint")
	}
	mainKV := store.NewPrefixKV(baseKV, mainPrefix)
	dalcKV := store.NewPrefixKV(baseKV, dalcPrefix)
	indexerKV := store.NewPrefixKV(baseKV, indexerPrefix)

	s := store.New(mainKV)

	dalc := registry.GetClient(conf.DALayer)
	if dalc == nil {
		return nil, fmt.Errorf("couldn't get data availability client named '%s'", conf.DALayer)
	}
	err = dalc.Init([]byte(conf.DAConfig), dalcKV, logger.With("module", "da_client"))
	if err != nil {
		return nil, fmt.Errorf("data availability layer client initialization error: %w", err)
	}

	indexerService, txIndexer, blockIndexer, err := createAndStartIndexerService(conf, indexerKV, eventBus, logger)
	if err != nil {
		return nil, err
	}

	mp := mempool.NewCListMempool(llcfg.DefaultMempoolConfig(), proxyApp.Mempool(), 0)
	mpIDs := newMempoolIDs()

	blockManager, err := block.NewManager(nodeKey, conf.BlockManagerConfig, genesis, s, mp, proxyApp.Consensus(), dalc, eventBus, logger.With("module", "BlockManager"))
	if err != nil {
		return nil, fmt.Errorf("BlockManager initialization error: %w", err)
	}

	node := &Node{
		proxyApp:       proxyApp,
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

	return node, nil
}

// initGenesisChunks creates a chunked format of the genesis document to make it easier to
// iterate through larger genesis structures.
func (n *Node) initGenesisChunks() error {
	if n.genChunks != nil {
		return nil
	}

	if n.genesis == nil {
		return nil
	}

	data, err := json.Marshal(n.genesis)
	if err != nil {
		return err
	}

	for i := 0; i < len(data); i += genesisChunkSize {
		end := i + genesisChunkSize

		if end > len(data) {
			end = len(data)
		}

		n.genChunks = append(n.genChunks, base64.StdEncoding.EncodeToString(data[i:end]))
	}

	return nil
}

func (n *Node) headerPublishLoop(ctx context.Context) {
	for {
		select {
		case header := <-n.blockManager.HeaderOutCh:
			headerBytes, err := header.MarshalBinary()
			if err != nil {
				n.Logger.Error("failed to serialize block header", "error", err)
			}
			err = n.P2P.GossipHeader(ctx, headerBytes)
			if err != nil {
				n.Logger.Error("failed to gossip block header", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// OnStart is a part of Service interface.
func (n *Node) OnStart() error {
	n.Logger.Info("starting P2P client")
	err := n.P2P.Start(n.ctx)
	if err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}
	err = n.dalc.Start()
	if err != nil {
		return fmt.Errorf("error while starting data availability layer client: %w", err)
	}
	err = n.initGenesisChunks()
	if err != nil {
		return fmt.Errorf("error while creating chunks of the genesis document: %w", err)
	}
	if n.conf.Aggregator {
		n.Logger.Info("working in aggregator mode", "block time", n.conf.BlockTime)
		go n.blockManager.AggregationLoop(n.ctx)
		go n.headerPublishLoop(n.ctx)
	}
	go n.blockManager.RetrieveLoop(n.ctx)
	go n.blockManager.SyncLoop(n.ctx)

	return nil
}

func (n *Node) GetGenesis() *tmtypes.GenesisDoc {
	return n.genesis
}

func (n *Node) GetGenisisChunks() []string {
	return n.genChunks
}

// OnStop is a part of Service interface.
func (n *Node) OnStop() {
	err := n.dalc.Stop()
	err = multierr.Append(err, n.P2P.Close())
	n.Logger.Error("errors while stopping node:", "errors", err)
}

// OnReset is a part of Service interface.
func (n *Node) OnReset() error {
	panic("OnReset - not implemented!")
}

// SetLogger sets the logger used by node.
func (n *Node) SetLogger(logger log.Logger) {
	n.Logger = logger
}

// GetLogger returns logger.
func (n *Node) GetLogger() log.Logger {
	return n.Logger
}

// EventBus gives access to Node's event bus.
func (n *Node) EventBus() *tmtypes.EventBus {
	return n.eventBus
}

// ProxyApp returns ABCI proxy connections to communicate with application.
func (n *Node) ProxyApp() proxy.AppConns {
	return n.proxyApp
}

// newTxValidator creates a pubsub validator that uses the node's mempool to check the
// transaction. If the transaction is valid, then it is added to the mempool
func (n *Node) newTxValidator() p2p.GossipValidator {
	return func(m *p2p.GossipMessage) bool {
		n.Logger.Debug("transaction received", "bytes", len(m.Data))
		checkTxResCh := make(chan *abci.Response, 1)
		err := n.Mempool.CheckTx(m.Data, func(resp *abci.Response) {
			checkTxResCh <- resp
		}, mempool.TxInfo{
			SenderID:    n.mempoolIDs.GetForPeer(m.From),
			SenderP2PID: corep2p.ID(m.From),
		})
		switch {
		case errors.Is(err, mempool.ErrTxInCache):
			return true
		case errors.Is(err, mempool.ErrMempoolIsFull{}):
			return true
		case errors.Is(err, mempool.ErrTxTooLarge{}):
			return false
		case errors.Is(err, mempool.ErrPreCheck{}):
			return false
		default:
		}
		res := <-checkTxResCh
		checkTxResp := res.GetCheckTx()

		return checkTxResp.Code == abci.CodeTypeOK
	}
}

// newHeaderValidator returns a pubsub validator that runs basic checks and forwards
// the deserialized header for further processing
func (n *Node) newHeaderValidator() p2p.GossipValidator {
	return func(headerMsg *p2p.GossipMessage) bool {
		n.Logger.Debug("header received", "from", headerMsg.From, "bytes", len(headerMsg.Data))
		var header types.Header
		err := header.UnmarshalBinary(headerMsg.Data)
		if err != nil {
			n.Logger.Error("failed to deserialize header", "error", err)
			return false
		}
		err = header.ValidateBasic()
		if err != nil {
			n.Logger.Error("failed to validate header", "error", err)
			return false
		}
		n.blockManager.HeaderInCh <- &header
		return true
	}
}

func createAndStartIndexerService(
	conf config.NodeConfig,
	kvStore store.KVStore,
	eventBus *tmtypes.EventBus,
	logger log.Logger,
) (*txindex.IndexerService, txindex.TxIndexer, indexer.BlockIndexer, error) {

	var (
		txIndexer    txindex.TxIndexer
		blockIndexer indexer.BlockIndexer
	)

	txIndexer = kv.NewTxIndex(kvStore)
	blockIndexer = blockidxkv.New(store.NewPrefixKV(kvStore, []byte("block_events")))

	indexerService := txindex.NewIndexerService(txIndexer, blockIndexer, eventBus)
	indexerService.SetLogger(logger.With("module", "txindex"))

	if err := indexerService.Start(); err != nil {
		return nil, nil, nil, err
	}

	return indexerService, txIndexer, blockIndexer, nil
}
