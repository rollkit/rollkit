package node

import (
	"context"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/celestiaorg/rollmint/types"

	abciclient "github.com/tendermint/tendermint/abci/client"
	llcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	tmtypes "github.com/tendermint/tendermint/types"

	"encoding/base64"
	"encoding/json"

	abci "github.com/tendermint/tendermint/abci/types"
	corep2p "github.com/tendermint/tendermint/p2p"
	"go.uber.org/multierr"

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
	go n.blockManager.SyncLoop(n.ctx)

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

// newCommitValidator returns a pubsub validator that runs basic checks and forwards
// the deserialized commit for further processing
func (n *FullNode) newCommitValidator() p2p.GossipValidator {
	return func(commitMsg *p2p.GossipMessage) bool {
		n.Logger.Debug("commit received", "from", commitMsg.From, "bytes", len(commitMsg.Data))
		var commit types.Commit
		err := commit.UnmarshalBinary(commitMsg.Data)
		if err != nil {
			n.Logger.Error("failed to deserialize commit", "error", err)
			return false
		}
		err = commit.ValidateBasic()
		if err != nil {
			n.Logger.Error("failed to validate commit", "error", err)
			return false
		}
		n.Logger.Debug("commit received", "height", commit.Height)
		n.blockManager.CommitInCh <- &commit
		return true
	}
}

// newFraudProofValidator returns a pubsub validator that validates a fraud proof and forwards
// it to be verified
func (n *FullNode) newFraudProofValidator() p2p.GossipValidator {
	return func(fraudProofMsg *p2p.GossipMessage) bool {
		n.Logger.Debug("fraud proof received", "from", fraudProofMsg.From, "bytes", len(fraudProofMsg.Data))
		var fraudProof types.FraudProof
		err := fraudProof.UnmarshalBinary(fraudProofMsg.Data)
		if err != nil {
			n.Logger.Error("failed to deserialize fraud proof", "error", err)
			return false
		}
		// TODO(manav): Add validation checks for fraud proof here
		n.blockManager.FraudProofCh <- &fraudProof
		return true
	}
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

func (n *FullNode) GetP2P() *p2p.Client {
	return n.P2P
}
func (n *FullNode) GetMempool() mempool.Mempool {
	return n.Mempool
}
func (n *FullNode) GetStore() store.Store {
	return n.Store
}
func (n *FullNode) GetTxIndexer() txindex.TxIndexer {
	return n.TxIndexer
}
func (n *FullNode) GetBlockIndexer() indexer.BlockIndexer {
	return n.BlockIndexer
}
