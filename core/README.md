# Rollkit Core Package

The `core` package is the zero-dependency foundation of Rollkit. It provides the essential interfaces and types that enable modular builds of different application formats (e.g., sequencer, full node, light client) while preventing circular dependencies.

## Purpose

The primary goal of the `core` package is to define common contracts (interfaces) for key components within Rollkit, such as execution, sequencing, and data availability (DA). By having all other Rollkit modules depend solely on `core`, we decouple their implementations. This allows each component to be compiled independently and avoids circular import issues, which can arise when components directly depend on each other.

## Key Interfaces

The `core` package defines several crucial interfaces that standardize interactions between different parts of a Rollkit node. Here are the main ones:

### Execution

The `Executor` interface defines how the execution layer processes transactions and manages state transitions.

```go
// core/execution/execution.go

// Executor defines the interface for the execution layer.
type Executor interface {
	// InitChain initializes the chain based on the genesis information.
	InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) (stateRoot []byte, maxBytes uint64, err error)
	// GetTxs retrieves transactions from the mempool.
	GetTxs(ctx context.Context) ([][]byte, error)
	// ExecuteTxs executes a block of transactions against the current state.
	ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (updatedStateRoot []byte, maxBytes uint64, err error)
	// SetFinal marks a block height as final.
	SetFinal(ctx context.Context, blockHeight uint64) error
}

```

### Sequencing

The `Sequencer` interface outlines the responsibilities of the component that orders and batches transactions.

```go
// core/sequencer/sequencing.go

// Sequencer defines the interface for the sequencing layer.
type Sequencer interface {
	// SubmitBatchTxs submits batch transactions to the sequencer.
	SubmitBatchTxs(ctx context.Context, req SubmitBatchTxsRequest) (*SubmitBatchTxsResponse, error)
	// GetNextBatch retrieves the next batch of transactions from the sequencer.
	GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error)
	// VerifyBatch verifies a batch of transactions.
	VerifyBatch(ctx context.Context, req VerifyBatchRequest) (*VerifyBatchResponse, error)
}
```

### Data Availability (DA)

The `DA` interface specifies how data is submitted to and retrieved from the underlying data availability layer.

```go
// core/da/da.go

// DA defines the interface for the data availability layer.
type DA interface {
	// MaxBlobSize returns the max blob size.
	MaxBlobSize(ctx context.Context) (uint64, error)
	// Get returns Blobs for given IDs.
	Get(ctx context.Context, ids []ID, namespace []byte) ([]Blob, error)
	// GetIDs returns IDs of Blobs at given height.
	GetIDs(ctx context.Context, height uint64, namespace []byte) (*GetIDsResult, error)
	// GetProofs returns inclusion proofs for Blobs specified by IDs at given height.
	GetProofs(ctx context.Context, ids []ID, namespace []byte) ([]Proof, error)
	// Commit creates commitments for Blobs. Submit uses Commit internally.
	Commit(ctx context.Context, blobs []Blob, namespace []byte) ([]Commitment, error)
	// Submit submits Blobs to DA layer.
	Submit(ctx context.Context, blobs []Blob, gasPrice float64, namespace []byte) ([]ID, error)
	// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
	Validate(ctx context.Context, ids []ID, proofs []Proof, namespace []byte) ([]bool, error)
	// GasPrice returns the gas price.
	GasPrice(ctx context.Context) (float64, error)
	// GasMultiplier returns the gas multiplier.
	GasMultiplier(ctx context.Context) (float64, error)
}
```

The `Client` interface provides a higher-level abstraction for interacting with the DA layer, often used by nodes.

```go
// core/da/client.go

// Client is the interface for the DA layer client.
type Client interface {
	// Submit submits block data to DA layer.
	Submit(ctx context.Context, data [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmit
	// Retrieve retrieves block data from DA layer.
	Retrieve(ctx context.Context, dataLayerHeight uint64) ResultRetrieve
	// MaxBlobSize returns the maximum blob size for the DA layer.
	MaxBlobSize(ctx context.Context) (uint64, error)
	// GasPrice returns the gas price for the DA layer.
	GasPrice(ctx context.Context) (float64, error)
	// GasMultiplier returns the gas multiplier for the DA layer.
	GasMultiplier(ctx context.Context) (float64, error)
	// GetNamespace returns the namespace for the DA layer.
	GetNamespace(ctx context.Context) ([]byte, error)
}
```

## Contributing

The `core` package is central to Rollkit's architecture. Modifications here can have wide-ranging effects. Please adhere to the following guidelines when contributing:

- **Compatibility:** Prioritize backward compatibility. Changes to existing interfaces should be made carefully.
- **ADRs for Significant Changes:** For substantial changes, especially to interfaces, please propose them via an Architecture Decision Record (ADR). The ADR should detail the proposed changes and the reasoning behind them.
- **Zero Dependencies:** This package **must** remain dependency-free. Do not add any external modules to the `core/go.mod` file. Its strength lies in its simplicity and lack of external entanglements.
