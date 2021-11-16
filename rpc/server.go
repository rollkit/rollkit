package rpc

import (
	"net"
	"net/http"

	"golang.org/x/net/netutil"

	"github.com/rs/cors"
	"github.com/tendermint/tendermint/config"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/libs/service"

	"github.com/celestiaorg/optimint/node"
	"github.com/celestiaorg/optimint/rpc/client"
	"github.com/celestiaorg/optimint/rpc/json"
)

type Server struct {
	*service.BaseService

	config *config.RPCConfig
	local  *client.Client

	server http.Server
}

func NewServer(node *node.Node, config *config.RPCConfig) *Server {
	return &Server{
		config: config,
		local:  client.NewClient(node),
	}
}

func (s *Server) Client() rpcclient.Client {
	return s.local
}

func (s *Server) OnStart() error {
	return s.startRPC()
}

func (s *Server) startRPC() error {
	if s.config.ListenAddress == "" {
		s.Logger.Info("Listen address not specified - RPC will not be exposed")
		return nil
	}
	handler, err := json.GetHttpHandler(s.local)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return err
	}

	if s.config.MaxOpenConnections != 0 {
		s.Logger.Debug("limiting number of connections", "limit", s.config.MaxOpenConnections)
		listener = netutil.LimitListener(listener, s.config.MaxOpenConnections)
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
