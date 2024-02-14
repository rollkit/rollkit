package node

import (
	"errors"
	"fmt"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"

	"github.com/rollkit/rollkit/config"
)

// Source is an enum representing different sources of height
type Source int

const (
	// Header is the source of height from the header service
	Header Source = iota
	// Block is the source of height from the block service
	Block
	// Store is the source of height from the block manager store
	Store
)

// MockTester is a mock testing.T
type MockTester struct {
	t *testing.T
}

// Fail is used to fail the test
func (m MockTester) Fail() {}

// FailNow is used to fail the test immediately
func (m MockTester) FailNow() {}

// Logf is used to log a message to the test logger
func (m MockTester) Logf(format string, args ...interface{}) {}

// Errorf is used to log an error to the test logger
func (m MockTester) Errorf(format string, args ...interface{}) {}

func waitForFirstBlock(node Node, source Source) error {
	return waitForAtLeastNBlocks(node, 1, source)
}

func getBMConfig() config.BlockManagerConfig {
	return config.BlockManagerConfig{
		DABlockTime: 100 * time.Millisecond,
		BlockTime:   1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
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

func isBlockHashSeen(node Node, blockHash string) bool {
	if fn, ok := node.(*FullNode); ok {
		return fn.blockManager.IsBlockHashSeen(blockHash)
	}
	return false
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

func waitUntilBlockHashSeen(node Node, blockHash string) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		if isBlockHashSeen(node, blockHash) {
			return nil
		}
		return fmt.Errorf("block hash %v not seen", blockHash)
	})
}
