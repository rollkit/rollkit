package proxy_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/execution/proxy"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	"github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

const bufSize = 1024 * 1024

// Helper function to start the gRPC server with a mock executor
func startTestServer(t *testing.T, executor execution.Executor) (v1connect.ExecutionServiceClient, func()) {
	t.Helper()

	proxyServer := proxy.NewServer(executor)
	mux := http.NewServeMux()
	path, handler := v1connect.NewExecutionServiceHandler(proxyServer)
	mux.Handle(path, handler)

	h2cHandler := h2c.NewHandler(mux, &http2.Server{})
	httpServer := &http.Server{
		Handler: h2cHandler,
	}

	lis := bufconn.Listen(bufSize)

	go func() {
		if err := httpServer.Serve(lis); err != nil && err != http.ErrServerClosed {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			},
		},
	}

	client := v1connect.NewExecutionServiceClient(
		httpClient,
		"http://bufnet", // Base URL for the service
		connect.WithGRPC(),
	)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			t.Logf("HTTP server Shutdown: %v", err)
		}
		lis.Close()
	}

	return client, cleanup
}

func TestInitChain(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// 1. Use a DummyExecutor (or a mock)
	dummyExecutor := execution.NewDummyExecutor()
	client, cleanup := startTestServer(t, dummyExecutor)
	defer cleanup()

	// 2. Prepare request
	genesisTime := time.Now()
	initialHeight := uint64(1)
	chainID := "testchain"

	pbReq := &pb.InitChainRequest{
		GenesisTime:   timestamppb.New(genesisTime),
		InitialHeight: initialHeight,
		ChainId:       chainID,
	}

	// 3. Call InitChain via gRPC client, wrapped with v1connect.NewRequest
	resp, err := client.InitChain(context.Background(), connect.NewRequest(pbReq))
	require.NoError(err, "InitChain failed")
	require.NotNil(resp, "Response should not be nil")
	require.NotNil(resp.Msg, "Response message should not be nil")

	// 4. Verify response
	assert.NotEmpty(resp.Msg.GetStateRoot(), "State root should not be empty")
	assert.True(resp.Msg.GetMaxBytes() > 0, "MaxBytes should be greater than 0")

	// 5. Optionally, verify internal state of dummyExecutor if needed
	// (This depends on what DummyExecutor exposes or if you use a more detailed mock)
	// For example, if DummyExecutor stored the chainID:
	// assert.Equal(chainID, dummyExecutor.GetChainID()) // Assuming GetChainID exists
}

func TestExecuteTxs(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dummyExecutor := execution.NewDummyExecutor()
	// Initialize the chain first, as ExecuteTxs might depend on an initialized state
	_, _, err := dummyExecutor.InitChain(context.Background(), time.Now(), 1, "testchain")
	require.NoError(err, "dummyExecutor.InitChain failed")

	client, cleanup := startTestServer(t, dummyExecutor)
	defer cleanup()

	tx1 := []byte("tx1_data")
	tx2 := []byte("tx2_data")
	blockHeight := uint64(2)
	timestamp := time.Now()
	prevStateRoot, _, _ := dummyExecutor.InitChain(context.Background(), time.Now(), 1, "testchain") // Get a valid root

	pbReq := &pb.ExecuteTxsRequest{
		Txs:           [][]byte{tx1, tx2},
		BlockHeight:   blockHeight,
		Timestamp:     timestamppb.New(timestamp), // Corrected timestamp
		PrevStateRoot: prevStateRoot,
	}

	resp, err := client.ExecuteTxs(context.Background(), connect.NewRequest(pbReq))
	require.NoError(err, "ExecuteTxs failed")
	require.NotNil(resp, "Response should not be nil")
	require.NotNil(resp.Msg, "Response message should not be nil")

	assert.NotEmpty(resp.Msg.GetUpdatedStateRoot(), "Updated state root should not be empty")
	assert.NotEqual(prevStateRoot, resp.Msg.GetUpdatedStateRoot(), "State root should have changed")
	assert.True(resp.Msg.GetMaxBytes() > 0, "MaxBytes should be greater than 0")
}

func TestGetTxs(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dummyExecutor := execution.NewDummyExecutor()
	tx1 := []byte("tx_for_get_1")
	tx2 := []byte("tx_for_get_2")
	dummyExecutor.InjectTx(tx1)
	dummyExecutor.InjectTx(tx2)

	client, cleanup := startTestServer(t, dummyExecutor)
	defer cleanup()

	pbReq := &emptypb.Empty{} // Corrected request type

	resp, err := client.GetTxs(context.Background(), connect.NewRequest(pbReq))
	require.NoError(err, "GetTxs failed")
	require.NotNil(resp, "Response should not be nil")
	require.NotNil(resp.Msg, "Response message should not be nil")

	assert.Len(resp.Msg.GetTxs(), 2, "Should retrieve 2 transactions")
	assert.Contains(resp.Msg.GetTxs(), tx1, "Response should contain tx1")
	assert.Contains(resp.Msg.GetTxs(), tx2, "Response should contain tx2")
}

func TestSetFinal(t *testing.T) {
	require := require.New(t)

	dummyExecutor := execution.NewDummyExecutor()
	// Initialize and execute some transactions to have a pending state
	initStateRoot, _, err := dummyExecutor.InitChain(context.Background(), time.Now(), 1, "testchain")
	require.NoError(err, "dummyExecutor.InitChain failed")

	blockHeight := uint64(2)
	_, _, err = dummyExecutor.ExecuteTxs(context.Background(), [][]byte{[]byte("tx_final")}, blockHeight, time.Now(), initStateRoot)
	require.NoError(err, "dummyExecutor.ExecuteTxs failed")

	client, cleanup := startTestServer(t, dummyExecutor)
	defer cleanup()

	pbReq := &pb.SetFinalRequest{
		BlockHeight: blockHeight,
	}

	resp, err := client.SetFinal(context.Background(), connect.NewRequest(pbReq)) // Corrected to connect.NewRequest
	require.NoError(err, "SetFinal failed")
	require.NotNil(resp, "Response should not be nil")
	require.NotNil(resp.Msg, "Response message should not be nil")

	// To verify, you might need to check the internal state of DummyExecutor
	// or ensure that a subsequent call that depends on finality behaves as expected.
	// For example, if DummyExecutor moves pending state to finalized state:
	// assert.Equal(dummyExecutor.GetStateRoot(), expectedFinalizedRoot) // Assuming methods exist
}

// You would add more tests for GetTxs, SetFinal etc. following a similar pattern.
// For GetTxs, you might need to add a method to DummyExecutor to inject transactions for testing.
// For SetFinal, you\'d execute some transactions first to create a state that can be finalized.
