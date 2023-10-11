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
// (Block.Verify is currenty unimplemented)
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

## [Header](https://github.com/rollkit/rollkit/blob/main/types/header.go#L25)

## [Commit](https://github.com/rollkit/rollkit/blob/main/types/block.go#L48)

## [ValidatorSet](https://github.com/cometbft/cometbft/blob/main/types/validator_set.go#L51)
