package node

import (
	"errors"
	"fmt"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/conv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"
)

var genesisValidatorKey = ed25519.GenPrivKey()

type MockTester struct {
	t *testing.T
}

func (m MockTester) Fail() {
	fmt.Println("Failed")
}

func (m MockTester) FailNow() {
	fmt.Println("MockTester FailNow called")
}

func (m MockTester) Logf(format string, args ...interface{}) {
	fmt.Println("MockTester Logf called")
}

func (m MockTester) Errorf(format string, args ...interface{}) {
	//fmt.Printf(format, args...)
	fmt.Println("Errorf called")
}

func waitForFirstBlock(assert *assert.Assertions, require *require.Assertions, node Node) {
	require.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
		fn, ok := node.(*FullNode)
		assert.Equal(ok, true)
		if fn.Store.Height() >= 1 {
			return nil
		} else {
			fmt.Println("Retrying...")
			return errors.New("awaiting first block")
		}
	}))
}

func waitForAtLeastNBlocks(assert *assert.Assertions, require *require.Assertions, node Node, n int) {
	require.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
		fn, ok := node.(*FullNode)
		assert.Equal(ok, true)
		if fn.Store.Height() >= uint64(n) {
			return nil
		} else {
			fmt.Println("Retrying...")
			return errors.New("awaiting n blocks...")
		}
	}))
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
