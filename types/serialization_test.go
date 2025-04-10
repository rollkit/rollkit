package types

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

func TestBlockSerializationRoundTrip(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// create random hashes
	h := []Hash{}
	for i := 0; i < 8; i++ {
		h1 := make(Hash, 32)
		n, err := rand.Read(h1[:])
		require.Equal(32, n)
		require.NoError(err)
		h = append(h, h1)
	}

	h1 := Header{
		Version: Version{
			Block: 1,
			App:   2,
		},
		BaseHeader: BaseHeader{
			Height: 3,
			Time:   4567,
		},
		LastHeaderHash:  h[0],
		LastCommitHash:  h[1],
		DataHash:        h[2],
		ConsensusHash:   h[3],
		AppHash:         h[4],
		LastResultsHash: h[5],
		ProposerAddress: []byte{4, 3, 2, 1},
	}

	pubKey1, _, err := crypto.GenerateEd25519Key(rand.Reader)
	require.NoError(err)
	signer1, err := NewSigner(pubKey1.GetPublic())
	require.NoError(err)

	cases := []struct {
		name   string
		header *SignedHeader
		data   *Data
	}{
		{"empty block", &SignedHeader{}, &Data{Metadata: &Metadata{}}},
		{"full", &SignedHeader{
			Header:    h1,
			Signature: Signature([]byte{1, 1, 1}),
			Signer:    signer1,
		}, &Data{
			Metadata: &Metadata{},
			Txs:      nil,
		},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			blob, err := c.header.MarshalBinary()
			assert.NoError(err)
			assert.NotEmpty(blob)
			deserializedHeader := &SignedHeader{}
			err = deserializedHeader.UnmarshalBinary(blob)
			assert.NoError(err)
			assert.Equal(c.header, deserializedHeader)

			blob, err = c.data.MarshalBinary()
			assert.NoError(err)
			assert.NotEmpty(blob)
			deserializedData := &Data{}
			err = deserializedData.UnmarshalBinary(blob)
			assert.NoError(err)

			assert.Equal(c.data, deserializedData)
		})
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state State
	}{
		{
			"with max bytes",
			State{},
		},
		{
			name: "with all fields set",
			state: State{
				Version: Version{
					Block: 123,
					App:   456,
				},
				ChainID:         "testchain",
				InitialHeight:   987,
				LastBlockHeight: 987654321,
				LastBlockTime:   time.Date(2022, 6, 6, 12, 12, 33, 44, time.UTC),
				DAHeight:        3344,
				LastResultsHash: Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2},
				AppHash:         Hash{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require := require.New(t)
			assert := assert.New(t)
			pState, err := c.state.ToProto()
			require.NoError(err)
			require.NotNil(pState)

			bytes, err := proto.Marshal(pState)
			require.NoError(err)
			require.NotEmpty(bytes)

			var newProtoState pb.State
			var newState State
			err = proto.Unmarshal(bytes, &newProtoState)
			require.NoError(err)

			err = newState.FromProto(&newProtoState)
			require.NoError(err)

			assert.Equal(c.state, newState)
		})
	}
}

func TestTxsRoundtrip(t *testing.T) {
	// Test the nil case
	var txs Txs
	byteSlices := txsToByteSlices(txs)
	newTxs := byteSlicesToTxs(byteSlices)
	assert.Nil(t, newTxs)

	// Generate 100 random transactions and convert them to byte slices
	txs = make(Txs, 100)
	for i := range txs {
		txs[i] = []byte{byte(i)}
	}
	byteSlices = txsToByteSlices(txs)

	// Convert the byte slices back to transactions
	newTxs = byteSlicesToTxs(byteSlices)

	// Check that the new transactions match the original transactions
	assert.Equal(t, len(txs), len(newTxs))
	for i := range txs {
		assert.Equal(t, txs[i], newTxs[i])
	}
}
