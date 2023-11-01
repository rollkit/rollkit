# Block Executor

## Abstract

The `BlockExecutor` is a component responsible for creating, applying, and maintaining blocks and state in the system. It interacts with the mempool and the application via the [ABCI interface].

## Detailed Description

The `BlockExecutor` is initialized with a proposer address, `namespace ID`, `chain ID`, `mempool`, `proxyApp`, `eventBus`, and `logger`. It uses these to manage the creation and application of blocks. It also validates blocks and commits them, updating the state as necessary.

- `NewBlockExecutor`: This method creates a new instance of `BlockExecutor`. It takes a proposer address, `namespace ID`, `chain ID`, `mempool`, `proxyApp`, `eventBus`, and `logger` as parameters. See [block manager] for details.

- `InitChain`: This method initializes the chain by calling ABCI `InitChainSync` using the consensus connection to the app. It takes a `GenesisDoc` as a parameter. It sends a ABCI `RequestInitChain` message with the genesis parameters including:
  - Genesis Time
  - Chain ID
  - Consensus Parameters including:
    - Block Max Bytes
    - Block Max Gas
    - Evidence Parameters
    - Validator Parameters
    - Version Parameters
  - Initial Validator Set using genesis validators
  - Initial Height

- `CreateBlock`: This method reaps transactions from the mempool and builds a block. It takes the state, the height of the block, last header hash, and the commit as parameters.

- `ApplyBlock`: This method applies the block to the state. Given the current state and block to be applied, it:
  - Validates the block, as described in `Validate`.
  - Executes the block using app, as described in `execute`.
  - Captures the validator updates done in the execute block.
  - Updates the state using the block, block execution responses, and validator updates as described in `updateState`.
  - Returns the updated state, validator updates and errors, if any, after applying the block.
  - It can return the following named errors:

    - `ErrEmptyValSetGenerate`: returned when applying the validator changes would result in empty set.
    - `ErrAddingValidatorToBased`: returned when adding validators to empty validator set.

- `Validate`: This method validates the block. It takes the state and the block as parameters. In addition to the basic [block validation] rules, it applies the following validations:

  - New block version must match block version of the state.
  - If state is at genesis, new block height must match initial height of the state.
  - New block height must be last block height + 1 of the state.
  - New block header `AppHash` must match state `AppHash`.
  - New block header `LastResultsHash` must match state `LastResultsHash`.
  - New block header `AggregatorsHash` must match state `Validators.Hash()`.

- `Commit`: This method commits the block and updates the mempool. Given the updated state, the block, and the ABCI `ResponseFinalizeBlock` as parameters, it:
  - Invokes app commit, basically finalizing the last execution, by  calling ABCI `Commit`.
  - Updates the mempool to inform that the transactions included in the block can be safely discarded.
  - Publishes the events produced during the block execution for indexing.

- `updateState`: This method updates the state. Given the current state, the block, the ABCI `ResponseFinalizeBlock` and the validator updates, it validates the updated validator set, updates the state by applying the block and returns the updated state and errors, if any. The state consists of:
  - Version
  - Chain ID
  - Initial Height
  - Last Block including:
    - Block Height
    - Block Time
    - Block ID
  - Next Validator Set
  - Current Validator Set
  - Last Validators
  - Whether Last Height Validators changed
  - Consensus Parameters
  - Whether Last Height Consensus Parameters changed
  - App Hash

- `execute`: This method executes the block. It takes the context, the state, and the block as parameters. It calls the ABCI method `FinalizeBlock` with the ABCI `RequestFinalizeBlock` containing the block hash, ABCI header, commit, transactions and returns the ABCI `ResponseFinalizeBlock` and errors, if any.

- `publishEvents`: This method publishes events related to the block. It takes the ABCI `ResponseFinalizeBlock`, the block, and the state as parameters.

## Message Structure/Communication Format

The `BlockExecutor` communicates with the application via the [ABCI interface]. It calls the ABCI methods `InitChainSync`, `FinalizeBlock`, `Commit` for initializing a new chain and creating blocks, respectively.

## Assumptions and Considerations

The `BlockExecutor` assumes that there is consensus connection available to the app, which can be used to send and receive ABCI messages. In addition there are some important pre-condition and post-condition invariants, as follows:

- `InitChain`:
  - pre-condition:
    - state is at genesis.
  - post-condition:
    - new chain is initialized.

- `CreateBlock`:
  - pre-condition:
    - chain is initialized
  - post-condition:
    - new block is created

- `ApplyBlock`:
  - pre-condition:
    - block is valid, using basic [block validation] rules as well as validations performed in `Validate`, as described above.
  - post-condition:
    - block is added to the chain, state is updated and block execution responses are captured.

- `Commit`:
  - pre-condition:
    - block has been applied
  - post-condition:
    - block is committed
    - mempool is cleared of block transactions
    - block events are published
    - state App Hash is updated with the result from `ResponseCommit`

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
