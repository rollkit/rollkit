package node

import (
	"errors"

	"github.com/celestiaorg/go-fraud"
	"github.com/celestiaorg/go-fraud/fraudserv"
	abci "github.com/cometbft/cometbft/abci/types"
	proxy "github.com/cometbft/cometbft/proxy"
	"github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/types"
)

var VerifierFn = func(proxyApp proxy.AppConns) func(fraudProof fraud.Proof) (bool, error) {
	return func(fraudProof fraud.Proof) (bool, error) {
		stateFraudProof, ok := fraudProof.(*types.StateFraudProof)
		if !ok {
			return false, errors.New("unknown fraud proof")
		}
		resp, err := proxyApp.Consensus().VerifyFraudProofSync(
			abci.RequestVerifyFraudProof{
				FraudProof:           &stateFraudProof.FraudProof,
				ExpectedValidAppHash: stateFraudProof.ExpectedValidAppHash,
			},
		)
		if err != nil {
			return false, err
		}
		return resp.Success, nil
	}
}

type ProofServiceFactory struct {
	client        *p2p.Client
	getter        fraud.HeaderFetcher
	ds            datastore.Datastore
	syncerEnabled bool
	networkID     string
}

func NewProofServiceFactory(c *p2p.Client, getter fraud.HeaderFetcher, ds datastore.Datastore, syncerEnabled bool, networkID string) ProofServiceFactory {
	return ProofServiceFactory{
		client:        c,
		getter:        getter,
		ds:            ds,
		syncerEnabled: syncerEnabled,
		networkID:     networkID,
	}
}

func (factory *ProofServiceFactory) CreateProofService() *fraudserv.ProofService {
	return fraudserv.NewProofService(
		factory.client.PubSub(),
		factory.client.Host(),
		factory.getter,
		factory.ds,
		factory.syncerEnabled,
		factory.networkID,
	)
}
