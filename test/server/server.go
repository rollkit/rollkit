package server

import (
	"context"
	"net"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	grpc2 "github.com/rollkit/go-da/proxy/grpc"
	"github.com/rollkit/go-da/proxy/jsonrpc"
	"github.com/rollkit/go-da/test"
)

// StartMockDAServGRPC starts a mock gRPC server with the given listenAddress.
//
// The server uses a test.NewDummyDA() as the service implementation and  insecure credentials.
// The function returns the created server instance.
func StartMockDAServGRPC(listenAddress string) *grpc.Server {
	server := grpc2.NewServer(test.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	addr, _ := url.Parse(listenAddress)
	lis, err := net.Listen("tcp", addr.Host)
	if err != nil {
		panic(err)
	}
	go func() {
		_ = server.Serve(lis)
	}()
	return server
}

// StartMockDAServJSONRPC starts a mock JSON-RPC server with the given listenAddress.
//
// The server uses a test.NewDummyDA() as the service implementation.
// The function returns the created server instance.
func StartMockDAServJSONRPC(ctx context.Context, listenAddress string) *jsonrpc.Server {
	addr, _ := url.Parse(listenAddress)
	srv := jsonrpc.NewServer(addr.Hostname(), addr.Port(), test.NewDummyDA())
	err := srv.Start(ctx)
	if err != nil {
		panic(err)
	}
	return srv
}
