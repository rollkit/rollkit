package rpcclient

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/node"
)

var expectedInfo = types.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

type MockApp struct {
}

// Info/Query Connection
// Return application info
func (m *MockApp) Info(_ types.RequestInfo) types.ResponseInfo {
	return expectedInfo
}

func (m *MockApp) Query(_ types.RequestQuery) types.ResponseQuery {
	panic("not implemented") // TODO: Implement
}

// Mempool Connection
// Validate a tx for the mempool
func (m *MockApp) CheckTx(_ types.RequestCheckTx) types.ResponseCheckTx {
	panic("not implemented") // TODO: Implement
}

// Consensus Connection
// Initialize blockchain w validators/other info from TendermintCore
func (m *MockApp) InitChain(_ types.RequestInitChain) types.ResponseInitChain {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) BeginBlock(_ types.RequestBeginBlock) types.ResponseBeginBlock {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) DeliverTx(_ types.RequestDeliverTx) types.ResponseDeliverTx {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) EndBlock(_ types.RequestEndBlock) types.ResponseEndBlock {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) Commit() types.ResponseCommit {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) PreprocessTxs(_ types.RequestPreprocessTxs) types.ResponsePreprocessTxs {
	panic("not implemented") // TODO: Implement
}

// State Sync Connection
// List available snapshots
func (m *MockApp) ListSnapshots(_ types.RequestListSnapshots) types.ResponseListSnapshots {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) OfferSnapshot(_ types.RequestOfferSnapshot) types.ResponseOfferSnapshot {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) LoadSnapshotChunk(_ types.RequestLoadSnapshotChunk) types.ResponseLoadSnapshotChunk {
	panic("not implemented") // TODO: Implement
}

func (m *MockApp) ApplySnapshotChunk(_ types.RequestApplySnapshotChunk) types.ResponseApplySnapshotChunk {
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
	node, err := node.NewNode(context.Background(), config.NodeConfig{}, key, proxy.NewLocalClientCreator(app), log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewLocal(node)
	require.NotNil(rpc)

	return rpc
}
