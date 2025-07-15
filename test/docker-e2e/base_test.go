//go:build docker_e2e

package docker_e2e

import (
	"context"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/require"
)

func (s *DockerTestSuite) TestBasicDockerE2E() {
	ctx := context.Background()
	s.SetupDockerResources()

	var (
		bridgeNode types.DANode
	)

	s.T().Run("start celestia chain", func(t *testing.T) {
		err := s.celestia.Start(ctx)
		s.Require().NoError(err)
	})

	s.T().Run("start bridge node", func(t *testing.T) {
		genesisHash := s.getGenesisHash(ctx)

		celestiaNodeHostname, err := s.celestia.GetNodes()[0].GetInternalHostName(ctx)
		s.Require().NoError(err)

		bridgeNode = s.daNetwork.GetBridgeNodes()[0]

		s.StartBridgeNode(ctx, bridgeNode, testChainID, genesisHash, celestiaNodeHostname)
	})

	s.T().Run("fund da wallet", func(t *testing.T) {
		daWallet, err := bridgeNode.GetWallet()
		s.Require().NoError(err)
		s.T().Logf("da node celestia address: %s", daWallet.GetFormattedAddress())

		s.FundWallet(ctx, daWallet, 100_000_000_00)
	})

	s.T().Run("start rollkit chain node", func(t *testing.T) {
		s.StartRollkitNode(ctx, bridgeNode, s.rollkitChain.GetNodes()[0])
	})

	s.T().Run("submit a transaction to the rollkit chain", func(t *testing.T) {
		rollkitNode := s.rollkitChain.GetNodes()[0]

// The http port resolvable by the test runner.
		httpPort := rollkitNode.GetHostHTTPPort()

		client, err := NewClient("localhost", httpPort)

		key := "key1"
		value := "value1"
		_, err = client.Post(ctx, "/tx", key, value)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			res, err := client.Get(ctx, "/kv?key="+key)
			if err != nil {
				return false
			}
			return string(res) == value
		}, 10*time.Second, time.Second)
	})
}
