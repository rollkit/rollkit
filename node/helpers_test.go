package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da"
)

func getMockDA(t *testing.T) *da.DAClient {
	namespace := make([]byte, len(MockDANamespace)/2)
	_, err := hex.Decode(namespace, []byte(MockDANamespace))
	require.NoError(t, err)
	return da.NewDAClient(goDATest.NewDummyDA(), -1, -1, namespace, log.TestingLogger())
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
	dalc := getMockDA(t)
	num := 2
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}
	bmConfig := getBMConfig()
	fullNode, _ := createAndConfigureNode(ctx, 0, true, false, keys, bmConfig, dalc, t)
	lightNode, _ := createNode(ctx, 1, true, true, keys, bmConfig, t)

	startNodeWithCleanup(t, fullNode)
	startNodeWithCleanup(t, lightNode)

	cases := []struct {
		desc   string
		node   Node
		source Source
	}{
		{"fullNode height from Header", fullNode, Header},
		{"fullNode height from Block", fullNode, Block},
		{"fullNode height from Store", fullNode, Store},
		{"lightNode height from Header", lightNode, Header},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			require.NoError(testutils.Retry(1000, 100*time.Millisecond, func() error {
				num, err := getNodeHeight(tc.node, tc.source)
				if err != nil {
					return err
				}
				if num > 0 {
					return nil
				}
				return errors.New("expected height > 0")
			}))
		})
	}
}
