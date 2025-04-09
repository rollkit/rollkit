package fsm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	// Import internal packages

	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/state"
	verification "github.com/rollkit/rollkit/attester/internal/verification"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
)

// LogEntryTypeSubmitBlock identifies log entries related to block submissions.
const (
	LogEntryTypeSubmitBlock byte = 0x01
)

// SignatureSubmitter defines the interface for submitting signatures.
// This allows mocking the gRPC client dependency.
type SignatureSubmitter interface {
	SubmitSignature(ctx context.Context, height uint64, hash []byte, attesterID string, signature []byte) error
}

// AggregatorService defines the combined interface for aggregator interactions needed by the FSM.
// It includes methods used by the FSM (SetBlockData) and potentially other components
// interacting with the aggregator implementation (AddSignature, GetAggregatedSignatures).
type AggregatorService interface {
	// SetBlockData informs the aggregator about the data to sign for a block hash (used by Leader FSM).
	SetBlockData(blockHash []byte, dataToSign []byte)

	// AddSignature adds a signature from an attester for a specific block.
	// The FSM itself doesn't call this directly, but the aggregator implementation likely uses it.
	AddSignature(blockHeight uint64, blockHash []byte, attesterID string, signature []byte) (bool, error)

	// GetAggregatedSignatures retrieves aggregated signatures for a block.
	// The FSM itself doesn't call this directly, but the aggregator implementation likely uses it.
	GetAggregatedSignatures(blockHeight uint64) ([][]byte, bool)
}

// AttesterFSM implements the raft.FSM interface for the attester service.
// It manages the state related to attested blocks.
type AttesterFSM struct {
	mu sync.RWMutex

	// State - These maps need to be persisted via Snapshot/Restore
	processedBlocks map[uint64]state.BlockHash           // Height -> Hash (ensures height uniqueness)
	blockDetails    map[state.BlockHash]*state.BlockInfo // Hash -> Full block info

	logger *slog.Logger
	signer signing.Signer
	// Use interfaces for dependencies
	aggregator AggregatorService
	sigClient  SignatureSubmitter // Interface for signature client (non-nil only if follower)
	verifier   verification.ExecutionVerifier
	nodeID     string
	// Explicitly store the role
	isLeader bool
}

// NewAttesterFSM creates a new instance of the AttesterFSM.
// Dependencies (signer, aggregator/client, verifier) are injected.
func NewAttesterFSM(
	logger *slog.Logger,
	signer signing.Signer,
	nodeID string,
	isLeader bool,
	agg AggregatorService,
	client SignatureSubmitter,
	verifier verification.ExecutionVerifier,
) (*AttesterFSM, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil for FSM")
	}
	if nodeID == "" {
		return nil, fmt.Errorf("node ID cannot be empty for FSM")
	}
	if isLeader && agg == nil {
		logger.Warn("FSM is leader but required aggregator dependency is nil")
		return nil, fmt.Errorf("leader but required aggregator dependency is nil")
	}
	if !isLeader && client == nil {
		logger.Warn("FSM is follower but required signature client dependency is nil")
		return nil, fmt.Errorf("follower but required signature client dependency is nil")
	}

	fsm := &AttesterFSM{
		processedBlocks: make(map[uint64]state.BlockHash),
		blockDetails:    make(map[state.BlockHash]*state.BlockInfo),
		logger:          logger.With("component", "fsm", "is_leader", isLeader),
		signer:          signer,
		aggregator:      agg,
		sigClient:       client,
		verifier:        verifier,
		nodeID:          nodeID,
		isLeader:        isLeader,
	}
	return fsm, nil
}

// Apply applies a Raft log entry to the FSM.
// This method is called by the Raft library when a new log entry is committed.
func (f *AttesterFSM) Apply(logEntry *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(logEntry.Data) == 0 {
		f.logger.Error("Apply called with empty data", "log_index", logEntry.Index)
		return fmt.Errorf("empty log data at index %d", logEntry.Index)
	}

	entryType := logEntry.Data[0]
	entryData := logEntry.Data[1:]

	switch entryType {
	case LogEntryTypeSubmitBlock:
		var req attesterv1.SubmitBlockRequest
		if err := proto.Unmarshal(entryData, &req); err != nil {
			f.logger.Error("Failed to unmarshal SubmitBlockRequest", "log_index", logEntry.Index, "error", err)
			return fmt.Errorf("failed to unmarshal log data at index %d: %w", logEntry.Index, err)
		}
		return f.applySubmitBlock(logEntry.Index, &req)
	default:
		f.logger.Error("Unknown log entry type", "type", fmt.Sprintf("0x%x", entryType), "log_index", logEntry.Index)
		return fmt.Errorf("unknown log entry type 0x%x at index %d", entryType, logEntry.Index)
	}
}

// applySubmitBlock handles the application of a SubmitBlockRequest log entry.
// It assumes the FSM lock is already held.
func (f *AttesterFSM) applySubmitBlock(logIndex uint64, req *attesterv1.SubmitBlockRequest) interface{} {
	blockHashStr := fmt.Sprintf("%x", req.BlockHash)
	logArgs := []any{"block_height", req.BlockHeight, "block_hash", blockHashStr, "log_index", logIndex}

	f.logger.Info("Applying block", logArgs...)

	// Validation checks
	if _, exists := f.processedBlocks[req.BlockHeight]; exists {
		f.logger.Warn("Block height already processed, skipping.", logArgs...)
		return nil // Not an error, just already processed
	}
	if len(req.BlockHash) != state.BlockHashSize {
		f.logger.Error("Invalid block hash size", append(logArgs, "size", len(req.BlockHash))...)
		return fmt.Errorf("invalid block hash size at index %d", logIndex)
	}
	blockHash, err := state.BlockHashFromBytes(req.BlockHash)
	if err != nil {
		f.logger.Error("Cannot convert block hash bytes", append(logArgs, "error", err)...)
		return fmt.Errorf("invalid block hash bytes at index %d: %w", logIndex, err)
	}
	if _, exists := f.blockDetails[blockHash]; exists {
		f.logger.Error("Block hash already exists, possible hash collision or duplicate submit.", logArgs...)
		return fmt.Errorf("block hash %s collision or duplicate at index %d", blockHash, logIndex)
	}

	// Create block info object early for use in verification
	info := &state.BlockInfo{
		Height:     req.BlockHeight,
		Hash:       blockHash,
		DataToSign: req.DataToSign,
	}

	if f.verifier != nil {
		f.logger.Debug("Performing execution verification", logArgs...)
		err := f.verifier.VerifyExecution(context.Background(), info.Height, info.Hash, info.DataToSign)
		if err != nil {
			f.logger.Error("Execution verification failed", append(logArgs, "error", err)...)
			return fmt.Errorf("execution verification failed at index %d for block %d (%s): %w", logIndex, info.Height, blockHashStr, err)
		}
		f.logger.Debug("Execution verification successful", logArgs...)
	} else {
		f.logger.Debug("Execution verification skipped (verifier not configured)", logArgs...)
	}

	// Sign the data (only if verification passed or was skipped)
	signature, err := f.signer.Sign(info.DataToSign)
	if err != nil {
		f.logger.Error("Failed to sign data for block", append(logArgs, "error", err)...)
		return fmt.Errorf("failed to sign data at index %d after verification: %w", logIndex, err)
	}

	// Update FSM state (only after successful verification and signing)
	f.processedBlocks[info.Height] = info.Hash
	f.blockDetails[info.Hash] = info

	// Leader specific logic
	if f.isLeader {
		if f.aggregator != nil {
			f.aggregator.SetBlockData(info.Hash[:], info.DataToSign)
		} else {
			f.logger.Error("CRITICAL: FSM is leader but aggregator is nil. This indicates an unrecoverable state.", logArgs...)
			panic("invariant violation: aggregator is nil for leader FSM")
		}
	}

	f.logger.Info("Successfully applied and signed block", logArgs...)

	// Follower specific logic: Submit signature (non-blocking)
	if !f.isLeader {
		if f.sigClient != nil {
			go f.submitSignatureAsync(info, signature)
		} else {
			f.logger.Error("CRITICAL: FSM is follower but signature client is nil. This indicates an unrecoverable state.", logArgs...)
			panic("invariant violation: signature client is nil for follower FSM")
		}
	}

	// Return the processed block info
	return info
}

// submitSignatureAsync submits the signature to the leader in a separate goroutine.
func (f *AttesterFSM) submitSignatureAsync(blockInfo *state.BlockInfo, signature []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger := f.logger.With("sub_task", "submit_signature")

	err := f.sigClient.SubmitSignature(ctx, blockInfo.Height, blockInfo.Hash[:], f.nodeID, signature)
	if err != nil {
		logger.Error("Failed to submit signature to leader",
			"block_height", blockInfo.Height,
			"block_hash", blockInfo.Hash.String(),
			"error", err)
	} else {
		logger.Info("Successfully submitted signature to leader",
			"block_height", blockInfo.Height,
			"block_hash", blockInfo.Hash.String())
	}
}

// Snapshot returns a snapshot of the FSM state.
func (f *AttesterFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.logger.Info("Creating FSM snapshot...")

	allProtoBlocks := make([]*attesterv1.BlockInfo, 0, len(f.blockDetails))
	for _, info := range f.blockDetails {
		protoBlock := &attesterv1.BlockInfo{
			Height:     info.Height,
			Hash:       info.Hash[:],
			DataToSign: info.DataToSign,
		}
		allProtoBlocks = append(allProtoBlocks, protoBlock)
	}

	fsmStateProto := attesterv1.FSMState{
		Blocks: allProtoBlocks,
	}

	dataBytes, err := proto.Marshal(&fsmStateProto)

	if err != nil {
		f.logger.Error("Failed to marshal FSM state for snapshot", "error", err)
		return nil, err
	}

	f.logger.Info("FSM snapshot created", "size_bytes", len(dataBytes), "blocks", len(allProtoBlocks))
	return &fsmSnapshot{state: dataBytes}, nil
}

// Restore restores the FSM state from a snapshot.
func (f *AttesterFSM) Restore(snapshot io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.logger.Info("Restoring FSM from snapshot...")

	dataBytes, err := io.ReadAll(snapshot)
	if err != nil {
		f.logger.Error("Failed to read snapshot data", "error", err)
		return err
	}

	var restoredStateProto attesterv1.FSMState
	if err := proto.Unmarshal(dataBytes, &restoredStateProto); err != nil {
		f.logger.Error("Failed to unmarshal FSM state from snapshot", "error", err)
		return err
	}

	f.processedBlocks = make(map[uint64]state.BlockHash)
	f.blockDetails = make(map[state.BlockHash]*state.BlockInfo)

	for _, protoInfo := range restoredStateProto.Blocks {
		if protoInfo == nil {
			f.logger.Warn("Found nil BlockInfo in restored snapshot, skipping")
			continue
		}
		blockHash, err := state.BlockHashFromBytes(protoInfo.Hash)
		if err != nil {
			f.logger.Error("Invalid BlockHash bytes found in snapshot, skipping", "height", protoInfo.Height, "hash_hex", fmt.Sprintf("%x", protoInfo.Hash), "error", err)
			continue
		}

		info := &state.BlockInfo{
			Height:     protoInfo.Height,
			Hash:       blockHash,
			DataToSign: protoInfo.DataToSign,
		}

		if len(info.Hash) != state.BlockHashSize {
			f.logger.Error("Invalid BlockHash size found in snapshot, skipping", "height", info.Height, "size", len(info.Hash))
			continue
		}

		if existingHash, heightExists := f.processedBlocks[info.Height]; heightExists {
			f.logger.Error("Duplicate height found during snapshot restore",
				"height", info.Height, "hash1", existingHash.String(), "hash2", info.Hash.String())
			return fmt.Errorf("duplicate height %d found in snapshot", info.Height)
		}
		if existingInfo, hashExists := f.blockDetails[info.Hash]; hashExists {
			f.logger.Error("Duplicate hash found during snapshot restore",
				"hash", info.Hash.String(), "height1", existingInfo.Height, "height2", info.Height)
			return fmt.Errorf("duplicate hash %s found in snapshot", info.Hash)
		}

		f.processedBlocks[info.Height] = info.Hash
		f.blockDetails[info.Hash] = info
	}

	f.logger.Info("FSM restored successfully from snapshot", "blocks_loaded", len(f.blockDetails))
	return nil
}

// fsmSnapshot implements the raft.FSMSnapshot interface.
type fsmSnapshot struct {
	state []byte
}

// Persist writes the FSM state to a sink (typically a file).
func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := sink.Write(s.state); err != nil {
		sink.Cancel()
		return fmt.Errorf("failed to write snapshot state to sink: %w", err)
	}
	return sink.Close()
}

// Release is called when the snapshot is no longer needed.
func (s *fsmSnapshot) Release() {
	// No-op for this simple byte slice implementation
}
