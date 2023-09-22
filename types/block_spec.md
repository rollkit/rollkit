# Block and Header Validity

## Abstract
Like all blockchains, rollups are composed of a chain of blocks from the rollup's genesis to its head, each consisting of metadata in the header, and transaction data in the body.

## Verification
Both full and light nodes perform validation of the block headers, full nodes additionally perform manual verification of the transaction data in the block body as well.

## Block specification
Blocks are as follows:
```
type Block struct {
	[SignedHeader](#signed-header-specification) SignedHeader
	Data         Data
}
```

## Signed Header Specification

```
type SignedHeader struct {
	Header
	Commit     Commit
	Validators *cmtypes.ValidatorSet
}
```