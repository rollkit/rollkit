package da

import (
	"context"
	"fmt"
)

// DummyClient implements the Client interface using a DA implementation
type DummyClient struct {
	da        DA
	namespace []byte
}

// NewDummyClient creates a new instance of DummyClient with the given DA implementation
func NewDummyClient(da DA, namespace []byte) *DummyClient {
	return &DummyClient{
		da:        da,
		namespace: namespace,
	}
}

// MaxBlobSize returns the maximum blob size for the DA layer
func (c *DummyClient) MaxBlobSize(ctx context.Context) (uint64, error) {
	// Delegate to the underlying DA implementation
	return c.da.MaxBlobSize(ctx)
}

// GasPrice returns the gas price for the DA layer
func (c *DummyClient) GasPrice(ctx context.Context) (float64, error) {
	// Delegate to the underlying DA implementation
	return c.da.GasPrice(ctx)
}

// GasMultiplier returns the gas multiplier for the DA layer
func (c *DummyClient) GasMultiplier(ctx context.Context) (float64, error) {
	// Delegate to the underlying DA implementation
	return c.da.GasMultiplier(ctx)
}

// GetNamespace returns the namespace for the DA layer
func (c *DummyClient) GetNamespace(ctx context.Context) ([]byte, error) {
	// Delegate to the underlying DA implementation
	return c.namespace, nil
}

// SubmitHeaders submits block headers to DA layer
func (c *DummyClient) Submit(ctx context.Context, data [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmit {
	// Convert headers to blobs
	blobs := make([]Blob, len(data))
	copy(blobs, data)

	// Submit blobs to DA layer
	ids, err := c.da.Submit(ctx, blobs, gasPrice, c.namespace, nil)
	if err != nil {
		return ResultSubmit{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to submit data: %v", err),
			},
		}
	}

	// Return success result
	return ResultSubmit{
		BaseResult: BaseResult{
			Code:           StatusSuccess,
			Message:        fmt.Sprintf("successfully submitted %d data", len(data)),
			SubmittedCount: uint64(len(data)),
			// Note: In a real implementation, we would set the DAHeight to the actual height
			// where the blobs were included in the DA layer
			IDs: ids,
		},
	}
}

// RetrieveData retrieves block data from DA layer
func (c *DummyClient) Retrieve(ctx context.Context, dataLayerHeight uint64) ResultRetrieve {
	// Get IDs of blobs at the given height
	result, err := c.da.GetIDs(ctx, dataLayerHeight, c.namespace)
	if err != nil {
		return ResultRetrieve{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to get IDs at height %d: %v", dataLayerHeight, err),
			},
		}
	}

	// If no blobs at the given height, return empty result
	if len(result.IDs) == 0 {
		return ResultRetrieve{
			BaseResult: BaseResult{
				Code:    StatusSuccess,
				Message: fmt.Sprintf("no data found at height %d", dataLayerHeight),
				Height:  dataLayerHeight,
			},
			Data: [][]byte{},
		}
	}

	// Get blobs for the IDs
	blobs, err := c.da.Get(ctx, result.IDs, c.namespace)
	if err != nil {
		return ResultRetrieve{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to get blobs at height %d: %v", dataLayerHeight, err),
			},
		}
	}

	// Convert blobs to data
	data := make([][]byte, len(blobs))
	copy(data, blobs)

	// Return success result
	return ResultRetrieve{
		BaseResult: BaseResult{
			Code:    StatusSuccess,
			Message: fmt.Sprintf("retrieved %d data from height %d", len(data), dataLayerHeight),
			Height:  dataLayerHeight,
		},
		Data: data,
	}
}
