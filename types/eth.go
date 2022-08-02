package types

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func (h *Header) ToEthHeader() (*ethtypes.Header, error) {
	// tempEthHeader := &ethtypes.Header{
	// 	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	// 	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	// 	Coinbase    common.Address `json:"miner"`
	// 	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	// 	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	// 	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	// 	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	// 	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	// 	Number      *big.Int       `json:"number"           gencodec:"required"`
	// 	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	// 	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	// 	Time        uint64         `json:"timestamp"        gencodec:"required"`
	// 	Extra       []byte         `json:"extraData"        gencodec:"required"`
	// 	MixDigest   common.Hash    `json:"mixHash"`
	// 	Nonce       BlockNonce     `json:"nonce"`

	// // BaseFee was added by EIP-1559 and is ignored in legacy headers.
	// BaseFee *big.Int `json:"baseFeePerGas" rlp:"optional"`
	// }
	tmHash := h.Hash()
	temp := map[string]interface{}{
		"number":           hexutil.Uint64(h.Height),
		"tm_hash":          hexutil.Bytes(tmHash[:]),
		"parentHash":       common.BytesToHash(h.LastHeaderHash[:]),
		"nonce":            ethtypes.BlockNonce{},   // PoW specific
		"sha3Uncles":       ethtypes.EmptyUncleHash, // No uncles in Tendermint
		"stateRoot":        hexutil.Bytes(h.AppHash[:]),
		"logsBloom":        ethtypes.Bloom{},
		"miner":            hexutil.Bytes(h.ProposerAddress),
		"mixHash":          common.Hash{},
		"difficulty":       (*hexutil.Big)(big.NewInt(0)),
		"extraData":        "0x",
		"size":             hexutil.Uint64(0),
		"gasLimit":         hexutil.Uint64(10000000), // Static gas limit
		"gasUsed":          hexutil.Uint64(0),
		"timestamp":        hexutil.Uint64(h.Time),
		"transactionsRoot": common.BytesToHash(h.DataHash[:]),
		"receiptsRoot":     ethtypes.EmptyRootHash,
		"uncles":           []common.Hash{},
		"totalDifficulty":  (*hexutil.Big)(big.NewInt(0)),
	}
	blockJson, err := json.Marshal(temp)
	if err != nil {
		return nil, err
	}
	var ethHeader ethtypes.Header
	err = json.Unmarshal(blockJson, &ethHeader)
	if err != nil {
		return nil, err
	}
	return &ethHeader, nil
	// return &ethtypes.Header{
	// 	ParentHash: h.LastHeaderHash,
	// 	Number:     big.NewInt(int64(h.Height)),
	// 	Time:       h.Time,
	// 	TxHash:     h.DataHash,
	// }
}
