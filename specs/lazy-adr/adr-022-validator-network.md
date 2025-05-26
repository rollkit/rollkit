# 022 Validator / Attester Network — Push-Sign-Collect

Date: 2025-05-25
Status: Draft

## Context

The rollup currently relies on one centralized sequencer to order and publish blocks. Down-stream chains connected via IBC have no cryptographic evidence that other parties have examined those blocks before they propagate, producing a single point of failure and attack.

## Decision

### High-level workflow

 1. Block broadcast — For every height h the sequencer broadcasts the canonical BlockBundle(h) (header, transactions, state root) to all active attesters over gRPC/WebSocket.
 2. Local verification — Each attester independently:
 • validates header → parent → state transition;
 • (optionally) re-executes the block using a connected full node;
 • signs bytesToSign = SHA-256(height || blockHash || stateRoot) with its private key.
 3. Signature return — The attester sends SubmitSignature{height, blockHash, pubKey, signature} back to the sequencer (one-way unary gRPC).
❏  5 s retry w/ exponential back-off.
 4. Aggregation & quorum — The sequencer collects signatures until ≥ ⅔ of current bonded voting power have signed (Cosmos-SDK staking module).
v0: embed bitmap + ordered Ed25519 signatures.
v1: support aggregated BLS signature once key migration is complete.
 5. Final block commit — Sequencer writes the quorum attestation into BlockHeader.ValidatorHash and gossips the block via IBC.

### Signing schemes

Version Crypto Encoding in header Verification cost on IBC chain
v0 (MVP) Ed25519 (Cosmos default) Attestation{bitArray, []Signature} O(#signers) Ed25519 checks
v1 BLS12-381 AggSig{aggregate, bitmap} 1 pairing check
v2 (optional) Dual-mode (Ed25519 ∥ BLS) Two parallel attestations, consumer picks Compatible with all relayers

Key generation — Every validator stores an Ed25519 signing key today. A BLS key can be added via a MsgAddBLSKey transaction once v1 is activated.

### Validator set & staking integration

 • The staking module remains the single source of truth for validator membership, voting power and slashing.
 • Create / Edit / Unbond transactions work unchanged. The EndBlocker outputs a ValidatorSetUpdate event that both the sequencer and attesters subscribe to.
 • Joining the Attester Network
– When a validator becomes bonded it must run the attester binary, configured with its signing key(s) and sequencer address.
– Non-participating bonded validators are jailed & slashed by the existing missed-signature mechanism.
 • Leaving
– When a validator unbonds or is slashed below MinSelfDelegation, the sequencer drops its pubKey from the next height’s validator set bitmap. Any late signatures are ignored.

2.4  Quorum and liveness
 • Quorum rule: signedVotingPower ≥ 2/3 * totalVotingPower
 • Timeouts
– AttesterTimeout = 2 s after broadcast.
– AggregationTimeout = 5 s; sequencer can still commit if quorum not met only in EmergencyMode (governance toggle) to avoid total halt.
 • Safety vs. liveness — Because verification is local and deterministic, equivocation is impossible: the worst failure mode is not reaching quorum (→ halt) which staking incentives should discourage.

## Architecture & Interfaces

```mermaid
graph TD
    SQ[Sequencer] -- gRPC broadcast --> A1[Attester 1]\nVerify & Sign
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
 • Forward compatibility — BLS upgrade path leaves Ed25519 untouched so IBC light clients that can’t do pairings still work.

## Consequences

### Easier

 • Removes RAFT complexity (no election timers, snapshots, log compaction).
 • Latency drops to 1 broadcast + 1 unary round-trip per block.
 • Natural fit with Cosmos validator-set semantics and existing slashing.

### Harder

 • Sequencer must handle DoS on /SubmitSignature endpoint.
 • Validator set changes require tight syncing between staking events and attester configs.
 • Bitmap + signatures can bloat header size at very large validator sets (mitigated by BLS upgrade).

## Future work

 1. BLS aggregated signatures — governance proposal & CLI tool to add BLS keys, plus pairing verification gadget for IBC.
 2. Multi-sequencer fail-over — once fast-leader-election is required we can revisit consensus (e.g., HotStuff) purely for sequencer rotation.
 3. Light-client proofs — expose AttestationProof object so external bridges can verify signedVotingPower without full header.
 4. Bundle attestation & DA availability proof to offer optimistic fast-finality bridges.

## Appendix A — gRPC proto (excerpt)

```proto
service AttesterService {
  rpc BroadcastBlock(stream BlockBundle) returns (stream Empty) {}
  rpc SubmitSignature(SignatureMsg) returns (SubmitSignatureResponse) {}
}

message BlockBundle {
  uint64  height      = 1;
  bytes   block_hash  = 2;
  bytes   state_root  = 3;
  bytes   header      = 4; // protobuf-encoded header
  bytes   txs         = 5; // amino bz2 concatenated
}

message SignatureMsg {
  uint64  height     = 1;
  bytes   block_hash = 2;
  bytes   pub_key    = 3;
  bytes   signature  = 4; // ed25519 or bls
}

message SubmitSignatureResponse {
  bool ack = 1;
}
```
