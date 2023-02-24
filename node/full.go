package node

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	"github.com/libp2p/go-libp2p/core/crypto"
	"go.uber.org/multierr"

	abciclient "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	llcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	corep2p "github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/block"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/registry"
	"github.com/rollkit/rollkit/mempool"
	mempoolv1 "github.com/rollkit/rollkit/mempool/v1"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/state/indexer"
	blockidxkv "github.com/rollkit/rollkit/state/indexer/block/kv"
	"github.com/rollkit/rollkit/state/txindex"
	"github.com/rollkit/rollkit/state/txindex/kv"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// prefixes used in KV store to separate main node data from DALC data
var (
	mainPrefix    = "0"
	dalcPrefix    = "1"
	indexerPrefix = "2" // indexPrefix uses "i", so using "0-2" to avoid clash
)

const (
	// genesisChunkSize is the maximum size, in bytes, of each
	// chunk in the genesis structure for the chunked API
	genesisChunkSize = 16 * 1024 * 1024 // 16 MiB
)

var _ Node = &FullNode{}

// FullNode represents a client node in Rollkit network.
// It connects all the components and orchestrates their work.
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

	cancel context.CancelFunc
}

// newFullNode creates a new Rollkit full node.
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

	var baseKV ds.TxnDatastore
	var err error
	if conf.RootDir == "" && conf.DBPath == "" { // this is used for testing
		logger.Info("WARNING: working in in-memory mode")
		baseKV, err = store.NewDefaultInMemoryKVStore()
	} else {
		baseKV, err = store.NewDefaultKVStore(conf.RootDir, conf.DBPath, "rollkit")
	}
	if err != nil {
		return nil, err
	}

	mainKV := newPrefixKV(baseKV, mainPrefix)
	dalcKV := newPrefixKV(baseKV, dalcPrefix)
	indexerKV := newPrefixKV(baseKV, indexerPrefix)

	client, err := p2p.NewClient(conf.P2P, p2pKey, genesis.ChainID, baseKV, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}
	s := store.New(ctx, mainKV)

	dalc := registry.GetClient(conf.DALayer)
	if dalc == nil {
		return nil, fmt.Errorf("couldn't get data availability client named '%s'", conf.DALayer)
	}
	err = dalc.Init(conf.NamespaceID, []byte(conf.DAConfig), dalcKV, logger.With("module", "da_client"))
	if err != nil {
		return nil, fmt.Errorf("data availability layer client initialization error: %w", err)
	}

	indexerService, txIndexer, blockIndexer, err := createAndStartIndexerService(ctx, conf, indexerKV, eventBus, logger)
	if err != nil {
		return nil, err
	}

	mp := mempoolv1.NewTxMempool(logger, llcfg.DefaultMempoolConfig(), appClient, 0)
	mpIDs := newMempoolIDs()

	blockManager, err := block.NewManager(signingKey, conf.BlockManagerConfig, genesis, s, mp, appClient, dalc, eventBus, logger.With("module", "BlockManager"))
	if err != nil {
		return nil, fmt.Errorf("BlockManager initialization error: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

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
		cancel:         cancel,
	}

	node.BaseService = *service.NewBaseService(logger, "Node", node)

	node.P2P.SetTxValidator(node.newTxValidator())
	node.P2P.SetHeaderValidator(node.newHeaderValidator())
	node.P2P.SetFraudProofValidator(node.newFraudProofValidator())

	return node, nil
}

// initGenesisChunks creates a chunked format of the genesis document to make it easier to
// iterate through larger genesis structures.
func (n *FullNode) initGenesisChunks() error {
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

func (n *FullNode) headerPublishLoop(ctx context.Context) {
	for {
		select {
		case signedHeader := <-n.blockManager.HeaderOutCh:
			headerBytes, err := signedHeader.MarshalBinary()
			if err != nil {
				n.Logger.Error("failed to serialize signed block header", "error", err)
			}
			err = n.P2P.GossipSignedHeader(ctx, headerBytes)
			if err != nil {
				n.Logger.Error("failed to gossip signed block header", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (n *FullNode) fraudProofPublishLoop(ctx context.Context) {
	for {
		select {
		case fraudProof := <-n.blockManager.GetFraudProofOutChan():
			n.Logger.Info("generated fraud proof: ", fraudProof.String())
			fraudProofBytes, err := fraudProof.Marshal()
			if err != nil {
				panic(fmt.Errorf("failed to serialize fraud proof: %w", err))
			}
			n.Logger.Info("gossipping fraud proof...")
			err = n.P2P.GossipFraudProof(context.Background(), fraudProofBytes)
			if err != nil {
				n.Logger.Error("failed to gossip fraud proof", "error", err)
			}
			_ = n.Stop()
		case <-ctx.Done():
			return
		}
	}
}

// OnStart is a part of Service interface.
func (n *FullNode) OnStart() error {
	n.Logger.Info("starting P2P client")
	err := n.P2P.Start(n.ctx)
	if err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}
	err = n.dalc.Start()
	if err != nil {
		return fmt.Errorf("error while starting data availability layer client: %w", err)
	}
	if n.conf.Aggregator {
		n.Logger.Info("working in aggregator mode", "block time", n.conf.BlockTime)
		go n.blockManager.AggregationLoop(n.ctx)
		go n.headerPublishLoop(n.ctx)
	}
	go n.blockManager.RetrieveLoop(n.ctx)
	go n.blockManager.SyncLoop(n.ctx, n.cancel)
	go n.fraudProofPublishLoop(n.ctx)

	return nil
}

// GetGenesis returns entire genesis doc.
func (n *FullNode) GetGenesis() *tmtypes.GenesisDoc {
	return n.genesis
}

// GetGenesisChunks returns chunked version of genesis.
func (n *FullNode) GetGenesisChunks() ([]string, error) {
	err := n.initGenesisChunks()
	if err != nil {
		return nil, err
	}
	return n.genChunks, err
}

// OnStop is a part of Service interface.
func (n *FullNode) OnStop() {
	n.Logger.Info("halting full node...")
	n.cancel()
	err := n.dalc.Stop()
	err = multierr.Append(err, n.P2P.Close())
	n.Logger.Error("errors while stopping node:", "errors", err)
}

// OnReset is a part of Service interface.
func (n *FullNode) OnReset() error {
	panic("OnReset - not implemented!")
}

// SetLogger sets the logger used by node.
func (n *FullNode) SetLogger(logger log.Logger) {
	n.Logger = logger
}

// GetLogger returns logger.
func (n *FullNode) GetLogger() log.Logger {
	return n.Logger
}

// EventBus gives access to Node's event bus.
func (n *FullNode) EventBus() *tmtypes.EventBus {
	return n.eventBus
}

// AppClient returns ABCI proxy connections to communicate with application.
func (n *FullNode) AppClient() abciclient.Client {
	return n.appClient
}

// newTxValidator creates a pubsub validator that uses the node's mempool to check the
// transaction. If the transaction is valid, then it is added to the mempool
func (n *FullNode) newTxValidator() p2p.GossipValidator {
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
func (n *FullNode) newHeaderValidator() p2p.GossipValidator {
	return func(headerMsg *p2p.GossipMessage) bool {
		n.Logger.Debug("header received", "from", headerMsg.From, "bytes", len(headerMsg.Data))
		var header types.SignedHeader
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

// newFraudProofValidator returns a pubsub validator that validates a fraud proof and forwards
// it to be verified
func (n *FullNode) newFraudProofValidator() p2p.GossipValidator {
	return func(fraudProofMsg *p2p.GossipMessage) bool {
		n.Logger.Debug("fraud proof received", "from", fraudProofMsg.From, "bytes", len(fraudProofMsg.Data))
		fraudProof := abci.FraudProof{}
		err := fraudProof.Unmarshal(fraudProofMsg.Data)
		if err != nil {
			n.Logger.Error("failed to deserialize fraud proof", "error", err)
			return false
		}
		// TODO(manav): Add validation checks for fraud proof here
		n.blockManager.FraudProofInCh <- &fraudProof
		return true
	}
}

func newPrefixKV(kvStore ds.Datastore, prefix string) ds.TxnDatastore {
	return (ktds.Wrap(kvStore, ktds.PrefixTransform{Prefix: ds.NewKey(prefix)}).Children()[0]).(ds.TxnDatastore)
}

func createAndStartIndexerService(
	ctx context.Context,
	conf config.NodeConfig,
	kvStore ds.TxnDatastore,
	eventBus *tmtypes.EventBus,
	logger log.Logger,
) (*txindex.IndexerService, txindex.TxIndexer, indexer.BlockIndexer, error) {
	var (
		txIndexer    txindex.TxIndexer
		blockIndexer indexer.BlockIndexer
	)

	txIndexer = kv.NewTxIndex(ctx, kvStore)
	blockIndexer = blockidxkv.New(ctx, newPrefixKV(kvStore, "block_events"))

	indexerService := txindex.NewIndexerService(txIndexer, blockIndexer, eventBus)
	indexerService.SetLogger(logger.With("module", "txindex"))

	if err := indexerService.Start(); err != nil {
		return nil, nil, nil, err
	}

	return indexerService, txIndexer, blockIndexer, nil
}
