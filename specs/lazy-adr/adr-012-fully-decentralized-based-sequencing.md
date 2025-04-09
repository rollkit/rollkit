# ADR 012: Fully Decentralized Based Sequencing

## Changelog

- 2025-04-09: Initial draft
- 2025-04-09: Added optional UX optimization where full nodes can relay user rollup txs to base layer
- 2025-04-09: Added rationale for VerifyBatch utility in fully decentralized setup
- 2025-04-09: Reworded forkchoice rule to use maxHeightDrift instead of time-based maxLatency

## Context

Most rollups today rely on centralized or semi-centralized sequencers to form batches of user transactions, despite the availability of base layers (like Celestia) that provide data availability and canonical ordering guarantees. Centralized sequencers introduce liveness and censorship risks, as well as complexity in proposer election, fault tolerance, and bridge security.

Based sequencing eliminates this reliance by having the base layer determine transaction ordering. However, previous implementations still assumed the existence of a proposer to prepare rollup batches.

This ADR proposes a **fully decentralized based sequencing model** in which **every full node acts as its own proposer** by independently:
- Reading rollup blobs from the base layer
- Applying a deterministic forkchoice rule
- Constructing rollup blocks
- Executing batches to compute state updates

This approach ensures consistency, removes the need for trusted intermediaries, and improves decentralization and resilience.

## Alternative Approaches

### Centralized Sequencer
- A designated sequencer collects rollup transactions and publishes them to the base layer.
- Simpler for UX and latency control, but introduces centralization and failure points.

### Leader-Elected Proposer (e.g., BFT committee or rotating proposer)
- Some nodes are elected to act as proposers for efficiency.
- Still introduces trust assumptions, coordination complexity, and MEV-related risks.

### Trusted Rollup Light Client Commitments
- Rollup blocks are committed to L1 (e.g., Ethereum) and verified by a light client.
- Adds delay and dependency on L1 finality, and often still relies on centralized proposers.

None of these provide the decentralization and self-sovereignty enabled by a fully deterministic, proposerless, based sequencing model.

## Decision

We adopt a fully decentralized based sequencing model where **every full node in the rollup network acts as its own proposer** by deterministically deriving the next batch using only:
- Base-layer data (e.g., Celestia blobs tagged by rollup ID)
- A forkchoice rule: MaxBytes + Bounded L1 Height Drift (maxHeightDrift)
- Local execution (e.g., EVM via reth)

This model removes the need for:
- A designated sequencer
- Coordination mechanisms
- Sequencer signatures

The canonical rollup state becomes a **function of the base layer** and a well-defined forkchoice rule.

Additionally, to improve UX for users who do not operate a base layer client or wallet, **full nodes may optionally relay user-submitted rollup transactions to the base layer**. This maintains decentralization while improving accessibility.

The following sequence diagram demonstrates two rollup fullnodes independently preparing batches and rollup states that are identical using the user transactions that are directly submitted to the base layer.

```mermaid
sequenceDiagram
    participant L1 as Base Layer (e.g., Celestia)
    participant NodeA as Rollup Full Node A
    participant NodeB as Rollup Full Node B
    participant ExecA as Execution Engine A
    participant ExecB as Execution Engine B

    Note over L1: Users post rollup transactions as blobs tagged with RollupID

    NodeA->>L1: Retrieve new blobs since last DA height
    NodeB->>L1: Retrieve new blobs since last DA height

    NodeA->>NodeA: Apply forkchoice rule<br/>MaxBytes or MaxHeightDrift met?
    NodeB->>NodeB: Apply forkchoice rule<br/>MaxBytes or MaxHeightDrift met?

    NodeA->>ExecA: ExecuteTxs(blobs)
    NodeB->>ExecB: ExecuteTxs(blobs)

    ExecA-->>NodeA: Updated state root
    ExecB-->>NodeB: Updated state root

    NodeA->>NodeA: Build rollup block with batch + state root
    NodeB->>NodeB: Build rollup block with batch + state root

    Note over NodeA, NodeB: Both nodes independently reach the same rollup block & state
```

The following sequence diagram shows the case where the user utilizes the full node to relay the transaction to the base layer and the rollup light client in action.

```mermaid
sequenceDiagram
    participant User as User
    participant Node as Rollup Full Node
    participant Base as Base Layer (e.g., Celestia)
    participant Exec as Execution Engine
    participant LightClient as Rollup Light Client

    Note over User: User submits rollup transaction

    User->>Node: SubmitRollupTx(tx)
    Node->>Base: Post tx blob (DA blob with RollupID)

    Note over Node: Node continuously scans base layer

    Node->>Base: Retrieve blobs since last height
    Node->>Node: Apply forkchoice rule (MaxBytes or MaxHeightDrift)
    Node->>Exec: ExecuteTxs(blobs)
    Exec-->>Node: Updated state root
    Node->>Node: Build rollup block

    Note over LightClient: Re-executes forkchoice & verifies state

    LightClient->>Base: Retrieve blobs
    LightClient->>LightClient: Apply forkchoice & re-execute
    LightClient->>LightClient: Verify state root & inclusion
```

## Detailed Design

### User Requirements
- Users submit transactions by:
  - Posting them directly to the base layer in tagged blobs, **or**
  - Sending them to any full node's RPC endpoint, which will relay them to the base layer on their behalf
- Users can verify finality by checking rollup light clients or DA inclusion

### Systems Affected
- Rollup full nodes
- Rollup light clients
- Batch building and execution logic

### Forkchoice Rule
A batch is constructed when:
1. The accumulated size of base-layer blobs >= `MAX_BYTES`
2. OR the L1 height difference since the last batch exceeds `MAX_HEIGHT_DRIFT`

All rollup full nodes:
- Track base-layer heights and timestamps
- Fetch all rollup-tagged blobs
- Apply the rule deterministically
- Execute batches to update state

### Data Structures
- Blob index: to track rollup blobs by height and timestamp
- Batch metadata: includes L1 timestamps, blob IDs, and state roots

### APIs
- `GetNextBatch(lastBatchData, maxBytes, maxHeightDrift)`: deterministically builds batch, `maxHeightDrift` can be configured locally instead of passing on every call.
- `VerifyBatch(batchData)`: re-derives and checks state
- `SubmitRollupBatchTxs(batch [][]byte)`: relays a user transaction(s) to the base layer

In fully decentralized based sequencing, full nodes do not need VerifyBatch to participate in consensus or build the canonical rollup state — because they derive everything deterministically from L1. However, VerifyBatch may still be useful in sync, light clients, testing, or cross-domain verification.

* Light Clients: L1 or cross-domain light clients can use VerifyBatch to validate that a given rollup state root or message was derived according to the forkchoice rule and execution logic.

* Syncing Peers: During peer synchronization or state catch-up, nodes may download batches and use VerifyBatch to confirm correctness before trusting the result.

* Auditing / Indexers: Off-chain services may verify batches as part of building state snapshots, fraud monitoring, or historical replays.

* Testing: Developers and test frameworks can validate batch formation correctness and execution determinism using VerifyBatch.

In all of these cases, VerifyBatch acts as a stateless, replayable re-computation check using base-layer data and rollup rules.

### Efficiency
- Deterministic block production without overhead of consensus
- Bound latency ensures timely progress even with low traffic

### Observability
- Each node can log forkchoice decisions, skipped blobs, and batch triggers

### Security
- No sequencer key or proposer trust required
- Replayable from public data (DA layer)
- Optional transaction relay must not allow censorship or injection

### Privacy
- No privacy regressions; same as base-layer visibility

### Testing
- Unit tests for forkchoice implementation
- End-to-end replay tests against base-layer data
- Mocked relayer tests for SubmitRollupTx

### Deployment
- No breaking changes to existing based rollup logic
- Can be rolled out by disabling proposer logic
- Optional relayer logic gated by config flag

## Status

Proposed

## Consequences

### Positive
- Removes centralized sequencer
- Fully deterministic and transparent
- Enables trustless bridges and light clients
- Optional relayer support improves UX for walletless or mobile users

### Negative
- Slight increase in complexity in forkchoice validation
- Must standardize timestamp and blob access for determinism
- Must prevent relayer misuse or spam

### Neutral
- Shifts latency tuning from proposer logic to forkchoice parameters

## References

- [EthResearch: Based Rollups](https://ethresear.ch/t/based-rollups-superpowers-from-l1-sequencing/15016)
- [Taiko](https://taiko.mirror.xyz/7dfMydX1FqEx9_sOvhRt3V8hJksKSIWjzhCVu7FyMZU)
- [Surge](https://www.surge.wtf/)
- [Spire](https://www.spire.dev/)
- [Unifi from Puffer](https://www.puffer.fi/unifi)

