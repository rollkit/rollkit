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
	// Height represents the block height (aka block number) of a given header
	Height uint64
	// Time contains Unix nanotime of a block
	Time uint64
	// The Chain ID
	ChainID string
	// Block and app version
	Version Version
	// prev header hash
	LastHeaderHash Hash
	// Pointer to location of associated block data in the DA layer
	DataCommitment Hash
	// Commitment representing the state linked to the header
	StateCommitment Hash
}
```

## Assumptions and Considerations

- The header format assumes that the ABCI Execution layer can handle the new structure without requiring CometBFT's full header.
- Security considerations include ensuring the integrity and authenticity of the header data, particularly the `DataCommitment` and `StateCommitment` fields.

## Implementation

Pending implementation.

## References
