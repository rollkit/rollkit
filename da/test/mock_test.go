package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rollkit/rollkit/da/mock"
	testlog "github.com/rollkit/rollkit/log/test"
	"github.com/rollkit/rollkit/store"
	"github.com/stretchr/testify/require"
)

func TestMockDA(t *testing.T) {
	require := require.New(t)

	store, err := store.NewDefaultInMemoryKVStore()
	require.NoError(err)

	dalc := mock.DataAvailabilityLayerClient{}
	err = dalc.Init([8]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte("1s"), store, testlog.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)
	
	blocks := getRandomBlock(0, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	go func () {
		for {
			select {
				<-cancel:

			}
		}
	}()

	time.Sleep(time.Second * 3)
	err = dalc.Stop()
	require.NoError(err)

}
