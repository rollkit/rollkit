# ADR 009: Data Availability Interface

## Changelog

- 19.02.2023: First Draft

## Authors

- @wondertan
- @nashqueue

## Context

The DA - Interface will create 2 major benefits:

- Removal of dependency to celestia-node
- Pathway to credible neutrality to implement multiple DA-Layers

The Goal would be to have Rollkit using the functions of the DA-Interface and the DA-Layer exposing those functions.

Furthermore, if other frameworks want to build on top of the DA-Layer they can be compatible with the DA-Interface. In the context of Celestia other infrastructure can be built on top of the DA-Interface using the same underlying Celestia implementation of the DA-Interface.

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

- Ref is the reference to the data, which is used to retrieve the data. It is used as a pointer to the data.
- Commit is the commitment to the data, which is used to verify the data inclusion. The Commit alone might not be enough to retrieve the data, but it is enough to verify it given a proof. The Commit is also used to create soft commitments before the finality of the Data Availability Layer.

What could a Reference in the case of Celestia look like?

- Type 1: Celestia height, PFB commitment (special hash of the data)
- Type 2: Celestia height, share index, amount of shares

What is the happy path of a Sequencer interacting with this interface?

The Sequencer aggregates transactions and submits a block through `Put`. He will receive a Ref and a proof of inclusion. He will then gossip this proof over the P2P network. The Ref can be submitted to a Settlement Layer or gossiped as well for example. Other Participants of the network can then use the Ref to retrieve the data or just verify inlcusion using the proof without having to download the data.

Why does Get give a proof?

We can assume that we are running a node of the DA-Layer locally which means that you can trust the results of those functions. Now we might assume that proof is not needed. This proof is not to validate the "Get" result directly. It is needed for a special liveness issue. Imagine the Sequencer submits a block to DA. At this point in time, he is waiting for a response but shuts down in the process. Now the proof of inclusion will not get gossiped. For that reason, a Full Node can do this instead by getting the proof from `Get

## Status

Proposed

## Consequences

### Positive

### Negative

### Neutral
