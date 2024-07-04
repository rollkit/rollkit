package lcn

import (
	"log"
)

// StatusCode is the return status for the LcnClient;
// Modeled after StatusCode in pkg da
type StatusCode uint64

// lcn return codes
const (
	StatusUnknown StatusCode = iota
	StatusSuccess
	StatusTimeout
	StatusError
)

// BaseResult contains basic information returned by the LCN;
// Modeled after BaseResult in pkg da
type BaseResult struct {
	// Code is to determine if the action succeeded
	Code StatusCode
	// Message may contain lcn specific information
	Message string
}

// Batch groups together multiple settlement operations to be performed at the same time;
// Modeled after Batch in pkg txindex
type Batch struct {
	// TO-DO: Add batch fields
}

// ResultRetrieveBlocks contains batch of blocks returned from the LCN client;
// Modeled after ResultRetrieveBlocks in pkg da
type ResultRetrieveBatch struct {
	BaseResult
	*Batch
}

// LcnClient is a new settlement implementation;
// Modeled after DAClient in pkg da
type LcnClient struct {
	Config *Config
	Logger log.Logger
}
