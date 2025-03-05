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

// SubmitHeaders submits block headers to DA layer
func (c *DummyClient) SubmitHeaders(ctx context.Context, headers [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmit {
	// Convert headers to blobs
	blobs := make([]Blob, len(headers))
	copy(blobs, headers)

	// Submit blobs to DA layer
	_, height, err := c.da.Submit(ctx, blobs, gasPrice, c.namespace, nil)
	if err != nil {
		return ResultSubmit{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to submit headers: %v", err),
			},
		}
	}

	// Return success result
	return ResultSubmit{
		BaseResult: BaseResult{
			Code:           StatusSuccess,
			Message:        fmt.Sprintf("successfully submitted %d headers", len(headers)),
			SubmittedCount: uint64(len(headers)),
			// Note: In a real implementation, we would set the DAHeight to the actual height
			// where the blobs were included in the DA layer
			DAHeight: height,
		},
	}
}

// RetrieveHeaders retrieves block headers from DA layer
func (c *DummyClient) RetrieveHeaders(ctx context.Context, dataLayerHeight uint64) ResultRetrieveHeaders {
	// Get IDs of blobs at the given height
	result, err := c.da.GetIDs(ctx, dataLayerHeight, c.namespace)
	if err != nil {
		return ResultRetrieveHeaders{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to get IDs at height %d: %v", dataLayerHeight, err),
			},
		}
	}

	// If no blobs at the given height, return empty result
	if len(result.IDs) == 0 {
		return ResultRetrieveHeaders{
			BaseResult: BaseResult{
				Code:     StatusSuccess,
				Message:  fmt.Sprintf("no headers found at height %d", dataLayerHeight),
				DAHeight: dataLayerHeight,
			},
			Headers: [][]byte{},
		}
	}

	// Get blobs for the IDs
	blobs, err := c.da.Get(ctx, result.IDs, c.namespace)
	if err != nil {
		return ResultRetrieveHeaders{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to get blobs at height %d: %v", dataLayerHeight, err),
			},
		}
	}

	// Convert blobs to headers
	headers := make([][]byte, len(blobs))
	for i, blob := range blobs {
		headers[i] = blob
	}

	// Return success result
	return ResultRetrieveHeaders{
		BaseResult: BaseResult{
			Code:     StatusSuccess,
			Message:  fmt.Sprintf("retrieved %d headers from height %d", len(headers), dataLayerHeight),
			DAHeight: dataLayerHeight,
		},
		Headers: headers,
	}
}
