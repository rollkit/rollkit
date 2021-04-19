# ADR 004: Blockchain Core Data Types

## Changelog

- {date}: {changelog}

## Context

> This section contains all the context one needs to understand the current state, and why there is a problem. It should be as succinct as possible and introduce the high level idea behind the solution.

This document describes the core data structures of any Optimint-powered blockchain.

## Alternative Approaches

> This section contains information around alternative options that are considered before making a decision. It should contain a explanation on why the alternative approach(es) were not chosen.

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
    // TODO this is redundant; understand if
    // anything like the ChainID in the Header is 
    // required for IBC though.
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
    LastResultsHash [32]byte // TODO this is ABCI specific: do we really need it though?
    
    // TODO: do we need this to be included in the header?
    // the address can be derived from the pubkey which can be derived 
    // from the signature when using secp256k.
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

`Evidence` represents a go-interface (or oneof in protobuf) of known set of concrete fraud-proofs:
- Same Aggregator signed two different blocks at the same height
  - TODO: figure out if this is actually malicious / slashable behaviour - eg. clients could simply accept the last block included in a LL block
- State Transition Fraud Proofs (for previous blocks)
- TODO: that's it, or are the more?


### Commit

```go
type Commit struct {
    Height     uint64
    HeaderHash [32]byte
    Signatures []Signature // most of the time this is a single signature
}
```

> This section does not need to be filled in at the start of the ADR, but must be completed prior to the merging of the implementation.
>
> Here are some common questions that get answered as part of the detailed design:
>
> - What are the user requirements?
>
> - What systems will be affected?
>
> - What new data structures are needed, what data structures will be changed?
>
> - What new APIs will be needed, what APIs will be changed?
>
> - What are the efficiency considerations (time/space)?
>
> - What are the expected access patterns (load/throughput)?
>
> - Are there any logging, monitoring or observability needs?
>
> - Are there any security considerations?
>
> - Are there any privacy considerations?
>
> - How will the changes be tested?
>
> - If the change is large, how will the changes be broken up for ease of review?
>
> - Will these changes require a breaking (major) release?
>
> - Does this change require coordination with the LazyLedger fork of the SDK or lazyledger-app?

## Status

> A decision may be "proposed" if it hasn't been agreed upon yet, or "accepted" once it is agreed upon. Once the ADR has been implemented mark the ADR as "implemented". If a later ADR changes or reverses a decision, it may be marked as "deprecated" or "superseded" with a reference to its replacement.

Proposed

## Consequences

> This section describes the consequences, after applying the decision. All consequences should be summarized here, not just the "positive" ones.

### Positive

### Negative

### Neutral

## References

> Are there any relevant PR comments, issues that led up to this, or articles referenced for why we made the given design choice? If so link them here!

- {reference link}

TODO link to PR adding go code
