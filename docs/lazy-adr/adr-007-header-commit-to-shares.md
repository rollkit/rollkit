# ADR 007: Commit to Shares in Header

## Changelog

- 2021-10-01: Initial draft

## Context

1. Only [a single data root must be included in the Celestia block header](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/data_structures.md#header); if more than one data root is included, light nodes will not have any guarantees that the data behind the other data roots is available unless they run a separate sampling process per data root.
1. [PayForMessaage transactions](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/data_structures.md#signedtransactiondatapayformessage) include a commitment of roots of subtrees of the Celestia data root. This is a requirement for compact proofs that a message was or was not included correctly.
1. [Over the wire](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/networking.md#wiretxpayformessage), PayForMessage transactions include (potentially) multiple signatures for the same message with different layouts in the Celestia block, to allow for [non-interactive message inclusion](https://github.com/celestiaorg/celestia-specs/blob/master/src/rationale/message_block_layout.md#non-interactive-default-rules).

Rollup blocks must follow a similar strategy to the above. Specifically, the data root in rollup block headers [must commit to subtree roots in the Celestia data root](https://github.com/celestiaorg/optimint/issues/133), otherwise rollup nodes would not have any guarantees that the data behind the data root in a rollup block header is available.

## Alternative Approaches

### Alternative 1: Don't Commit to Data Root in Rollup Block Header

One proposal is to simply not include any commitment to data in the rollup block header, and use the `MessageShareCommitment` field in the respective PayForMessage transaction to commit to _both_ the rollup block header and rollup block data.

This may only work securely under certain scenarios, but not in the general case. Since rollup block data is not committed to in the rollup block header, is will not be signed over by the rollup block producer set (e.g. if the rollup block producers are using a Tendermint-like protocol) and can therefore be malleated by this parties.

### Alternative 2: One Message Per Layout

Another proposal is having the rollup block header commit to subtree roots. However, since the layout of shares (and thus, the subtree structures) might change depending on the size of the data square, this means the message that is paid for in PayForMessage is different depending on the layout (since the message includes the rollup block header).

A different message can be includes per witness for PayForMessage transactions over the wire. This would lead to unacceptably high networking overhead unfortunately, since messages are expected to be quite large individually.

### Alternative 3: Auction Off Rows Instead of Shares

Instead of auctioning off individual shares, rows can be auctioned off entirely. In other words, only a single message can be included per row (though a message may span more than one row), and always begins at the start of a row. As a consequence, the layout of each message is known, and there is no longer a need for multiple witnesses or messages in each PayForMessage over the wire. This will also simplify various codepaths that no longer need to account for the potential of multiple messages and namespaces per row.

This should solve the issue, but is also a design pivot. It is not yet decided whether this pivot is worth taking.

## Decision

> This section records the decision that was made.
> It is best to record as much info as possible from the discussion that happened. This aids in not having to go back to the Pull Request to get the needed information.

TODO

## Detailed Design

TODO

## Status

Proposed

## Consequences

### Positive

TODO

### Negative

TODO

### Neutral

TODO

## References

1. <https://github.com/celestiaorg/optimint/issues/133>
