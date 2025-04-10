package state

import (
	"encoding/hex"
	"fmt"
)

const BlockHashSize = 32

type BlockHash [BlockHashSize]byte

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
type FSMState struct {
	Blocks []*BlockInfo `protobuf:"bytes,1,rep,name=blocks,proto3" json:"blocks,omitempty"`
}

// BlockHashFromBytes converts a byte slice into a BlockHash, checking the length.
func BlockHashFromBytes(b []byte) (BlockHash, error) {
	var hash BlockHash
	if len(b) != BlockHashSize {
		return hash, fmt.Errorf("invalid block hash size: expected %d, got %d", BlockHashSize, len(b))
	}
	copy(hash[:], b)
	return hash, nil
}
