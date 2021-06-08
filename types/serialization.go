package types

import (
	pb "github.com/lazyledger/optimint/types/pb/optimint"
)

func (b *Block) MarshalBinary() ([]byte, error) {
	return b.ToProto().Marshal()
}

func (h *Header) MarshalBinary() ([]byte, error) {
	return h.ToProto().Marshal()
}

func (d *Data) MarshalBinary() ([]byte, error) {
	return d.ToProto().Marshal()
}

func (c *Commit) MarshalBinary() ([]byte, error) {
	return c.ToProto().Marshal()
}

func (b *Block) UnmarshalBinary(data []byte) error {
	var pBlock pb.Block
	err := pBlock.Unmarshal(data)
	if err != nil {
		return err
	}
	err = b.FromProto(&pBlock)

	return err
}
