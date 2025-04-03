package state

import (
	"encoding/hex"
	"fmt"
)

// BlockHashSize defines the expected size of a block hash (e.g., 32 for SHA-256)
const BlockHashSize = 32

// BlockHash represents a block hash.
type BlockHash [BlockHashSize]byte

// String returns the hex-encoded representation of the BlockHash.
func (bh BlockHash) String() string {
	return hex.EncodeToString(bh[:])
}

// BlockInfo represents the relevant information about a block maintained by the FSM
type BlockInfo struct {
	Height     uint64    `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	Hash       BlockHash `protobuf:"bytes,2,opt,name=hash,proto3,casttype=BlockHash" json:"hash,omitempty"` // Needs custom marshaller for proto or use []byte
	DataToSign []byte    `protobuf:"bytes,3,opt,name=data_to_sign,json=dataToSign,proto3" json:"data_to_sign,omitempty"`
	Signature  []byte    `protobuf:"bytes,4,opt,name=signature,proto3" json:"signature,omitempty"` // Individual signature or share
}

// FSMState represents the complete state of the FSM to be serialized for Snapshots.
// The exact structure will depend on the chosen serialization method (Protobuf, gob, etc.).
// If using Protobuf, this struct should match a .proto message.
// Example with Protobuf (requires .proto file and code generation):
type FSMState struct {
	// We could serialize the list of BlockInfo instead of separate maps
	// to simplify serialization/deserialization.
	Blocks []*BlockInfo `protobuf:"bytes,1,rep,name=blocks,proto3" json:"blocks,omitempty"`
}

// --- Helper functions for BlockHash ---

// BlockHashFromBytes converts a byte slice into a BlockHash, checking the length.
func BlockHashFromBytes(b []byte) (BlockHash, error) {
	var hash BlockHash
	if len(b) != BlockHashSize {
		return hash, fmt.Errorf("invalid block hash size: expected %d, got %d", BlockHashSize, len(b))
	}
	copy(hash[:], b)
	return hash, nil
}
