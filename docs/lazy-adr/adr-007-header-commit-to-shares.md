# ADR 007: Commit to Shares in Header

## Changelog

- 2021-10-01: Initial draft

## Context

1. Only [a single data root must be included in the Celestia block header](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/data_structures.md#header); if more than one data root is included, light nodes will not have any guarantees that the data behind the other data roots is available unless they run a separate sampling process per data root.
1. [PayForMessaage transactions](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/data_structures.md#signedtransactiondatapayformessage) include a commitment of roots of subtrees of the Celestia data root. This is a requirement for compact proofs that a message was or was not included correctly.
1. [Over the wire](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/networking.md#wiretxpayformessage), PayForMessage transactions include (potentially) multiple signatures for the same message with different sizes of Celestia block, to allow for [non-interactive message inclusion](https://github.com/celestiaorg/celestia-specs/blob/master/src/rationale/message_block_layout.md#non-interactive-default-rules).

Rollup blocks must follow a similar strategy to the above. Specifically, the data root in rollup block headers [must commit to subtree roots in the Celestia data root](https://github.com/celestiaorg/optimint/issues/133), otherwise rollup nodes would not have any guarantees that the data behind the data root in a rollup block header is available.

## Alternative Approaches

### Alternative 1: Don't Commit to Data Root in Rollup Block Header

One proposal is to simply not include any commitment to data in the rollup block header, and use the `MessageShareCommitment` field in the respective PayForMessage transaction to commit to _both_ the rollup block header and rollup block data.

This may only work securely under certain scenarios, but not in the general case. Since rollup block data is not committed to in the rollup block header, it will not be signed over by the rollup block producer set (e.g. if the rollup block producers are using a Tendermint-like protocol) and can therefore be malleated by these parties.

### Alternative 2: One Message Per Layout

Another proposal is having the rollup block header commit to subtree roots. However, since the layout of shares (and thus, the subtree structures) might change depending on the size of the data square, this means the message that is paid for in PayForMessage is different depending on the layout (since the message includes the rollup block header).

A different message can be included per witness for PayForMessage transactions over the wire. This would lead to unacceptably high networking overhead unfortunately, since messages are expected to be quite large individually.

### Alternative 3: Auction Off Rows Instead of Shares

Instead of auctioning off individual shares, rows can be auctioned off entirely. In other words, only a single message can be included per row (though a message may span more than one row), and always begins at the start of a row. This will also simplify various codepaths that no longer need to account for the potential of multiple messages and namespaces per row.

This would not solve anything since the size of the square will still change, and is also a design pivot. It is not yet decided whether this pivot is worth taking.

An alternative to this alternative would be to make the block size fixed. Combined with auctioning off rows, this would enormously simplify message inclusion logic.

## Decision

Resolving this issue is accomplished with two components.

First, the rollup transactions are committed to _in share form and layout_ in the rollup block header, similar to the commitment in PayForMessage. For this, only rollup transactions are committed to, and begin aligned with power of 2 shares. This commitment will be different than the one in the PayForMessage transaction that pays for this message since that commits to both the rollup transactions and rollup block header. A new rollup block header and commitment is constructed for each of the different block sizes possible.

To preserve the property that rollup transactions begin aligned at a power of 2 boundary, we append the rollup block header _after_ the transactions, effectively turning it into a footer. Instead of downloading the first X shares of each message to extract the header for DoS prevention, rollup nodes will instead download the last X shares of each message.

In order to avoid having to include many different messages with PayForMessage over the wire, we modify the WirePayForMessage structure to include a new field, `footers`. The number of footers must be equal to the number of witnesses. When verifying each witness, the associated footer is appended to the message, which combined is the effective message.

![Proposal.](figures/header_shares_commit.jpg)

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
