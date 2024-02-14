package node

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da"
)

func getMockDA() *da.DAClient {
	return &da.DAClient{DA: goDATest.NewDummyDA(), GasPrice: -1, Logger: log.TestingLogger()}
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
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dalc := getMockDA()
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
		assert.NoError(fullNode.Stop())
	}()

	assert.NoError(lightNode.Start())
	defer func() {
		assert.NoError(lightNode.Stop())
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
