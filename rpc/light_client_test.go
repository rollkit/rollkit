package rpc

import (
	"context"
	"crypto/rand"
	"strconv"
	"strings"
	"sync"

	//"crypto/rand"
	//"fmt"
	"testing"
	"time"

	mockda "github.com/rollkit/rollkit/da/mock"
	//storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/store"
	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/rollkit/rollkit/conv"
	rollnode "github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/types"

	//"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	//tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto/ed25519"
	//"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/proxy"

	//tmclient "github.com/tendermint/tendermint/rpc/client"
	tmtypes "github.com/tendermint/tendermint/types"
)

func TestQuery(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())

	num := 3
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	_ = dalc.Start()
	sequencer, _ := createNode(aggCtx, 0, false, true, false, keys, &wg, t)
	sequencer.(*rollnode.FullNode).dalc = dalc
	sequencer.(*rollnode.FullNode).blockManager.SetDALC(dalc)

}

var genesisValidatorKey = ed25519.GenPrivKey()

func createNode(ctx context.Context, n int, isMalicious bool, aggregator bool, isLight bool, keys []crypto.PrivKey, wg *sync.WaitGroup, t *testing.T) (rollnode.Node, abcitypes.Application) {
	t.Helper()
	require := require.New(t)
	// nodes will listen on consecutive ports on local interface
	// random connections to other nodes will be added
	startPort := 10000
	p2pConfig := config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+n),
	}
	bmConfig := config.BlockManagerConfig{
		DABlockTime: 100 * time.Millisecond,
		BlockTime:   1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		NamespaceID: types.NamespaceID{8, 7, 6, 5, 4, 3, 2, 1},
		FraudProofs: true,
	}
	for i := 0; i < len(keys); i++ {
		if i == n {
			continue
		}
		r := i
		id, err := peer.IDFromPrivateKey(keys[r])
		require.NoError(err)
		p2pConfig.Seeds += "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+r) + "/p2p/" + id.Pretty() + ","
	}
	p2pConfig.Seeds = strings.TrimSuffix(p2pConfig.Seeds, ",")

	app := NewMerkleApp()
	if ctx == nil {
		ctx = context.Background()
	}

	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	genesis := &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}
	// TODO: need to investigate why this needs to be done for light nodes
	genesis.InitialHeight = 1
	node, err := rollnode.NewNode(
		ctx,
		config.NodeConfig{
			P2P:                p2pConfig,
			DALayer:            "mock",
			Aggregator:         aggregator,
			BlockManagerConfig: bmConfig,
			Light:              isLight,
		},
		keys[n],
		signingKey,
		proxy.NewLocalClientCreator(app),
		genesis,
		log.TestingLogger().With("node", n))
	require.NoError(err)
	require.NotNil(node)

	return node, app
}

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

/*func getGenesisValidatorSetWithSigner(n int) ([]tmtypes.GenesisValidator, crypto.PrivKey) {
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

func TestTrustMinimizedQuery(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	//app := kvstore.NewApplication()
	app := NewMerkleApp()

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	key2, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}

	ctx, cancel := context.WithCancel(context.Background())
	node, err := rollnode.NewNode(ctx, config.NodeConfig{
		DALayer:            "mock",
		Aggregator:         true,
		BlockManagerConfig: blockManagerConfig,
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/26656",
		},
	}, key, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{
		ChainID:       "test",
		Validators:    genesisValidators,
		InitialHeight: 1,
	}, log.TestingLogger())
	assert.False(node.IsRunning())
	assert.NoError(err)
	err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())

	require.NoError(err)

	client := node.GetClient()

	rpcServer := NewServer(node, tmconfig.DefaultRPCConfig(), nil)
	err = rpcServer.Start()
	assert.NoError(err)

	lightNodeConfig := config.LightNodeConfig{
		UntrustedRPC: "http://127.0.0.1:26657",
	}
	lightNode, err := rollnode.NewNode(ctx, config.NodeConfig{
		DALayer:            "mock",
		Aggregator:         false,
		BlockManagerConfig: blockManagerConfig,
		Light:              true,
		LightNodeConfig:    lightNodeConfig,
		P2P: config.P2PConfig{
			Seeds: "/ip4/127.0.0.1/tcp/26656",
		},
	}, key2, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{
		ChainID:       "test",
		Validators:    genesisValidators,
		InitialHeight: 1,
	}, log.TestingLogger())
	//assert.False(lightNode.IsRunning())
	assert.NoError(err)
	err = lightNode.Start()
	assert.NoError(err)
	defer func() {
		err := lightNode.Stop()
		assert.NoError(err)
	}()
	//assert.True(lightNode.IsRunning())
	lightClient := lightNode.GetClient().(*rollnode.LightClient)
	decoder := merkle.NewProofRuntime()
	decoder.RegisterOpDecoder("ics23:iavl", storetypes.CommitmentOpDecoder)
	lightClient.SetProofRuntime(decoder)
	lightClient.SetKeyPathFn(func(path string, key []byte) (merkle.KeyPath, error) {
		kP := merkle.KeyPath{}
		kP.AppendKey([]byte("/"), merkle.KeyEncodingURL)
		kP.AppendKey([]byte(key), merkle.KeyEncodingURL)
		return kP, nil
	})

	txResponse, err := client.BroadcastTxCommit(ctx, []byte("rollkit=cool"))
	assert.NoError(err)

	q, err := lightClient.ABCIQueryWithOptions(ctx, "/key", []byte("rollkit"), tmclient.ABCIQueryOptions{
		Prove:  true,
		Height: txResponse.Height,
	})
	require.NoError(err)
	fmt.Println("GOT QUERY RESULT")
	fmt.Println(q)
	cancel()
}
*/
