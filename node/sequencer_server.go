package node

import (
	//"context"
	"errors"
	"net"
	"net/http"
	"strings"

	//"time"

	//"github.com/rs/cors"
	"github.com/rs/cors"
	"github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/net/netutil"
	//"github.com/tendermint/tendermint/libs/service"
	/*rpcclient "github.com/tendermint/tendermint/rpc/client"
	"golang.org/x/net/netutil"*///"github.com/rollkit/rollkit/rpc/json"
)

// Nodes of centralized roll-ups may connect directly to the sequencer
// p2p gossip is unnecessary and inefficient if you always know the sequencer
// this server will expose an RPC endpoint for receiving transactions directly
type SequencerServer struct {
	server http.Server
	node   *FullNode
	Logger log.Logger
	config *config.RPCConfig
}

func NewSequencerServer(node *FullNode, conf *config.RPCConfig, logger log.Logger) *SequencerServer {
	srv := &SequencerServer{
		node:   node,
		config: conf,
		Logger: logger,
	}
	return srv
}

func (s *SequencerServer) Start() error {
	if s.node.conf.SequencerListenAddress == "" {
		s.Logger.Info("SequencerListenAddress not specified -  sequencer server will not be started")
		return nil
	}
	parts := strings.SplitN(s.node.conf.SequencerListenAddress, "://", 2)
	if len(parts) != 2 {
		return errors.New("invalid RPC listen address: expecting tcp://host:port")
	}
	proto := parts[0]
	addr := parts[1]

	listener, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}

	if s.node.conf.SequencerMaxOpenConnections != 0 {
		s.Logger.Debug("limiting number of connections", "limit", s.node.conf.SequencerMaxOpenConnections)
		listener = netutil.LimitListener(listener, int(s.node.conf.SequencerMaxOpenConnections))
	}

	if s.config.IsCorsEnabled() {
		s.Logger.Debug("Sequencer CORS enabled",
			"origins", s.config.CORSAllowedOrigins,
			"methods", s.config.CORSAllowedMethods,
			"headers", s.config.CORSAllowedHeaders,
		)
		c := cors.New(cors.Options{
			AllowedOrigins: s.config.CORSAllowedOrigins,
			AllowedMethods: s.config.CORSAllowedMethods,
			AllowedHeaders: s.config.CORSAllowedHeaders,
		})
	}

	return nil
}
