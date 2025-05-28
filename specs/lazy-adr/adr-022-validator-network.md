# 022 Validator Network

Date: 2025-05-25
Status: Draft

## Context

When a single sequencer is used there is a limited design space for the token and a limited set of security guarantees. The validator network offers an alternative to using a full consensus protocol, but offers security guarantees with more than one participant verifying the execution and ordering.

The validator network acts as an extra security layer and soft confirmation enabling the rollup to move faster than the underlying DA layer. Secondly a validator network introduces the opportunity to do more with the token of the chain.

The original design and implementation was centered around IBC and adding an extra layer of security for counter party chains, so that the user is not solely trusting the sequencer to act correctly

## Decision

Rollkit will introduce a validator network in which there will be a set of validators verifying execution and construction.

- The design is centered around the proposer producing blocks as fast as possible but asking for signatures after fact. This design is optimized for block production performance. The validators will need to submit there attestations as a transaction to the state machine before the end of the epoch. If a validator does not submit their attesttation within the epoch, they will not be slashed but instead they will not get a reward.

### High-level workflow

 1. Block broadcast — For every height h the sequencer broadcasts the canonical BlockBundle(h) (header, transactions, state root) to all active attesters over gRPC/WebSocket.
 2. Local verification — Each attester independently:
 • validates header → parent → state transition;
 • (optionally) re-executes the block using a connected full node;
 • signs bytesToSign = SHA-256(height || blockHash || stateRoot) with its private key.
 3. Signature transaction submission — The attester sends vote as a transaction to the state machine. It will be included in subsequent blocks
 4. Aggregation & quorum — The attester module or contract collects signatures until ≥ ⅔ of current bonded voting power have signed providing a soft confirmation of the block
    - If qurorom is not met at the epoch boundry, the network will halt waiting for the signatures.
 5. Final block commit — After the block is included in the DA layer it will be considered to have a hard confirmation.

### Signing schemes

Different signature schemes can be doused in conjunction with the validator network. To start we will support ED25519 and later one we plan on adding other signature schemes based on how user demand requires.

Some potential future additions could be BLS12-381 aggregate and/or a BLS threshold signature.

### Validator set & staking integration

The attester layer can plug into different validator‑set providers. Below we outline the existing Cosmos‑SDK flow and an alternative Reth / EVM flow patterned after UniChain’s staking design. Both share the same quorum rule (≥ ⅔ voting power) and slashing philosophy.

#### Cosmos‑SDK

For the Cosmos SDK the attester system will be located in the ABCI execution environment.

- The staking module is the single source of truth for membership and voting power.
- Create / Edit / Unbond msgs emit ValidatorSetUpdate events every block; sequencer & attesters rebuild the bitmap each height.
- Joining — once bonded, a validator runs the attester daemon.
- Leaving — when voting power reaches 0 the sequencer drops the validator from the next bitmap, ignoring late sigs.

The design adds a dedicated x/attestation Cosmos-SDK module that intercepts every IBC message in the AnteHandler, parks it in a state-kept queue, and releases it only after ≥ ⅔ validator voting-power has submitted attestations. Validators sign a hash of the queued tx; the module tracks signatures with a compact bitmap, finalises transactions in EndBlock (executing them through the normal IBC router), rewards timely signers, and optionally jails low-participation validators. Parameters cover quorum fraction, attestation window, pruning window, and queue size, keeping the solution pluggable and light-touch on existing IBC code.

#### Reth/EVM Rollup

- Stake manager contract holds the validator stake/weight and maps an address to a key. It will emit `StakeSnapshot(epoch)` events that will be consumed by the consensus client.
- Stake mirror listens for staking snapshot events in order to re build the validtor set. The proposer will always be the same, we do not support rotation at this time. Once the validator set is rebuilt any changes that are witnessed will be applied to the validator network.
- The EVM will work in the non blocking way. The validators will be able to join and leave as they please with the requirement that they submit attestations of execution in order to provide a soft confirmation within an epoch if they would like a reward for their work.

Solidity Contract

```sol
contract StakeManager {
    struct Validator { uint96 power; bytes32 edKey; bytes blsKey; }
    mapping(address => Validator) public validators;

    function stake(uint96 amount, bytes32 edKey, bytes calldata blsKey) external;
    function unstake(uint96 amount) external;
    function slash(address val, uint96 amt) external /* onlyEvidence */;
    function snapshot() external returns (bytes32 root); // called by sequencer each epoch
}
```

### Quorum and liveness

 • Quorum rule: signedVotingPower ≥ 2/3 * totalVotingPower
 • Timeouts
– AttesterTimeout = 2 s after broadcast.
– AggregationTimeout = 5 s; sequencer can still commit if quorum not met only in EmergencyMode (governance toggle) to avoid total halt.
 • Safety vs. liveness — Because verification is local and deterministic, equivocation is impossible: the worst failure mode is not reaching quorum (→ halt) which staking incentives should discourage.

## Architecture & Interfaces

```mermaid
graph TD
    SQ[Sequencer] -- gRPC broadcast --> A1[Attester 1]
    SQ -- gRPC --> A2[Attester 2]
    SQ -- gRPC --> A3[Attester N]
    A1 -- SubmitSignature --> SQ
    A2 -- SubmitSignature --> SQ
    A3 -- SubmitSignature --> SQ
    SQ -- quorum aggregate --> HB[Block h header with Attestation]
    HB --> IBC[IBC Relayer]
```

### Sequencer additions

 • /broadcastBlock (server-stream): pushes BlockBundle(h) to every connected attester.
 • /SubmitSignature (rpc): receives SignatureMsg.
 • Aggregation cache: map[height]SignatureAccumulator with bitset of pubkeys.
 • State hooks: listens to staking.ValidatorSetUpdate to rebuild ActiveValidatorBitmap each height.

### Attester service

 • Conn manager — maintains persistent stream to /broadcastBlock and unary client to /SubmitSignature.
 • Verifier pipeline:

 1. basic header checks;
 2. optional re-execution (--verify-full=true flag);
 3. produce signature;
 4. async submit with retries.
 • Metrics / Prom: per-block verification time, submit latency, signed / missed counter.

## Security considerations

 • Double-sign protection — Deterministic bytesToSign makes replay impossible; Ed25519 prevents malleability.
 • Slashing — Existing Cosmos evidence (MsgEvidence) for missed or duplicate signatures applies unchanged.
 • Sybil resistance — validator power is staked; no separate token.

## Consequences

- Increased code complexity, more to maintain

## Future work

- Multi-sequencer fail-over — once fast-leader-election is required we can revisit consensus (e.g., HotStuff) purely for sequencer rotation.
- Light-client proofs — expose AttestationProof object so external bridges can verify signedVotingPower without full header.
- Bundle attestation & DA availability proof to offer optimistic fast-finality bridges.
