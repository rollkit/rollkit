package types

import (
	"bytes"
	"encoding/gob"
)

func SerializeBlock(block *Block) ([]byte, error) {
	return actuallySerialize(block)
}

func DeserializeBlock(data []byte) (*Block, error) {
	var block Block
	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&block)
	if err != nil {
		return nil, err
	}
	return &block, nil
}

func SerializeBlockHeader(block *Block) ([]byte, error) {
	return actuallySerialize(block.Header)
}

// actuallySerialize is a single function that unifies serialization.
// Currently, temporarily, it uses encoding/gob
func actuallySerialize(object interface{}) ([]byte, error) {
	var data bytes.Buffer
	enc := gob.NewEncoder(&data)
	err := enc.Encode(object)
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}
