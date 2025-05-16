package node

import (
	"fmt"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/stretchr/testify/require"
)

func TestBlockSynchronization(t *testing.T) {
	// Start primary node
	primary, cleanup := createNodeWithCleanup(t, getTestConfig(t, 1))
	defer cleanup()

	// Create syncing node
	syncNode, syncCleanup := createNodeWithCleanup(t, getTestConfig(t, 2))
	defer syncCleanup()

	// Verify sync
	require.NoError(t, waitForSync(syncNode, primary))
}

func waitForSync(syncNode, source Node) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		syncHeight, _ := getNodeHeight(syncNode, Header)
		sourceHeight, _ := getNodeHeight(source, Header)

		if syncHeight >= sourceHeight {
			return nil
		}
		return fmt.Errorf("node at height %d, source at %d", syncHeight, sourceHeight)
	})
}
