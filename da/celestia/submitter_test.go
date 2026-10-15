package celestia

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/celestiaorg/celestia-app/v10/app"
	"github.com/celestiaorg/celestia-app/v10/app/encoding"
	"github.com/celestiaorg/celestia-app/v10/app/grpc/gasestimation"
	"github.com/celestiaorg/celestia-app/v10/pkg/user"
	sharev3 "github.com/celestiaorg/go-square/v3/share"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
)

var testSigner = sdk.AccAddress(make([]byte, 20)).String()

func testBlobs(t *testing.T) ([]*blobrpc.Blob, sharev3.Namespace) {
	ns, err := sharev3.NewV0Namespace([]byte("ev-data"))
	require.NoError(t, err)
	b, err := blobrpc.NewBlobV0(ns, []byte("hello"))
	require.NoError(t, err)
	return []*blobrpc.Blob{b}, ns
}

func TestToV4(t *testing.T) {
	blobs, ns := testBlobs(t)
	v4, err := toV4(blobs)
	require.NoError(t, err)
	require.Len(t, v4, 1)
	require.Equal(t, ns.Bytes(), v4[0].Namespace().Bytes())
	require.Equal(t, []byte("hello"), v4[0].Data())
}

func TestTxOptions(t *testing.T) {
	blobs, _ := testBlobs(t)
	v4, err := toV4(blobs)
	require.NoError(t, err)
	txCfg := encoding.MakeConfig(app.ModuleEncodingRegisters...).TxConfig

	apply := func(opts []user.TxOption) (gas uint64, fee string) {
		b := txCfg.NewTxBuilder()
		for _, o := range opts {
			b = o(b)
		}
		return b.GetTx().GetGas(), b.GetTx().GetFee().String()
	}
	estimate := func(p float64) priceEstimator {
		return func(context.Context, gasestimation.TxPriority) (float64, error) { return p, nil }
	}
	failing := func(context.Context, gasestimation.TxPriority) (float64, error) {
		return 0, errors.New("boom")
	}

	t.Run("nothing set delegates to the tx client", func(t *testing.T) {
		opts, err := txOptions(t.Context(), testSigner, v4, &blobrpc.SubmitOptions{}, failing)
		require.NoError(t, err)
		require.Empty(t, opts)
	})

	t.Run("explicit gas and price", func(t *testing.T) {
		opts, err := txOptions(t.Context(), testSigner, v4, &blobrpc.SubmitOptions{Gas: 100_000, GasPrice: 0.5, IsGasPriceSet: true}, failing)
		require.NoError(t, err)
		gas, fee := apply(opts)
		require.Equal(t, uint64(100_000), gas)
		require.Equal(t, "50000utia", fee)
	})

	t.Run("estimated price is capped and gas is derived from the blobs", func(t *testing.T) {
		opts, err := txOptions(t.Context(), testSigner, v4, &blobrpc.SubmitOptions{TxPriority: blobrpc.TxPriorityHigh, MaxGasPrice: 0.01}, estimate(5))
		require.NoError(t, err)
		gas, fee := apply(opts)
		require.NotZero(t, gas)
		require.Equal(t, fmt.Sprintf("%dutia", int(math.Ceil(0.01*float64(gas)))), fee)
	})

	t.Run("estimator error surfaces", func(t *testing.T) {
		_, err := txOptions(t.Context(), testSigner, v4, &blobrpc.SubmitOptions{TxPriority: blobrpc.TxPriorityLow}, failing)
		require.ErrorContains(t, err, "estimate gas price")
	})
}
