package json

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/proxy"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func prepareProposalResponse(_ context.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	return &abci.ResponsePrepareProposal{
		Txs: req.Txs,
	}, nil
}

var MockServerAddr = "http://localhost:7980"

var MockNamespace = "deadbeef"

// copied from rpc
func getRPC(t *testing.T) (*mocks.Application, rpcclient.Client) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(
		func(_ context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
			txResults := make([]*abci.ExecTxResult, len(req.Txs))
			for idx := range req.Txs {
				txResults[idx] = &abci.ExecTxResult{
					Code: abci.CodeTypeOK,
				}
			}

			return &abci.ResponseFinalizeBlock{
				TxResults: txResults,
			}, nil
		},
	)
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{
		GasWanted: 1000,
		GasUsed:   1000,
	}, nil)
	app.On("Info", mock.Anything, mock.Anything).Return(&abci.ResponseInfo{
		Data:             "mock",
		Version:          "mock",
		AppVersion:       123,
		LastBlockHeight:  345,
		LastBlockAppHash: nil,
	}, nil)
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	validatorKey := ed25519.GenPrivKey()
	nodeKey := &p2p.NodeKey{
		PrivKey: validatorKey,
	}
	signingKey, _ := types.GetNodeKey(nodeKey)
	pubKey := validatorKey.PubKey()

	genesisValidators := []cmtypes.GenesisValidator{
		{Address: pubKey.Address(), PubKey: pubKey, Power: int64(100), Name: "gen #1"},
	}
	n, err := node.NewNode(context.Background(), config.NodeConfig{DAAddress: MockServerAddr, DANamespace: MockNamespace, Aggregator: true, BlockManagerConfig: config.BlockManagerConfig{BlockTime: 1 * time.Second}, Light: false}, key, signingKey, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, node.DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), log.TestingLogger())
	require.NoError(err)
	require.NotNil(n)

	err = n.Start()
	require.NoError(err)

	local := n.GetClient()
	require.NotNil(local)

	return app, local
}
