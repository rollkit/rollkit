# Block and Header Validity

## Abstract

Like all blockchains, rollups are defined as the chain of **valid** blocks from the genesis, to the head. Thus, the block and header validity rules define the chain.

Verifying a block / header is done in 3 parts:

1. Verify correct serialization according to the protobuf spec

2. Perform basic validation of the types

3. Perform verification of the new block against the previous accepted block

## Basic Validation

Each type contains a `.ValidateBasic()` method, which verifies that certain basic invariants hold. The `ValidateBasic()` calls are nested, starting from the `Block` struct, all the way down to each subfield.

The nested basic validation, and validation checks, are called as follows:

```go
Block.ValidateBasic()
  // Make sure the block's SignedHeader passes basic validation
  SignedHeader.ValidateBasic()
    // Make sure the SignedHeader's Header passes basic validation
    Header.ValidateBasic()
	  verify ProposerAddress not nil
	// Make sure the SignedHeader's Commit passes basic validation
	Commit.ValidateBasic()
	  // Ensure that someone signed the block
	  verify len(c.Signatures) not 0
	If sh.Validators is nil, or len(sh.Validators.Validators) is 0, assume based rollup, pass validation, and skip all remaining checks.
	Validators.ValidateBasic()
	  // github.com/rollkit/cometbft/blob/main/types/validator.go#L37
	  verify sh.Validators is not nil, and len(sh.Validators.Validators) != 0
	  // apply basic validation to all Validators
	  for each validator:
	    validator.ValidateBasic()
		  validate not nil
		  validator.PubKey not nil
		  validator.VotingPower >= 0
		  validator.Address == correct size
	  // apply ValidateBasic to the proposer field:
	  sh.Validators.Proposer.ValidateBasic()
		validate not nil
		validator.PubKey not nil
		validator.VotingPower >= 0
		validator.Address == correct size
    Assert that SignedHeader.Validators.Hash() == SignedHeader.AggregatorsHash
    Assert that len(SignedHeader.Commit.Signatures) == 1 (Exactly one signer check)
	Verify the 1 signature
  Data.ValidateBasic() // always passes
  // make sure the SignedHeader's DataHash is equal to the hash of the actual data in the block.
  Data.Hash() == SignedHeader.DataHash
```

## Verification Against Previous Block

```go
// code does not match spec: see https://github.com/rollkit/rollkit/issues/1277
Block.Verify()
  SignedHeader.Verify(untrustH *SignedHeader)
    // basic validation removed in #1231, because go-header already validates it
    //untrustH.ValidateBasic()
	Header.Verify(untrustH *SignedHeader)
	  if untrustH.Height == h.Height + 1, then apply the following check:
	    untrstH.AggregatorsHash[:], h.NextAggregatorsHash[:]
	if untrustH.Height > h.Height + 1:
	  soft verification failure	
	// We should know they're adjacent now,
	// verify the link to previous.
	untrustH.LastHeaderHash == h.Header.Hash()
	// Verify LastCommit hash
	untrustH.LastCommitHash == sh.Commit.GetCommitHash(...)
	
```

## [Block](https://github.com/rollkit/rollkit/blob/main/types/block.go#L26)

| **Field Name** | **Valid State**                         | **Validation**                     |
|----------------|-----------------------------------------|------------------------------------|
| SignedHeader   | Header of the block, signed by proposer | (See SignedHeader)                 |
| Data           | Transaction data of the block           | Data.Hash == SignedHeader.DataHash |

## [SignedHeader](https://github.com/rollkit/rollkit/blob/main/types/signed_header.go#L16)

| **Field Name** | **Valid State**                                                          | **Validation**                                                                              |
|----------------|--------------------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| Header         | Valid header for the block                                               | `Header` passes `ValidateBasic()` and `Verify()`                                            |
| Commit         | 1 valid signature from the expected proposer                             | `Commit` passes `ValidateBasic()`, with additional checks in `SignedHeader.ValidateBasic()` |
| Validators     | Array of Aggregators, should be length 1 (or zero for based rollup case) | `Validators` passes `ValidateBasic()`                                                       |

## [Header](https://github.com/rollkit/rollkit/blob/main/types/header.go#L25)

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
| ProposerAddress     | Address of the expected proposer                                                           | checked in the `Verify()` step          |
| AggregatorsHash     | Matches the NextAggregatorsHash of the previous accepted block                             | checked in the `Verify()` step          |
| NextAggregatorsHash | Set during block execution, according to the ABCI app                                      | checked during block execution        |

## [Commit](https://github.com/rollkit/rollkit/blob/main/types/block.go#L48)

| **Field Name** | **Valid State**                                         | **Validation**             |
|----------------|---------------------------------------------------------|----------------------------|
| Signatures     | Array containing a signature from the expected proposer | checked in `ValidateBasic()`,  signature verification occurs in `SignedHeader.ValidateBasic()` |

## [ValidatorSet](https://github.com/cometbft/cometbft/blob/main/types/validator_set.go#L51)

| **Field Name** | **Valid State**                                                 | **Validation**              |
|--------------|-----------------------------------------------------------------|-----------------------------|
| Validators   | Array of validators, each must pass `Validator.ValidateBasic()` | `Validator.ValidateBasic()` |
| Proposer    | Must pass `Validator.ValidateBasic()`                           | `Validator.ValidateBasic()` |
