# Rollkit Minimal Header

## Abstract

This document specifies a minimal header format for Rollkit, designed to eliminate the dependency on CometBFT's header format. This new format can be then be used to produce an execution layer tailored header if needed, for e.g. the new ABCI Execution layer can have an ABCI-specific header for IBC compatibility. This allows Rollkit to define its own header structure while maintaining backward compatibility where necessary.

## Protocol/Component Description

The Rollkit minimal header is a streamlined version of the traditional header, focusing on essential information required for block processing and state management for rollup nodes. This header format is designed to be lightweight and efficient, facilitating faster processing and reduced overhead.

## Message Structure/Communication Format

The header is defined in GoLang as follows:

```go
// Header struct focusing on header information
type Header struct {
	// Hash of the previous rollup block header.
	ParentHash Hash
    // Height represents the block height (aka block number) of a given header
	Height uint64
	// Block creation timestamp
	Timestamp uint64
	// The Chain ID
	ChainID string
	// Pointer to location of associated block data aka transactions in the DA layer
	DataCommitment Hash
	// Commitment representing the state linked to the header
	StateRoot Hash
    // Arbitrary field for additional metadata
    extraData []bytes
}
```

In case, the rollup has a specific designer proposer or a proposer set, that information can be put in the `extraData` field. So in centralized sequencer mode, the `sequencerAddress` can live in `extraData`. For based sequencer mode, this information is not relevant.

This minimal Rollkit header can be transformed to be tailored to a specific execution layer as well. For example, a smart contact execution layer typically has:

- `transactionsRoot`: Merkle root of all transactions in the block. Can be constructed from unpacking the `DataCommitment` in Rollkit Header
- `receiptsRoot`: Merkle root of all transaction receipts, which store the results of transaction execution.

This can be filled in by the execution layer and inserted on top of this minimal block header.

In order to transform this header into an ABCI-specific header for IBC compatibility, the ABCI execution layer can insert the following information into the header:

- `Version`: Required by IBC clients to correctly interpret the block's structure and contents.
- `LastCommitHash`: The hash of the previous block's commit, used by IBC clients to verify the legitimacy of the block's state transitions.
- `DataHash`: A hash of the block's transaction data, enabling IBC clients to verify that the data has not been tampered with. Can be constructed from unpacking the `DataCommitment` in rollkit header
- `ValidatorHash`: Current validator set's hash, which IBC clients use to verify that the block was validated by the correct set of validators. This can be the IBC attester set of the chain for backwards compatibility with the IBC tendermint client, if needed.
- `NextValidatorsHash`: The hash of the next validator set, allowing IBC clients to anticipate and verify upcoming validators.
- `ConsensusHash`: Denotes the hash of the consensus parameters, ensuring that IBC clients are aligned with the consensus rules of the blockchain.
- `AppHash`: Same as the `StateRoot` in the Rollkit Header.
- `EvidenceHash`: A hash of evidence of any misbehavior by validators, which IBC clients use to assess the trustworthiness of the validator set.
- `LastResultsHash`: Root hash of all results from the txs from the previous block.
- `ProposerAddress`: The address of the block proposer, allowing IBC clients to track and verify the entities proposing new blocks. Can be constructed from the `extraData` field in the Rollkit Header.

## Assumptions and Considerations

- The header format assumes that the ABCI Execution layer can handle the new structure without requiring CometBFT's full header.
- Security considerations include ensuring the integrity and authenticity of the header data, particularly the `DataCommitment` and `StateRoot` fields.

## Implementation

Pending implementation.

## References
