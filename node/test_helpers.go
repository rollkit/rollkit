package node

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/libs/log"
	p2p "github.com/cometbft/cometbft/p2p"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/store"
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

func (m MockTester) Fail() {}

func (m MockTester) FailNow() {}

func (m MockTester) Logf(format string, args ...interface{}) {}

func (m MockTester) Errorf(format string, args ...interface{}) {}

func waitForFirstBlock(node Node, source Source) error {
	return waitForAtLeastNBlocks(node, 1, source)
}

func getBMConfig() config.BlockManagerConfig {
	return config.BlockManagerConfig{
		DABlockTime: 100 * time.Millisecond,
		BlockTime:   1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		NamespaceID: types.NamespaceID{8, 7, 6, 5, 4, 3, 2, 1},
	}
}

func getNodeHeight(node Node, source Source) (uint64, error) {
	switch source {
	case Header:
		return getNodeHeightFromHeader(node)
	case Block:
		return getNodeHeightFromBlock(node)
	case Store:
		return getNodeHeightFromStore(node)
	default:
		return 0, errors.New("invalid source")
	}
}

func getNodeHeightFromHeader(node Node) (uint64, error) {
	if fn, ok := node.(*FullNode); ok {
		return fn.hSyncService.HeaderStore().Height(), nil
	}
	if ln, ok := node.(*LightNode); ok {
		return ln.hSyncService.HeaderStore().Height(), nil
	}
	return 0, errors.New("not a full or light node")
}

func getNodeHeightFromBlock(node Node) (uint64, error) {
	if fn, ok := node.(*FullNode); ok {
		return fn.bSyncService.BlockStore().Height(), nil
	}
	return 0, errors.New("not a full node")
}

func getNodeHeightFromStore(node Node) (uint64, error) {
	if fn, ok := node.(*FullNode); ok {
		return fn.blockManager.GetStoreHeight(), nil
	}
	return 0, errors.New("not a full node")
}

// safeClose closes the channel if it's not closed already
func safeClose(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
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

func getGenesisValidatorSetWithSigner() ([]cmtypes.GenesisValidator, crypto.PrivKey) {
	nodeKey := &p2p.NodeKey{
		PrivKey: genesisValidatorKey,
	}
	signingKey, _ := GetNodeKey(nodeKey)
	pubKey := genesisValidatorKey.PubKey()

	genesisValidators := []cmtypes.GenesisValidator{{
		Address: pubKey.Address(),
		PubKey:  pubKey,
		Power:   int64(100),
		Name:    "gen #1",
	}}
	return genesisValidators, signingKey
}

func TestMockTester(t *testing.T) {
	m := MockTester{t}
	m.Fail()
	m.FailNow()
	m.Logf("hello")
	m.Errorf("goodbye")
}

func TestGetNodeHeight(t *testing.T) {
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	_ = dalc.Start()
	defer func() {
		require.NoError(dalc.Stop())
	}()
	num := 2
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}
	bmConfig := getBMConfig()
	fullNode, _ := createNode(ctx, 0, true, false, keys, bmConfig, t)
	lightNode, _ := createNode(ctx, 1, true, true, keys, bmConfig, t)
	fullNode.(*FullNode).dalc = dalc
	fullNode.(*FullNode).blockManager.SetDALC(dalc)
	require.NoError(fullNode.Start())
	defer func() {
		require.NoError(fullNode.Stop())
	}()

	require.NoError(lightNode.Start())
	defer func() {
		require.NoError(lightNode.Stop())
	}()

	require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
		num, err := getNodeHeight(fullNode, Header)
		if err != nil {
			return err
		}
		if num > 0 {
			return nil
		}
		return errors.New("expected height > 0")
	}))
	require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
		num, err := getNodeHeight(fullNode, Block)
		if err != nil {
			return err
		}
		if num > 0 {
			return nil
		}
		return errors.New("expected height > 0")
	}))
	require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
		num, err := getNodeHeight(fullNode, Store)
		if err != nil {
			return err
		}
		if num > 0 {
			return nil
		}
		return errors.New("expected height > 0")
	}))
	require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
		num, err := getNodeHeight(lightNode, Header)
		if err != nil {
			return err
		}
		if num > 0 {
			return nil
		}
		return errors.New("expected height > 0")
	}))
}
