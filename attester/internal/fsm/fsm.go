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

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
)

// LogEntryTypeSubmitBlock identifies log entries related to block submissions.
const (
	LogEntryTypeSubmitBlock byte = 0x01
)

// BlockDataSetter defines the interface for setting block data in the aggregator.
// This allows mocking the aggregator dependency.
type BlockDataSetter interface {
	SetBlockData(blockHash []byte, dataToSign []byte)
}

// SignatureSubmitter defines the interface for submitting signatures.
// This allows mocking the gRPC client dependency.
type SignatureSubmitter interface {
	SubmitSignature(ctx context.Context, height uint64, hash []byte, attesterID string, signature []byte) error
}

// AttesterFSM implements the raft.FSM interface for the attester service.
// It manages the state related to attested blocks.
type AttesterFSM struct {
	mu sync.RWMutex

	// State - These maps need to be persisted via Snapshot/Restore
	processedBlocks map[uint64]state.BlockHash           // Height -> Hash (ensures height uniqueness)
	blockDetails    map[state.BlockHash]*state.BlockInfo // Hash -> Full block info (signature is not stored here)

	logger *slog.Logger
	signer signing.Signer
	// Use interfaces for dependencies
	aggregator BlockDataSetter    // Interface for aggregator (non-nil only if leader)
	sigClient  SignatureSubmitter // Interface for signature client (non-nil only if follower)
	nodeID     string
	// Explicitly store the role
	isLeader bool
}

// NewAttesterFSM creates a new instance of the AttesterFSM.
// It now accepts interfaces for aggregator and sigClient for better testability,
// and an explicit isLeader flag.
func NewAttesterFSM(logger *slog.Logger, signer signing.Signer, nodeID string, isLeader bool, aggregator BlockDataSetter, sigClient SignatureSubmitter) *AttesterFSM {
	if nodeID == "" {
		panic("node ID cannot be empty for FSM")
	}
	// Basic validation: Leader should have aggregator, follower should have client
	if isLeader && aggregator == nil {
		logger.Warn("FSM created as leader but aggregator is nil")
	}
	if !isLeader && sigClient == nil {
		logger.Warn("FSM created as follower but signature client is nil")
	}

	return &AttesterFSM{
		processedBlocks: make(map[uint64]state.BlockHash),
		blockDetails:    make(map[state.BlockHash]*state.BlockInfo),
		logger:          logger.With("component", "fsm", "is_leader", isLeader), // Add role to logger
		signer:          signer,
		aggregator:      aggregator,
		sigClient:       sigClient,
		nodeID:          nodeID,
		isLeader:        isLeader, // Store the role
	}
}

// Apply applies a Raft log entry to the FSM.
// It returns a value which is stored in the ApplyFuture.
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

		blockHashStr := fmt.Sprintf("%x", req.BlockHash)
		logArgs := []any{"block_height", req.BlockHeight, "block_hash", blockHashStr, "log_index", logEntry.Index}

		f.logger.Info("Applying block", logArgs...)

		if _, exists := f.processedBlocks[req.BlockHeight]; exists {
			f.logger.Warn("Block height already processed, skipping.", logArgs...)
			return nil
		}

		if len(req.BlockHash) != state.BlockHashSize {
			f.logger.Error("Invalid block hash size", append(logArgs, "size", len(req.BlockHash))...)
			return fmt.Errorf("invalid block hash size at index %d", logEntry.Index)
		}
		blockHash, err := state.BlockHashFromBytes(req.BlockHash)
		if err != nil {
			f.logger.Error("Cannot convert block hash bytes", append(logArgs, "error", err)...)
			return fmt.Errorf("invalid block hash bytes at index %d: %w", logEntry.Index, err)
		}

		if _, exists := f.blockDetails[blockHash]; exists {
			f.logger.Error("Block hash already exists, possible hash collision or duplicate submit.", logArgs...)
			return fmt.Errorf("block hash %s collision or duplicate at index %d", blockHash, logEntry.Index)
		}

		info := &state.BlockInfo{
			Height:     req.BlockHeight,
			Hash:       blockHash,
			DataToSign: req.DataToSign,
		}

		signature, err := f.signer.Sign(info.DataToSign)
		if err != nil {
			f.logger.Error("Failed to sign data for block", append(logArgs, "error", err)...)
			return fmt.Errorf("failed to sign data at index %d: %w", logEntry.Index, err)
		}

		f.processedBlocks[info.Height] = info.Hash
		f.blockDetails[info.Hash] = info

		// Use isLeader flag to determine action
		if f.isLeader {
			// Leader stores block data for signature verification
			if f.aggregator != nil { // Still good practice to check for nil
				f.aggregator.SetBlockData(info.Hash[:], info.DataToSign)
			} else {
				f.logger.Error("FSM is leader but aggregator is nil, cannot set block data")
			}
		} // No specific leader action needed beyond SetBlockData for Apply

		f.logger.Info("Successfully applied and signed block", logArgs...)

		// Only followers submit signatures
		if !f.isLeader {
			if f.sigClient != nil { // Still good practice to check for nil
				go func(blockInfo *state.BlockInfo) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					err := f.sigClient.SubmitSignature(ctx, blockInfo.Height, blockInfo.Hash[:], f.nodeID, signature)
					if err != nil {
						f.logger.Error("Failed to submit signature to leader",
							"block_height", blockInfo.Height,
							"block_hash", blockInfo.Hash.String(),
							"error", err)
					}
				}(info)
			} else {
				f.logger.Error("FSM is follower but signature client is nil, cannot submit signature")
			}
		}

		return info

	default:
		f.logger.Error("Unknown log entry type", "type", fmt.Sprintf("0x%x", entryType), "log_index", logEntry.Index)
		return fmt.Errorf("unknown log entry type 0x%x at index %d", entryType, logEntry.Index)
	}
}

// Snapshot returns a snapshot of the FSM state.
func (f *AttesterFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.logger.Info("Creating FSM snapshot...")

	allProtoBlocks := make([]*attesterv1.BlockInfo, 0, len(f.blockDetails))
	for _, info := range f.blockDetails {
		// Convert internal state.BlockInfo to proto BlockInfo for marshaling
		// Note: proto BlockInfo no longer has signature field
		protoBlock := &attesterv1.BlockInfo{
			Height:     info.Height,
			Hash:       info.Hash[:], // Convert state.BlockHash (array) to bytes slice
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
		// Convert proto BlockInfo back to internal state.BlockInfo
		blockHash, err := state.BlockHashFromBytes(protoInfo.Hash)
		if err != nil {
			f.logger.Error("Invalid BlockHash bytes found in snapshot, skipping", "height", protoInfo.Height, "hash_hex", fmt.Sprintf("%x", protoInfo.Hash), "error", err)
			continue
		}

		// Reconstruct the internal state type
		info := &state.BlockInfo{
			Height:     protoInfo.Height,
			Hash:       blockHash,
			DataToSign: protoInfo.DataToSign,
			// Signature field no longer exists here
		}

		if len(info.Hash) != state.BlockHashSize { // Redundant check due to BlockHashFromBytes, but safe
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
