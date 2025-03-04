package sequencer

import (
	"bytes"
	"testing"
)

func TestBatchHash(t *testing.T) {
	tests := []struct {
		name    string
		batch   *Batch
		wantErr bool
	}{
		{
			name:    "nil batch",
			batch:   nil,
			wantErr: false,
		},
		{
			name:    "empty batch",
			batch:   &Batch{},
			wantErr: false,
		},
		{
			name: "single transaction",
			batch: &Batch{
				Transactions: [][]byte{[]byte("transaction1")},
			},
			wantErr: false,
		},
		{
			name: "multiple transactions",
			batch: &Batch{
				Transactions: [][]byte{
					[]byte("transaction1"),
					[]byte("transaction2"),
					[]byte("transaction3"),
				},
			},
			wantErr: false,
		},
		{
			name: "transactions with different lengths",
			batch: &Batch{
				Transactions: [][]byte{
					[]byte("short"),
					[]byte("medium length transaction"),
					[]byte("very long transaction with lots of data"),
				},
			},
			wantErr: false,
		},
		{
			name: "transactions with empty data",
			batch: &Batch{
				Transactions: [][]byte{
					[]byte{},
					[]byte("normal transaction"),
					[]byte{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First hash
			hash1, err1 := tt.batch.Hash()
			if (err1 != nil) != tt.wantErr {
				t.Errorf("Batch.Hash() error = %v, wantErr %v", err1, tt.wantErr)
				return
			}

			// Hash should be deterministic, so hashing the same batch again should give the same result
			hash2, err2 := tt.batch.Hash()
			if (err2 != nil) != tt.wantErr {
				t.Errorf("Batch.Hash() second call error = %v, wantErr %v", err2, tt.wantErr)
				return
			}

			if !bytes.Equal(hash1, hash2) {
				t.Errorf("Batch.Hash() not deterministic: first hash = %x, second hash = %x", hash1, hash2)
			}

			// For non-nil batches, verify that different batches have different hashes
			if tt.batch != nil && len(tt.batch.Transactions) > 0 {
				// Create a modified batch
				modifiedBatch := &Batch{
					Transactions: make([][]byte, len(tt.batch.Transactions)),
				}
				copy(modifiedBatch.Transactions, tt.batch.Transactions)

				// Modify the batch (add a byte to the first transaction)
				if len(modifiedBatch.Transactions) > 0 {
					modifiedBatch.Transactions[0] = append(modifiedBatch.Transactions[0], byte('!'))
				}

				modifiedHash, err := modifiedBatch.Hash()
				if err != nil {
					t.Errorf("Modified batch hash error = %v", err)
					return
				}

				if bytes.Equal(hash1, modifiedHash) {
					t.Errorf("Different batches produced the same hash: %x", hash1)
				}
			}

			// Verify hash is non-empty for all cases
			if hash1 != nil && len(hash1) == 0 {
				t.Errorf("Hash returned empty result")
			}

			// Verify hash is the correct length for SHA-256 (32 bytes)
			if hash1 != nil && len(hash1) != 32 {
				t.Errorf("Hash returned incorrect length: got %d, want 32", len(hash1))
			}
		})
	}
}
