# DA

## Abstract

The `Data Availability` package is a component responsible for submitting and retrieving data from the DA layer. The [Data Availability Interface][data availability interface] describes the methods required to implement a DA layer. By default a [celestia implementation] of the DA interface is provided, but support can be added to any layer.

## Detailed Description

`pkg da` defines the generic DA interface.

The interface definition is as follows:

```go
// DA defines very generic interface for interaction with Data Availability layers.
type DA interface {
	// Get returns Blob for each given ID, or an error.
	//
	// Error should be returned if ID is not formatted properly, there is no Blob for given ID or any other client-level
	// error occurred (dropped connection, timeout, etc).
	Get(ids []ID) ([]Blob, error)

	// GetIDs returns IDs of all Blobs located in DA at given height.
	GetIDs(height uint64) ([]ID, error)

	// Commit creates a Commitment for each given Blob.
	Commit(blobs []Blob) ([]Commitment, error)

	// Submit submits the Blobs to Data Availability layer.
	//
	// This method is synchronous. Upon successful submission to Data Availability layer, it returns ID identifying blob
	// in DA and Proof of inclusion.
	Submit(blobs []Blob) ([]ID, []Proof, error)

	// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
	Validate(ids []ID, proofs []Proof) ([]bool, error)
}
```
## Message Structure/Communication Format

The package provides protobuf definitions for the message formats used to transport the messages across the grpc client and server.

`service DAService` defines the grpc service which provides a grpc service that implements the above DA interface.

## Assumptions and Considerations

Upon successful submission to the Data Availability layer, it returns an ID identifying the blob in the DA and a proof of inclusion.

When trying to submit a block to the Celestia, the following failure cases are possible:

* submitting the Celestia transaction (rollup Block) into the mempool, but is not accepted for some reason e.g. mempool is full
* submitting the Celestia transaction (rollup Block) into the mempool, but there is a nonce clash with a different transaction but the same nonce but different transaction. Example PFB submission plus TIA transfer using the same nonce. The one from rollkit will fail and never be included
* submitting the Celestia transaction (rollup Block) but the gas was misconfigured too low so it will be stuck in the mempool
* we call submit but get no response (node crashed for example)
* we submit a tx but it is impossible to fit as it is too big (too many bytes) so it fails

In case of a failure, the request will be retried with an exponentially increasing back-off starting with 100ms and capped at the DA block time, until the request is successful.

## Implementation

See [data availability interface]

## References

[1] [Data Availability Interface][data availability interface]
[2] [Celestia Implementation][celestia implementation]

[data availability interface]: https://github.com/rollkit/go-da
[celestia implementation]: https://github.com/rollkit/celestia-da
