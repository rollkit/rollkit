package node

import (
	"context"
  "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"go.uber.org/multierr"

	abciclient "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	corep2p "github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/rollmint/config"
	"github.com/celestiaorg/rollmint/mempool"
	"github.com/celestiaorg/rollmint/p2p"
	"github.com/celestiaorg/rollmint/state/indexer"
	blockidxkv "github.com/celestiaorg/rollmint/state/indexer/block/kv"
	"github.com/celestiaorg/rollmint/state/txindex"
	"github.com/celestiaorg/rollmint/state/txindex/kv"
	"github.com/celestiaorg/rollmint/store"
	"github.com/celestiaorg/rollmint/types"
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
  SetLogger()
  OnReset() error
  OnStop()
  OnStart() error
  GetGenesisChunks() ([]string, error)
  GetGenesis() *tmtypes.GenesisDoc
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
