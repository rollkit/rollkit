package node

import (
	"context"
	"fmt"

	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	abciclient "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
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

	app abciclient.Client

	hExService *HeaderExchangeService

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
	appClient abciclient.Client,
	genesis *tmtypes.GenesisDoc,
	logger log.Logger,
) (*LightNode, error) {
	datastore, err := openDatastore(conf, logger)
	if err != nil {
		return nil, err
	}
	client, err := p2p.NewClient(conf.P2P, p2pKey, genesis.ChainID, datastore, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	headerExchangeService, err := NewHeaderExchangeService(ctx, datastore, conf, genesis, client, make(chan *types.SignedHeader), nil, logger.With("module", "HeaderExchangeService"))
	if err != nil {
		return nil, fmt.Errorf("HeaderExchangeService initialization error: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	node := &LightNode{
		P2P:        client,
		app:        appClient,
		hExService: headerExchangeService,
		cancel:     cancel,
		ctx:        ctx,
	}

	node.P2P.SetTxValidator(node.falseValidator())
	node.P2P.SetHeaderValidator(node.falseValidator())
	node.P2P.SetFraudProofValidator(node.newFraudProofValidator())

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

func (ln *LightNode) newFraudProofValidator() p2p.GossipValidator {
	return func(fraudProofMsg *p2p.GossipMessage) bool {
		ln.Logger.Info("fraud proof received", "from", fraudProofMsg.From, "bytes", len(fraudProofMsg.Data))
		fraudProof := abci.FraudProof{}
		err := fraudProof.Unmarshal(fraudProofMsg.Data)
		if err != nil {
			ln.Logger.Error("failed to deserialize fraud proof", "error", err)
			return false
		}

		resp, err := ln.app.VerifyFraudProofSync(abci.RequestVerifyFraudProof{
			FraudProof:           &fraudProof,
			ExpectedValidAppHash: fraudProof.ExpectedValidAppHash,
		})
		if err != nil {
			return false
		}

		if resp.Success {
			panic("received valid fraud proof! halting light client")
		}

		return false
	}
}
