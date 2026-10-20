// Package celestia submits ev-node blobs to Celestia by signing PayForBlobs
// locally and broadcasting them over consensus gRPC. It exists for DA
// endpoints that serve reads but cannot sign, such as da-proxy.
//
// Submitter implements block.BlobSubmitter. Inject it with
// block.WithSubmitter, and the DA client routes submissions through it.
// Reads (blobs, headers, proofs) still use the celestia-node JSON-RPC API
// at da.address.
//
// Each Submit signs one PayForBlobs with the configured key, broadcasts it
// with celestia-app's TxClient, waits until it is committed, and returns the
// inclusion height. The chain-id is read from the gRPC endpoint. New fails
// if the key is missing from the keyring or its account does not exist on
// chain.
//
// Typical wiring, as done by the testapp and evm binaries:
//
//	cfg, err := celestia.ConfigFromNode(nodeConfig)
//	if err != nil {
//		return err
//	}
//	s, err := celestia.New(ctx, cfg, logger)
//	if err != nil {
//		return err
//	}
//	defer s.Close()
//	opts := []block.DAClientOption{block.WithSubmitter(s)}
//
// This is a separate Go module because celestia-app requires replace
// directives, which do not propagate to dependents. Keeping it out of the
// root module keeps those replaces out of ev-node's own dependency graph.
package celestia
