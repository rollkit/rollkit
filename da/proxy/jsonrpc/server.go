package jsonrpc

import (
	"context"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	logger "cosmossdk.io/log"
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/rollkit/rollkit/core/da"
)

var log = logger.NewLogger(os.Stdout).With("module", "da-rpc")

// Server is a jsonrpc service that can serve the DA interface
type Server struct {
	srv      *http.Server
	rpc      *jsonrpc.RPCServer
	listener net.Listener

	started atomic.Bool
}

// RegisterService registers a service onto the RPC server. All methods on the service will then be
// exposed over the RPC.
func (s *Server) RegisterService(namespace string, service interface{}, out interface{}) {
	s.rpc.Register(namespace, service)
}

// NewServer accepts the host address port and the DA implementation to serve as a jsonrpc service
func NewServer(address, port string, DA da.DA) *Server {
	rpc := jsonrpc.NewServer(jsonrpc.WithServerErrors(getKnownErrorsMapping()))
	srv := &Server{
		rpc: rpc,
		srv: &http.Server{
			Addr: address + ":" + port,
			// the amount of time allowed to read request headers. set to the default 2 seconds
			ReadHeaderTimeout: 2 * time.Second,
		},
	}
	srv.srv.Handler = http.HandlerFunc(rpc.ServeHTTP)
	// Wrap the provided DA implementation with the logging decorator
	srv.RegisterService("da", DA, &API{}) // Register the wrapper
	return srv
}

// Start starts the RPC Server.
// This function can be called multiple times concurrently
// Once started, subsequent calls are a no-op
func (s *Server) Start(context.Context) error {
	couldStart := s.started.CompareAndSwap(false, true)
	if !couldStart {
		log.Warn("cannot start server: already started")
		return nil
	}
	listener, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	s.listener = listener
	log.Info("server started", "listening on", s.srv.Addr)
	//nolint:errcheck
	go s.srv.Serve(listener)
	return nil
}

// Stop stops the RPC Server.
// This function can be called multiple times concurrently
// Once stopped, subsequent calls are a no-op
func (s *Server) Stop(ctx context.Context) error {
	couldStop := s.started.CompareAndSwap(true, false)
	if !couldStop {
		log.Warn("cannot stop server: already stopped")
		return nil
	}
	err := s.srv.Shutdown(ctx)
	if err != nil {
		return err
	}
	s.listener = nil
	log.Info("server stopped")
	return nil
}
