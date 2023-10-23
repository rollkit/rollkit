# Block Executor

## Abstract

The `BlockExecutor` is a component responsible for creating, applying, and maintaining blocks and state in the system. It interacts with the mempool and the application via the [ABCI interface].

## Detailed Description

The `BlockExecutor` is initialized with a proposer address, `namespace ID`, `chain ID`, `mempool`, `proxyApp`, `eventBus`, and `logger`. It uses these to manage the creation and application of blocks. It also validates blocks and commits them, updating the state as necessary.

- `NewBlockExecutor`: This method creates a new instance of `BlockExecutor`. It takes a proposer address, `namespace ID`, `chain ID`, `mempool`, `proxyApp`, `eventBus`, and `logger` as parameters. See [block manager] for details.
- `InitChain`: This method initializes the chain by calling `InitChainSync` using the consensus connection to the app. It takes a `GenesisDoc` as a parameter.
- `CreateBlock`: This method reaps transactions from the mempool and builds a block. It takes the current state, the height of the block, last header hash, and the commit as parameters.
- `ApplyBlock`: This method applies the block to the state. It takes the current state, the block ID, and the block itself as parameters. It can return the following named errors:
  - `ErrEmptyValSetGenerate`: returned when applying the validator changes would result in empty set.
  - `ErrAddingValidatorToBased`: returned when adding validators to empty validator set.
- `Validate`: This method validates the block. It takes the state and the block as parameters. In addition to the basic [block validation] rules, it applies the following validations:
  - New block version must match state block version.
  - New block height must match state initial height.
  - New block height must be adjacent to state last block height.
  - New block header `AppHash` must match state `AppHash`.
  - New block header `LastResultsHash` must match state `LastResultsHash`.
  - New block header `AggregatorsHash` must match state `Validators.Hash()`.
- `commit`: This method commits the block and updates the state. It takes the state, the block, and the ABCI `ResponseFinalizeBlock` as parameters.
- `execute`: This method executes the block. It takes the context, the state, and the block as parameters. It calls the ABCI method `FinalizeBlock` with the ABCI `RequestFinalizeBlock` containing the block hash, ABCI header, commit, transactions and returns the ABCI `ResponseFinalizeBlock` and errors, if any.
- `publishEvents`: This method publishes events related to the block. It takes the ABCI `ResponseFinalizeBlock`, the block, and the state as parameters.

## Message Structure/Communication Format

The `BlockExecutor` communicates with the application via the ABCI interface. It sends and receives ABCI messages, such as `RequestFinalizeBlock` and `ResponseFinalizeBlock`.

## Assumptions and Considerations

The `BlockExecutor` assumes that the mempool and the application are functioning correctly. It also assumes that the blocks it receives are well-formed and that the state is consistent.

## Implementation

See [block executor]

## References

[1] [Block Executor][block executor]
[2] [Block Manager][block manager]
[3] [Block Validation][block validation]
[4] [ABCI documentation][ABCI interface]


[block executor]: https://github.com/rollkit/rollkit/blob/v0.11.x/state/executor.go
[block manager]: https://github.com/rollkit/rollkit/blob/v0.11.x/block/block-manager.md
[block validation]: https://github.com/rollkit/rollkit/blob/v0.11.x/types/block_spec.md
[ABCI interface]: https://github.com/cometbft/cometbft/blob/main/spec/abci/abci%2B%2B_basic_concepts.md
