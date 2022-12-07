package node

import (
	abciclient "github.com/tendermint/tendermint/abci/client"
	tmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tendermint/libs/log"
)

type LightNode struct {}

func (n *LightNode) AppClient() abciclient.Client {
  panic("Todo")
}
func (n *LightNode) EventBus() *tmtypes.EventBus {
  panic("Todo")
}
func (n *LightNode) GetLogger() log.Logger {
  panic("Todo")
}
func (n *LightNode) SetLogger() {
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
