window.BENCHMARK_DATA = {
  "lastUpdate": 1792506094467,
  "repoUrl": "https://github.com/evstack/ev-node",
  "entries": {
    "EVM Contract Roundtrip": [
      {
        "commit": {
          "author": {
            "email": "damiannolan@gmail.com",
            "name": "Damian Nolan",
            "username": "damiannolan"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "a2b13652294807eb0f6dd40d033a5ed449e4345f",
          "message": "feat(da): sign blob submissions and broadcast over consensus gRPC (#3483)\n\n* feat(block): allow injecting a blob submitter into the DA client\n\nAdd a BlobSubmitter seam to the DA client so blob submission can be routed\nsomewhere other than celestia-node's JSON-RPC blob.Submit, e.g. to sign\nPayForBlobs locally. Reads keep using the JSON-RPC client.\n\n- da.Config.Submitter; nil keeps the existing blob.Submit path.\n- block.WithSubmitter and a variadic NewDAClient(..., opts ...DAClientOption),\n  so existing callers compile unchanged.\n- pkg/cmd.StartNode accepts DA client options for apps that cannot be\n  imported by the root module.\n\nThe error-to-status mapping applies to both paths.\n\n* feat(config): add da.tx_signer section\n\nConfiguration for signing PayForBlobs locally and broadcasting them over\nconsensus gRPC (grpc_address, grpc_auth_token, grpc_tls, network, keyring\ndir/backend/key, worker_accounts, passphrase_file). Config only; it adds no\ndependencies to the root module and is disabled by default.\n\nWhen enabled, validation requires grpc_address, network and key_name,\nrestricts keyring_backend to file or test, and rejects da.signing_addresses\n(which would inject a signer_address the local signer does not use).\ngrpc_auth_token falls back to da.auth_token since one da-proxy token covers\nboth endpoints.\n\n* fix(config): require da.address for tx_signer, drop network and worker_accounts\n\nThe chain-id comes from the consensus gRPC endpoint, so network is not\nneeded. Parallel worker accounts write new keys into the operator keyring\nand move funds at startup, and ev-node submits sequentially anyway.\n\n* fix(da): map current celestia submit errors to blob statuses\n\ncelestia-app v10 reports oversized blobs as \"too large\" and a dropped\ntransaction as \"eviction poll timeout\". Neither matched the sentinels\nthe status switch looked for, so both ended up as StatusError.\n\n* feat(da/celestia): sign PayForBlobs locally with the celestia-app tx client\n\nAdds a block.BlobSubmitter that signs PayForBlobs from a local keyring\nand broadcasts them over consensus gRPC with x-token auth, for DA\nendpoints that cannot sign such as da-proxy. Built on celestia-app\npkg/user only; no celestia-node dependency.\n\nThe replace to ../.. is temporary until root is tagged.\n\n* feat(apps): wire da.tx_signer into testapp and evm\n\nThe submitter is built only for aggregator and promotable nodes.\n\n* test(e2e): cover signed blob submission\n\nRuns celestia-app v10.4.0 and celestia-node v0.34.3, funds a test\nkeyring key, starts testapp with tx_signer and asserts blobs are\nincluded and the signer paid fees.\n\n* docs: document da.tx_signer\n\n* fix(da): map celestia-app max tx size errors to StatusTooBig\n\ncelestia-app rejects txs over its max size with \"transaction size\nexceeds maximum allowed limit\", which matched no sentinel, so batch\nhalving never engaged.\n\n* fix(da/celestia): stop the tx client on Close and validate config at startup\n\nSetupTxClient starts a queue worker that runs until its context is\ncancelled, so Close now cancels it. ConfigFromNode rejects signer_address\nin da.submit_options, reads the passphrase only for the file backend,\nand resolves an empty keyring_dir to the node home.\n\n* test(e2e): fail on signer balance query errors\n\nA failed query returned zero, so the fee assertion passed vacuously.\n\n* docs(da/celestia): add package overview\n\n* docs: fix dead links flagged by the link checker\n\nThe Arabica explorer is gone and the libp2p fan-out page moved. celestia.org\nreturns 403 to the checker but serves browsers, so it is ignored.\n\n* test(e2e): signed blob submission with the evm binary and reth\n\nRun the evm binary on ev-reth with da.tx_signer and a passphrase-protected\nfile keyring, send transactions, and check the blocks carrying them reach DA\npaid by ev-node's key. Shared DA setup moves into helpers used by the testapp\ntest too. The test is evm-tagged, so it gets its own just recipe and CI job\nlike the other evm docker tests.\n\n* fix(evm): sign force inclusion submissions with the DA tx signer\n\nThe force inclusion server submits to DA from any node role, but the signer\nwas only attached on aggregator and promotable nodes. A full node with\nda.tx_signer enabled and the server running fell back to node-side\nblob.Submit, which fails behind endpoints that cannot sign.\n\n* fix(da/celestia): bound startup gRPC queries\n\nThe account and balance queries in New ran on the node's command context with\nno deadline, so an endpoint that accepted connections but never answered hung\nstartup. Give each a 30s timeout. SetupTxClient keeps the long-lived context\nbecause it also scopes the tx queue worker.",
          "timestamp": "2026-10-20T13:49:35Z",
          "tree_id": "96e6565123b135a0da1a81055a24a3b1e9c16040",
          "url": "https://github.com/evstack/ev-node/commit/a2b13652294807eb0f6dd40d033a5ed449e4345f"
        },
        "date": 1792506088626,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 914394138,
            "unit": "ns/op\t 4224836 B/op\t   36767 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 914394138,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4224836,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36767,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      }
    ]
  }
}