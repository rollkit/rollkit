package rpcclient

import (
	"context"
	"crypto/rand"
	"testing"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/node"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

type MockApp struct {
}

// Info/Query Connection
// Return application info
func (m *MockApp) Info(_ abci.RequestInfo) abci.ResponseInfo {
	return expectedInfo
}

func (m *MockApp) Query(_ abci.RequestQuery) abci.ResponseQuery {
	panic("not implemented") // TODO: Implement
}

// Mempool Connection
// Validate a tx for the mempool
func (m *MockApp) CheckTx(_ abci.RequestCheckTx) abci.ResponseCheckTx {
	panic("not implemented") // TODO: Implement
}

// Consensus Connection
// Initialize blockchain w validators/other info from TendermintCore
func (m *MockApp) InitChain(_ abci.RequestInitChain) abci.ResponseInitChain {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) BeginBlock(_ abci.RequestBeginBlock) abci.ResponseBeginBlock {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) DeliverTx(_ abci.RequestDeliverTx) abci.ResponseDeliverTx {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) EndBlock(_ abci.RequestEndBlock) abci.ResponseEndBlock {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) Commit() abci.ResponseCommit {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) PreprocessTxs(_ abci.RequestPreprocessTxs) abci.ResponsePreprocessTxs {
	panic("not implemented") // TODO: Implement
}

// State Sync Connection
// List available snapshots
func (m *MockApp) ListSnapshots(_ abci.RequestListSnapshots) abci.ResponseListSnapshots {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) OfferSnapshot(_ abci.RequestOfferSnapshot) abci.ResponseOfferSnapshot {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) LoadSnapshotChunk(_ abci.RequestLoadSnapshotChunk) abci.ResponseLoadSnapshotChunk {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) ApplySnapshotChunk(_ abci.RequestApplySnapshotChunk) abci.ResponseApplySnapshotChunk {
	panic("not implemented") // TODO: Implement
}

func TestInfo(t *testing.T) {
	assert := assert.New(t)

	rpc := getRPC(t)

	info, err := rpc.ABCIInfo(context.Background())
	assert.NoError(err)
	assert.Equal(expectedInfo, info.Response)
}

func getRPC(t *testing.T) *Local {
	t.Helper()
	require := require.New(t)
	app := &MockApp{}
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	node, err := node.NewNode(context.Background(), config.NodeConfig{}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewLocal(node)
	require.NotNil(rpc)

	return rpc
}
