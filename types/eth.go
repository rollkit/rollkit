package types

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func (h *Header) ToEthHeader() (*ethtypes.Header, error) {
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
		"baseFeePerGas":    (*hexutil.Big)(big.NewInt(0)),
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
}
