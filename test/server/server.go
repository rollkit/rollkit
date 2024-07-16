package server

import (
	"context"
	grpc2 "github.com/rollkit/go-da/proxy/grpc"
	"github.com/rollkit/go-da/proxy/jsonrpc"
	"github.com/rollkit/go-da/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/url"
)

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

func StartMockDAServJSONRPC(ctx context.Context, listenAddress string) *jsonrpc.Server {
	addr, _ := url.Parse(listenAddress)
	srv := jsonrpc.NewServer(addr.Hostname(), addr.Port(), test.NewDummyDA())
	err := srv.Start(ctx)
	if err != nil {
		panic(err)
	}
	return srv
}
