package node

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"

	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/store"
)

func TestMockTester(t *testing.T) {
	m := MockTester{t}
	m.Fail()
	m.FailNow()
	m.Logf("hello")
	m.Errorf("goodbye")
}

func TestGetNodeHeight(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	_ = dalc.Start()
	num := 2
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}
	fullNode, _ := createNode(ctx, 0, false, true, false, keys, t)
	lightNode, _ := createNode(ctx, 1, false, true, true, keys, t)
	fullNode.(*FullNode).dalc = dalc
	fullNode.(*FullNode).blockManager.SetDALC(dalc)
	require.NoError(fullNode.Start())
	require.NoError(lightNode.Start())
	require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
		num, err := getNodeHeight(fullNode)
		if err != nil {
			return err
		}
		if num > 0 {
			return nil
		}
		return errors.New("expected height > 0")
	}))
	require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
		num, err := getNodeHeight(lightNode)
		if err != nil {
			return err
		}
		if num > 0 {
			return nil
		}
		return errors.New("expected height > 0")
	}))
}
