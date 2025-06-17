package docker_e2e

import (
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/suite"
	"testing"
)

func (s *DockerTestSuite) TestBasicDockerE2E() {
	ctx := context.Background()
	s.SetupDockerResources()

	var (
		bridgeNode types.DANode
	)

	s.T().Run("celestia started", func(t *testing.T) {
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

	s.T().Run("fund da wallet address", func(t *testing.T) {
		daWallet, err := bridgeNode.GetWallet()
		s.Require().NoError(err)
		s.T().Logf("da node celestia address: %s", daWallet.GetFormattedAddress())

		s.FundWallet(ctx, daWallet, 100_000_000_00)
	})

	s.T().Run("start rollkit node", func(t *testing.T) {
		s.StartRollkitNode(ctx, bridgeNode, s.rollkitChain.GetNodes()[0])
	})
}

func TestDockerSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	suite.Run(t, new(DockerTestSuite))
}
