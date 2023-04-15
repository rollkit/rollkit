package node

import (
	"context"
	crand "crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/config"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/abci/example/kvstore"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"
)

func TestTrustMinimizedQuery(t *testing.T) {
	vKeys := make([]tmcrypto.PrivKey, 2)
	validators := make([]*tmtypes.Validator, len(vKeys))
	genesisValidators := make([]tmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		validators[i] = &tmtypes.Validator{
			Address:          vKeys[i].PubKey().Address(),
			PubKey:           vKeys[i].PubKey(),
			VotingPower:      int64(i + 100),
			ProposerPriority: int64(i),
		}
		genesisValidators[i] = tmtypes.GenesisValidator{
			Address: vKeys[i].PubKey().Address(),
			PubKey:  vKeys[i].PubKey(),
			Power:   int64(i + 100),
			Name:    "one",
		}
	}
	app := kvstore.NewApplication()
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	sequencer, err := newFullNode(
		context.Background(),
		config.NodeConfig{
			DALayer: "mock",
			P2P: config.P2PConfig{
				ListenAddress: "/ip4/0.0.0.0/tcp/26656",
			},
			Aggregator: true,
			BlockManagerConfig: config.BlockManagerConfig{
				BlockTime: 10 * time.Millisecond,
			},
		},
		key,
		signingKey,
		proxy.NewLocalClientCreator(app),
		&tmtypes.GenesisDoc{
			ChainID:    "test",
			Validators: genesisValidators,
		},
		log.TestingLogger(),
	)
	require.NoError(t, err)
	require.NotNil(t, sequencer)
	err = sequencer.Start()
	require.NoError(t, err)
	sequencerClient := sequencer.GetClient()
	sequencerClient.BroadcastTxCommit(context.Background(), []byte("connor=cool"))
}
