package node

import (
	"errors"
	"fmt"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/conv"
	"github.com/rollkit/rollkit/types"
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

func getBMConfig() config.BlockManagerConfig {
	return config.BlockManagerConfig{
		DABlockTime: 100 * time.Millisecond,
		BlockTime:   1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		NamespaceID: types.NamespaceID{8, 7, 6, 5, 4, 3, 2, 1},
		FraudProofs: true,
	}
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

func getNodeHeightFromStore(node Node) (uint64, error) {
	if fn, ok := node.(*FullNode); ok {
		return fn.blockManager.GetStoreHeight(), nil
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

func waitForAtLeastNBlocksWithStoreHeight(node Node, n int) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		nHeight, err := getNodeHeightFromStore(node)
		if err != nil {
			return err
		}
		if nHeight >= uint64(n) {
			return nil
		}
		return fmt.Errorf("expected height > %v, got %v", n, nHeight)
	})
}

func verifyNodesSyncedWithStoreHeight(node1, node2 Node) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		n1Height, err := getNodeHeightFromStore(node1)
		if err != nil {
			return err
		}
		n2Height, err := getNodeHeightFromStore(node2)
		if err != nil {
			return err
		}
		if n1Height == n2Height {
			return nil
		}
		return fmt.Errorf("nodes not synced: node1 at height %v, node2 at height %v", n1Height, n2Height)
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
