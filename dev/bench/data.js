window.BENCHMARK_DATA = {
  "lastUpdate": 1791816188619,
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
    ],
    "Block Executor Benchmark": [
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
        "date": 1791816187950,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 36165,
            "unit": "ns/op\t    4864 B/op\t      51 allocs/op",
            "extra": "32764 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 36165,
            "unit": "ns/op",
            "extra": "32764 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4864,
            "unit": "B/op",
            "extra": "32764 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "32764 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 36938,
            "unit": "ns/op\t    5050 B/op\t      55 allocs/op",
            "extra": "33020 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 36938,
            "unit": "ns/op",
            "extra": "33020 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 5050,
            "unit": "B/op",
            "extra": "33020 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "33020 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 43550,
            "unit": "ns/op\t   10365 B/op\t      55 allocs/op",
            "extra": "27708 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 43550,
            "unit": "ns/op",
            "extra": "27708 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10365,
            "unit": "B/op",
            "extra": "27708 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "27708 times\n4 procs"
          }
        ]
      }
    ]
  }
}