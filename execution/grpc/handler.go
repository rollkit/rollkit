package grpc

import (
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// NewExecutorServiceHandler creates a new HTTP handler for the ExecutorService.
// It follows the same pattern as other services in the codebase, supporting
// both HTTP/1.1 and HTTP/2 without TLS using h2c.
//
// Parameters:
// - executor: The execution implementation to serve
// - opts: Optional server options for configuring the service
//
// Returns:
// - http.Handler: The configured HTTP handler
func NewExecutorServiceHandler(executor execution.Executor, opts ...connect.HandlerOption) http.Handler {
	server := NewServer(executor)
	
	mux := http.NewServeMux()
	
	// Configure compression to start at 1KB
	compress1KB := connect.WithCompressMinBytes(1024)
	
	// Set up gRPC reflection for debugging and discovery
	reflector := grpcreflect.NewStaticReflector(
		v1connect.ExecutorServiceName,
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, compress1KB))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, compress1KB))
	
	// Register the ExecutorService
	path, handler := v1connect.NewExecutorServiceHandler(server, append(opts, compress1KB)...)
	mux.Handle(path, handler)
	
	// Use h2c to support HTTP/2 without TLS
	return h2c.NewHandler(mux, &http2.Server{
		IdleTimeout:          120 * time.Second,
		MaxReadFrameSize:     1 << 24, // 16MB
		MaxConcurrentStreams: 100,
		ReadIdleTimeout:      30 * time.Second,
		PingTimeout:          15 * time.Second,
	})
}