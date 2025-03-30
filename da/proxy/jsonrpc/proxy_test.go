package jsonrpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	proxy "github.com/rollkit/rollkit/da/proxy/jsonrpc"
	"github.com/rollkit/rollkit/da/test"
)

const (
	// ServerHost is the listen host for the test JSONRPC server
	ServerHost = "localhost"
	// ServerPort is the listen port for the test JSONRPC server
	ServerPort = "3450"
	// ClientURL is the url to dial for the test JSONRPC client
	ClientURL = "http://localhost:3450"
)

// TestProxy runs the go-da DA test suite against the JSONRPC service
// NOTE: This test requires a test JSONRPC service to run on the port
// 3450 which is chosen to be sufficiently distinct from the default port
func TestProxy(t *testing.T) {
	dummy := test.NewDummyDA()
	server := proxy.NewServer(ServerHost, ServerPort, dummy)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			require.NoError(t, err)
		}
	}()

	client, err := proxy.NewClient(context.Background(), ClientURL, "")
	require.NoError(t, err)
	test.RunDATestSuite(t, &client.DA)
}
