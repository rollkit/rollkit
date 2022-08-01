package types

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func (h *Header) ToEthHeaderHash() (*ethtypes.Header, error) {
	tmHash := h.Hash()
	temp := map[string]interface{}{
		"number":           hexutil.Uint64(h.Height),
		"tm_hash":          hexutil.Bytes(tmHash[:]),
		"parentHash":       h.LastHeaderHash,
		"nonce":            ethtypes.BlockNonce{},   // PoW specific
		"sha3Uncles":       ethtypes.EmptyUncleHash, // No uncles in Tendermint
		"stateRoot":        hexutil.Bytes(h.AppHash[:]),
		"miner":            h.ProposerAddress,
		"mixHash":          common.Hash{},
		"difficulty":       (*hexutil.Big)(big.NewInt(0)),
		"extraData":        "0x",
		"timestamp":        hexutil.Uint64(h.Time),
		"transactionsRoot": h.DataHash,
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
