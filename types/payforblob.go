package types

import (
	"github.com/celestiaorg/go-square/v2/share"
)

const (
	CelestiaVersion   uint64 = 4
	TxSizeCostPerByte uint64 = 10
	GasPerBlobByte    uint32 = 8

	// PFBGasFixedCost is a rough estimate for the "fixed cost" in the gas cost
	// formula: gas cost = gas per byte * bytes per share * shares occupied by
	// blob + "fixed cost". In this context, "fixed cost" accounts for the gas
	// consumed by operations outside the blob's GasToConsume function (i.e.
	// signature verification, tx size, read access to accounts).
	//
	// Since the gas cost of these operations is not easy to calculate, linear
	// regression was performed on a set of observed data points to derive an
	// approximate formula for gas cost. Assuming gas per byte = 8 and bytes per
	// share = 512, we can solve for "fixed cost" and arrive at 65,000. gas cost
	// = 8 * 512 * number of shares occupied by the blob + 65,000 has a
	// correlation coefficient of 0.996. To be conservative, we round up "fixed
	// cost" to 75,000 because the first tx always takes up 10,000 more gas than
	// subsequent txs.
	PFBGasFixedCost = 75000

	// BytesPerBlobInfo is a rough estimation for the amount of extra bytes in
	// information a blob adds to the size of the underlying transaction.
	BytesPerBlobInfo = 70

	// DefaultMinGasPrice is the default minimum gas price for transactions.
	// todo: estimate gas price via client
	DefaultMinGasPrice = 0.002
)

// GasToConsume works out the extra gas charged to pay for a set of blobs in a PFB.
// Note that transactions will incur other gas costs, such as the signature verification
// and reads to the user's account.
func GasToConsume(blobSizes []uint32, gasPerByte uint32) uint64 {
	var totalSharesUsed uint64
	for _, size := range blobSizes {
		totalSharesUsed += uint64(share.SparseSharesNeeded(size))
	}

	return totalSharesUsed * share.ShareSize * uint64(gasPerByte)
}

// EstimateGas estimates the total gas required to pay for a set of blobs in a PFB.
// It is based on a linear model that is dependent on the governance parameters:
// gasPerByte and txSizeCost. It assumes other variables are constant. This includes
// assuming the PFB is the only message in the transaction.
func EstimateGas(blobs []uint32, gasPerByte uint32, txSizeCost uint64) uint64 {
	return GasToConsume(blobs, gasPerByte) + (txSizeCost * BytesPerBlobInfo * uint64(len(blobs))) + PFBGasFixedCost
}

// DefaultEstimateGas runs EstimateGas with the system defaults.
func DefaultEstimateGas(blobs []uint32) uint64 {
	return EstimateGas(blobs, GasPerBlobByte, TxSizeCostPerByte)
}
