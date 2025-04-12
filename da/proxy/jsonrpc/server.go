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

// loggingDA wraps a da.DA implementation with logging capabilities.
type loggingDA struct {
	baseDA da.DA
	logger logger.Logger
}

// NewLoggingDA creates a new logging wrapper for a da.DA implementation.
func NewLoggingDA(base da.DA, log logger.Logger) da.DA {
	return &loggingDA{
		baseDA: base,
		logger: log.With("layer", "server"),
	}
}

// Implement da.DA interface for loggingDA
func (l *loggingDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	l.logger.Debug("RPC call received", "method", "MaxBlobSize")
	res, err := l.baseDA.MaxBlobSize(ctx)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "MaxBlobSize", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "MaxBlobSize", "result", res)
	}
	return res, err
}

func (l *loggingDA) Get(ctx context.Context, ids []da.ID, ns []byte) ([]da.Blob, error) {
	l.logger.Debug("RPC call received", "method", "Get", "num_ids", len(ids), "namespace", string(ns))
	res, err := l.baseDA.Get(ctx, ids, ns)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "Get", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "Get", "num_blobs_returned", len(res))
	}
	return res, err
}

func (l *loggingDA) GetIDs(ctx context.Context, height uint64, ns []byte) (*da.GetIDsResult, error) {
	l.logger.Debug("RPC call received", "method", "GetIDs", "height", height, "namespace", string(ns))
	res, err := l.baseDA.GetIDs(ctx, height, ns)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "GetIDs", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "GetIDs", "num_ids_returned", len(res.IDs))
	}
	return res, err
}

func (l *loggingDA) GetProofs(ctx context.Context, ids []da.ID, ns []byte) ([]da.Proof, error) {
	l.logger.Debug("RPC call received", "method", "GetProofs", "num_ids", len(ids), "namespace", string(ns))
	res, err := l.baseDA.GetProofs(ctx, ids, ns)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "GetProofs", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "GetProofs", "num_proofs_returned", len(res))
	}
	return res, err
}

func (l *loggingDA) Commit(ctx context.Context, blobs []da.Blob, ns []byte) ([]da.Commitment, error) {
	l.logger.Debug("RPC call received", "method", "Commit", "num_blobs", len(blobs), "namespace", string(ns))
	res, err := l.baseDA.Commit(ctx, blobs, ns)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "Commit", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "Commit", "num_commitments_returned", len(res))
	}
	return res, err
}

func (l *loggingDA) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, ns []byte) ([]bool, error) {
	l.logger.Debug("RPC call received", "method", "Validate", "num_ids", len(ids), "num_proofs", len(proofs), "namespace", string(ns))
	res, err := l.baseDA.Validate(ctx, ids, proofs, ns)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "Validate", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "Validate", "num_results_returned", len(res))
	}
	return res, err
}

func (l *loggingDA) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, ns []byte, options []byte) ([]da.ID, error) {
	l.logger.Debug("RPC call received", "method", "Submit", "num_blobs", len(blobs), "gas_price", gasPrice, "namespace", string(ns))
	res, err := l.baseDA.Submit(ctx, blobs, gasPrice, ns, options)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "Submit", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "Submit", "num_ids_returned", len(res))
	}
	return res, err
}

func (l *loggingDA) GasMultiplier(ctx context.Context) (float64, error) {
	l.logger.Debug("RPC call received", "method", "GasMultiplier")
	res, err := l.baseDA.GasMultiplier(ctx)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "GasMultiplier", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "GasMultiplier", "result", res)
	}
	return res, err
}

func (l *loggingDA) GasPrice(ctx context.Context) (float64, error) {
	l.logger.Debug("RPC call received", "method", "GasPrice")
	res, err := l.baseDA.GasPrice(ctx)
	if err != nil {
		l.logger.Error("RPC call failed", "method", "GasPrice", "error", err)
	} else {
		l.logger.Debug("RPC call successful", "method", "GasPrice", "result", res)
	}
	return res, err
}

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
	loggingWrapper := NewLoggingDA(DA, log)
	srv.RegisterService("da", loggingWrapper, &API{}) // Register the wrapper
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
