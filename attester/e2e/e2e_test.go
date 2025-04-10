package e2e

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

const (
	defaultWaitTime  = 5 * time.Second
	testBlockHeight  = 100
	testBlockDataStr = "test-block-data-e2e"
)

// TestE2E_BasicAttestation runs a basic E2E scenario
func TestE2E_BasicAttestation(t *testing.T) {
	lvl := new(slog.LevelVar)
	lvl.Set(slog.LevelDebug)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	cluster := setupCluster(t)
	defer cluster.Cleanup(t)

	cluster.LaunchAllNodes(t)
	cluster.WaitFor(t, defaultWaitTime, "network stabilization")

	cluster.TriggerBlockProposalOnLeader(t, testBlockHeight, testBlockDataStr)
	cluster.WaitFor(t, defaultWaitTime, "attestation completion")

	aggregatedSigsResp := cluster.GetAggregatedSignaturesFromLeader(t, testBlockHeight)
	// Verify quorum was met and the number of signatures is correct
	cluster.VerifyQuorumMet(t, aggregatedSigsResp, testBlockHeight)
	// Verify the cryptographic validity of each signature
	cluster.VerifySignatures(t, aggregatedSigsResp, testBlockHeight, []byte(testBlockDataStr))

	// Optional: Add more checks, e.g., verify the signatures themselves if public keys are known

	t.Log("E2E Test Completed Successfully!")
}
