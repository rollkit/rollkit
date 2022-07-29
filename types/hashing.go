package types

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tmtypes "github.com/tendermint/tendermint/types"
	"golang.org/x/crypto/sha3"
)

// Hash returns ABCI-compatible hash of a header.
func (h *Header) Hash() [32]byte {
	abciHeader := tmtypes.Header{
		Version: tmversion.Consensus{
			Block: h.Version.Block,
			App:   h.Version.App,
		},
		Height: int64(h.Height),
		Time:   time.Unix(int64(h.Time), 0),
		LastBlockID: tmtypes.BlockID{
			Hash: h.LastHeaderHash[:],
			PartSetHeader: tmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     h.LastCommitHash[:],
		DataHash:           h.DataHash[:],
		ValidatorsHash:     h.AggregatorsHash[:],
		NextValidatorsHash: nil,
		ConsensusHash:      h.ConsensusHash[:],
		AppHash:            h.AppHash[:],
		LastResultsHash:    h.LastResultsHash[:],
		EvidenceHash:       new(tmtypes.EvidenceData).Hash(),
		ProposerAddress:    h.ProposerAddress,
	}
	var hash [32]byte
	copy(hash[:], abciHeader.Hash())
	return hash
}

// Hash returns ethereum-compatible hash of a block.
func (b *Block) RlpHash() [32]byte {
	return b.Header.RlpHash()
}

// Hash returns ABCI-compatible hash of a block.
func (b *Block) Hash() [32]byte {
	return b.Header.Hash()
}

var hasherPool = sync.Pool{
	New: func() interface{} { return sha3.NewLegacyKeccak256() },
}

func (h *Header) RlpHash() [32]byte {
	return rlpHash(h)
}

func rlpHash(x interface{}) (h common.Hash) {
	sha := hasherPool.Get().(crypto.KeccakState)
	defer hasherPool.Put(sha)
	sha.Reset()
	rlp.Encode(sha, x)
	sha.Read(h[:])
	return h
}
