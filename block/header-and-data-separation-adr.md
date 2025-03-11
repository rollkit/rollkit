# Header and Data Separation ADR

## Abstract

The separation of header and data structures in Rollkit unlocks expanding the sequencing scheme beyond centralized sequencing and unlocks the use of a decentralized sequencer mode. This means that the creation of list of the transactions can be done by another network as well while rollup nodes still produce headers after executing that list of transactions. This overall change is akin to the proposer-builder separation in the Ethereum protocol, where the Rollkit header producer acts as the proposer, and the sequencer, which produces a list of transactions, acts as the builder.

## Protocol/Component Description

Before, Rollkit only supported the use of a centralized sequencer that was responsible for creating a list of rollup tx by reaping its mempool, executing them to produce a header, and putting them together in a rollup block. Rollkit headers and data were encapsulated within a single block structure. The block struct looked like this:

```go
// Block defines the structure of Rollkit block.
type Block struct {
	SignedHeader SignedHeader
	Data         Data
}
```

The separation of header and data into distinct structures allows them to be processed independently. The `SignedHeader` struct now focuses on the header information, while the `Data` struct handles transaction data separately. This separation is particularly beneficial in unlocking based rollups, where users submit transactions directly to the Data Availability layer which acts as the entity responsible for creating the list of transactions.

This change also affects how rollup full nodes sync. Previously, rollup full nodes would apply the transactions from the `Block` struct and verify that the `header` in `SignedHeader` matched their locally produced header. Now, with the separation, full nodes obtain the transaction data separately (via the DA layer directly in based sequencer mode, or via p2p gossip/DA layer in centralized sequencer mode) and verify it against the header signed by the header producer once they have both components. This ensures that the data integrity and consistency are maintained across the network.

```go
// SignedHeader struct focusing on header information
 type SignedHeader struct {
    Header // Rollkit Header
    Signature  Signature // Signature of the header producer
    ...
}

// Data defines Rollkit block data.
type Data struct {
	*Metadata // Defines metadata for Data struct to help with p2p gossiping.
	Txs Txs // List of transactions to be executed
}
```

The `publishBlock` method in `manager.go` now creates the header and data structures separately. This decoupling allows for the header to be submitted to the DA layer independently of the rollup block data, which can be built by a separate network. This change supports the transition from a centralized sequencer mode to a decentralized sequencer mode, making the system more modular.

## Message Structure/Communication Format

Before, only the entire `Block` struct composed of both header and data was submitted to the DA layer but after the separation, the `SignedHeader` and `Data` are submitted separately to the DA layer. The `SignedHeader` is linked to the `Data` via a Data commitment from the DA layer.

In addition, before the `Block` and `SignedHeader` were both gossipped over a p2p layer to full nodes as well. But after the separation, `SignedHeader` and `Data` are gossipped over the p2p layer since the `Block` struct does not exist anymore and is broken down.

## Assumptions and Considerations

- Considerations include ensuring that headers and data are correctly synchronized and validated to prevent inconsistencies.
- Ensure that all components interacting with headers and data are updated to handle them as separate entities.
- Security measures should be in place to prevent unauthorized access or tampering with headers and data during transmission and storage.
- Performance optimizations may be necessary to handle the increased complexity of managing separate header and data structures, especially in high-throughput environments.
- Testing and validation processes should be updated to account for the new structure and ensure that all components function correctly in both centralized and decentralized sequencer modes.

## Implementation

The implementation of this separation can be found in the Rollkit repository, specifically in the changes made to the `manager.go` file. The `publishBlock` method illustrates the creation of separate header and data structures, and the associated logic for handling them independently. See [Rollkit PR #1789](https://github.com/rollkit/rollkit/pull/1789)

## References

- [Rollkit PR #1789](https://github.com/rollkit/rollkit/pull/1789)
- [Proposer-Builder Separation](https://www.alchemy.com/overviews/proposer-builder-separation)
