package types

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/evstack/ev-node/types/pb/evnode/v1"
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
			Txs:      Txs{},
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

			// When deserialized, nil Txs are converted to empty slices to prevent for-loop panics.
			if c.data.Txs == nil {
				c.data.Txs = Txs{}
			}
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
	assert.Equal(t, Txs{}, newTxs)

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

// TestUnmarshalBinary_InvalidData checks that all UnmarshalBinary methods return an error on corrupted/invalid input.
func TestUnmarshalBinary_InvalidData(t *testing.T) {
	invalid := []byte{0xFF, 0x00, 0x01}
	assert := assert.New(t)

	typesToTest := []struct {
		name string
		obj  interface{ UnmarshalBinary([]byte) error }
	}{
		{"Header", &Header{}},
		{"SignedHeader", &SignedHeader{}},
		{"Data", &Data{}},
		{"Metadata", &Metadata{}},
		{"SignedData", &SignedData{}},
	}
	for _, tt := range typesToTest {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.obj.UnmarshalBinary(invalid)
			assert.Error(err)
		})
	}
}

// TestFromProto_NilInput checks that all FromProto methods return an error when given nil input.
func TestFromProto_NilInput(t *testing.T) {
	assert := assert.New(t)
	// Header
	h := &Header{}
	assert.Error(h.FromProto(nil))
	// SignedHeader
	sh := &SignedHeader{}
	assert.Error(sh.FromProto(nil))
	// Data
	d := &Data{}
	assert.Error(d.FromProto(nil))
	// Metadata
	m := &Metadata{}
	assert.Error(m.FromProto(nil))
	// State
	s := &State{}
	assert.Error(s.FromProto(nil))
	// SignedData
	sd := &SignedData{}
	assert.Error(sd.FromProto(nil))
}

// TestSignedHeader_SignerNilPubKey ensures ToProto/FromProto handle a SignedHeader with a nil PubKey in the Signer.
func TestSignedHeader_SignerNilPubKey(t *testing.T) {
	sh := &SignedHeader{
		Header:    Header{},
		Signature: []byte{1, 2, 3},
		Signer:    Signer{PubKey: nil, Address: nil},
	}
	protoMsg, err := sh.ToProto()
	assert.NoError(t, err)
	assert.NotNil(t, protoMsg)
	// Should have empty Signer in proto
	assert.NotNil(t, protoMsg.Signer)
	assert.Empty(t, protoMsg.Signer.PubKey)
	assert.Empty(t, protoMsg.Signer.Address)

	// FromProto with nil pubkey
	sh2 := &SignedHeader{}
	err = sh2.FromProto(protoMsg)
	assert.NoError(t, err)
	assert.Nil(t, sh2.Signer.PubKey)
	assert.Empty(t, sh2.Signer.Address)
}

// TestSignedHeader_EmptySignatureAndNilFields checks roundtrip serialization for a SignedHeader with empty signature and nil fields.
func TestSignedHeader_EmptySignatureAndNilFields(t *testing.T) {
	sh := &SignedHeader{
		Header:    Header{},
		Signature: nil,
		Signer:    Signer{},
	}
	b, err := sh.MarshalBinary()
	assert.NoError(t, err)
	sh2 := &SignedHeader{}
	err = sh2.UnmarshalBinary(b)
	assert.NoError(t, err)
	assert.Equal(t, sh, sh2)
}

// TestSignedData_MarshalUnmarshal_Roundtrip checks roundtrip binary serialization for SignedData, including nil and fully populated fields.
func TestSignedData_MarshalUnmarshal_Roundtrip(t *testing.T) {
	// With nil fields
	sd := &SignedData{}
	b, err := sd.MarshalBinary()
	assert.NoError(t, err)
	sd2 := &SignedData{}
	assert.NoError(t, sd2.UnmarshalBinary(b))
	// Normalize nil and empty Txs for comparison
	if sd.Txs == nil {
		sd.Txs = Txs{}
	}
	if sd2.Txs == nil {
		sd2.Txs = Txs{}
	}
	assert.Equal(t, sd, sd2)

	// With all fields set
	pubKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	assert.NoError(t, err)
	signer, err := NewSigner(pubKey.GetPublic())
	assert.NoError(t, err)
	sd = &SignedData{
		Data: Data{
			Metadata: &Metadata{ChainID: "c", Height: 1, Time: 2, LastDataHash: []byte{1, 2}},
			Txs:      Txs{[]byte("tx1"), []byte("tx2")},
		},
		Signature: []byte{1, 2, 3},
		Signer:    signer,
	}
	b, err = sd.MarshalBinary()
	assert.NoError(t, err)
	sd2 = &SignedData{}
	assert.NoError(t, sd2.UnmarshalBinary(b))
	assert.Equal(t, sd.Data, sd2.Data)
	assert.Equal(t, sd.Signature, sd2.Signature)
	assert.Equal(t, sd.Signer.Address, sd2.Signer.Address)
}

// TestSignedData_FromProto_NilAndEmptyFields checks that FromProto on SignedData handles nil and empty fields gracefully.
func TestSignedData_FromProto_NilAndEmptyFields(t *testing.T) {
	sd := &SignedData{}
	// All fields nil
	protoMsg := &pb.SignedData{}
	assert.NoError(t, sd.FromProto(protoMsg))
	assert.Nil(t, sd.Signature)
	assert.Empty(t, sd.Signer.Address)
	assert.Nil(t, sd.Signer.PubKey)
}

// TestData_ToProtoFromProto_NilMetadataAndTxs checks Data ToProto/FromProto with nil and empty Metadata/Txs.
func TestData_ToProtoFromProto_NilMetadataAndTxs(t *testing.T) {
	d := &Data{Metadata: nil, Txs: nil}
	protoMsg := d.ToProto()
	assert.Nil(t, protoMsg.Metadata)
	assert.Nil(t, protoMsg.Txs)

	d2 := &Data{}
	assert.NoError(t, d2.FromProto(protoMsg))
	assert.Nil(t, d2.Metadata)
	assert.Empty(t, d2.Txs)

	// Txs empty
	d = &Data{Metadata: &Metadata{ChainID: "c"}, Txs: Txs{}}
	protoMsg = d.ToProto()
	assert.NotNil(t, protoMsg.Metadata)
	assert.Empty(t, protoMsg.Txs)
}

// TestState_ToProtoFromProto_ZeroFields checks State ToProto/FromProto with all fields zeroed.
func TestState_ToProtoFromProto_ZeroFields(t *testing.T) {
	s := &State{}
	protoMsg, err := s.ToProto()
	assert.NoError(t, err)
	assert.NotNil(t, protoMsg)

	s2 := &State{}
	assert.NoError(t, s2.FromProto(protoMsg))
	assert.Equal(t, s, s2)
}

// TestHeader_HashFields_NilAndEmpty checks Header ToProto/FromProto with nil and empty hash/byte slice fields.
func TestHeader_HashFields_NilAndEmpty(t *testing.T) {
	h := &Header{}
	protoMsg := h.ToProto()
	assert.NotNil(t, protoMsg)
	// All hash fields should be empty slices
	assert.Empty(t, protoMsg.LastHeaderHash)
	assert.Empty(t, protoMsg.LastCommitHash)
	assert.Empty(t, protoMsg.DataHash)
	assert.Empty(t, protoMsg.ConsensusHash)
	assert.Empty(t, protoMsg.AppHash)
	assert.Empty(t, protoMsg.LastResultsHash)
	assert.Empty(t, protoMsg.ProposerAddress)
	assert.Empty(t, protoMsg.ValidatorHash)

	h2 := &Header{}
	assert.NoError(t, h2.FromProto(protoMsg))
	assert.Nil(t, h2.LastHeaderHash)
	assert.Nil(t, h2.LastCommitHash)
	assert.Nil(t, h2.DataHash)
	assert.Nil(t, h2.ConsensusHash)
	assert.Nil(t, h2.AppHash)
	assert.Nil(t, h2.LastResultsHash)
	assert.Nil(t, h2.ProposerAddress)
	assert.Nil(t, h2.ValidatorHash)
}

// TestProtoConversionConsistency_AllTypes checks that ToProto/FromProto are true inverses for all major types.
func TestProtoConversionConsistency_AllTypes(t *testing.T) {
	// Header
	h := &Header{
		Version:         Version{Block: 1, App: 2},
		BaseHeader:      BaseHeader{Height: 3, Time: 4, ChainID: "cid"},
		LastHeaderHash:  []byte{1, 2},
		LastCommitHash:  []byte{3, 4},
		DataHash:        []byte{5, 6},
		ConsensusHash:   []byte{7, 8},
		AppHash:         []byte{9, 10},
		LastResultsHash: []byte{11, 12},
		ProposerAddress: []byte{13, 14},
		ValidatorHash:   []byte{15, 16},
	}
	protoMsg := h.ToProto()
	h2 := &Header{}
	assert.NoError(t, h2.FromProto(protoMsg))
	assert.Equal(t, h, h2)

	// Metadata
	m := &Metadata{ChainID: "cid", Height: 1, Time: 2, LastDataHash: []byte{1, 2}}
	protoMeta := m.ToProto()
	m2 := &Metadata{}
	assert.NoError(t, m2.FromProto(protoMeta))
	assert.Equal(t, m, m2)

	// Data
	d := &Data{Metadata: m, Txs: Txs{[]byte("a"), []byte("b")}}
	protoData := d.ToProto()
	d2 := &Data{}
	assert.NoError(t, d2.FromProto(protoData))
	assert.Equal(t, d, d2)

	// State
	s := &State{Version: Version{Block: 1, App: 2}, ChainID: "cid", InitialHeight: 1, LastBlockHeight: 2, LastBlockTime: time.Now().UTC(), DAHeight: 3, LastResultsHash: []byte{1, 2}, AppHash: []byte{3, 4}}
	protoState, err := s.ToProto()
	assert.NoError(t, err)
	s2 := &State{}
	assert.NoError(t, s2.FromProto(protoState))
	assert.Equal(t, s, s2)
}
