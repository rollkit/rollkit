package types

// StatusCode is the return status for the LcnClient;
// Modeled after StatusCode in pkg da
type StatusCode uint64

// BaseResult contains basic information returned by the LCN;
// Modeled after BaseResult in pkg da
type BaseResult struct {
	// Code is to determine if the action succeeded
	Code StatusCode
	// Message may contain lcn specific information
	Message string
}
