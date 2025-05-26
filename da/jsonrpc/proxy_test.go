package jsonrpc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/rollkit/rollkit/da/internal/mocks"
	proxy "github.com/rollkit/rollkit/da/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
)

const (
	// ServerHost is the listen host for the test JSONRPC server
	ServerHost = "localhost"
	// ServerPort is the listen port for the test JSONRPC server
	ServerPort = "3450"
	// ClientURL is the url to dial for the test JSONRPC client
	ClientURL = "http://localhost:3450"

	testMaxBlobSize = 100
)

var testNamespace = []byte("test")
var emptyOptions = []byte{}

// TestProxy runs the go-da DA test suite against the JSONRPC service
// NOTE: This test requires a test JSONRPC service to run on the port
// 3450 which is chosen to be sufficiently distinct from the default port

func getTestDABlockTime() time.Duration {
	return 100 * time.Millisecond
}

func TestProxy(t *testing.T) {
	dummy := coreda.NewDummyDA(100_000, 0, 0, getTestDABlockTime())
	dummy.StartHeightTicker()
	logger := log.NewTestLogger(t)
	server := proxy.NewServer(logger, ServerHost, ServerPort, dummy)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		if err := server.Stop(context.Background()); err != nil {
			require.NoError(t, err)
		}
	}()

	client, err := proxy.NewClient(context.Background(), logger, ClientURL, "", "74657374")
	require.NoError(t, err)

	t.Run("Basic DA test", func(t *testing.T) {
		BasicDATest(t, &client.DA)
	})
	t.Run("Get IDs and all data", func(t *testing.T) {
		GetIDsTest(t, &client.DA)
	})
	t.Run("Check Errors", func(t *testing.T) {
		CheckErrors(t, &client.DA)
	})
	t.Run("Concurrent read/write test", func(t *testing.T) {
		ConcurrentReadWriteTest(t, &client.DA)
	})
	t.Run("Given height is from the future", func(t *testing.T) {
		HeightFromFutureTest(t, &client.DA)
	})
	dummy.StopHeightTicker()
}

// BasicDATest tests round trip of messages to DA and back.
func BasicDATest(t *testing.T, d coreda.DA) {
	msg1 := []byte("message 1")
	msg2 := []byte("message 2")

	ctx := t.Context()
	id1, err := d.Submit(ctx, []coreda.Blob{msg1}, 0, testNamespace)
	assert.NoError(t, err)
	assert.NotEmpty(t, id1)

	id2, err := d.Submit(ctx, []coreda.Blob{msg2}, 0, testNamespace)
	assert.NoError(t, err)
	assert.NotEmpty(t, id2)

	time.Sleep(getTestDABlockTime())

	id3, err := d.SubmitWithOptions(ctx, []coreda.Blob{msg1}, 0, testNamespace, []byte("random options"))
	assert.NoError(t, err)
	assert.NotEmpty(t, id3)

	assert.NotEqual(t, id1, id2)
	assert.NotEqual(t, id1, id3)

	ret, err := d.Get(ctx, id1, testNamespace)
	assert.NoError(t, err)
	assert.Equal(t, []coreda.Blob{msg1}, ret)

	commitment1, err := d.Commit(ctx, []coreda.Blob{msg1}, []byte{})
	assert.NoError(t, err)
	assert.NotEmpty(t, commitment1)

	commitment2, err := d.Commit(ctx, []coreda.Blob{msg2}, []byte{})
	assert.NoError(t, err)
	assert.NotEmpty(t, commitment2)

	ids := []coreda.ID{id1[0], id2[0], id3[0]}
	proofs, err := d.GetProofs(ctx, ids, testNamespace)
	assert.NoError(t, err)
	assert.NotEmpty(t, proofs)
	oks, err := d.Validate(ctx, ids, proofs, testNamespace)
	assert.NoError(t, err)
	assert.NotEmpty(t, oks)
	for _, ok := range oks {
		assert.True(t, ok)
	}
}

// CheckErrors ensures that errors are handled properly by DA.
func CheckErrors(t *testing.T, d coreda.DA) {
	ctx := t.Context()
	blob, err := d.Get(ctx, []coreda.ID{[]byte("invalid blob id")}, testNamespace)
	assert.Error(t, err)
	assert.ErrorContains(t, err, coreda.ErrBlobNotFound.Error())
	assert.Empty(t, blob)
}

// GetIDsTest tests iteration over DA
func GetIDsTest(t *testing.T, d coreda.DA) {
	msgs := []coreda.Blob{[]byte("msg1"), []byte("msg2"), []byte("msg3")}

	ctx := t.Context()
	ids, err := d.Submit(ctx, msgs, 0, testNamespace)
	time.Sleep(getTestDABlockTime())
	assert.NoError(t, err)
	assert.Len(t, ids, len(msgs))
	found := false
	end := time.Now().Add(1 * time.Second)

	// To Keep It Simple: we assume working with DA used exclusively for this test (mock, devnet, etc)
	// As we're the only user, we don't need to handle external data (that could be submitted in real world).
	// There is no notion of height, so we need to scan the DA to get test data back.
	for i := uint64(1); !found && !time.Now().After(end); i++ {
		ret, err := d.GetIDs(ctx, i, []byte{})
		if err != nil {
			if strings.Contains(err.Error(), coreda.ErrHeightFromFuture.Error()) {
				break
			}
			t.Error("failed to get IDs:", err)
		}
		assert.NotNil(t, ret)
		assert.NotZero(t, ret.Timestamp)
		if len(ret.IDs) > 0 {
			blobs, err := d.Get(ctx, ret.IDs, testNamespace)
			assert.NoError(t, err)

			// Submit ensures atomicity of batch, so it makes sense to compare actual blobs (bodies) only when lengths
			// of slices is the same.
			if len(blobs) >= len(msgs) {
				found = true
				for _, msg := range msgs {
					msgFound := false
					for _, blob := range blobs {
						if bytes.Equal(blob, msg) {
							msgFound = true
							break
						}
					}
					if !msgFound {
						found = false
						break
					}
				}
			}
		}
	}

	assert.True(t, found)
}

// ConcurrentReadWriteTest tests the use of mutex lock in DummyDA by calling separate methods that use `d.data` and making sure there's no race conditions
func ConcurrentReadWriteTest(t *testing.T, d coreda.DA) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	writeDone := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := uint64(1); i <= 50; i++ {
			_, err := d.Submit(ctx, []coreda.Blob{[]byte(fmt.Sprintf("test-%d", i))}, 0, []byte("test"))
			assert.NoError(t, err)
		}
		close(writeDone)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-writeDone:
				return
			default:
				d.GetIDs(ctx, 0, []byte("test"))
			}
		}
	}()

	wg.Wait()
}

// HeightFromFutureTest tests the case when the given height is from the future
func HeightFromFutureTest(t *testing.T, d coreda.DA) {
	ctx := t.Context()
	_, err := d.GetIDs(ctx, 999999999, []byte("test"))
	assert.Error(t, err)
	// Specifically check if the error contains the error message ErrHeightFromFuture
	assert.ErrorContains(t, err, coreda.ErrHeightFromFuture.Error())
}

// TestSubmitWithOptions tests the SubmitWithOptions method with various scenarios
func TestSubmitWithOptions(t *testing.T) {
	ctx := context.Background()
	testNamespace := []byte("options_test")
	testOptions := []byte("test_options")
	gasPrice := 0.0

	// Helper function to create a client with a mocked internal API
	createMockedClient := func(internalAPI *mocks.DA) *proxy.Client {
		client := &proxy.Client{}
		client.DA.Internal.SubmitWithOptions = internalAPI.SubmitWithOptions
		client.DA.Namespace = testNamespace
		client.DA.MaxBlobSize = testMaxBlobSize
		client.DA.Logger = log.NewTestLogger(t)
		return client
	}

	t.Run("Happy Path - All blobs fit", func(t *testing.T) {
		mockAPI := mocks.NewDA(t)
		client := createMockedClient(mockAPI)

		blobs := []coreda.Blob{[]byte("blob1"), []byte("blob2")}
		expectedIDs := []coreda.ID{[]byte("id1"), []byte("id2")}

		mockAPI.On("SubmitWithOptions", ctx, blobs, gasPrice, testNamespace, testOptions).Return(expectedIDs, nil).Once()

		ids, err := client.DA.SubmitWithOptions(ctx, blobs, gasPrice, testNamespace, testOptions)

		require.NoError(t, err)
		assert.Equal(t, expectedIDs, ids)
		mockAPI.AssertExpectations(t)
	})

	t.Run("Single Blob Too Large", func(t *testing.T) {
		mockAPI := mocks.NewDA(t)
		client := createMockedClient(mockAPI)

		largerBlob := make([]byte, testMaxBlobSize+1)
		blobs := []coreda.Blob{largerBlob, []byte("this blob is definitely too large")}

		_, err := client.DA.SubmitWithOptions(ctx, blobs, gasPrice, testNamespace, testOptions)

		require.Error(t, err)
		mockAPI.AssertExpectations(t)
	})

	t.Run("Total Size Exceeded", func(t *testing.T) {
		mockAPI := mocks.NewDA(t)
		client := createMockedClient(mockAPI)

		blobsizes := make([]byte, testMaxBlobSize/3)
		blobsizesOver := make([]byte, testMaxBlobSize)

		blobs := []coreda.Blob{blobsizes, blobsizes, blobsizesOver}

		expectedSubmitBlobs := []coreda.Blob{blobs[0], blobs[1]}
		expectedIDs := []coreda.ID{[]byte("idA"), []byte("idB")}
		mockAPI.On("SubmitWithOptions", ctx, expectedSubmitBlobs, gasPrice, testNamespace, testOptions).Return(expectedIDs, nil).Once()

		ids, err := client.DA.SubmitWithOptions(ctx, blobs, gasPrice, testNamespace, testOptions)

		require.NoError(t, err)
		assert.Equal(t, expectedIDs, ids)
		mockAPI.AssertExpectations(t)
	})

	t.Run("First Blob Too Large", func(t *testing.T) {
		mockAPI := mocks.NewDA(t)
		client := createMockedClient(mockAPI)

		largerBlob := make([]byte, testMaxBlobSize+1)
		blobs := []coreda.Blob{largerBlob, []byte("small")}

		ids, err := client.DA.SubmitWithOptions(ctx, blobs, gasPrice, testNamespace, testOptions)

		require.Error(t, err)
		assert.ErrorIs(t, err, coreda.ErrBlobSizeOverLimit)
		assert.Nil(t, ids)

		mockAPI.AssertNotCalled(t, "SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockAPI.AssertExpectations(t)
	})

	t.Run("Empty Input Blobs", func(t *testing.T) {
		mockAPI := mocks.NewDA(t)
		client := createMockedClient(mockAPI)

		var blobs []coreda.Blob

		ids, err := client.DA.SubmitWithOptions(ctx, blobs, gasPrice, testNamespace, testOptions)

		require.NoError(t, err)
		assert.Empty(t, ids)

		mockAPI.AssertNotCalled(t, "SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mockAPI.AssertExpectations(t)
	})

	t.Run("Error During SubmitWithOptions RPC", func(t *testing.T) {
		mockAPI := mocks.NewDA(t)
		client := createMockedClient(mockAPI)

		blobs := []coreda.Blob{[]byte("blob1")}
		expectedError := errors.New("rpc submit failed")

		mockAPI.On("SubmitWithOptions", ctx, blobs, gasPrice, testNamespace, testOptions).Return(nil, expectedError).Once()

		ids, err := client.DA.SubmitWithOptions(ctx, blobs, gasPrice, testNamespace, testOptions)

		require.Error(t, err)
		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, ids)
		mockAPI.AssertExpectations(t)
	})
}
