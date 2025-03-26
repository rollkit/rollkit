package noop

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"
)

func TestNoopSigner(t *testing.T) {
	t.Run("create new signer", func(t *testing.T) {
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(t, err)
		signer, err := NewNoopSigner(privKey)
		require.NoError(t, err)
		require.NotNil(t, signer)
	})

	t.Run("get public key", func(t *testing.T) {
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(t, err)
		signer, err := NewNoopSigner(privKey)
		require.NoError(t, err)

		pubKey, err := signer.GetPublic()
		require.NoError(t, err)
		require.NotNil(t, pubKey)
	})

	t.Run("sign and verify", func(t *testing.T) {
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(t, err)
		signer, err := NewNoopSigner(privKey)
		require.NoError(t, err)

		message := []byte("test message")
		signature, err := signer.Sign(message)
		require.NoError(t, err)
		require.NotNil(t, signature)

		// Verify signature using the public key
		pubKey, err := signer.GetPublic()
		require.NoError(t, err)

		valid, err := pubKey.Verify(message, signature)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("different signers have different keys", func(t *testing.T) {
		privKey1, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(t, err)
		signer1, err := NewNoopSigner(privKey1)
		require.NoError(t, err)

		privKey2, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(t, err)
		signer2, err := NewNoopSigner(privKey2)
		require.NoError(t, err)

		pub1, err := signer1.GetPublic()
		require.NoError(t, err)

		pub2, err := signer2.GetPublic()
		require.NoError(t, err)

		pub1Bytes, err := pub1.Raw()
		require.NoError(t, err)

		pub2Bytes, err := pub2.Raw()
		require.NoError(t, err)

		require.NotEqual(t, pub1Bytes, pub2Bytes)
	})

	t.Run("verify wrong message fails", func(t *testing.T) {
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(t, err)
		signer, err := NewNoopSigner(privKey)
		require.NoError(t, err)

		message := []byte("test message")
		wrongMessage := []byte("wrong message")

		signature, err := signer.Sign(message)
		require.NoError(t, err)

		pubKey, err := signer.GetPublic()
		require.NoError(t, err)

		valid, err := pubKey.Verify(wrongMessage, signature)
		require.NoError(t, err)
		require.False(t, valid)
	})
}
