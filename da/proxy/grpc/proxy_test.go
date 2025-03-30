package grpc_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	proxy "github.com/rollkit/rollkit/da/proxy/grpc"
	"github.com/rollkit/rollkit/da/test"
)

func TestProxy(t *testing.T) {
	dummy := test.NewDummyDA()
	server := proxy.NewServer(dummy, grpc.Creds(insecure.NewCredentials()))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		_ = server.Serve(lis)
	}()

	client := proxy.NewClient()
	err = client.Start(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	test.RunDATestSuite(t, client)
}
