package rpc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/cors"
	"github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	"golang.org/x/net/netutil"

	"github.com/celestiaorg/optimint/node"
	"github.com/celestiaorg/optimint/rpc/client"
	"github.com/celestiaorg/optimint/rpc/json"
)

type Server struct {
	*service.BaseService

	config *config.RPCConfig
	client *client.Client

	server http.Server
}

func NewServer(node *node.Node, config *config.RPCConfig, logger log.Logger) *Server {
	srv := &Server{
		config: config,
		client: client.NewClient(node),
	}
	srv.BaseService = service.NewBaseService(logger, "RPC", srv)
	return srv
}

func (s *Server) Client() rpcclient.Client {
	return s.client
}

func (s *Server) OnStart() error {
	return s.startRPC()
}

func (s *Server) OnStop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		s.Logger.Error("error while shuting down RPC server", "error", err)
	}
}

func (s *Server) startRPC() error {
	if s.config.ListenAddress == "" {
		s.Logger.Info("Listen address not specified - RPC will not be exposed")
		return nil
	}
	parts := strings.SplitN(s.config.ListenAddress, "://", 2)
	if len(parts) != 2 {
		return errors.New("invalid RPC listen address: expecting tcp://host:port")
	}
	proto := parts[0]
	addr := parts[1]

	listener, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}

	if s.config.MaxOpenConnections != 0 {
		s.Logger.Debug("limiting number of connections", "limit", s.config.MaxOpenConnections)
		listener = netutil.LimitListener(listener, s.config.MaxOpenConnections)
	}

	handler, err := json.GetHttpHandler(s.client, s.Logger)
	if err != nil {
		return err
	}

	if s.config.IsCorsEnabled() {
		s.Logger.Debug("CORS enabled",
			"origins", s.config.CORSAllowedOrigins,
			"methods", s.config.CORSAllowedMethods,
			"headers", s.config.CORSAllowedHeaders,
		)
		c := cors.New(cors.Options{
			AllowedOrigins: s.config.CORSAllowedOrigins,
			AllowedMethods: s.config.CORSAllowedMethods,
			AllowedHeaders: s.config.CORSAllowedHeaders,
		})
		handler = c.Handler(handler)
	}

	go func() {
		err := s.serve(listener, handler)
		if err != http.ErrServerClosed {
			s.Logger.Error("error while serving HTTP", "error", err)
		}
	}()

	return nil
}

func (s *Server) serve(listener net.Listener, handler http.Handler) error {
	s.Logger.Info("serving HTTP", "listen address", listener.Addr())
	s.server = http.Server{Handler: handler}
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		return s.server.ServeTLS(listener, s.config.CertFile(), s.config.KeyFile())
	}
	return s.server.Serve(listener)
}
