# Staking (Differences from Tendermint)

## Abstract

Rollkit will support customizable functionality for verifying the correctness of a block proposer. However, it currently works by delegating this functionality to the Application, by checking that the `AggregatorsHash` of a header corresponds to the `NextAggregatorsHash` of the previous, where setting the `NextAggregtorsHash` is the responsibility of the ABCI Application.

## Assumptions and Considerations

Assumes that the ABCI Application (such as apps built with Cosmos-SDK), has correct code for setting the `Validators` field in `SignedHeader`, and `NextAggregatorsHash` `AggregatorsHash` fields in `Header`, according to the rollup's desired sequencer scheme.

## Implementation

[types/header.go](https://github.com/rollkit/rollkit/blob/main/types/header.go#L86)
