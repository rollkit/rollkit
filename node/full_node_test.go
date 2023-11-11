package node

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	testutils "github.com/celestiaorg/utils/test"

	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/test/mocks"
)

// TestStartup checks if the node starts and stops without any errors
func TestStartup(t *testing.T) {
	ctx := context.Background()
	node := initializeAndStartFullNode(ctx, t)
	defer cleanUpNode(node, t)
}

func TestMempoolDirectly(t *testing.T) {
	ctx := context.Background()

	node := initializeAndStartFullNode(ctx, t)
	defer cleanUpNode(node, t)
	assert := assert.New(t)

	peerID := getPeerID(assert)
	verifyTransactions(node, peerID, assert)
	verifyMempoolSize(node, assert)
}

// setupMockApplication initializes a mock application
func setupMockApplication() *mocks.Application {
	app := &mocks.Application{}
	app.On(InitChain, mock.Anything).Return(abci.ResponseInitChain{})
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	return app
}

// generateSingleKey generates a single private key
func generateSingleKey() crypto.PrivKey {
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	return key
}

// getPeerID generates a peer ID
func getPeerID(assert *assert.Assertions) peer.ID {
	key := generateSingleKey()
	peerID, err := peer.IDFromPrivateKey(key)
	assert.NoError(err)
	return peerID
}

// verifyTransactions checks if transactions are valid
func verifyTransactions(node *FullNode, peerID peer.ID, assert *assert.Assertions) {
	transactions := []string{"tx1", "tx2", "tx3", "tx4"}
	for _, tx := range transactions {
		err := node.Mempool.CheckTx([]byte(tx), func(r *abci.Response) {}, mempool.TxInfo{
			SenderID: node.mempoolIDs.GetForPeer(peerID),
		})
		assert.NoError(err)
	}
}

// verifyMempoolSize checks if the mempool size is as expected
func verifyMempoolSize(node *FullNode, assert *assert.Assertions) {
	assert.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
		expectedSize := int64(4 * len("tx*"))
		actualSize := node.Mempool.SizeBytes()
		if expectedSize == actualSize {
			return nil
		}
		return fmt.Errorf("expected size %v, got size %v", expectedSize, actualSize)
	}))
}
