package key

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeKey_ID(t *testing.T) {
	// Generate a test key pair
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)

	nodeKey := &NodeKey{
		PrivKey: privKey,
		PubKey:  pubKey,
	}

	id := nodeKey.ID()
	assert.NotEmpty(t, id)
	assert.Equal(t, PubKeyToID(pubKey), id)
}

func TestPubKeyToID(t *testing.T) {
	// Create a valid key for testing
	_, validPubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	validID := PubKeyToID(validPubKey)
	require.NotEmpty(t, validID)

	tests := []struct {
		name    string
		pubKey  crypto.PubKey
		wantID  string
		wantErr bool
	}{
		{
			name:    "valid Ed25519 key",
			pubKey:  validPubKey,
			wantID:  validID,
			wantErr: false,
		},
		{
			name:    "nil public key",
			pubKey:  nil,
			wantID:  "",
			wantErr: false, // We now handle nil case gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := PubKeyToID(tt.pubKey)
			if tt.wantErr {
				assert.Empty(t, id)
			} else {
				if tt.wantID != "" {
					assert.Equal(t, tt.wantID, id)
				}
				if tt.pubKey != nil {
					// Verify the ID is a valid hex string
					_, err := hex.DecodeString(id)
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestNodeKey_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		setup    func() (*NodeKey, string)
		wantErr  bool
		validate func(*testing.T, *NodeKey, *NodeKey)
	}{
		{
			name: "save and load valid key",
			setup: func() (*NodeKey, string) {
				privKey, pubKey, _ := crypto.GenerateKeyPair(crypto.Ed25519, 256)
				nodeKey := &NodeKey{
					PrivKey: privKey,
					PubKey:  pubKey,
				}
				return nodeKey, filepath.Join(tempDir, "valid_key.json")
			},
			wantErr: false,
			validate: func(t *testing.T, original, loaded *NodeKey) {
				assert.NotNil(t, loaded)
				originalPubBytes, err := original.PubKey.Raw()
				require.NoError(t, err)
				loadedPubBytes, err := loaded.PubKey.Raw()
				require.NoError(t, err)
				assert.Equal(t, originalPubBytes, loadedPubBytes)

				originalPrivBytes, err := original.PrivKey.Raw()
				require.NoError(t, err)
				loadedPrivBytes, err := loaded.PrivKey.Raw()
				require.NoError(t, err)
				assert.Equal(t, originalPrivBytes, loadedPrivBytes)
			},
		},
		{
			name: "load from non-existent file",
			setup: func() (*NodeKey, string) {
				return nil, filepath.Join(tempDir, "nonexistent.json")
			},
			wantErr: true,
			validate: func(t *testing.T, original, loaded *NodeKey) {
				assert.Nil(t, loaded)
			},
		},
		{
			name: "save to invalid path",
			setup: func() (*NodeKey, string) {
				privKey, pubKey, _ := crypto.GenerateKeyPair(crypto.Ed25519, 256)
				nodeKey := &NodeKey{
					PrivKey: privKey,
					PubKey:  pubKey,
				}
				return nodeKey, filepath.Join(tempDir, "invalid", "path", "key.json")
			},
			wantErr: true,
			validate: func(t *testing.T, original, loaded *NodeKey) {
				assert.Nil(t, loaded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeKey, filePath := tt.setup()

			var err error
			if nodeKey != nil {
				err = nodeKey.SaveAs(filePath)
				if tt.wantErr {
					assert.Error(t, err)
					return
				}
				require.NoError(t, err)
			}

			loaded, err := LoadNodeKey(filePath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			tt.validate(t, nodeKey, loaded)
		})
	}
}

func TestLoadOrGenNodeKey(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
	}{
		{
			name: "generate new key",
			setup: func() string {
				return filepath.Join(tempDir, "new_key.json")
			},
			wantErr: false,
		},
		{
			name: "load existing key",
			setup: func() string {
				path := filepath.Join(tempDir, "existing_key.json")
				_, err := LoadOrGenNodeKey(path)
				require.NoError(t, err)
				return path
			},
			wantErr: false,
		},
		{
			name: "invalid path",
			setup: func() string {
				return filepath.Join(tempDir, "invalid", "path", "key.json")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup()
			nodeKey, err := LoadOrGenNodeKey(filePath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, nodeKey)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, nodeKey)
				assert.NotNil(t, nodeKey.PrivKey)
				assert.NotNil(t, nodeKey.PubKey)

				// Verify the key was persisted
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Error("key file was not created")
				}

				// Load the key again and verify it matches
				loadedKey, err := LoadNodeKey(filePath)
				require.NoError(t, err)
				assert.Equal(t, nodeKey.ID(), loadedKey.ID())
			}
		})
	}
}
