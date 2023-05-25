package node

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	abciconv "github.com/rollkit/rollkit/conv/abci"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tendermint/tendermint/crypto/encoding"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/conv"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/mocks"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/store"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

var mockTxProcessingTime = 10 * time.Millisecond

var genesisValidatorKey = ed25519.GenPrivKey()

// TODO: accept argument for number of validators / proposer index
func getRandomValidatorSet() *tmtypes.ValidatorSet {
	pubKey := ed25519.GenPrivKey().PubKey()
	return &tmtypes.ValidatorSet{
		Proposer: &tmtypes.Validator{PubKey: pubKey, Address: pubKey.Address()},
		Validators: []*tmtypes.Validator{
			{PubKey: pubKey, Address: pubKey.Address()},
		},
	}
}

// TODO: use n and return n validators
func getGenesisValidatorSetWithSigner(n int) ([]tmtypes.GenesisValidator, crypto.PrivKey) {
	nodeKey := &p2p.NodeKey{
		PrivKey: genesisValidatorKey,
	}
	signingKey, _ := conv.GetNodeKey(nodeKey)
	pubKey := genesisValidatorKey.PubKey()

	genesisValidators := []tmtypes.GenesisValidator{
		{Address: pubKey.Address(), PubKey: pubKey, Power: int64(100), Name: "gen #1"},
	}
	return genesisValidators, signingKey
}

func createGenesisValidators(numNodes int, appCreator func(require *require.Assertions, vKeyToRemove tmcrypto.PrivKey, wg *sync.WaitGroup) *mocks.Application, require *require.Assertions, wg *sync.WaitGroup) *FullClient {
	vKeys := make([]tmcrypto.PrivKey, numNodes)
	apps := make([]*mocks.Application, numNodes)
	nodes := make([]*FullNode, numNodes)

	genesisValidators := make([]tmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		genesisValidators[i] = tmtypes.GenesisValidator{Address: vKeys[i].PubKey().Address(), PubKey: vKeys[i].PubKey(), Power: int64(i + 100), Name: fmt.Sprintf("gen #%d", i)}
		apps[i] = appCreator(require, vKeys[0], wg)
		wg.Add(1)
	}

	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, err := store.NewDefaultInMemoryKVStore()
	require.Nil(err)
	err = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	require.Nil(err)
	err = dalc.Start()
	require.Nil(err)

	for i := 0; i < len(nodes); i++ {
		nodeKey := &p2p.NodeKey{
			PrivKey: vKeys[i],
		}
		signingKey, err := conv.GetNodeKey(nodeKey)
		require.NoError(err)
		nodes[i], err = newFullNode(
			context.Background(),
			config.NodeConfig{
				DALayer:    "mock",
				Aggregator: true,
				BlockManagerConfig: config.BlockManagerConfig{
					BlockTime:   1 * time.Second,
					DABlockTime: 100 * time.Millisecond,
				},
			},
			signingKey,
			signingKey,
			proxy.NewLocalClientCreator(apps[i]),
			&tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators},
			log.TestingLogger(),
		)
		require.NoError(err)
		require.NotNil(nodes[i])

		// use same, common DALC, so nodes can share data
		nodes[i].dalc = dalc
		nodes[i].blockManager.SetDALC(dalc)
	}

	rpc := NewFullClient(nodes[0])
	require.NotNil(rpc)

	for i := 0; i < len(nodes); i++ {
		err := nodes[i].Start()
		require.NoError(err)
	}
	return rpc
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	return getRandomBlockWithProposer(height, nTxs, getRandomBytes(20))
}

func getRandomBlockWithProposer(height uint64, nTxs int, proposerAddr []byte) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				Version:         types.Version{Block: types.InitStateVersion.Consensus.Block},
				ProposerAddress: proposerAddr,
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.SignedHeader.Header.AppHash = getRandomBytes(32)

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	tmprotoLC, err := tmtypes.CommitFromProto(&tmproto.Commit{})
	if err != nil {
		return nil
	}
	lastCommitHash := make(types.Hash, 32)
	copy(lastCommitHash, tmprotoLC.Hash().Bytes())
	block.SignedHeader.Header.LastCommitHash = lastCommitHash

	block.SignedHeader.Validators = getRandomValidatorSet()

	return block
}

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = crand.Read(data)
	return data
}

// From state/indexer/block/kv/kv_test
func indexBlocks(t *testing.T, rpc *FullClient, heights []int64) {
	t.Helper()

	for _, h := range heights {
		require.NoError(t, rpc.node.BlockIndexer.Index(tmtypes.EventDataNewBlockHeader{
			Header: tmtypes.Header{Height: h},
			ResultBeginBlock: abci.ResponseBeginBlock{
				Events: []abci.Event{
					{
						Type: "begin_event",
						Attributes: []abci.EventAttribute{
							{
								Key:   []byte("proposer"),
								Value: []byte("FCAA001"),
								Index: true,
							},
						},
					},
				},
			},
			ResultEndBlock: abci.ResponseEndBlock{
				Events: []abci.Event{
					{
						Type: "end_event",
						Attributes: []abci.EventAttribute{
							{
								Key:   []byte("foo"),
								Value: []byte(fmt.Sprintf("%d", h)),
								Index: true,
							},
						},
					},
				},
			},
		}))
	}

}

func getBlockMeta(rpc *FullClient, n int64) *tmtypes.BlockMeta {
	b, err := rpc.node.Store.LoadBlock(uint64(n))
	if err != nil {
		return nil
	}
	bmeta, err := abciconv.ToABCIBlockMeta(b)
	if err != nil {
		return nil
	}

	return bmeta
}

func getRPC(t *testing.T) (*mocks.Application, *FullClient) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	node, err := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewFullClient(node)
	require.NotNil(rpc)

	return app, rpc
}

func checkValSet(rpc *FullClient, assert *assert.Assertions, h int64, expectedValCount int) {
	vals, err := rpc.Validators(context.Background(), &h, nil, nil)
	assert.NoError(err)
	assert.NotNil(vals)
	assert.EqualValues(expectedValCount, vals.Total)
	assert.Len(vals.Validators, expectedValCount)
	assert.EqualValues(vals.BlockHeight, h)
}

func checkValSetLatest(rpc *FullClient, assert *assert.Assertions, lastBlockHeight int64, expectedValCount int) {
	vals, err := rpc.Validators(context.Background(), nil, nil, nil)
	assert.NoError(err)
	assert.NotNil(vals)
	assert.EqualValues(expectedValCount, vals.Total)
	assert.Len(vals.Validators, expectedValCount)
	assert.GreaterOrEqual(vals.BlockHeight, lastBlockHeight)
}

func createApp(require *require.Assertions, vKeyToRemove tmcrypto.PrivKey, wg *sync.WaitGroup) *mocks.Application {
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})
	app.On("GenerateFraudProof", mock.Anything).Return(abci.ResponseGenerateFraudProof{})

	pbValKey, err := encoding.PubKeyToProto(vKeyToRemove.PubKey())
	require.NoError(err)

	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Times(2)
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{ValidatorUpdates: []abci.ValidatorUpdate{{PubKey: pbValKey, Power: 0}}}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{ValidatorUpdates: []abci.ValidatorUpdate{{PubKey: pbValKey, Power: 100}}}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Times(5)
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Run(func(args mock.Arguments) {
		wg.Done()
	}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	return app
}
