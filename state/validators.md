# Sequencer Selection Scheme

## Abstract

The Sequencer Selection scheme describes the process of selecting a block proposer i.e. sequencer from the validator set.

In the first version of Rollkit, this is a fixed pubkey, belonging to the centralized sequencer. The validator set may only ever have one "validator", the centralized sequencer.

## Protocol/Component Description

There is exactly one sequencer which is configured at genesis. `GenesisDoc` usually contains an array of validators as it is imported from `CometBFT`. If there is more than one validator defined
in the genesis validator set, an error is thrown.

The `Header` struct defines a field called `ProposerAddress` which is the pubkey of the original proposer of the block.

The `SignedHeader` struct commits over the header and the proposer address and stores the result in `LastCommitHash`.

A new untrusted header is verified by checking its `ProposerAddress` and matching it against the best-known header. In case of a mismatch, an error is thrown.

## Message Structure/Communication Format

The primary structures encompassing validator information include `SignedHeader`, `Header`, and `State`. Some fields are repurposed from CometBFT as seen in `GenesisDoc` `Validators`.

## Assumptions and Considerations

1. There must be exactly one validator defined in the genesis file, which determines the sequencer for all the blocks.

## Implementation

The implementation is split across multiple functions including `IsProposer`, `publishBlock`, `CreateBlock`, and `Verify` among others, which are defined in various files like `state.go`, `manager.go`, `block.go`, `header.go` etc., within the repository.

See [block manager]

## References

[1] [Block Manager][block manager]

[block manager]: https://github.com/rollkit/rollkit/blob/main/block/block-manager.md
