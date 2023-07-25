package node

import (
	"errors"
	"fmt"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/p2p"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/conv"
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

func waitForFirstBlock(node *FullNode, useBlockExchange bool) error {
	return waitForAtLeastNBlocks(node, 1, useBlockExchange)
}

func getNodeHeight(node Node, useBlockExchange bool) (uint64, error) {
	if useBlockExchange {
		return getNodeHeightFromBlock(node)
	}
	if fn, ok := node.(*FullNode); ok {
		return fn.hExService.headerStore.Height(), nil
	}
	if ln, ok := node.(*LightNode); ok {
		return ln.hExService.headerStore.Height(), nil
	}
	return 0, errors.New("not a full or light node")
}

func getNodeHeightFromBlock(node Node) (uint64, error) {
	if fn, ok := node.(*FullNode); ok {
		return fn.bExService.blockStore.Height(), nil
	}
	return 0, errors.New("not a full node")
}

func verifyNodesSynced(node1, node2 Node, useBlockExchange bool) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		n1Height, err := getNodeHeight(node1, useBlockExchange)
		if err != nil {
			return err
		}
		n2Height, err := getNodeHeight(node2, useBlockExchange)
		if err != nil {
			return err
		}
		if n1Height == n2Height {
			return nil
		}
		return fmt.Errorf("nodes not synced: node1 at height %v, node2 at height %v", n1Height, n2Height)
	})
}

func waitForAtLeastNBlocks(node Node, n int, useBlockExchange bool) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		nHeight, err := getNodeHeight(node, useBlockExchange)
		if err != nil {
			return err
		}
		if nHeight >= uint64(n) {
			return nil
		}
		return fmt.Errorf("expected height > %v, got %v", n, nHeight)
	})
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
