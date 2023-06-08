package node

import (
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/conv"
	"github.com/rollkit/rollkit/mocks"
)

var genesisValidatorKey = ed25519.GenPrivKey()

func countFunctionCalls(app *mocks.Application, functionName string) uint {
	var count uint = 0
	for _, call := range app.Calls {
		if call.Method == functionName {
			count++
		}
	}
	return count
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
