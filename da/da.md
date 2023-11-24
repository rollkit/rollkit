# DA

## Abstract

The `Data Availability Layer Client (DALC)` defines the interaction between the rollup and the Data Availability layer. It provides an interface for submitting and retrieving blocks, ensuring consistency and reliability in data storage and retrieval. By default a [celestia implementation] of the [DALC Interface][data availability layer client interface] is provided.

## Detailed Description

The DALC interface is defined as:

```protobuf
service DALCService {
	rpc SubmitBlocks(SubmitBlocksRequest) returns (SubmitBlocksResponse) {}
	rpc RetrieveBlocks(RetrieveBlocksRequest) returns (RetrieveBlocksResponse) {}
}
```

The `SubmitBlocks` method submits rollup blocks to the Data Availability layer. The response is defined as:

```protobuf
message DAResponse {
	StatusCode code = 1;
	string message = 2;
	uint64 da_height = 3 [(gogoproto.customname) = "DAHeight"];
}
```

Where `StatusCode` is defined as:

```protobuf
enum StatusCode {
	STATUS_CODE_UNSPECIFIED = 0;
	STATUS_CODE_SUCCESS = 1;
	STATUS_CODE_TIMEOUT = 2;
	STATUS_CODE_ERROR   = 3;
}
```

`DAResponse` `code` contains the status code:

* `STATUS_CODE_SUCCESS` is returned in case of a successful submission.
* `STATUS_CODE_TIMEOUT` is returned in case of a submission timeout.
* `STATUS_CODE_ERROR` is returned in case of a submission failure.

`DAResponse` `message` field may contain any additional data layer information for e.g. block hash in case of success or a detailed error message in case of failure.

`DAResponse` `da_height` field contains the block height of inclusion of the Data Availability layer transaction.

## Message Structure/Communication Format

The gRPC service uses protobufs for communication between the rollup and the Data Availability layer.

Rollup blocks are serialised into binary as protobufs as per the encoding/decoding scheme defined in [block serialization].

## Assumptions and Considerations

When trying to submit a block to the Celestia, the following failure cases are possible:

* submitting the Celestia transaction (rollup Block) into the mempool, but is not accepted for some reason e.g. mempool is full
* submitting the Celestia transaction (rollup Block) into the mempool, but there is a nonce clash with a different transaction but the same nonce but different transaction. Example PFB submission plus TIA transfer using the same nonce. The one from rollkit will fail and never be included
* submitting the Celestia transaction (rollup Block) but the gas was misconfigured too low so it will be stuck in the mempool
* we call submit but get no response (node crashed for example)
* we submit a tx but it is impossible to fit as it is too big (too many bytes) so it fails

In case of an error, the request will be retried with an exponentially increasing back-off starting with 100ms and capped at the DA block time, until the request is successful.

## Implementation

See [data availability layer client interface]

## References

[1] [DALC Interface][data availability layer client interface]

[2] [Celestia Implementation][celestia implementation]

[3] [Block Serialization] [block serialization]

[data availability layer client interface]: https://github.com/rollkit/rollkit/blob/v0.11.4/proto/dalc/dalc.proto
[celestia implementation]: https://github.com/rollkit/rollkit/tree/v0.11.4/da/celestia
[block serialization]: https://github.com/rollkit/rollkit/tree/v0.11.4/types/serialization.go
