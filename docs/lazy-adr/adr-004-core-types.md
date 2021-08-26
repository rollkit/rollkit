# ADR 004: Blockchain Core Data Types

## Changelog

- 19-04-2021: Initial Draft

## Context

This document describes the core data structures of any Optimint-powered blockchain.

## Alternative Approaches

Alternatives for ChainID:
 - an integer type like unit64
 - a string that fulfills some basic rules like the ChainID for Cosmos chains

## Decision

We design the core data types as minimalistic as possible, i.e. they only contain the absolute necessary 
data for an optimistic rollup to function properly. 
If there are any additional fields that conflict with above's claimed minimalism, then they are necessarily inherited 
by the ABCI imposed separation between application state machine and consensus/networking (often also referred to as ABCI-server and -client).
Where such tradeoffs are made, we explicitly comment on them.

## Detailed Design

### Transactions

In Optimint, like in Tendermint, Transactions are just an opaque slice of bytes:

```go
type Tx []byte
type Txs []Tx
```

If necessary `Tx` could be turned into a struct. Currently, there is no need for that though.

### Block Header

```go
type Header struct {
    // Block and App version
    Version Version 
    // NamespaceID identifies this chain e.g. when connected to other rollups via IBC.
    NamespaceID [8]byte  
    
    Height  uint64               
    Time    uint64 // time in tai64 format           
    
    // prev block info
    LastHeaderHash [32]byte 
    
    // hashes of block data
    LastCommitHash     [32]byte // commit from aggregator(s) from the last block
    DataHash           [32]byte // Block.Data root aka Transactions
    ConsensusHash      [32]byte // consensus params for current block
    AppHash            [32]byte  // state after applying txs from the current block
    
    // root hash of all results from the txs from the previous block
    // This is ABCI specific but smart-contract chains require some way of committing to transaction receipts/results.
    LastResultsHash [32]byte
    
    
    // Note that the address can be derived from the pubkey which can be derived 
    // from the signature when using secp256k.
    // We keep this in case users choose another signature format where the 
    // pubkey can't be recovered by the signature (e.g. ed25519).
    ProposerAddress Address  // original proposer of the block
}

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
// This is equivalent to the tmversion.Consensus type in Tendermint.
type Version struct { 
    Block uint32 
    App   uint32
}
```

### Block and Block.Data

```go
type Block struct {
    Header      Header
    Data        Data
    LastCommit  *Commit
}

type Data struct {
    Txs Txs
    IntermediateStateRoots IntermediateStateRoots
    Evidence EvidenceData
}

type EvidenceData struct {
    Evidence []Evidence
}
```

#### Evidence

`Evidence` represents a go-interface (or oneof in protobuf) of known set of concrete fraud-proofs.
The details for this will be defined in a separated adr/PR.

Here is an incomplete list of potenial evidence types:
- Same Aggregator signed two different blocks at the same height
  - figure out if this is actually malicious / slashable behaviour - e.g. clients could simply accept the last block included in a LL block
- State Transition Fraud Proofs (for previous blocks)


#### Commit

```go
type Commit struct {
    Height     uint64
    HeaderHash [32]byte
    Signatures []Signature // most of the time this is a single signature
}
```

#### ConsensusParams

[ConsensusParams](https://docs.tendermint.com/master/spec/core/state.html#consensusparams) can be updated by the application through ABCI.
This could be seen as a state transition and the ConsensusHash in the header would then require a dedicated state fraud proof. 
That said, none of the existing default Cosmos-SDK modules actually make use of this functionality though.
Hence, we can treat the ConsensusParams as constants (for the same app version). 
We clearly need to communicate this to optimistic rollup chain developers. 
Ideally, we should ensure this programmatically to guarantee that this assumption always holds inside optimint.

The ConsensusParams have the exact same structure as in Tendermint. For the sake of self-containedness we still list them here:


```go
// ConsensusParams contains consensus critical parameters that determine the
// validity of blocks.
type ConsensusParams struct {
    Block     BlockParams     
    Evidence  EvidenceParams  
    Validator ValidatorParams 
    Version   VersionParams   
}

// BlockParams contains limits on the block size.
type BlockParams struct {
    // Max block size, in bytes.
    // Note: must be greater than 0
    MaxBytes int64 
    // Max gas per block.
    // Note: must be greater or equal to -1
    MaxGas int64
    // Minimum time increment between consecutive blocks (in milliseconds) If the
    // block header timestamp is ahead of the system clock, decrease this value.
    //
    // Not exposed to the application.
    TimeIotaMs int64
}

// EvidenceParams determine how we handle evidence of malfeasance.
type EvidenceParams struct {
    // Max age of evidence, in blocks.
    //
    // The basic formula for calculating this is: MaxAgeDuration / {average block
    // time}.
    MaxAgeNumBlocks int64 
    // Max age of evidence, in time.
    //
    // It should correspond with an app's "unbonding period" or other similar
    // mechanism for handling [Nothing-At-Stake
    // attacks](https://github.com/ethereum/wiki/wiki/Proof-of-Stake-FAQ#what-is-the-nothing-at-stake-problem-and-how-can-it-be-fixed).
    MaxAgeDuration time.Duration
    // This sets the maximum size of total evidence in bytes that can be committed in a single block.
    // and should fall comfortably under the max block bytes.
    // Default is 1048576 or 1MB
    MaxBytes int64
}

// ValidatorParams restrict the public key types validators can use.
type ValidatorParams struct {
    PubKeyTypes []string
}

// VersionParams contains the ABCI application version.
type VersionParams struct {
    AppVersion uint64
}
```

## Status

Proposed and partly implemented.

For finishing the implementation these items need to be tackled at least:

- [ ] methods on core types (e.g. for hashing and basic validation etc)
- [ ] equivalent types for serialization purposes (probably protobuf)
- [ ] conversion from and to protobuf


## Consequences

### Positive

- very close to the original Tendermint types which makes on-boarding devs familiar for the Cosmos-SDK and Tendermint easier

### Negative

- dependency on abci types for evidence interface (in the current implementation at least)

### Neutral

## References

- https://github.com/celestiaorg/optimint/pull/41


