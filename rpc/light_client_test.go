package rpc

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/conv"
	rollnode "github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/abci/example/kvstore"
	tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto/ed25519"
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

	app := kvstore.NewApplication()

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	key2, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}

	node, err := rollnode.NewNode(context.Background(), config.NodeConfig{
		DALayer:            "mock",
		Aggregator:         true,
		BlockManagerConfig: blockManagerConfig,
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
	lightNode, err := rollnode.NewNode(context.Background(), config.NodeConfig{
		DALayer:            "mock",
		Aggregator:         false,
		BlockManagerConfig: blockManagerConfig,
		Light:              true,
		LightNodeConfig:    lightNodeConfig,
		/*RPC: config.RPCConfig{
			ListenAddress: "0.0.0.0",
		},*/
	}, key2, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
	//assert.False(lightNode.IsRunning())
	assert.NoError(err)
	err = lightNode.Start()
	assert.NoError(err)
	defer func() {
		err := lightNode.Stop()
		assert.NoError(err)
	}()
	//assert.True(lightNode.IsRunning())
	lightClient := lightNode.GetClient()

	_, err = client.BroadcastTxCommit(context.Background(), []byte("connor=cool"))
	assert.NoError(err)

	q, err := lightClient.ABCIQueryWithOptions(context.Background(), "/store", []byte("connor"), tmclient.ABCIQueryOptions{
		Prove: true,
	})
	require.NoError(err)
	fmt.Println(q)
}
