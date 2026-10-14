package block

import (
	"context"
	"testing"

	"github.com/celestiaorg/go-square/v3/share"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/config"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
	datypes "github.com/evstack/ev-node/pkg/da/types"
)

type fakeSubmitter struct{ calls int }

func (f *fakeSubmitter) Submit(context.Context, []*blobrpc.Blob, *blobrpc.SubmitOptions) (uint64, error) {
	f.calls++
	return 7, nil
}

func TestNewDAClient_WithSubmitter(t *testing.T) {
	sub := &fakeSubmitter{}
	cl := NewDAClient(&blobrpc.Client{}, config.DefaultConfig(), zerolog.Nop(), WithSubmitter(sub))

	ns := share.MustNewV0Namespace([]byte("ns")).Bytes()
	res := cl.Submit(context.Background(), [][]byte{[]byte("data")}, 0, ns, nil)
	require.Equal(t, datypes.StatusSuccess, res.Code)
	require.Equal(t, uint64(7), res.Height)
	require.Equal(t, 1, sub.calls)
}
