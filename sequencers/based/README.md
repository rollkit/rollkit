# Based Sequencer for Rollkit

This package implements a DA-based sequencer for Rollkit. The based sequencer leverages the underlying Data Availability (DA) layer for transaction ordering and availability, providing a more streamlined and efficient sequencing mechanism compared to the centralized sequencer.

## Architecture

The based sequencer architecture has the following key components:

1. **BasedSequencer**: Implements the Sequencer interface and interfaces with any DA layer
2. **DA Interface**: A generic interface for interacting with Data Availability layers

The based sequencing approach differs from centralized sequencing in the following ways:

1. **Transaction Ordering**: Transaction order is determined by the DA layer, not a centralized sequencer
2. **Data Availability**: Block data is stored directly in the DA layer instead of a separate service
3. **Block Propagation**: Only headers are gossiped via P2P, reducing bandwidth overhead
4. **Finalization**: Headers are immediately marked as finalized after verification, reducing confirmation time

## Configuration

Based sequencing is used by default when no sequencer address is provided:

```toml
[node]
# No sequencer_address = uses based sequencing automatically
```

To use centralized sequencing instead, specify a sequencer address:

```toml
[node]
sequencer_address = "localhost:50051"
```

## Flow

The based sequencing flow works as follows:

1. **Proposer (Header Producer)**:
   - Gets transactions from DA layer using the DA interface
   - Executes transactions 
   - Finalizes block
   - Posts header to DA
   - Gossips header only (not block)

2. **Full Node**:
   - Retrieves headers from gossip & DA
   - Verifies headers match from both sources
   - Marks as finalized immediately when verified
   - Retrieves block data from DA when needed

## Implementation Details

The implementation follows these principles:

1. **Generic DA Interface**: 
   ```go
   type DA interface {
       SubmitBlob(ctx context.Context, namespace []byte, data []byte) (string, error)
       GetHeight(ctx context.Context) (uint64, error)
       GetBlobsByNamespace(ctx context.Context, height uint64, namespace []byte) ([][]byte, error)
       VerifyBlob(ctx context.Context, height uint64, namespace []byte, blobHash []byte) (bool, error)
   }
   ```

2. **BasedSequencer**:
   ```go
   type BasedSequencer struct {
       da da.DA
       // internal state management
   }
   ```

3. **Automatic Detection**: The system automatically detects based sequencing mode by checking the sequencer implementation type.

## Requirements

- A compatible DA layer implementation
- Properly configured namespace for the rollup data 