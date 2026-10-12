window.BENCHMARK_DATA = {
  "lastUpdate": 1791826901561,
  "repoUrl": "https://github.com/evstack/ev-node",
  "entries": {
    "EVM Contract Roundtrip": [
      {
        "commit": {
          "author": {
            "email": "49699333+dependabot[bot]@users.noreply.github.com",
            "name": "dependabot[bot]",
            "username": "dependabot[bot]"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "758708de15432a5a4aae0cec45733bf307b0baf4",
          "message": "build(deps): Bump source-map-js from 1.2.1 to 1.2.2 in /docs in the npm_and_yarn group across 1 directory (#3472)\n\nbuild(deps): Bump source-map-js\n\nBumps the npm_and_yarn group with 1 update in the /docs directory: [source-map-js](https://github.com/7rulnik/source-map-js).\n\n\nUpdates `source-map-js` from 1.2.1 to 1.2.2\n- [Release notes](https://github.com/7rulnik/source-map-js/releases)\n- [Changelog](https://github.com/7rulnik/source-map-js/blob/main/CHANGELOG.md)\n- [Commits](https://github.com/7rulnik/source-map-js/compare/v1.2.1...v1.2.2)\n\n---\nupdated-dependencies:\n- dependency-name: source-map-js\n  dependency-version: 1.2.2\n  dependency-type: indirect\n  dependency-group: npm_and_yarn\n...\n\nSigned-off-by: dependabot[bot] <support@github.com>\nCo-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>",
          "timestamp": "2026-10-12T19:37:50+02:00",
          "tree_id": "43b68e9f283bf2dfa437b9192cceaf3f4fa9b496",
          "url": "https://github.com/evstack/ev-node/commit/758708de15432a5a4aae0cec45733bf307b0baf4"
        },
        "date": 1791826893310,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 912659268,
            "unit": "ns/op\t 4292296 B/op\t   36292 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 912659268,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4292296,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36292,
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
            "email": "49699333+dependabot[bot]@users.noreply.github.com",
            "name": "dependabot[bot]",
            "username": "dependabot[bot]"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "758708de15432a5a4aae0cec45733bf307b0baf4",
          "message": "build(deps): Bump source-map-js from 1.2.1 to 1.2.2 in /docs in the npm_and_yarn group across 1 directory (#3472)\n\nbuild(deps): Bump source-map-js\n\nBumps the npm_and_yarn group with 1 update in the /docs directory: [source-map-js](https://github.com/7rulnik/source-map-js).\n\n\nUpdates `source-map-js` from 1.2.1 to 1.2.2\n- [Release notes](https://github.com/7rulnik/source-map-js/releases)\n- [Changelog](https://github.com/7rulnik/source-map-js/blob/main/CHANGELOG.md)\n- [Commits](https://github.com/7rulnik/source-map-js/compare/v1.2.1...v1.2.2)\n\n---\nupdated-dependencies:\n- dependency-name: source-map-js\n  dependency-version: 1.2.2\n  dependency-type: indirect\n  dependency-group: npm_and_yarn\n...\n\nSigned-off-by: dependabot[bot] <support@github.com>\nCo-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>",
          "timestamp": "2026-10-12T19:37:50+02:00",
          "tree_id": "43b68e9f283bf2dfa437b9192cceaf3f4fa9b496",
          "url": "https://github.com/evstack/ev-node/commit/758708de15432a5a4aae0cec45733bf307b0baf4"
        },
        "date": 1791826900871,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 39259,
            "unit": "ns/op\t    4923 B/op\t      51 allocs/op",
            "extra": "30402 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 39259,
            "unit": "ns/op",
            "extra": "30402 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4923,
            "unit": "B/op",
            "extra": "30402 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "30402 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 40047,
            "unit": "ns/op\t    5114 B/op\t      55 allocs/op",
            "extra": "30464 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 40047,
            "unit": "ns/op",
            "extra": "30464 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 5114,
            "unit": "B/op",
            "extra": "30464 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "30464 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 47119,
            "unit": "ns/op\t   10424 B/op\t      55 allocs/op",
            "extra": "26010 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 47119,
            "unit": "ns/op",
            "extra": "26010 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10424,
            "unit": "B/op",
            "extra": "26010 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "26010 times\n4 procs"
          }
        ]
      }
    ]
  }
}