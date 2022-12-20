package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	abciclient "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/rollmint/config"
	"github.com/celestiaorg/rollmint/mempool"
	"github.com/celestiaorg/rollmint/p2p"
	"github.com/celestiaorg/rollmint/state/indexer"
	"github.com/celestiaorg/rollmint/state/txindex"
	"github.com/celestiaorg/rollmint/store"
)

func newLightNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	appClient abciclient.Client,
	genesis *tmtypes.GenesisDoc,
	logger log.Logger,
) (*LightNode, error) {
	panic("Todo")
}

type LightNode struct{}

func (n *LightNode) AppClient() abciclient.Client {
	panic("Todo")
}
func (n *LightNode) EventBus() *tmtypes.EventBus {
	panic("Todo")
}
func (n *LightNode) GetLogger() log.Logger {
	panic("Todo")
}
func (n *LightNode) SetLogger(log.Logger) {
	panic("Todo")
}
func (n *LightNode) OnReset() error {
	panic("Todo")
}
func (n *LightNode) OnStop() {
	panic("Todo")
}
func (n *LightNode) OnStart() error {
	panic("Todo")
}
func (n *LightNode) GetGenesisChunks() ([]string, error) {
	panic("Todo")
}
func (n *LightNode) GetGenesis() *tmtypes.GenesisDoc {
	panic("Todo")
}

func (n *LightNode) GetP2P() *p2p.Client {
	panic("Todo")
}
func (n *LightNode) GetMempool() mempool.Mempool           { panic("Todo") }
func (n *LightNode) GetStore() store.Store                 { panic("Todo") }
func (n *LightNode) GetTxIndexer() txindex.TxIndexer       { panic("Todo") }
func (n *LightNode) GetBlockIndexer() indexer.BlockIndexer { panic("Todo") }
