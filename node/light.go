package node

import (
	"context"
	"fmt"

	"github.com/celestiaorg/go-fraud/fraudserv"
	"github.com/celestiaorg/go-header"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	proxy "github.com/tendermint/tendermint/proxy"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	tmtypes "github.com/tendermint/tendermint/types"
	"go.uber.org/multierr"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

var _ Node = &LightNode{}

type LightNode struct {
	service.BaseService

	P2P *p2p.Client

	proxyApp proxy.AppConns

	hExService          *HeaderExchangeService
	fraudService        *fraudserv.ProofService
	proofServiceFactory ProofServiceFactory

	ctx    context.Context
	cancel context.CancelFunc
}

func (ln *LightNode) GetClient() rpcclient.Client {
	return NewLightClient(ln)
}

func newLightNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	clientCreator proxy.ClientCreator,
	genesis *tmtypes.GenesisDoc,
	logger log.Logger,
) (*LightNode, error) {
	// Create the proxyApp and establish connections to the ABCI app (consensus, mempool, query).
	proxyApp := proxy.NewAppConns(clientCreator)
	proxyApp.SetLogger(logger.With("module", "proxy"))
	if err := proxyApp.Start(); err != nil {
		return nil, fmt.Errorf("error starting proxy app connections: %v", err)
	}

	datastore, err := openDatastore(conf, logger)
	if err != nil {
		return nil, err
	}
	client, err := p2p.NewClient(conf.P2P, p2pKey, genesis.ChainID, datastore, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	headerExchangeService, err := NewHeaderExchangeService(ctx, datastore, conf, genesis, client, logger.With("module", "HeaderExchangeService"))
	if err != nil {
		return nil, fmt.Errorf("HeaderExchangeService initialization error: %w", err)
	}

	fraudProofFactory := NewProofServiceFactory(
		client,
		func(ctx context.Context, u uint64) (header.Header, error) {
			return headerExchangeService.headerStore.GetByHeight(ctx, u)
		},
		datastore,
		true,
		genesis.ChainID,
	)

	ctx, cancel := context.WithCancel(ctx)

	node := &LightNode{
		P2P:                 client,
		proxyApp:            proxyApp,
		hExService:          headerExchangeService,
		proofServiceFactory: fraudProofFactory,
		cancel:              cancel,
		ctx:                 ctx,
	}

	node.P2P.SetTxValidator(node.falseValidator())
	node.P2P.SetHeaderValidator(node.falseValidator())

	node.BaseService = *service.NewBaseService(logger, "LightNode", node)

	return node, nil
}

func openDatastore(conf config.NodeConfig, logger log.Logger) (ds.TxnDatastore, error) {
	if conf.RootDir == "" && conf.DBPath == "" { // this is used for testing
		logger.Info("WARNING: working in in-memory mode")
		return store.NewDefaultInMemoryKVStore()
	}
	return store.NewDefaultKVStore(conf.RootDir, conf.DBPath, "rollkit-light")
}

func (ln *LightNode) OnStart() error {
	if err := ln.P2P.Start(ln.ctx); err != nil {
		return err
	}

	if err := ln.hExService.Start(); err != nil {
		return fmt.Errorf("error while starting header exchange service: %w", err)
	}

	ln.fraudService = ln.proofServiceFactory.CreateProofService()
	if err := ln.fraudService.Start(ln.ctx); err != nil {
		return fmt.Errorf("error while starting fraud exchange service: %w", err)
	}

	go ln.ProcessFraudProof()

	return nil
}

func (ln *LightNode) OnStop() {
	ln.Logger.Info("halting light node...")
	ln.cancel()
	err := ln.P2P.Close()
	err = multierr.Append(err, ln.hExService.Stop())
	ln.Logger.Error("errors while stopping node:", "errors", err)
}

// Dummy validator that always returns a callback function with boolean `false`
func (ln *LightNode) falseValidator() p2p.GossipValidator {
	return func(*p2p.GossipMessage) bool {
		return false
	}
}

func (ln *LightNode) ProcessFraudProof() {
	// subscribe to state fraud proof
	sub, err := ln.fraudService.Subscribe(types.StateFraudProofType)
	if err != nil {
		ln.Logger.Error("failed to subscribe to fraud proof gossip", "error", err)
		return
	}

	// continuously process the fraud proofs received via subscription
	for {
		proof, err := sub.Proof(ln.ctx)
		if err != nil {
			ln.Logger.Error("failed to receive gossiped fraud proof", "error", err)
			return
		}

		// only handle the state fraud proofs for now
		fraudProof, ok := proof.(*types.StateFraudProof)
		if !ok {
			ln.Logger.Error("unexpected type received for state fraud proof", "error", err)
			return
		}
		ln.Logger.Debug("fraud proof received",
			"block height", fraudProof.BlockHeight,
			"pre-state app hash", fraudProof.PreStateAppHash,
			"expected valid app hash", fraudProof.ExpectedValidAppHash,
			"length of state witness", len(fraudProof.StateWitness),
		)

		resp, err := ln.proxyApp.Consensus().VerifyFraudProofSync(abci.RequestVerifyFraudProof{
			FraudProof:           &fraudProof.FraudProof,
			ExpectedValidAppHash: fraudProof.ExpectedValidAppHash,
		})
		if err != nil {
			ln.Logger.Error("failed to verify fraud proof", "error", err)
			continue
		}

		if resp.Success {
			panic("received valid fraud proof! halting light client")
		}
	}
}
