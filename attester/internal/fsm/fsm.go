package fsm

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	// Import internal packages
	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/state"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attesterv1"
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

	logger *log.Logger // TODO: Replace with a structured logger (e.g., slog)
	signer signing.Signer
	// db *bolt.DB // Optional: Direct DB access if state becomes too complex for memory/simple snapshot
}

// NewAttesterFSM creates a new instance of the AttesterFSM.
func NewAttesterFSM(logger *log.Logger, signer signing.Signer, dataDir string /* unused for now */) *AttesterFSM {
	return &AttesterFSM{
		processedBlocks: make(map[uint64]state.BlockHash),
		blockDetails:    make(map[state.BlockHash]*state.BlockInfo),
		logger:          logger,
		signer:          signer,
	}
}

// Apply applies a Raft log entry to the FSM.
// It returns a value which is stored in the ApplyFuture.
func (f *AttesterFSM) Apply(logEntry *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(logEntry.Data) == 0 {
		f.logger.Printf("ERROR: Apply called with empty data log index: %d", logEntry.Index)
		return fmt.Errorf("empty log data at index %d", logEntry.Index)
	}

	entryType := logEntry.Data[0]
	entryData := logEntry.Data[1:]

	switch entryType {
	case LogEntryTypeSubmitBlock:
		// Use the generated proto type now
		var req attesterv1.SubmitBlockRequest

		if err := proto.Unmarshal(entryData, &req); err != nil {
			f.logger.Printf("ERROR: Failed to unmarshal SubmitBlockRequest at index %d: %v", logEntry.Index, err)
			// Returning an error here might be appropriate depending on failure modes.
			// Raft might retry or handle this based on configuration.
			return fmt.Errorf("failed to unmarshal log data at index %d: %w", logEntry.Index, err)
		}

		f.logger.Printf("INFO: Applying block Height: %d, Hash: %x (Log Index: %d)", req.BlockHeight, req.BlockHash, logEntry.Index)

		// Check if height is already processed
		if _, exists := f.processedBlocks[req.BlockHeight]; exists {
			f.logger.Printf("WARN: Block height %d already processed, skipping. (Log Index: %d)", req.BlockHeight, logEntry.Index)
			return nil // Indicate no change or already processed
		}

		// Validate hash size
		if len(req.BlockHash) != state.BlockHashSize {
			f.logger.Printf("ERROR: Invalid block hash size (%d) for height %d at log index %d", len(req.BlockHash), req.BlockHeight, logEntry.Index)
			return fmt.Errorf("invalid block hash size at index %d", logEntry.Index)
		}
		blockHash, err := state.BlockHashFromBytes(req.BlockHash)
		if err != nil { // Should be caught by len check, but belt-and-suspenders
			f.logger.Printf("ERROR: Cannot convert block hash bytes at index %d: %v", logEntry.Index, err)
			return fmt.Errorf("invalid block hash bytes at index %d: %w", logEntry.Index, err)
		}

		// Check if hash is already processed (collision? unlikely but possible)
		if _, exists := f.blockDetails[blockHash]; exists {
			f.logger.Printf("ERROR: Block hash %s already exists, possible hash collision or duplicate submit. (Log Index: %d)", blockHash, logEntry.Index)
			return fmt.Errorf("block hash %s collision or duplicate at index %d", blockHash, logEntry.Index)
		}

		// Create block info and sign
		// NOTE: We use state.BlockInfo for internal storage.
		// The proto definition attesterv1.BlockInfo is used for serialization (Snapshot/Restore).
		info := &state.BlockInfo{
			Height:     req.BlockHeight,
			Hash:       blockHash,
			DataToSign: req.DataToSign, // This is the data we actually sign
		}

		signature, err := f.signer.Sign(info.DataToSign)
		if err != nil {
			f.logger.Printf("ERROR: Failed to sign data for block %d (%s) at log index %d: %v", req.BlockHeight, blockHash, logEntry.Index, err)
			// This is a critical internal error.
			return fmt.Errorf("failed to sign data at index %d: %w", logEntry.Index, err)
		}
		info.Signature = signature

		// Update state
		f.processedBlocks[info.Height] = info.Hash
		f.blockDetails[info.Hash] = info

		f.logger.Printf("INFO: Successfully applied and signed block Height: %d, Hash: %s (Log Index: %d)", info.Height, info.Hash, logEntry.Index)
		// Return the processed info. This can be retrieved via ApplyFuture.Response().
		return info

	default:
		f.logger.Printf("ERROR: Unknown log entry type: 0x%x at log index %d", entryType, logEntry.Index)
		return fmt.Errorf("unknown log entry type 0x%x at index %d", entryType, logEntry.Index)
	}
}

// Snapshot returns a snapshot of the FSM state.
func (f *AttesterFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	f.logger.Printf("INFO: Creating FSM snapshot...")

	// Collect all BlockInfo from the map
	allBlocks := make([]*state.BlockInfo, 0, len(f.blockDetails))
	for _, info := range f.blockDetails {
		allBlocks = append(allBlocks, info)
	}

	// Create the state object to be serialized
	fsmState := state.FSMState{
		Blocks: allBlocks,
	}

	// Serialize the state using protobuf (or gob, json, etc.)
	// TODO: Replace placeholder with actual generated proto type and marshal
	// dataBytes, err := proto.Marshal(&fsmState)
	// For now, simulate marshaling error possibility
	var dataBytes []byte
	var err error = fmt.Errorf("TODO: Implement FSM state marshaling (protobuf/gob)") // Placeholder error

	if err != nil {
		f.logger.Printf("ERROR: Failed to marshal FSM state for snapshot: %v", err)
		return nil, err
	}

	f.logger.Printf("INFO: FSM snapshot created, size: %d bytes (NOTE: Marshaling not implemented)", len(dataBytes))
	return &fsmSnapshot{state: dataBytes}, nil
}

// Restore restores the FSM state from a snapshot.
func (f *AttesterFSM) Restore(snapshot io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.logger.Printf("INFO: Restoring FSM from snapshot...")

	dataBytes, err := io.ReadAll(snapshot)
	if err != nil {
		f.logger.Printf("ERROR: Failed to read snapshot data: %v", err)
		return err
	}

	var restoredState state.FSMState
	// Deserialize the state using protobuf (or gob, json, etc.)
	// TODO: Replace placeholder with actual generated proto type and unmarshal
	// err = proto.Unmarshal(dataBytes, &restoredState)
	// For now, simulate unmarshaling error possibility
	err = fmt.Errorf("TODO: Implement FSM state unmarshaling (protobuf/gob)") // Placeholder error

	if err != nil {
		f.logger.Printf("ERROR: Failed to unmarshal FSM state from snapshot: %v", err)
		return err
	}

	// Reset current state and repopulate from the restored state
	f.processedBlocks = make(map[uint64]state.BlockHash)
	f.blockDetails = make(map[state.BlockHash]*state.BlockInfo)

	for _, info := range restoredState.Blocks {
		if info == nil {
			f.logger.Printf("WARN: Found nil BlockInfo in restored snapshot, skipping")
			continue
		}
		// Basic validation on restored data
		if len(info.Hash) != state.BlockHashSize {
			f.logger.Printf("ERROR: Invalid BlockHash size found in snapshot for height %d, skipping", info.Height)
			continue // Or return error?
		}
		if _, heightExists := f.processedBlocks[info.Height]; heightExists {
			f.logger.Printf("ERROR: Duplicate height %d found during snapshot restore, hash1: %s, hash2: %s",
				info.Height, f.processedBlocks[info.Height], info.Hash)
			return fmt.Errorf("duplicate height %d found in snapshot", info.Height)
		}
		if _, hashExists := f.blockDetails[info.Hash]; hashExists {
			f.logger.Printf("ERROR: Duplicate hash %s found during snapshot restore, height1: %d, height2: %d",
				info.Hash, f.blockDetails[info.Hash].Height, info.Height)
			return fmt.Errorf("duplicate hash %s found in snapshot", info.Hash)
		}

		f.processedBlocks[info.Height] = info.Hash
		f.blockDetails[info.Hash] = info
	}

	f.logger.Printf("INFO: FSM restored successfully from snapshot. Blocks loaded: %d (NOTE: Unmarshaling not implemented)", len(f.blockDetails))
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
		sink.Cancel() // Make sure to cancel the sink on error
		return fmt.Errorf("failed to write snapshot state to sink: %w", err)
	}
	return sink.Close()
}

// Release is called when the snapshot is no longer needed.
func (s *fsmSnapshot) Release() {
	// No-op for this simple byte slice implementation
}
