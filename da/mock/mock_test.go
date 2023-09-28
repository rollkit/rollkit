package mock

import (
	"testing"

	testlog "github.com/rollkit/rollkit/log/test"
	"github.com/rollkit/rollkit/store"
	"github.com/stretchr/testify/require"
)

func TestMockDA(t *testing.T) {
	require := require.New(t)

	store, err := store.NewDefaultInMemoryKVStore()
	require.NoError(err)

	dalc := DataAvailabilityLayerClient{}
	err = dalc.Init([8]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte("1s"), store, testlog.NewLogger(t))
	require.NoError(err)
}
