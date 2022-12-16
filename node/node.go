package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	abciclient "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/rollmint/config"
	"github.com/celestiaorg/rollmint/state/indexer"
	blockidxkv "github.com/celestiaorg/rollmint/state/indexer/block/kv"
	"github.com/celestiaorg/rollmint/state/txindex"
	"github.com/celestiaorg/rollmint/state/txindex/kv"
	"github.com/celestiaorg/rollmint/store"
	"github.com/celestiaorg/rollmint/p2p"
	"github.com/celestiaorg/rollmint/mempool"
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

// Node represents a client node in rollmint network.
// It connects all the components and orchestrates their work.
type Node interface {
	AppClient() abciclient.Client
	EventBus() *tmtypes.EventBus
	GetLogger() log.Logger
	SetLogger(log.Logger)
	OnReset() error
	OnStop()
	OnStart() error
	GetGenesisChunks() ([]string, error)
	GetGenesis() *tmtypes.GenesisDoc

  GetP2P() *p2p.Client
  GetMempool() mempool.Mempool
	GetStore() store.Store
	GetTxIndexer() txindex.TxIndexer
	GetBlockIndexer() indexer.BlockIndexer
}

func NewNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	appClient abciclient.Client,
	genesis *tmtypes.GenesisDoc,
	logger log.Logger,
) (*FullNode, error) {
	n, err := newFullNode(ctx, conf, p2pKey, signingKey, appClient, genesis, logger)
	return n, err
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
