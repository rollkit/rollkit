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

type Source int

const (
	Header Source = iota
	Block
	Store
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

func waitForFirstBlock(node *FullNode, source Source) error {
	return waitForAtLeastNBlocks(node, 1, source)
}

func getBMConfig() config.BlockManagerConfig {
	return config.BlockManagerConfig{
		DABlockTime: 100 * time.Millisecond,
		BlockTime:   1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		NamespaceID: types.NamespaceID{8, 7, 6, 5, 4, 3, 2, 1},
		FraudProofs: true,
	}
}

func getNodeHeight(node Node, source Source) (uint64, error) {
	if source == Header {
		return getNodeHeightFromHeader(node)
	} else if source == Block {
		return getNodeHeightFromBlock(node)
	} else if source == Store {
		return getNodeHeightFromStore(node)
	} else {
		return 0, errors.New("invalid source")
	}
}

func getNodeHeightFromHeader(node Node) (uint64, error) {
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

func verifyNodesSynced(node1, node2 Node, source Source) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		n1Height, err := getNodeHeight(node1, source)
		if err != nil {
			return err
		}
		n2Height, err := getNodeHeight(node2, source)
		if err != nil {
			return err
		}
		if n1Height == n2Height {
			return nil
		}
		return fmt.Errorf("nodes not synced: node1 at height %v, node2 at height %v", n1Height, n2Height)
	})
}

func waitForAtLeastNBlocks(node Node, n int, source Source) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		nHeight, err := getNodeHeight(node, source)
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
