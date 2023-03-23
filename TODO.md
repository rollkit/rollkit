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
