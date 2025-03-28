/*
This pkg defines a set of rollup configurations that can be used out of the box.

The configurations are:

RETH:
- Centralized rollup with RETH
- Based rollup with RETH

ABCI:
- Based rollup with ABCI
- Centralized sequencer with ABCI and an attestor set

Custom:
- Centralized rollup with grpc to an external execution environment
- Based rollup with grpc to an external execution environment

These configurations can be used as examples to develop your own rollup.
*/
package rollups
