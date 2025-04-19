package proxy

import (
	"context"
	"fmt"
	"net/url"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/core/da"
	proxyjsonrpc "github.com/rollkit/rollkit/da/proxy/jsonrpc"
)

// NewClient returns a DA backend based on the uri
// and auth token. Supported schemes: grpc, http, https
func NewClient(logger log.Logger, uri, token string) (da.DA, error) {
	addr, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	var client da.DA
	switch addr.Scheme {
	case "http", "https":
		jsonrpcClient, err := proxyjsonrpc.NewClient(context.Background(), logger, uri, token)
		if err != nil {
			return nil, err
		}
		client = &jsonrpcClient.DA
	default:
		return nil, fmt.Errorf("unknown url scheme '%s'", addr.Scheme)
	}
	return client, nil
}
