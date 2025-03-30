package proxy

import (
	"context"
	"fmt"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rollkit/rollkit/da"
	proxygrpc "github.com/rollkit/rollkit/da/proxy/grpc"
	proxyjsonrpc "github.com/rollkit/rollkit/da/proxy/jsonrpc"
)

// NewClient returns a DA backend based on the uri
// and auth token. Supported schemes: grpc, http, https
func NewClient(uri, token string) (da.DA, error) {
	addr, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	var client da.DA
	switch addr.Scheme {
	case "grpc":
		grpcClient := proxygrpc.NewClient()
		if err := grpcClient.Start(addr.Host, grpc.WithTransportCredentials(insecure.NewCredentials())); err != nil {
			return nil, err
		}
		client = grpcClient
	case "http", "https":
		jsonrpcClient, err := proxyjsonrpc.NewClient(context.Background(), uri, token)
		if err != nil {
			return nil, err
		}
		client = &jsonrpcClient.DA
	default:
		return nil, fmt.Errorf("unknown url scheme '%s'", addr.Scheme)
	}
	return client, nil
}
