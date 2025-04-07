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
	"github.com/rollkit/rollkit/attester/internal/aggregator"
	internalgrpc "github.com/rollkit/rollkit/attester/internal/grpc"
	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/state"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
)

// LogEntryTypeSubmitBlock identifies log entries related to block submissions.
const (
	LogEntryTypeSubmitBlock byte = 0x01
)

// AttesterFSM implements the raft.FSM interface for the attester service.
// It manages the state related to attested blocks.
type AttesterFSM struct {
	mu sync.RWMutex

	// State - These maps need to be persisted via Snapshot/Restore
	processedBlocks map[uint64]state.BlockHash           // Height -> Hash (ensures height uniqueness)
	blockDetails    map[state.BlockHash]*state.BlockInfo // Hash -> Full block info including signature

	logger     *slog.Logger
	signer     signing.Signer
	aggregator *aggregator.SignatureAggregator
	sigClient  *internalgrpc.SignatureClient
	nodeID     string
	// db *bolt.DB // Optional: Direct DB access if state becomes too complex for memory/simple snapshot
}

// NewAttesterFSM creates a new instance of the AttesterFSM.
func NewAttesterFSM(logger *slog.Logger, signer signing.Signer, nodeID string, dataDir string, aggregator *aggregator.SignatureAggregator, sigClient *internalgrpc.SignatureClient) *AttesterFSM {
	if nodeID == "" {
		panic("node ID cannot be empty for FSM")
	}
	return &AttesterFSM{
		processedBlocks: make(map[uint64]state.BlockHash),
		blockDetails:    make(map[state.BlockHash]*state.BlockInfo),
		logger:          logger.With("component", "fsm"),
		signer:          signer,
		aggregator:      aggregator,
		sigClient:       sigClient,
		nodeID:          nodeID,
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
		info.Signature = signature

		f.processedBlocks[info.Height] = info.Hash
		f.blockDetails[info.Hash] = info

		if f.aggregator != nil {
			f.aggregator.SetBlockData(info.Hash[:], info.DataToSign)
		}

		f.logger.Info("Successfully applied and signed block", logArgs...)

		if f.sigClient != nil {
			go func(blockInfo *state.BlockInfo) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				err := f.sigClient.SubmitSignature(ctx, blockInfo.Height, blockInfo.Hash[:], f.nodeID, blockInfo.Signature)
				if err != nil {
					f.logger.Error("Failed to submit signature to leader",
						"block_height", blockInfo.Height,
						"block_hash", blockInfo.Hash.String(),
						"error", err)
				}
			}(info)
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

	allBlocks := make([]*state.BlockInfo, 0, len(f.blockDetails))
	for _, info := range f.blockDetails {
		allBlocks = append(allBlocks, info)
	}

	fsmState := state.FSMState{
		Blocks: allBlocks,
	}

	var dataBytes []byte
	err := fmt.Errorf("TODO: Implement FSM state marshaling (protobuf/gob)")

	if err != nil {
		f.logger.Error("Failed to marshal FSM state for snapshot", "error", err)
		return nil, err
	}

	f.logger.Info("FSM snapshot created", "size_bytes", len(dataBytes), "note", "Marshaling not implemented")
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

	var restoredState state.FSMState
	err = fmt.Errorf("TODO: Implement FSM state unmarshaling (protobuf/gob)")

	if err != nil {
		f.logger.Error("Failed to unmarshal FSM state from snapshot", "error", err)
		return err
	}

	f.processedBlocks = make(map[uint64]state.BlockHash)
	f.blockDetails = make(map[state.BlockHash]*state.BlockInfo)

	for _, info := range restoredState.Blocks {
		if info == nil {
			f.logger.Warn("Found nil BlockInfo in restored snapshot, skipping")
			continue
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

	f.logger.Info("FSM restored successfully from snapshot", "blocks_loaded", len(f.blockDetails), "note", "Unmarshaling not implemented")
	return nil
}

// --- FSMSnapshot --- //

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
