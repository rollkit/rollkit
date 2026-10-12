window.BENCHMARK_DATA = {
  "lastUpdate": 1791816186219,
  "repoUrl": "https://github.com/evstack/ev-node",
  "entries": {
    "EVM Contract Roundtrip": [
      {
        "commit": {
          "author": {
            "email": "gopher@2980.com",
            "name": "gophersg",
            "username": "gophersg"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "120fde96b5898a0fd71bc91d7736f0c9a46dba44",
          "message": "refactor: replace []byte(fmt.Sprintf) with fmt.Appendf (#3468)\n\nSigned-off-by: gophersg <gopher@2980.com>",
          "timestamp": "2026-10-12T16:31:34+02:00",
          "tree_id": "cb218e3312901df65fbb187500522a19c4b21f60",
          "url": "https://github.com/evstack/ev-node/commit/120fde96b5898a0fd71bc91d7736f0c9a46dba44"
        },
        "date": 1791816181040,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 911730258,
            "unit": "ns/op\t 4267564 B/op\t   36511 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 911730258,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4267564,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36511,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      }
    ]
  }
}