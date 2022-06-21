package types

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tendermint/tendermint/crypto/ed25519"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	pb "github.com/celestiaorg/optimint/types/pb/optimint"
)

func TestBlockSerializationRoundTrip(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// create random hashes
	h := [][32]byte{}
	for i := 0; i < 8; i++ {
		var h1 [32]byte
		n, err := rand.Read(h1[:])
		require.Equal(32, n)
		require.NoError(err)
		h = append(h, h1)
	}

	cases := []struct {
		name  string
		input *Block
	}{
		{"empty block", &Block{}},
		{"full", &Block{
			Header: Header{
				Version: Version{
					Block: 1,
					App:   2,
				},
				NamespaceID:     [8]byte{0, 1, 2, 3, 4, 5, 6, 7},
				Height:          3,
				Time:            4567,
				LastHeaderHash:  h[0],
				LastCommitHash:  h[1],
				DataHash:        h[2],
				ConsensusHash:   h[3],
				AppHash:         h[4],
				LastResultsHash: h[5],
				ProposerAddress: []byte{4, 3, 2, 1},
				AggregatorsHash: h[6],
			},
			Data: Data{
				Txs:                    nil,
				IntermediateStateRoots: IntermediateStateRoots{RawRootsList: [][]byte{{0x1}}},
				// TODO(tzdybal): update when we have actual evidence types
				Evidence: EvidenceData{Evidence: nil},
			},
			LastCommit: Commit{
				Height:     8,
				HeaderHash: h[7],
				Signatures: []Signature{Signature([]byte{1, 1, 1}), Signature([]byte{2, 2, 2})},
			},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			blob, err := c.input.MarshalBinary()
			assert.NoError(err)
			assert.NotEmpty(blob)

			deserialized := &Block{}
			err = deserialized.UnmarshalBinary(blob)
			assert.NoError(err)

			assert.Equal(c.input, deserialized)
		})
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Parallel()

	valSet := getRandomValidatorSet()

	cases := []struct {
		name  string
		state State
	}{
		{"with max bytes", State{
			LastValidators: valSet,
			Validators:     valSet,
			NextValidators: valSet,
			ConsensusParams: tmproto.ConsensusParams{
				Block: tmproto.BlockParams{
					MaxBytes:   123,
					MaxGas:     456,
					TimeIotaMs: 789,
				},
			}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require := require.New(t)
			assert := assert.New(t)
			pState, err := c.state.ToProto()
			require.NoError(err)
			require.NotNil(pState)

			bytes, err := pState.Marshal()
			require.NoError(err)
			require.NotEmpty(bytes)

			var newProtoState pb.State
			var newState State
			err = newProtoState.Unmarshal(bytes)
			require.NoError(err)

			err = newState.FromProto(&newProtoState)
			require.NoError(err)

			assert.Equal(c.state, newState)
		})
	}
}

// copied from store_test.go
func getRandomValidatorSet() *tmtypes.ValidatorSet {
	pubKey := ed25519.GenPrivKey().PubKey()
	return &tmtypes.ValidatorSet{
		Proposer: &tmtypes.Validator{PubKey: pubKey, Address: pubKey.Address()},
		Validators: []*tmtypes.Validator{
			{PubKey: pubKey, Address: pubKey.Address()},
		},
	}
}
