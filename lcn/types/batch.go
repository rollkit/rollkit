package types

// Batch groups together multiple settlement operations to be performed at the same time;
// Modeled after Batch in pkg txindex
type Batch struct {
	// TO-DO: Add batch fields
}

// ResultSubmitBatch is the result of the SubmitBatch method;
// Modeled after ResultSubmitBatch in pkg da
type ResultSubmitBatch struct {
	BaseResult
}

// ResultRetrieveBatch is the result of the RetrieveBatch method;
// Modeled after ResultRetrieveBlocks in pkg da
type ResultRetrieveBatch struct {
	BaseResult
	*Batch
}
