Decentralized Sequencing Hackathon
==================================


This document describes the implementation plan for a decentralized sequencer
using a simple Round Robin sequencer selection.

TODO
====

* Deploy cosmos-sdk chain with staking module

* Research staking module

* Validate signatures by using staking module

* Initialize with genesis state

* Next sequencer selection using Round Robin

## Query Staking Module
How to query the staking module:

Show all commands:
```
gmd query staking --help
```
Show all validators
```
gmd query staking validators
```
Show specific validator
```
gmd query staking validator [validator address]
```

## Implementation

* Query validators for pubkeys, staking power etc

* Instantiate ValidatorSet from validators

* Use Proposer to create next block

* Add SequencerSet (instance of ValidatorSet) to Manager

* Query stake using gRPC:

    gmd query staking

* Create NewValidatorSet see: https://github.com/tendermint/tendermint/blob/64747b2b184184ecba4f4bffc54ffbcb47cfbcb0/types/validator_set_test.go#L188

* Use tendermint types for ValidatorSet

* Commit + test
