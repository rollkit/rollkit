package sequencing

import (
	"os"
	"testing"

	testServer "github.com/rollkit/rollkit/test/server"
)

const (
	// MockSequencerAddress is the mock address for the gRPC server
	MockSequencerAddress = "grpc://localhost:50051"
)

// TestMain starts the mock gRPC server
func TestMain(m *testing.M) {
	grpcSrv := testServer.StartMockSequencerServerGRPC(MockSequencerAddress)
	exitCode := m.Run()

	// teardown servers
	// nolint:errcheck,gosec
	grpcSrv.Stop()

	os.Exit(exitCode)
}
