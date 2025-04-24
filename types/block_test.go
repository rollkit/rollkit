package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

// TestValidateBasic tests the ValidateBasic method of Signature.
func TestValidateBasic(t *testing.T) {
	sig := Signature([]byte("signature"))
	assert.NoError(t, sig.ValidateBasic())

	emptySig := Signature([]byte{})
	assert.ErrorIs(t, emptySig.ValidateBasic(), ErrSignatureEmpty)

	nilSig := Signature(nil)
	assert.ErrorIs(t, nilSig.ValidateBasic(), ErrSignatureEmpty)
}

// TestValidate tests the Validate function for Data against a SignedHeader.
func TestValidate(t *testing.T) {
	header, data := GetRandomBlock(1, 10, "testchain")

	// Case 1: Valid header and data
	t.Run("valid header and data", func(t *testing.T) {
		err := Validate(header, data)
		assert.NoError(t, err)
	})

	// Case 2: Mismatched ChainID
	t.Run("mismatched chainID", func(t *testing.T) {
		invalidData := data.New()
		*invalidData = *data
		invalidData.Metadata = &Metadata{}
		*invalidData.Metadata = *data.Metadata
		invalidData.Metadata.ChainID = "wrongchain"
		err := Validate(header, invalidData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "header and data do not match")
	})

	// Case 3: Mismatched Height
	t.Run("mismatched height", func(t *testing.T) {
		invalidData := data.New()
		*invalidData = *data
		invalidData.Metadata = &Metadata{}
		*invalidData.Metadata = *data.Metadata
		invalidData.Metadata.Height = header.Height() + 1
		err := Validate(header, invalidData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "header and data do not match")
	})

	// Case 4: Mismatched Time
	t.Run("mismatched time", func(t *testing.T) {
		invalidData := data.New()
		*invalidData = *data
		invalidData.Metadata = &Metadata{}
		*invalidData.Metadata = *data.Metadata
		invalidData.Metadata.Time = uint64(header.Time().Unix() + 1)
		err := Validate(header, invalidData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "header and data do not match")
	})

	// Case 5: Mismatched DataHash (due to different Txs)
	t.Run("mismatched data hash", func(t *testing.T) {
		invalidData := data.New()
		*invalidData = *data
		invalidData.Txs = Txs{Tx("different tx")}
		err := Validate(header, invalidData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dataHash from the header does not match with hash of the block's data")
	})

	// Case 6: Data with nil Metadata (should still validate DataHash)
	t.Run("nil metadata", func(t *testing.T) {
		dataWithNilMeta := &Data{
			Txs: data.Txs,
		}
		err := Validate(header, dataWithNilMeta)
		assert.NoError(t, err)

		// Now test nil metadata with wrong Txs
		dataWithNilMetaWrongHash := &Data{
			Txs: Txs{Tx("different tx")},
		}
		err = Validate(header, dataWithNilMetaWrongHash)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dataHash from the header does not match with hash of the block's data")
	})
}

// TestDataGetters tests the getter methods on the Data struct.
func TestDataGetters(t *testing.T) {
	_, data := GetRandomBlock(5, 3, "getter-test")

	assert.Equal(t, "getter-test", data.ChainID())
	assert.Equal(t, uint64(5), data.Height())
	assert.Equal(t, data.LastDataHash, data.LastHeader())
	assert.Equal(t, time.Unix(0, int64(data.Metadata.Time)), data.Time())

	nilMetaData := &Data{Txs: data.Txs}
	assert.Panics(t, func() { nilMetaData.ChainID() })
	assert.Panics(t, func() { nilMetaData.Height() })
	assert.Panics(t, func() { nilMetaData.LastHeader() })
	assert.Panics(t, func() { nilMetaData.Time() })
}

// TestVerify tests the Verify method for comparing two Data objects.
func TestVerify(t *testing.T) {
	_, trustedData := GetRandomBlock(1, 5, "verify-test")
	_, untrustedData := GetRandomBlock(2, 5, "verify-test")

	trustedDataHash := trustedData.Hash()
	untrustedData.LastDataHash = trustedDataHash

	// Case 1: Valid verification
	t.Run("valid verification", func(t *testing.T) {
		err := trustedData.Verify(untrustedData)
		assert.NoError(t, err)
	})

	// Case 2: Invalid verification (wrong LastDataHash)
	t.Run("invalid last data hash", func(t *testing.T) {
		invalidUntrustedData := untrustedData.New()
		*invalidUntrustedData = *untrustedData
		invalidUntrustedData.Metadata = &Metadata{}
		*invalidUntrustedData.Metadata = *untrustedData.Metadata
		invalidUntrustedData.LastDataHash = []byte("clearly wrong hash")
		err := trustedData.Verify(invalidUntrustedData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "data hash of the trusted data does not match with last data hash of the untrusted data")
	})

	// Case 3: Untrusted data is nil
	t.Run("nil untrusted data", func(t *testing.T) {
		err := trustedData.Verify(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "untrusted block cannot be nil")
	})

	_, data3 := GetRandomBlock(3, 2, "another-chain")
	data3.LastDataHash = trustedDataHash
	err := trustedData.Verify(data3)
	assert.NoError(t, err, "Verify should only compare trusted.Hash() and untrusted.LastDataHash")
}

// TestDataSize tests the Size method of Data.
func TestDataSize(t *testing.T) {
	_, data := GetRandomBlock(1, 5, "size-test")
	protoData := data.ToProto()
	expectedSize := proto.Size(protoData)
	assert.Equal(t, expectedSize, data.Size())
	assert.True(t, data.Size() > 0)

	emptyData := &Data{}
	protoEmpty := emptyData.ToProto()
	expectedEmptySize := proto.Size(protoEmpty)
	assert.Equal(t, expectedEmptySize, emptyData.Size())
}

// TestIsZero tests the IsZero method of Data.
func TestIsZero(t *testing.T) {
	var nilData *Data
	assert.True(t, nilData.IsZero())

	data := &Data{}
	assert.False(t, data.IsZero())

	_, data = GetRandomBlock(1, 1, "not-zero")
	assert.False(t, data.IsZero())
}

// TestDataValidate tests the Validate method of Data returns nil
func TestDataValidate(t *testing.T) {
	_, data := GetRandomBlock(1, 5, "validate-test")
	assert.NoError(t, data.Validate())
}
