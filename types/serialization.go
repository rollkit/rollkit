package types

import (
	pb "github.com/lazyledger/optimint/types/pb/optimint"
)

type Serializable interface {
	Serialize() ([]byte, error)
}

func (b *Block) Serialize() ([]byte, error) {
	return b.ToProto().Marshal()
}

func (h *Header) Serialize() ([]byte, error) {
	return h.ToProto().Marshal()
}

func (d *Data) Serialize() ([]byte, error) {
	return d.ToProto().Marshal()
}

func (c *Commit) Serialize() ([]byte, error) {
	return c.ToProto().Marshal()
}

func DeserializeBlock(data []byte) (*Block, error) {
	var pBlock pb.Block
	err := pBlock.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	block := &Block{}
	err = block.FromProto(&pBlock)

	return block, err
}
