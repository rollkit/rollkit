//go:build docker_e2e

package docker_e2e

import (
	"context"
	"fmt"
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

// TestFullNodeSyncFromDA tests that a Rollkit full node can sync from DA without P2P
func (s *DockerTestSuite) TestFullNodeSyncFromDA() {
	ctx := context.Background()
	
	// Configure to create 2 Rollkit nodes: one sequencer, one full node
	s.SetupDockerResources(func(cfg *tastoradocker.Config) {
		cfg.RollkitChainConfig.NumNodes = 2
	})

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

	// Get the namespace from the sequencer for the full node to use
	var daNamespace string
	s.T().Run("get sequencer namespace", func(t *testing.T) {
		// For this test, we'll use a fixed namespace that both nodes share
		daNamespace = generateValidNamespaceHex()
	})

	s.T().Run("start rollkit sequencer node", func(t *testing.T) {
		sequencerNode := s.rollkitChain.GetNodes()[0]
		s.StartRollkitNodeWithNamespace(ctx, bridgeNode, sequencerNode, daNamespace)
	})

	s.T().Run("submit transactions to the rollkit sequencer", func(t *testing.T) {
		sequencerNode := s.rollkitChain.GetNodes()[0]
		httpPort := sequencerNode.GetHostHTTPPort()
		client, err := NewClient("localhost", httpPort)
		s.Require().NoError(err)

		// Submit a few transactions to create some blocks to sync
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)
			_, err = client.Post(ctx, "/tx", key, value)
			s.Require().NoError(err)
		}

		// Wait for transactions to be processed
		time.Sleep(5 * time.Second)
	})

	s.T().Run("start rollkit full node", func(t *testing.T) {
		fullNode := s.rollkitChain.GetNodes()[1]
		s.StartRollkitFullNode(ctx, bridgeNode, fullNode, daNamespace)
	})

	s.T().Run("verify full node syncs from DA", func(t *testing.T) {
		fullNode := s.rollkitChain.GetNodes()[1]
		httpPort := fullNode.GetHostHTTPPort()
		
		// Give the full node time to sync from DA
		time.Sleep(10 * time.Second)
		
		client, err := NewClient("localhost", httpPort)
		s.Require().NoError(err)

		// Verify the full node has synced the data from DA
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key%d", i)
			expectedValue := fmt.Sprintf("value%d", i)
			
			require.Eventually(s.T(), func() bool {
				res, err := client.Get(ctx, "/kv?key="+key)
				if err != nil {
					s.T().Logf("Error getting key %s: %v", key, err)
					return false
				}
				return string(res) == expectedValue
			}, 30*time.Second, 2*time.Second, "Full node should sync key %s with value %s", key, expectedValue)
		}
	})
}
