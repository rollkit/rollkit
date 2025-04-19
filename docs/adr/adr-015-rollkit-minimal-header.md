# Rollkit Minimal Header

## Abstract

This document specifies a minimal header format for Rollkit, designed to eliminate the dependency on CometBFT's header format. This new format can then be used to produce an execution layer tailored header if needed. For example, the new ABCI Execution layer can have an ABCI-specific header for IBC compatibility. This allows Rollkit to define its own header structure while maintaining backward compatibility where necessary.

## Protocol/Component Description

The Rollkit minimal header is a streamlined version of the traditional header, focusing on essential information required for block processing and state management for rollup nodes. This header format is designed to be lightweight and efficient, facilitating faster processing and reduced overhead.

### Rollkit Minimal Header Structure

```ascii
┌─────────────────────────────────────────────┐
│             Rollkit Minimal Header          │
├─────────────────────┬───────────────────────┤
│ ParentHash          │ Hash of previous block│
├─────────────────────┼───────────────────────┤
│ Height              │ Block number          │
├─────────────────────┼───────────────────────┤
│ Timestamp           │ Creation time         │
├─────────────────────┼───────────────────────┤
│ ChainID             │ Chain identifier      │
├─────────────────────┼───────────────────────┤
│ DataCommitment      │ Pointer to block data │
│                     │ on DA layer           │
├─────────────────────┼───────────────────────┤
│ StateRoot           │ State commitment      │
├─────────────────────┼───────────────────────┤
│ ExtraData           │ Additional metadata   │
│                     │ (e.g. sequencer info) │
└─────────────────────┴───────────────────────┘
```

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
    DataCommitment []byte
    // Commitment representing the state linked to the header
    StateRoot Hash
    // Arbitrary field for additional metadata
    ExtraData []byte
}
```

In case the rollup has a specific designated proposer or a proposer set, that information can be put in the `extraData` field. So in centralized sequencer mode, the `sequencerAddress` can live in `extraData`. For base sequencer mode, this information is not relevant.

This minimal Rollkit header can be transformed to be tailored to a specific execution layer as well by inserting additional information typically needed.

### EVM execution client

- `transactionsRoot`: Merkle root of all transactions in the block. Can be constructed from unpacking the `DataCommitment` in Rollkit Header.
- `receiptsRoot`: Merkle root of all transaction receipts, which store the results of transaction execution. This can be inserted by the EVM execution client.
- `Gas Limit`: Max gas allowed in the block.
- `Gas Used`: Total gas consumed in this block.

#### Transformation to EVM Header

```ascii
┌─────────────────────────────────────────────┐
│             Rollkit Minimal Header          │
└───────────────────┬─────────────────────────┘
                    │
                    ▼ Transform
┌─────────────────────────────────────────────┐
│               EVM Header                    │
├─────────────────────┬───────────────────────┤
│ ParentHash          │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ Height/Number       │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ Timestamp           │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ ChainID             │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ TransactionsRoot    │ Derived from          │
│                     │ DataCommitment        │
├─────────────────────┼───────────────────────┤
│ StateRoot           │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ ReceiptsRoot        │ Added by EVM client   │
├─────────────────────┼───────────────────────┤
│ GasLimit            │ Added by EVM client   │
├─────────────────────┼───────────────────────┤
│ GasUsed             │ Added by EVM client   │
├─────────────────────┼───────────────────────┤
│ ExtraData           │ From Rollkit Header   │
└─────────────────────┴───────────────────────┘
```

### ABCI Execution

This header can be transformed into an ABCI-specific header for IBC compatibility.

- `Version`: Required by IBC clients to correctly interpret the block's structure and contents.
- `LastCommitHash`: The hash of the previous block's commit, used by IBC clients to verify the legitimacy of the block's state transitions.
- `DataHash`: A hash of the block's transaction data, enabling IBC clients to verify that the data has not been tampered with. Can be constructed from unpacking the `DataCommitment` in Rollkit header.
- `ValidatorHash`: Current validator set's hash, which IBC clients use to verify that the block was validated by the correct set of validators. This can be the IBC attester set of the chain for backward compatibility with the IBC Tendermint client, if needed.
- `NextValidatorsHash`: The hash of the next validator set, allowing IBC clients to anticipate and verify upcoming validators.
- `ConsensusHash`: Denotes the hash of the consensus parameters, ensuring that IBC clients are aligned with the consensus rules of the blockchain.
- `AppHash`: Same as the `StateRoot` in the Rollkit Header.
- `EvidenceHash`: A hash of evidence of any misbehavior by validators, which IBC clients use to assess the trustworthiness of the validator set.
- `LastResultsHash`: Root hash of all results from the transactions from the previous block.
- `ProposerAddress`: The address of the block proposer, allowing IBC clients to track and verify the entities proposing new blocks. Can be constructed from the `extraData` field in the Rollkit Header.

#### Transformation to ABCI Header

```ascii
┌─────────────────────────────────────────────┐
│             Rollkit Minimal Header          │
└───────────────────┬─────────────────────────┘
                    │
                    ▼ Transform
┌─────────────────────────────────────────────┐
│               ABCI Header                   │
├─────────────────────┬───────────────────────┤
│ Height              │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ Time                │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ ChainID             │ From Rollkit Header   │
├─────────────────────┼───────────────────────┤
│ AppHash             │ From StateRoot        │
├─────────────────────┼───────────────────────┤
│ DataHash            │ From DataCommitment   │
├─────────────────────┼───────────────────────┤
│ Version             │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ LastCommitHash      │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ ValidatorHash       │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ NextValidatorsHash  │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ ConsensusHash       │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ EvidenceHash        │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ LastResultsHash     │ Added for IBC         │
├─────────────────────┼───────────────────────┤
│ ProposerAddress     │ From ExtraData        │
└─────────────────────┴───────────────────────┘
```

## Assumptions and Considerations

- The Rollkit minimal header is designed to be flexible and adaptable, allowing for integration with various execution layers such as EVM and ABCI, without being constrained by CometBFT's header format.
- The `extraData` field provides a mechanism for including additional metadata, such as sequencer information, which can be crucial for certain rollup configurations.
- The transformation of the Rollkit header into execution layer-specific headers should be done carefully to ensure compatibility and correctness, especially for IBC and any other cross-chain communication protocols.

### Header Transformation Flow

```ascii
┌─────────────────────────────────────────────┐
│             Rollkit Minimal Header          │
│                                             │
│  A lightweight, flexible header format      │
│  with essential fields for block processing │
└───────────┬─────────────────┬───────────────┘
            │                 │
            ▼                 ▼
┌───────────────────┐ ┌─────────────────────┐
│  EVM Header       │ │  ABCI Header        │
│                   │ │                     │
│  For EVM-based    │ │  For IBC-compatible │
│  execution layers │ │  execution layers   │
└───────────────────┘ └─────────────────────┘
```

## Implementation

Pending implementation.

## References

- [Ethereum Developer Documentation](https://ethereum.org/en/developers/docs/): Comprehensive resources for understanding Ethereum's architecture, including block and transaction structures.
- [Tendermint Core Documentation](https://docs.tendermint.com/master/spec/): Detailed documentation on Tendermint, which includes information on ABCI and its header format.
- [ABCI Specification](https://github.com/tendermint/spec/blob/master/spec/abci/abci.md): The official specification for the Application Blockchain Interface (ABCI), which describes how applications can interact with the Tendermint consensus engine.
- [IBC Protocol Specification](https://github.com/cosmos/ibc): Documentation on the Inter-Blockchain Communication (IBC) protocol, which includes details on how headers are used for cross-chain communication.
