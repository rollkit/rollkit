package rpc

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/conv"
	rollnode "github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/proxy"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	tmtypes "github.com/tendermint/tendermint/types"
)

var genesisValidatorKey = ed25519.GenPrivKey()

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
		/*RPC: config.RPCConfig{
			ListenAddress: "127.0.0.1:46657",
		},*/
	}, key, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
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
		/*RPC: config.RPCConfig{
			ListenAddress: "0.0.0.0",
		},*/
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
		kP.AppendKey([]byte("/"))
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
