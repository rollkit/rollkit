# ADR 007: Commit to Shares in Header

## Changelog

- 2021-10-01: Initial draft

## Context

1. Only [a single data root must be included in the Celestia block header](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/data_structures.md#header); if more than one data root is included, light nodes will not have any guarantees that the data behind the other data roots is available unless they run a separate sampling process per data root.
1. [PayForMessaage transactions](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/data_structures.md#signedtransactiondatapayformessage) include a commitment of roots of subtrees of the Celestia data root. This is a requirement for compact proofs that a message was or was not included correctly.
1. [Over the wire](https://github.com/celestiaorg/celestia-specs/blob/master/src/specs/networking.md#wiretxpayformessage), PayForMessage transactions include (potentially) multiple signatures for the same message with different sizes of Celestia block, to allow for [non-interactive message inclusion](https://github.com/celestiaorg/celestia-specs/blob/master/src/rationale/message_block_layout.md#non-interactive-default-rules).

Rollup blocks must follow a similar strategy to the above. Specifically, the data root in rollup block headers [must commit to subtree roots in the Celestia data root](https://github.com/rollkit/rollkit/issues/133), otherwise rollup nodes would not have any guarantees that the data behind the data root in a rollup block header is available.

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

The detailed design consists of the following components:

1. **Rollup Transaction Layout**: Rollup transactions are formatted and aligned to start at power-of-2 share boundaries. This ensures consistent commitment structure regardless of the data square size.

2. **Footer Approach**: The rollup block header is appended after the transactions as a footer, rather than preceding them. This allows for:
   - Preserving the power-of-2 alignment requirement for transaction data
   - Enabling rollup nodes to download the last X shares of each message to extract the header/footer for verification

3. **Commitment Structure**: The rollup block header includes a commitment to the transactions in share form, similar to the structure used in PayForMessage. The commitment uses Merkle subtree roots that correspond to the shares containing the transaction data.

4. **Modified WirePayForMessage**: The WirePayForMessage structure is extended with a new `footers` field that contains a footer for each witness. When verifying a witness, the corresponding footer is appended to the message.

5. **Verification Process**:
   - Rollup nodes verify the commitment in the footer against the actual transaction data
   - Full nodes on Celestia verify that the PayForMessage transaction correctly commits to both the rollup transactions and the footer

This design ensures data availability guarantees while maintaining compatibility with different data square sizes and the non-interactive message inclusion mechanism.

## Status

Proposed

## Consequences

### Positive

- Maintains data availability guarantees for rollup data by ensuring proper commitment to subtree roots
- Allows for seamless handling of different block sizes without requiring multiple different messages over the wire
- Preserves the non-interactive message inclusion capability
- Simplifies the verification process for rollup nodes by having a predictable location (end of message) for the header/footer
- Reduces networking overhead compared to alternative approaches
- Maintains backward compatibility with Celestia's existing PayForMessage structure with minimal modifications

### Negative

- Slightly increases complexity by using a footer approach rather than the more traditional header-first design
- Requires modifications to the WirePayForMessage structure to include the new footers field
- May increase storage requirements by potentially requiring multiple footers for the same data (one per possible square size)
- Introduces a small verification overhead as nodes need to process both the message and its footer

### Neutral

- Changes the conceptual model from header-first to footer-last, which is different from many blockchain designs but functionally equivalent
- Shifts some verification responsibility from Celestia's consensus layer to the rollup verification layer
- Requires coordination between Celestia and rollup teams for implementation

## References

1. <https://github.com/rollkit/rollkit/issues/133>
