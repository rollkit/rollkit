# ADR 009: Data Availability Interface

## Changelog

- 19.02.2023: First Draft

## Authors

- @wondertan
- @nashqueue

## Context

## Alternative Approaches

## Decision

## Detailed Design

```go

type Proof []byte // In case of Celestia this could be the inclusion proof to the blob 
type Data []byte // This could be a block or a transaction for example
type Ref []byte
type Commit []byte

type DataAvailability interface {
		Commit(Data) (Commit, error)
		Put([]Data) ([]Ref, []Proof, error)
		Get(Ref) (Data, Proof, error)
		Validate(Ref, Proof) (bool, error)
}

```

What is the difference between 'Commit' and 'Ref'?

- Ref is the reference to the data, which is used to retrieve the data. It is used like a pointer to the data.
- Commit is the commitment to the data, which is used to verify the data inclusion. The Commit alone might not be enough to retrieve the data, but it is enough to verify it given a proof. The Commit is also used to create soft commitments before finality of the Data Availability Layer.

How could a Reference in case of Celestia look like?

- Type 1: Celestia height, PFB commitment (special hash of the data)
- Type 2: Celestia height, share index, amount of shares

## Status

Proposed

## Consequences

### Positive

### Negative

### Neutral
