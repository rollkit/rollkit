package types

import (
	"crypto/sha256"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewSigner tests the creation of a new Signer.
func TestNewSigner(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)

	signer, err := NewSigner(pubKey)
	require.NoError(err)

	assert.Equal(pubKey, signer.PubKey)
	assert.True(pubKey.Equals(signer.PubKey))

	pubKeyBytes, err := pubKey.Raw()
	require.NoError(err)
	expectedAddress := sha256.Sum256(pubKeyBytes)
	assert.Equal(expectedAddress[:], signer.Address)

}

// TestSignerVerify tests the Verify method of the Signer.
func TestSignerVerify(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)

	signer, err := NewSigner(pubKey)
	require.NoError(err)

	message := []byte("this is a message to sign")

	// Create a valid signature
	signature, err := privKey.Sign(message)
	require.NoError(err)

	// Verify valid signature
	valid, err := signer.Verify(message, signature)
	assert.NoError(err)
	assert.True(valid, "Verification should succeed with a valid signature")

	// Verify invalid signature (tampered signature)
	invalidSignature := append(signature[:len(signature)-1], signature[len(signature)-1]+1) // Modify last byte
	invalid, err := signer.Verify(message, invalidSignature)
	assert.False(invalid, "Verification should fail with an invalid signature")

	// Verify wrong message
	wrongMessage := []byte("this is not the signed message")
	invalid, err = signer.Verify(wrongMessage, signature)
	assert.False(invalid, "Verification should fail with the wrong message")

	// Verify with a different key
	_, otherPubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	otherSigner, err := NewSigner(otherPubKey)
	require.NoError(err)
	invalid, err = otherSigner.Verify(message, signature)
	assert.False(invalid, "Verification should fail with the wrong public key")
}

// TestKeyAddress tests the KeyAddress helper function.
func TestKeyAddress(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)

	address := KeyAddress(pubKey)
	assert.NotNil(address)

	pubKeyBytes, err := pubKey.Raw()
	require.NoError(err)
	expectedAddress := sha256.Sum256(pubKeyBytes)

	assert.Equal(expectedAddress[:], address, "KeyAddress should produce the correct SHA256 hash")

	signer, err := NewSigner(pubKey)
	require.NoError(err)
	assert.Equal(signer.Address, address, "KeyAddress should match address from NewSigner")
}
