# BlockExecutor

## Abstract

 The `BlockExecutor` is a component responsible for creating, applying, and maintaining blocks and state in the system. It interacts with the mempool and the application via the ABCI interface.

## Component Description

 The `BlockExecutor` is initialized with a proposer address, `namespace ID`, `chain ID`, `mempool`, `proxyApp`, `eventBus`, and `logger`. It uses these to manage the creation and application of blocks. It also validates blocks and commits them, updating the state as necessary.

## Detailed Description

- `NewBlockExecutor`: This method creates a new instance of `BlockExecutor`. It takes a proposer address, `namespace ID`, `chain ID`, `mempool`, `proxyApp`, `eventBus`, and `logger` as parameters.
- `InitChain`: This method initializes the chain by calling `InitChainSync` using the consensus connection to the app. It takes a `GenesisDoc` as a parameter.
- `CreateBlock`: This method reaps transactions from the mempool and builds a block. It takes the current state, the height of the block, and the commit as parameters.
- `ApplyBlock`: This method applies the block to the state. It takes the current state, the block ID, and the block itself as parameters. It can return the following named errors:
    - `ErrEmptyValSetGenerate`: returned when applying the validator changes would result in empty set.
    - `ErrAddingValidatorToBased`: return because we cannot add validators to empty validator set.
- `Validate`: This method validates the block. It takes the state and the block as parameters. It applies the following validations:
    - New block version must match state block version.
    - New block height must match state initial height.
    - New block height must be adjacent to state last block height.
    - New block header `AppHash` must match state `AppHash`.
    - New block header `LastResultsHash` must match state `LastResultsHash`.
    - New block headeer `AggregatorsHash` must match state `Validators.Hash()`.
- `commit`: This method commits the block and updates the state. It takes the state, the block ID, and the block as parameters.
- `execute`: This method executes the block. It takes the context, the state, and the block as parameters.
- `publishEvents`: This method publishes events related to the block. It takes the ABCI responses, the block, and the state as parameters.

## Message Structure/Communication Format

The `BlockExecutor` communicates with the application via the ABCI interface. It sends and receives ABCI messages, such as `RequestInitChain`, `RequestBeginBlock`, `RequestDeliverTx`, and `RequestEndBlock`.

## Assumptions and Considerations

The `BlockExecutor` assumes that the mempool and the application are functioning correctly. It also assumes that the blocks it receives are well-formed and that the state is consistent.

## Implementation

The implementation of the `BlockExecutor` can be found in the file `state/executor.go`.

## References

- `state/executor.go`: Contains the implementation of the `BlockExecutor`.
- ABCI documentation: Provides information on the ABCI interface and its messages.
