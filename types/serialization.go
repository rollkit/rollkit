package types

import (
	pb "github.com/lazyledger/optimint/types/pb/optimint"
)

func SerializeBlock(block *Block) ([]byte, error) {
	return block.ToProto().Marshal()
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

func SerializeBlockHeader(block *Block) ([]byte, error) {
	return block.Header.ToProto().Marshal()
}
