package aggregation

import (
	//"fmt"
	"testing"

	"github.com/cometbft/cometbft/crypto/ed25519"
	//"github.com/cometbft/cometbft/p2p"
	cmtypes "github.com/cometbft/cometbft/types"
	//"github.com/rollkit/rollkit/conv"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
)

func TestCentralized(t *testing.T) {
	assert := assert.New(t)

	sequencerKey := ed25519.GenPrivKey()
	attackerKey := ed25519.GenPrivKey()
	/*nodeKey := &p2p.NodeKey{
		PrivKey: genesisValidatorKey,
	}
	signingKey, _ := conv.GetNodeKey(nodeKey)*/
	sequencerPubKey := sequencerKey.PubKey()
	attackerPubKey := attackerKey.PubKey()
	validator := cmtypes.GenesisValidator{
		Address: sequencerPubKey.Address(),
		PubKey:  sequencerPubKey,
		Power:   int64(100),
		Name:    "Coolguy",
	}
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesisid",
		InitialHeight: 1,
		Validators:    []cmtypes.GenesisValidator{validator},
	}
	c, err := NewCentralizedAggregation(genesis)
	assert.NoError(err)

	block1 := types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader:      types.BaseHeader{},
				ProposerAddress: sequencerPubKey.Address().Bytes(),
			},
		},
		Data: types.Data{},
	}
	block2 := types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader:      types.BaseHeader{},
				ProposerAddress: sequencerPubKey.Address().Bytes(),
			},
		},
		Data: types.Data{},
	}
	junkBlock := types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader:      types.BaseHeader{},
				ProposerAddress: attackerPubKey.Address().Bytes(),
			},
		},
		Data: types.Data{},
	}
	assert.Equal(c.CheckSafetyInvariant(&block1, []*types.Block{}), Ok)
	assert.Equal(c.CheckSafetyInvariant(&junkBlock, []*types.Block{}), Junk)
	assert.Equal(c.CheckSafetyInvariant(&block1, []*types.Block{&block2}), ConsensusFault)
}
