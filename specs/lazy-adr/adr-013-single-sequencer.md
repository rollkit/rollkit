# ADR 13: Single Sequencer

## Changelog

- 2024-10-01: Initial draft

## Context

Rollkit supports modular sequencer implementations and a single sequencer is a simple and efficient solution that can serve as a starting point for rollup developers who don't need the complexity of a decentralized sequencing solution.

The single sequencer needs to implement the Generic Sequencer interface defined in the `core/sequencer` package, provide transaction batching capabilities, and reliably submit these batches to a DA layer. It should also maintain state to track submitted batches and provide verification capabilities.

## Alternative Approaches

### Decentralized Sequencer

A decentralized sequencer would distribute the sequencing responsibility across multiple nodes, providing better censorship resistance and fault tolerance. However, this approach introduces significant complexity in terms of consensus, leader election, and coordination between nodes. It would also require more resources to operate and maintain.

This approach was not chosen for the initial implementation because:

1. It adds unnecessary complexity for many use cases
2. It requires more development time and resources
3. Many rollup projects start with a single sequencer and gradually move towards decentralization

### Embedded Sequencer in Rollup Nodes

Another approach would be to embed sequencing functionality directly into rollup nodes. This would simplify the architecture by eliminating a separate sequencer component.

This approach was not chosen because:

1. It couples sequencing logic with rollup node logic, reducing modularity
2. It makes it harder to upgrade or replace the sequencing component independently
3. It doesn't allow for a dedicated sequencing service that can be optimized separately

## Decision

We implement a standalone single sequencer that:

1. Implements the Generic Sequencer interface from the `core/sequencer` package
2. Batches transactions and submits them to a DA layer at regular intervals
3. Provides metrics for monitoring and observability

The single sequencer is a separate repository and can be deployed as a standalone service or as a Docker container.

## Detailed Design

### User Requirements

- Rollup developers need a simple, reliable sequencer that can order transactions and submit them to a DA layer
- The sequencer should be easy to deploy and configure
- The sequencer should provide metrics for monitoring
- The sequencer should be able to recover from crashes and maintain state

### Systems Affected

- Rollup nodes that interact with the sequencer
- DA layer where batches are submitted

### Data Structures

The single sequencer uses the following key data structures:

1. **BatchQueue**: A queue to store batches of transactions waiting to be processed

   ```go
   type BatchQueue struct {
       queue []sequencing.Batch  // In-memory queue of batches waiting to be processed
       mu    sync.Mutex          // Mutex to ensure thread-safe access to the queue
   }
   ```

2. **Sequencer**: The main sequencer structure that implements the Generic Sequencer interface

   ```go
   type Sequencer struct {
       dalc      *da.DAClient       // Client for interacting with the Data Availability layer
       batchTime time.Duration      // Time interval between batch submissions
       ctx       context.Context    // Context for controlling the sequencer's lifecycle
       maxSize   uint64             // Maximum size of a batch in bytes

       rollupId sequencing.RollupId // Identifier for the rollup this sequencer serves

       tq                 *TransactionQueue  // Queue for storing pending transactions
       lastBatchHash      []byte             // Hash of the last processed batch
       lastBatchHashMutex sync.RWMutex       // Mutex for thread-safe access to lastBatchHash

       seenBatches      map[string]struct{}  // Map to track batches that have been processed
       seenBatchesMutex sync.Mutex           // Mutex for thread-safe access to seenBatches
       bq               *BatchQueue          // Queue for storing batches ready for processing

       db    *badger.DB  // BadgerDB instance for persistent storage
       dbMux sync.Mutex  // Mutex for safe concurrent DB access

       metrics *Metrics  // Structure to hold metrics for monitoring
   }
   ```

3. **Metrics**: Structure to hold metrics for monitoring

   ```go
   type Metrics struct {
       GasPrice            metrics.Gauge    // Tracks the gas price used for DA submissions
       LastBlobSize        metrics.Gauge    // Tracks the size of the last submitted blob
       TransactionStatus   metrics.Counter  // Counts transaction status outcomes
       NumPendingBlocks    metrics.Gauge    // Tracks the number of blocks waiting to be submitted
       IncludedBlockHeight metrics.Gauge    // Tracks the height of the last included block in the DA layer
   }
   ```

### APIs

The single sequencer implements the Generic Sequencer interface from the `core/sequencer` package:

```go
type Sequencer interface {
    SubmitRollupBatchTxs(ctx context.Context, req SubmitRollupBatchTxsRequest) (*SubmitRollupBatchTxsResponse, error)
    GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error)
    VerifyBatch(ctx context.Context, req VerifyBatchRequest) (*VerifyBatchResponse, error)
}
```

1. **SubmitRollupBatchTxs**:
   - This method is responsible for accepting a batch of transactions from a rollup client. It takes a context and a request containing the rollup ID and the batch of transactions to be submitted.
   - The method first validates the rollup ID to ensure it matches the expected ID for the sequencer. If the ID is invalid, it returns an error.
   - Upon successful validation, the method adds the transactions to the internal transaction queue (`TransactionQueue`) for processing.
   - It then triggers the batch submission process, which involves retrieving the next batch of transactions and submitting them to the designated Data Availability (DA) layer.
   - Finally, it returns a response indicating the success or failure of the submission.

2. **GetNextBatch**:
   - This method retrieves the next batch of transactions that are ready to be processed by the rollup. It takes a context and a request containing the rollup ID and the last batch hash.
   - The method first checks if the rollup ID is valid. If not, it returns an error.
   - It then verifies the last batch hash to ensure that the rollup client is requesting the correct next batch.
   - If a valid batch is found, it prepares the batch response, which includes the batch of transactions and a timestamp.
   - If no transactions are available, it returns an empty batch response.

   Note that this method is used by the rollup node to get a sequencer soft-confirmed batch that the sequencer promises to publish to the DA layer.

3. **VerifyBatch**:
   - This method is used to verify the that a batch received (soft-confirmed) from the sequencer was actually published on the DA layer. It takes a context and a request containing the rollup ID and the batch hash.
   - Similar to the other methods, it first validates the rollup ID.
   - It then checks if the provided batch hash exists in the internal data structure that tracks seen batches.
   - If the batch hash is found, it returns a response indicating that the batch is valid. If not, it returns a response indicating that the batch is invalid.

   Once this method returns true for batch, a rollup node can mark the rollup block associated to this batch as `DA included` and mark it as fully confirmed from its view.

These methods work together to ensure that the single sequencer can effectively manage transaction submissions, retrievals, and verifications, providing a reliable interface for rollup clients to interact with the sequencer.

### Efficiency Considerations

- The sequencer uses a configurable batch time to balance between latency and efficiency
- Transactions are batched to reduce the number of DA submissions
- The sequencer maintains an in-memory queue for fast access and a persistent database for durability
- Exponential backoff is used for DA submission retries to handle temporary failures

### Access Patterns

- Rollup clients will submit transactions to the sequencer at varying rates
- The sequencer will batch transactions and submit them to the DA layer at regular intervals
- Rollup nodes will request the next batch from the sequencer to process transactions

### Logging, Monitoring, and Observability

The sequencer provides the following metrics:

- Gas price of DA submissions
- Size of the last submitted blob
- Transaction status counts
- Number of pending blocks
- Last included block height

These metrics can be exposed via Prometheus for monitoring.

### Security Considerations

- The single sequencer is a single point of failure and control
- Access control is not implemented in the initial version, but can be added in future versions
- The sequencer validates rollup IDs to ensure transactions are submitted to the correct rollup

### Privacy Considerations

- The sequencer has access to all transactions before they are submitted to the DA layer
- Transactions are not encrypted, so sensitive data should not be included in transactions

### Testing

The single sequencer includes:

- Unit tests for core functionality
- Integration tests with a mock DA layer
- Test coverage reporting via Codecov

### Breaking Changes

This is a new component and does not introduce breaking changes to existing systems.

## Status

Proposed

## Consequences

### Positive

- Provides a simple, production-ready sequencer for rollup developers
- Implements the Generic Sequencer interface, making it compatible with existing Rollkit components
- Includes metrics for monitoring and observability
- Maintains state to track submitted batches and provide verification
- Can be deployed as a standalone service or as a Docker container

### Negative

- Single design introduces a single point of failure
- No built-in access control or authentication in the initial version
- Limited scalability compared to a distributed sequencer

### Neutral

- Requires a separate deployment and management of the sequencer service
- Developers need to configure the sequencer to connect to their chosen DA layer

## References

- [Generic Sequencer Interface](https://github.com/rollkit/rollkit/blob/main/core/sequencer/sequencing.go)
- [Rollkit Repository](https://github.com/rollkit/rollkit)
- [Single Sequencer Repository](https://github.com/rollkit/centralized-sequencer)
