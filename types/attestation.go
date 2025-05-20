package types

import (
	"encoding/json"
	"time"
)

// RollkitSequencerAttestation holds the essential data signed by the sequencer for a given block.
// This data is used by the ABCI Adapter to construct a CometBFT-style Commit.
type RollkitSequencerAttestation struct {
	Height           uint64
	Round            int32 // Round can be 0 for Rollkit as it has a single sequencer
	BlockHeaderHash  []byte
	BlockDataHash    []byte
	SequencerAddress []byte
	Timestamp        time.Time
	Signature        []byte // Signature over (Height, Round, BlockHeaderHash, BlockDataHash, Timestamp)
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (att *RollkitSequencerAttestation) MarshalBinary() ([]byte, error) {
	return json.Marshal(att)
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (att *RollkitSequencerAttestation) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, att)
}
