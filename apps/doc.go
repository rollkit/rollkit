/*
This pkg defines a set of rollup configurations that can be used out of the box.

The configurations are:

RETH:
- Single Sequencer rollup with RETH
- Based rollup with RETH

ABCI:
- Based rollup with ABCI
- Single sequencer with ABCI and an attestor set

Custom:
- Single sequencer rollup with grpc to an external execution environment
- Based rollup with grpc to an external execution environment

These configurations can be used as examples to develop your own rollup.
*/
package rollups
