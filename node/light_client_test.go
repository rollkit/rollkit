package node

import (
	"context"
	"testing"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/stretchr/testify/assert"
)

// TestLightClient_Panics tests that all methods of LightClient and ensures that
// they panic as they are not implemented. This is to ensure that when the
// methods are implemented in the future we don't forget to add testing. When
// methods are implemented, they should be removed from this test and have their
// own test written.
func TestLightClient_Panics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln := initializeAndStartLightNode(ctx, t)
	defer cleanUpNode(ln, t)

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "ABCIInfo",
			fn: func() {
				_, _ = ln.GetClient().ABCIInfo(ctx)
			},
		},
		{
			name: "ABCIQuery",
			fn: func() {
				_, _ = ln.GetClient().ABCIQuery(ctx, "", nil)
			},
		},
		{
			name: "ABCIQueryWithOptions",
			fn: func() {
				_, _ = ln.GetClient().ABCIQueryWithOptions(ctx, "", nil, rpcclient.ABCIQueryOptions{})
			},
		},
		{
			name: "BroadcastTxCommit",
			fn: func() {
				_, _ = ln.GetClient().BroadcastTxSync(ctx, []byte{})
			},
		},
		{
			name: "BroadcastTxAsync",
			fn: func() {
				_, _ = ln.GetClient().BroadcastTxSync(ctx, []byte{})
			},
		},
		{
			name: "BroadcastTxSync",
			fn: func() {
				_, _ = ln.GetClient().BroadcastTxSync(ctx, []byte{})
			},
		},
		{
			name: "Subscribe",
			fn: func() {
				_, _ = ln.GetClient().Subscribe(ctx, "", "", 0)
			},
		},
		{
			name: "Block",
			fn: func() {
				_, _ = ln.GetClient().Block(ctx, nil)
			},
		},
		{
			name: "BlockByHash",
			fn: func() {
				_, _ = ln.GetClient().BlockByHash(ctx, []byte{})
			},
		},
		{
			name: "BlockResults",
			fn: func() {
				_, _ = ln.GetClient().BlockResults(ctx, nil)
			},
		},
		{
			name: "BlockSearch",
			fn: func() {
				_, _ = ln.GetClient().BlockSearch(ctx, "", nil, nil, "")
			},
		},
		{
			name: "BlockchainInfo",
			fn: func() {
				_, _ = ln.GetClient().BlockchainInfo(ctx, 0, 0)
			},
		},
		{
			name: "BroadcastEvidence",
			fn: func() {
				_, _ = ln.GetClient().BroadcastEvidence(ctx, nil)
			},
		},
		{
			name: "CheckTx",
			fn: func() {
				_, _ = ln.GetClient().CheckTx(ctx, []byte{})
			},
		},
		{
			name: "Commit",
			fn: func() {
				_, _ = ln.GetClient().Commit(ctx, nil)
			},
		},
		{
			name: "ConsensusParams",
			fn: func() {
				_, _ = ln.GetClient().ConsensusParams(ctx, nil)
			},
		},
		{
			name: "ConsensusState",
			fn: func() {
				_, _ = ln.GetClient().ConsensusState(ctx)
			},
		},
		{
			name: "DumpConsensusState",
			fn: func() {
				_, _ = ln.GetClient().DumpConsensusState(ctx)
			},
		},
		{
			name: "Genesis",
			fn: func() {
				_, _ = ln.GetClient().Genesis(ctx)
			},
		},
		{
			name: "GenesisChunked",
			fn: func() {
				_, _ = ln.GetClient().GenesisChunked(ctx, 0)
			},
		},
		{
			name: "Header",
			fn: func() {
				_, _ = ln.GetClient().Header(ctx, nil)
			},
		},
		{
			name: "HeaderByHash",
			fn: func() {
				_, _ = ln.GetClient().HeaderByHash(ctx, []byte{})
			},
		},
		{
			name: "Health",
			fn: func() {
				_, _ = ln.GetClient().Health(ctx)
			},
		},
		{
			name: "NetInfo",
			fn: func() {
				_, _ = ln.GetClient().NetInfo(ctx)
			},
		},
		{
			name: "NumUnconfirmedTxs",
			fn: func() {
				_, _ = ln.GetClient().NumUnconfirmedTxs(ctx)
			},
		},
		{
			name: "Status",
			fn: func() {
				_, _ = ln.GetClient().Status(ctx)
			},
		},
		{
			name: "Tx",
			fn: func() {
				_, _ = ln.GetClient().Tx(ctx, []byte{}, false)
			},
		},
		{
			name: "TxSearch",
			fn: func() {
				_, _ = ln.GetClient().TxSearch(ctx, "", false, nil, nil, "")
			},
		},
		{
			name: "UnconfirmedTxs",
			fn: func() {
				_, _ = ln.GetClient().UnconfirmedTxs(ctx, nil)
			},
		},
		{
			name: "Unsubscribe",
			fn: func() {
				_ = ln.GetClient().Unsubscribe(ctx, "", "")
			},
		},
		{
			name: "Validators",
			fn: func() {
				_, _ = ln.GetClient().Validators(ctx, nil, nil, nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Panics(t, test.fn)
		})
	}
}
