package block

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goDA "github.com/rollkit/go-da"
	goDATest "github.com/rollkit/go-da/test"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

// Returns a minimalistic block manager
func getManager(t *testing.T, backend goDA.DA) *Manager {
	logger := test.NewFileLoggerCustom(t, test.TempLogFileName(t, t.Name()))
	return &Manager{
		dalc:       &da.DAClient{DA: backend, GasPrice: -1, GasMultiplier: -1, Logger: logger},
		blockCache: NewBlockCache(),
		logger:     logger,
	}
}

// getBlockBiggerThan generates a block with the given height bigger than the specified limit.
func getBlockBiggerThan(blockHeight, limit uint64) (*types.Block, error) {
	for numTxs := 0; ; numTxs += 100 {
		block := types.GetRandomBlock(blockHeight, numTxs)
		blob, err := block.MarshalBinary()
		if err != nil {
			return nil, err
		}

		if uint64(len(blob)) > limit {
			return block, nil
		}
	}
}

func TestInitialStateClean(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 1,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(es)
	s, err := getInitialState(emptyStore, genesis)
	require.Equal(s.LastBlockHeight, uint64(genesis.InitialHeight-1))
	require.NoError(err)
	require.Equal(uint64(genesis.InitialHeight), s.InitialHeight)
}

func TestInitialStateStored(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 1,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "myChain",
		InitialHeight:   1,
		LastBlockHeight: 100,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	s, err := getInitialState(store, genesis)
	require.Equal(s.LastBlockHeight, uint64(100))
	require.NoError(err)
	require.Equal(s.InitialHeight, uint64(1))
}

func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 2,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "myChain",
		InitialHeight:   1,
		LastBlockHeight: 0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	_, err = getInitialState(store, genesis)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")
}

func TestIsDAIncluded(t *testing.T) {
	require := require.New(t)

	// Create a minimalistic block manager
	m := &Manager{
		blockCache: NewBlockCache(),
	}
	hash := types.Hash([]byte("hash"))

	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(hash))

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.blockCache.setDAIncluded(hash.String())
	require.True(m.IsDAIncluded(hash))
}

func TestSubmitBlocksToMockDA(t *testing.T) {
	ctx := context.Background()

	mockDA := &mock.MockDA{}
	m := getManager(t, mockDA)
	m.conf.DABlockTime = time.Millisecond
	m.conf.DAMempoolTTL = 1
	m.dalc.GasPrice = 1.0
	m.dalc.GasMultiplier = 1.2
	kvStore, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	m.store = store.New(kvStore)

	t.Run("handle_tx_already_in_mempool", func(t *testing.T) {
		var blobs [][]byte
		block := types.GetRandomBlock(1, 5)
		blob, err := block.MarshalBinary()
		require.NoError(t, err)

		err = m.store.SaveBlock(ctx, block, &types.Commit{})
		require.NoError(t, err)
		m.store.SetHeight(ctx, 1)

		blobs = append(blobs, blob)
		// Set up the mock to
		// * throw timeout waiting for tx to be included exactly once
		// * wait for tx to drop from mempool exactly DABlockTime * DAMempoolTTL seconds
		// * retry with a higher gas price
		// * successfully submit
		mockDA.On("MaxBlobSize").Return(uint64(12345), nil)
		mockDA.
			On("Submit", blobs, 1.0, []byte(nil)).
			Return([][]byte{}, da.ErrTxTimedout).Once()
		mockDA.
			On("Submit", blobs, 1.0*1.2, []byte(nil)).
			Return([][]byte{}, da.ErrTxAlreadyInMempool).Times(int(m.conf.DAMempoolTTL))
		mockDA.
			On("Submit", blobs, 1.0*1.2*1.2, []byte(nil)).
			Return([][]byte{bytes.Repeat([]byte{0x00}, 8)}, nil)

		m.pendingBlocks, err = NewPendingBlocks(m.store, m.logger)
		require.NoError(t, err)
		err = m.submitBlocksToDA(ctx)
		require.NoError(t, err)
		mockDA.AssertExpectations(t)
	})
}

func TestSubmitBlocksToDA(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	m := getManager(t, goDATest.NewDummyDA())

	maxDABlobSizeLimit, err := m.dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

	testCases := []struct {
		name                        string
		blocks                      []*types.Block
		isErrExpected               bool
		expectedPendingBlocksLength int
	}{
		{
			name:                        "happy path, all blocks A, B, C combine to less than maxDABlobSize",
			blocks:                      []*types.Block{types.GetRandomBlock(1, 5), types.GetRandomBlock(2, 5), types.GetRandomBlock(3, 5)},
			isErrExpected:               false,
			expectedPendingBlocksLength: 0,
		},
		{
			name: "blocks A and B are submitted together without C because including C triggers blob size limit. C is submitted in a separate round",
			blocks: func() []*types.Block {
				// Find three blocks where two of them are under blob size limit
				// but adding the third one exceeds the blob size limit
				block1 := types.GetRandomBlock(1, 100)
				blob1, err := block1.MarshalBinary()
				require.NoError(err)

				block2 := types.GetRandomBlock(2, 100)
				blob2, err := block2.MarshalBinary()
				require.NoError(err)

				block3, err := getBlockBiggerThan(3, maxDABlobSizeLimit-uint64(len(blob1)+len(blob2)))
				require.NoError(err)

				return []*types.Block{block1, block2, block3}
			}(),
			isErrExpected:               false,
			expectedPendingBlocksLength: 0,
		},
		{
			name: "A and B are submitted successfully but C is too big on its own, so C never gets submitted",
			blocks: func() []*types.Block {
				numBlocks, numTxs := 3, 5
				blocks := make([]*types.Block, numBlocks)
				for i := 0; i < numBlocks-1; i++ {
					blocks[i] = types.GetRandomBlock(uint64(i+1), numTxs)
				}
				blocks[2], err = getBlockBiggerThan(3, maxDABlobSizeLimit)
				require.NoError(err)
				return blocks
			}(),
			isErrExpected:               true,
			expectedPendingBlocksLength: 1,
		},
	}

	for _, tc := range testCases {
		// there is a limitation of value size for underlying in-memory KV store, so (temporary) on-disk store is needed
		kvStore := getTempKVStore(t)
		m.store = store.New(kvStore)
		m.pendingBlocks, err = NewPendingBlocks(m.store, m.logger)
		require.NoError(err)
		t.Run(tc.name, func(t *testing.T) {
			// PendingBlocks depend on store, so blocks needs to be saved and height updated
			for _, block := range tc.blocks {
				require.NoError(m.store.SaveBlock(ctx, block, &types.Commit{}))
			}
			m.store.SetHeight(ctx, uint64(len(tc.blocks)))

			err := m.submitBlocksToDA(ctx)
			assert.Equal(tc.isErrExpected, err != nil)
			blocks, err := m.pendingBlocks.getPendingBlocks(ctx)
			assert.NoError(err)
			assert.Equal(tc.expectedPendingBlocksLength, len(blocks))

			// ensure that metadata is updated in KV store
			raw, err := m.store.GetMetadata(ctx, LastSubmittedHeightKey)
			require.NoError(err)
			lshInKV, err := strconv.ParseUint(string(raw), 10, 64)
			require.NoError(err)
			assert.Equal(m.store.Height(), lshInKV+uint64(tc.expectedPendingBlocksLength))
		})
	}
}

func getTempKVStore(t *testing.T) ds.TxnDatastore {
	dbPath, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dbPath)
	})
	kvStore, err := store.NewDefaultKVStore(os.TempDir(), dbPath, t.Name())
	require.NoError(t, err)
	return kvStore
}

func Test_isProposer(t *testing.T) {
	require := require.New(t)

	type args struct {
		genesis       *cmtypes.GenesisDoc
		signerPrivKey crypto.PrivKey
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Signing key matches genesis proposer public key",
			args: func() args {
				genesisData, privKey := types.GetGenesisWithPrivkey()
				signingKey, err := types.PrivKeyToSigningKey(privKey)
				require.NoError(err)
				return args{
					genesisData,
					signingKey,
				}
			}(),
			want:    true,
			wantErr: false,
		},
		{
			name: "Signing key does not match genesis proposer public key",
			args: func() args {
				genesisData, _ := types.GetGenesisWithPrivkey()
				randomPrivKey := ed25519.GenPrivKey()
				signingKey, err := types.PrivKeyToSigningKey(randomPrivKey)
				require.NoError(err)
				return args{
					genesisData,
					signingKey,
				}
			}(),
			want:    false,
			wantErr: false,
		},
		{
			name: "No validators found in genesis",
			args: func() args {
				genesisData, privKey := types.GetGenesisWithPrivkey()
				genesisData.Validators = nil
				signingKey, err := types.PrivKeyToSigningKey(privKey)
				require.NoError(err)
				return args{
					genesisData,
					signingKey,
				}
			}(),
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isProposer(tt.args.genesis, tt.args.signerPrivKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("isProposer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isProposer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_publishBlock_ManagerNotProposer(t *testing.T) {
	require := require.New(t)
	m := getManager(t, &mock.MockDA{})
	m.isProposer = false
	err := m.publishBlock(context.Background())
	require.ErrorContains(err, "not a proposer")
}
