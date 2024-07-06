package lcn

import "github.com/LastL2/cuberollkit/lcn/types"

// Settler is the interface that LcnClient must implement
type Settler interface {
	Start() error
	Stop() error
	// SubmitBatch submits a batch to the LCN;
	// Modeled after SubmitBlocks in pkg da
	SubmitBatch() types.ResultSubmitBatch
	// RetrieveBatch retrieves a batch from the LCN;
	// Modeled after RetrieveBlocks in pkg da
	RetrieveBatch() types.ResultRetrieveBatch
}
