# CHANGELOG

## v0.1.1 | 08.03.2022

Minor bugfix release, to ensure that Optimint uses the same version of gRPC as Cosmos SDK.

### BUG FIXES

- Use google.golang.org/grpc v1.33.2 to be compatible with cosmos-sdk ([#315](https://github.com/celestiaorg/optimint/pull/315)) [@jbowen93](https://github.com/jbowen93/)
- Make TestValidatorSetHandling even more stable ([#314](https://github.com/celestiaorg/optimint/pull/314)) [@tzdybal](https://github.com/tzdybal/)

## v0.1.0 | 07.03.2022

This is the first Optimint release.
Optimint supports all ABCI methods and all Tendermint RPCs.

### FEATURES

- Minimal implementation of ConsensusParams method  ([#292](https://github.com/celestiaorg/optimint/pull/292)) [@tzdybal](https://github.com/tzdybal/)
- Implement GenesisChunked method  ([#287](https://github.com/celestiaorg/optimint/pull/287)) [@mauriceLC92](https://github.com/mauriceLC92/)
- Minimalistic validator set handling  ([#286](https://github.com/celestiaorg/optimint/pull/286)) [@tzdybal](https://github.com/tzdybal/)
- Implement BlockChainInfo RPC method  ([#282](https://github.com/celestiaorg/optimint/pull/282)) [@Raneet10](https://github.com/Raneet10/)
- ConsensusState/DumpConsensusState implementation  ([#273](https://github.com/celestiaorg/optimint/pull/273)) [@tzdybal](https://github.com/tzdybal/)
- Tx Method implementation  ([#272](https://github.com/celestiaorg/optimint/pull/272)) [@mauriceLC92](https://github.com/mauriceLC92/)
- Implement BlockResults RPC function  ([#263](https://github.com/celestiaorg/optimint/pull/263)) [@tzdybal](https://github.com/tzdybal/)
- Implement Commit and BlockSearch  ([#258](https://github.com/celestiaorg/optimint/pull/258)) [@Raneet10](https://github.com/Raneet10/)
- BlockByHash function implementation  ([#256](https://github.com/celestiaorg/optimint/pull/256)) [@mauriceLC92](https://github.com/mauriceLC92/)
- Implement NumUnconfirmedTxs RPC call  ([#255](https://github.com/celestiaorg/optimint/pull/255)) [@tzdybal](https://github.com/tzdybal/)
- RPC: subscription methods  ([#252](https://github.com/celestiaorg/optimint/pull/252)) [@tzdybal](https://github.com/tzdybal/)
- Tendermint URI RPC  ([#224](https://github.com/celestiaorg/optimint/pull/224)) [@tzdybal](https://github.com/tzdybal/)
- Create CHANGELOG.md, CHANGELOG-PENDING.md, and corresponding GH action  ([#203](https://github.com/celestiaorg/optimint/pull/203)) [@jbowen93](https://github.com/jbowen93/)
- Block and Tx indexing backend for optimint  ([#202](https://github.com/celestiaorg/optimint/pull/202)) [@mattdf](https://github.com/mattdf/)
- Tx Events  ([#193](https://github.com/celestiaorg/optimint/pull/193)) [@tzdybal](https://github.com/tzdybal/)
- Block RPC  ([#187](https://github.com/celestiaorg/optimint/pull/187)) [@tzdybal](https://github.com/tzdybal/)
- Set ChainID in ABCI Header  ([#185](https://github.com/celestiaorg/optimint/pull/185)) [@tzdybal](https://github.com/tzdybal/)
- Expose Tendermint HTTP RPC  ([#183](https://github.com/celestiaorg/optimint/pull/183)) [@tzdybal](https://github.com/tzdybal/)
- Create CODEOWNERS  ([#179](https://github.com/celestiaorg/optimint/pull/179)) [@tzdybal](https://github.com/tzdybal/)
- Add InitChain ABCI logic  ([#159](https://github.com/celestiaorg/optimint/pull/159)) [@tzdybal](https://github.com/tzdybal/)
- gRPC DALC and mock server  ([#158](https://github.com/celestiaorg/optimint/pull/158)) [@tzdybal](https://github.com/tzdybal/)
- Add batch for KVstore  ([#149](https://github.com/celestiaorg/optimint/pull/149)) [@Raneet10](https://github.com/Raneet10/)
- Add new ErrKeyNotFound  ([#148](https://github.com/celestiaorg/optimint/pull/148)) [@pmareke](https://github.com/pmareke/)
- Change mock implementation to use store.KVStore instead of maps  ([#146](https://github.com/celestiaorg/optimint/pull/146)) [@jbowen93](https://github.com/jbowen93/)
- Block sync  ([#139](https://github.com/celestiaorg/optimint/pull/139)) [@tzdybal](https://github.com/tzdybal/)
- ADR: header commits to shares  ([#138](https://github.com/celestiaorg/optimint/pull/138)) [@adlerjohn](https://github.com/adlerjohn/)
- On-disk storage  ([#122](https://github.com/celestiaorg/optimint/pull/122)) [@tzdybal](https://github.com/tzdybal/)
- Block Propagation  ([#92](https://github.com/celestiaorg/optimint/pull/92)) [@tzdybal](https://github.com/tzdybal/)
- Extend DA layer client interface  ([#83](https://github.com/celestiaorg/optimint/pull/83)) [@tzdybal](https://github.com/tzdybal/)
- Transaction aggregation  ([#82](https://github.com/celestiaorg/optimint/pull/82)) [@tzdybal](https://github.com/tzdybal/)
- lazyledger DA client implementation  ([#81](https://github.com/celestiaorg/optimint/pull/81)) [@tzdybal](https://github.com/tzdybal/)
- Serialization and Hashing  ([#79](https://github.com/celestiaorg/optimint/pull/79)) [@tzdybal](https://github.com/tzdybal/)
- Protobuf definition for Optimint types  ([#73](https://github.com/celestiaorg/optimint/pull/73)) [@tzdybal](https://github.com/tzdybal/)
- State and block execution  ([#58](https://github.com/celestiaorg/optimint/pull/58)) [@tzdybal](https://github.com/tzdybal/)
- Data Availability Submission API  ([#71](https://github.com/celestiaorg/optimint/pull/71)) [@tzdybal](https://github.com/tzdybal/)
- ADR: serialization  ([#59](https://github.com/celestiaorg/optimint/pull/59)) [@tzdybal](https://github.com/tzdybal/)
- Protobuf definition for Optimint types  ([#57](https://github.com/celestiaorg/optimint/pull/57)) [@tzdybal](https://github.com/tzdybal/)
- Block store  ([#42](https://github.com/celestiaorg/optimint/pull/42)) [@tzdybal](https://github.com/tzdybal/)
- Add core types  ([#41](https://github.com/celestiaorg/optimint/pull/41)) [@liamsi](https://github.com/liamsi/)
- Integrate Tendermint mempool  ([#34](https://github.com/celestiaorg/optimint/pull/34)) [@tzdybal](https://github.com/tzdybal/)
- Describe peer discovery in ADR  ([#33](https://github.com/celestiaorg/optimint/pull/33)) [@tzdybal](https://github.com/tzdybal/)
- Transaction gossiping  ([#29](https://github.com/celestiaorg/optimint/pull/29)) [@tzdybal](https://github.com/tzdybal/)
- Drop-in replacement of Tendermint Node  ([#13](https://github.com/celestiaorg/optimint/pull/13)) [@tzdybal](https://github.com/tzdybal/)
- Initial project setup  ([#12](https://github.com/celestiaorg/optimint/pull/12)) [@tzdybal](https://github.com/tzdybal/)
- Add design doc to readme  ([#9](https://github.com/celestiaorg/optimint/pull/9)) [@musalbas](https://github.com/musalbas/)

### IMPROVEMENTS

- Remove extra variable  ([#280](https://github.com/celestiaorg/optimint/pull/280)) [@Raneet10](https://github.com/Raneet10/)
- Replace tm-db dependency with store package  ([#268](https://github.com/celestiaorg/optimint/pull/268)) [@tzdybal](https://github.com/tzdybal/)
- Use enum instead of strings for DB type  ([#259](https://github.com/celestiaorg/optimint/pull/259)) [@adlerjohn](https://github.com/adlerjohn/)
- docs: unify entries format in CHANGELOG-PENDING.md  ([#221](https://github.com/celestiaorg/optimint/pull/221)) [@tzdybal](https://github.com/tzdybal/)
- ci: add more linters  ([#219](https://github.com/celestiaorg/optimint/pull/219)) [@tzdybal](https://github.com/tzdybal/)
- time.Sleep removal from tests  ([#178](https://github.com/celestiaorg/optimint/pull/178)) [@ntsanov](https://github.com/ntsanov/)
- Configuration  ([#170](https://github.com/celestiaorg/optimint/pull/170)) [@tzdybal](https://github.com/tzdybal/)
- Remove Handler from Gossiper  ([#167](https://github.com/celestiaorg/optimint/pull/167)) [@Raneet10](https://github.com/Raneet10/)
- Re-apply changes from #144  ([#154](https://github.com/celestiaorg/optimint/pull/154)) [@tzdybal](https://github.com/tzdybal/)
- add dependabot.yml  ([#105](https://github.com/celestiaorg/optimint/pull/105)) [@liamsi](https://github.com/liamsi/)
- Add valid link to Twitter badge  ([#103](https://github.com/celestiaorg/optimint/pull/103)) [@tzdybal](https://github.com/tzdybal/)
- Add go report card and Twitter badge to README.md  ([#102](https://github.com/celestiaorg/optimint/pull/102)) [@tzdybal](https://github.com/tzdybal/)
- Add validator to pubsub and stop gossiping transactions twice  ([#97](https://github.com/celestiaorg/optimint/pull/97)) [@evan-forbes](https://github.com/evan-forbes/)
- Extract gossiping logic into a type  ([#95](https://github.com/celestiaorg/optimint/pull/95)) [@tzdybal](https://github.com/tzdybal/)
- Rebrand: rename lazyledger to celestia  ([#91](https://github.com/celestiaorg/optimint/pull/91)) [@tzdybal](https://github.com/tzdybal/)
- Updated Store interface  ([#78](https://github.com/celestiaorg/optimint/pull/78)) [@tzdybal](https://github.com/tzdybal/)
- Submit signed transactions instead of messages  ([#76](https://github.com/celestiaorg/optimint/pull/76)) [@evan-forbes](https://github.com/evan-forbes/)
- Enable test workflow on all PRs  ([#72](https://github.com/celestiaorg/optimint/pull/72)) [@tzdybal](https://github.com/tzdybal/)
- Enable golangci-lint GitHub action  ([#43](https://github.com/celestiaorg/optimint/pull/43)) [@tzdybal](https://github.com/tzdybal/)
- Bring back address conversion method  ([#40](https://github.com/celestiaorg/optimint/pull/40)) [@tzdybal](https://github.com/tzdybal/)
- Get rid of reflect in mempool code  ([#39](https://github.com/celestiaorg/optimint/pull/39)) [@tzdybal](https://github.com/tzdybal/)
- Update lazy ADR 001  ([#31](https://github.com/celestiaorg/optimint/pull/31)) [@tzdybal](https://github.com/tzdybal/)
- Refactoring of P2P unit tests  ([#30](https://github.com/celestiaorg/optimint/pull/30)) [@tzdybal](https://github.com/tzdybal/)
- Use addresses in multiaddr format.  ([#19](https://github.com/celestiaorg/optimint/pull/19)) [@tzdybal](https://github.com/tzdybal/)

### BUG FIXES

- fix: make `TestValidatorSetHandling` stable ([#313](https://github.com/celestiaorg/optimint/pull/313)) [@tzdybal](https://github.com/tzdybal/)
- Fix linter on `main`  ([#308](https://github.com/celestiaorg/optimint/pull/308)) [@tzdybal](https://github.com/tzdybal/)
- Fix multiple bugs for Ethermint  ([#305](https://github.com/celestiaorg/optimint/pull/305)) [@tzdybal](https://github.com/tzdybal/)
- fix: map LastBlockID.Hash to LastHeaderHash in conversion  ([#303](https://github.com/celestiaorg/optimint/pull/303)) [@tzdybal](https://github.com/tzdybal/)
- fix: do save ABCI responses for blocks  ([#285](https://github.com/celestiaorg/optimint/pull/285)) [@tzdybal](https://github.com/tzdybal/)
- fix: use KeyCopy instead of Key in BadgerIterator  ([#274](https://github.com/celestiaorg/optimint/pull/274)) [@tzdybal](https://github.com/tzdybal/)
- Fix missed breaks in selects  ([#265](https://github.com/celestiaorg/optimint/pull/265)) [@adlerjohn](https://github.com/adlerjohn/)
- Fix ineffective breaks  ([#262](https://github.com/celestiaorg/optimint/pull/262)) [@adlerjohn](https://github.com/adlerjohn/)
- Break out of loop instead of doing nothing  ([#260](https://github.com/celestiaorg/optimint/pull/260)) [@adlerjohn](https://github.com/adlerjohn/)
- fix: gofmt block/manager.go and remove typo  ([#222](https://github.com/celestiaorg/optimint/pull/222)) [@tzdybal](https://github.com/tzdybal/)
- Actually fix a ChainID issue  ([#186](https://github.com/celestiaorg/optimint/pull/186)) [@tzdybal](https://github.com/tzdybal/)
- Fix typos in node/node.go  ([#86](https://github.com/celestiaorg/optimint/pull/86)) [@tzdybal](https://github.com/tzdybal/)
- Fixing linter errors  ([#55](https://github.com/celestiaorg/optimint/pull/55)) [@tzdybal](https://github.com/tzdybal/)
- Add peer discovery  ([#17](https://github.com/celestiaorg/optimint/pull/17)) [@tzdybal](https://github.com/tzdybal/)
- P2P bootstrapping  ([#14](https://github.com/celestiaorg/optimint/pull/14)) [@tzdybal](https://github.com/tzdybal/)
