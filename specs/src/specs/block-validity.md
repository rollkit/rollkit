# Block and Header Validity

## Abstract

Like all blockchains, rollups are defined as the chain of **valid** blocks from the genesis, to the head. Thus, the block and header validity rules define the chain.

Verifying a block/header is done in 3 parts:

1. Verify correct serialization according to the protobuf spec

2. Perform basic validation of the types

3. Perform verification of the new block against the previously accepted block

Rollkit uses a header/data separation architecture where headers and data can be validated independently. The system has moved from a multi-validator model to a single signer model for simplified sequencer management.

## Basic Validation

Each type contains a `.ValidateBasic()` method, which verifies that certain basic invariants hold. The `ValidateBasic()` calls are nested for each structure.

### SignedHeader Validation

```go
SignedHeader.ValidateBasic()
  // Make sure the SignedHeader's Header passes basic validation
  Header.ValidateBasic()
    verify ProposerAddress not nil
  // Make sure the SignedHeader's signature passes basic validation
  Signature.ValidateBasic()
    // Ensure that someone signed the header
    verify len(c.Signatures) not 0
  // For based rollups (sh.Signer.IsEmpty()), pass validation
  If !sh.Signer.IsEmpty():
    // Verify the signer matches the proposer address
    verify sh.Signer.Address == sh.ProposerAddress
    // Verify signature using custom verifier if set, otherwise use default
    if sh.verifier != nil:
      verify sh.verifier(sh) == nil
    else:
      verify sh.Signature.Verify(sh.Signer.PubKey, sh.Header.MarshalBinary())
```

### SignedData Validation

```go
SignedData.ValidateBasic()
  // Always passes basic validation for the Data itself
  Data.ValidateBasic() // always passes
  // Make sure the signature is valid
  Signature.ValidateBasic()
    verify len(c.Signatures) not 0
  // Verify the signer
  If !sd.Signer.IsEmpty():
    verify sd.Signature.Verify(sd.Signer.PubKey, sd.Data.MarshalBinary())
```

### Block Validation

Blocks are composed of SignedHeader and Data:

```go
// Block validation happens by validating header and data separately
// then ensuring data hash matches
verify SignedHeader.ValidateBasic() == nil
verify Data.Hash() == SignedHeader.DataHash
```

## Verification Against Previous Block

```go
SignedHeader.Verify(untrustedHeader *SignedHeader)
  // Basic validation is handled by go-header before this
  Header.Verify(untrustedHeader)
    // Verify height sequence
    if untrustedHeader.Height != h.Height + 1:
      if untrustedHeader.Height > h.Height + 1:
        return soft verification failure
      return error "headers are not adjacent"
    // Verify the link to previous header
    verify untrustedHeader.LastHeaderHash == h.Header.Hash()
    // Verify LastCommit hash matches previous signature
    verify untrustedHeader.LastCommitHash == sh.Signature.GetCommitHash(...)
    // Note: ValidatorHash field exists for compatibility but is not validated
```

## [Data](https://github.com/rollkit/rollkit/blob/main/types/data.go)

| **Field Name** | **Valid State**                         | **Validation**                     |
|----------------|-----------------------------------------|------------------------------------|
| Txs            | Transaction data of the block           | Data.Hash() == SignedHeader.DataHash |
| Metadata       | Optional p2p gossiping metadata         | Not validated                       |

## [SignedHeader](https://github.com/rollkit/rollkit/blob/main/types/signed_header.go)

| **Field Name** | **Valid State**                                                          | **Validation**                                                                              |
|----------------|--------------------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| Header         | Valid header for the block                                               | `Header` passes `ValidateBasic()` and `Verify()`                                            |
| Signature      | Valid signature from the single sequencer                                | `Signature` passes `ValidateBasic()`, verified against signer                               |
| Signer         | Information about who signed the header                                  | Must match ProposerAddress if not empty (based rollup case)                                |
| verifier       | Optional custom signature verification function                          | Used instead of default verification if set                                                 |

## [Header](https://github.com/rollkit/rollkit/blob/main/types/header.go)

***Note***: Rollkit has moved to a single signer model. The multi-validator architecture has been replaced with a simpler single sequencer approach.

| **Field Name**      | **Valid State**                                                                            | **Validation**                        |
|---------------------|--------------------------------------------------------------------------------------------|---------------------------------------|
| **BaseHeader** .    |                                                                                            |                                       |
| Height              | Height of the previous accepted header, plus 1.                                            | checked in the `Verify()`` step          |
| Time                | Timestamp of the block                                                                     | Not validated in Rollkit              |
| ChainID             | The hard-coded ChainID of the chain                                                        | Should be checked as soon as the header is received |
| **Header** .        |                                                                                            |                                       |
| Version             | unused                                                                                     |                                       |
| LastHeaderHash      | The hash of the previous accepted block                                                    | checked in the `Verify()`` step          |
| LastCommitHash      | The hash of the previous accepted block's commit                                           | checked in the `Verify()`` step          |
| DataHash            | Correct hash of the block's Data field                                                     | checked in the `ValidateBasic()`` step   |
| ConsensusHash       | unused                                                                                     |                                       |
| AppHash             | The correct state root after executing the block's transactions against the accepted state | checked during block execution        |
| LastResultsHash     | Correct results from executing transactions                                                | checked during block execution        |
| ProposerAddress     | Address of the expected proposer                                                           | Must match Signer.Address in SignedHeader |
| ValidatorHash       | Compatibility field for Tendermint light client                                            | Not validated                             |

## [Signer](https://github.com/rollkit/rollkit/blob/main/types/signed_header.go)

The Signer type replaces the previous ValidatorSet for single sequencer operation:

| **Field Name** | **Valid State**                                                 | **Validation**              |
|----------------|-----------------------------------------------------------------|-----------------------------|
| PubKey         | Public key of the signer                                        | Must not be nil if Signer is not empty |
| Address        | Address derived from the public key                             | Must match ProposerAddress              |
