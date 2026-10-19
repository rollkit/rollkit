window.BENCHMARK_DATA = {
  "lastUpdate": 1792391422133,
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
      },
      {
        "commit": {
          "author": {
            "email": "marko@baricevic.me",
            "name": "Marko",
            "username": "tac0turtle"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "cd339dcc81c9ceb98d26069cca0c16c765fefafa",
          "message": "fix(submitter): don't halt the node when SetFinal fails during DA backlog (#3473)\n\n* fix(submitter): don't halt the node when SetFinal fails during DA backlog\n\nWhen the DA-included height lags far behind the head, the DA inclusion\nloop finalizes historical heights one by one. EVM SetFinal resolves each\nheight through HeaderByNumber; once Reth has pruned that history, every\nretry fails and the submitter reports a critical error that shuts down an\notherwise healthy node.\n\n- EVM SetFinal: if the block lookup fails but the EL head is at or above\n  the requested height, the block is historical and unavailable, so skip\n  the finalized update with a warning. Lookups above the head, or when the\n  head cannot be read, still return an error.\n- Submitter: on exhausted SetFinal retries, log and retry on the next tick\n  instead of sending a critical error. The DA-included height does not\n  advance past a height that failed to finalize.\n\n* chore: add changelog entry for #3473\n\n* fix(evm): only skip SetFinal for pruned or missing history\n\nRestrict the historical-block skip in SetFinal to lookup errors that mean\nthe execution layer no longer serves the block: JSON-RPC code 4444\n(pruned history, EIP-4444, used by geth and reth) or ethereum.NotFound.\nTransient lookup failures are returned so the submitter retries the\nheight instead of advancing the DA-included height past it.",
          "timestamp": "2026-10-12T20:17:03+02:00",
          "tree_id": "589771274f6c86613ad057330346f761c0a6cef4",
          "url": "https://github.com/evstack/ev-node/commit/cd339dcc81c9ceb98d26069cca0c16c765fefafa"
        },
        "date": 1791829239366,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 904706816,
            "unit": "ns/op\t 4181568 B/op\t   36194 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 904706816,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4181568,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36194,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "committer": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "distinct": true,
          "id": "8053e8f0c570fdcf56509f3641cc4b76aea60efd",
          "message": "fix(build): compile current ev-node modules in apps and images\n\nPin EVM and testapp dependencies to cd339dcc81c9, which includes the pruning and DA-finalization fixes. Apply local module replacements inside Docker builds so later main and PR images compile the checked-out root, core, and EVM execution modules instead of stale release dependencies.",
          "timestamp": "2026-10-12T20:39:43+02:00",
          "tree_id": "ba1f3a8b8ec42cffc8f0da5ad8190cccf436493e",
          "url": "https://github.com/evstack/ev-node/commit/8053e8f0c570fdcf56509f3641cc4b76aea60efd"
        },
        "date": 1791830573409,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 913145091,
            "unit": "ns/op\t 4255624 B/op\t   36451 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 913145091,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4255624,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36451,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "committer": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "distinct": true,
          "id": "fab7190c4e88cffc922d8e6e1218eef8c25b81b9",
          "message": "fix(build): honor pinned modules in Docker images",
          "timestamp": "2026-10-12T20:44:09+02:00",
          "tree_id": "d202f4998eaa6fdb1a29f141a05588458a819995",
          "url": "https://github.com/evstack/ev-node/commit/fab7190c4e88cffc922d8e6e1218eef8c25b81b9"
        },
        "date": 1791830823892,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 897263136,
            "unit": "ns/op\t 4130604 B/op\t   36047 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 897263136,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4130604,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36047,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "zerozebrazone@outlook.com",
            "name": "zerozebrazone",
            "username": "zerozebrazone"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "a64ea554e471f24458d65fa0c8a9ecbaa9fe23c2",
          "message": "Clarify public API and store comments (#3479)\n\nSigned-off-by: zerozebrazone <zerozebrazone@outlook.com>",
          "timestamp": "2026-10-13T14:36:59+02:00",
          "tree_id": "841b982af3a0baf61d6c2d83bc772b949624784b",
          "url": "https://github.com/evstack/ev-node/commit/a64ea554e471f24458d65fa0c8a9ecbaa9fe23c2"
        },
        "date": 1791895116114,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 914609302,
            "unit": "ns/op\t 4294184 B/op\t   36801 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 914609302,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4294184,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36801,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "luangucun@outlook.com",
            "name": "luangucun",
            "username": "luangucun"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "de0c64425d12f88083b26503b8ab75efa21d2a0f",
          "message": "docs: correct Data and FilterPostpone comments (#3482)\n\nSigned-off-by: luangucun <luangucun@outlook.com>",
          "timestamp": "2026-10-16T14:00:43+02:00",
          "tree_id": "a4f098749f282fd56964016a7d54b4c2486ccc12",
          "url": "https://github.com/evstack/ev-node/commit/de0c64425d12f88083b26503b8ab75efa21d2a0f"
        },
        "date": 1792152471977,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 916021016,
            "unit": "ns/op\t 4227568 B/op\t   36761 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 916021016,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4227568,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36761,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "zoupingcenter@outlook.com",
            "name": "zoupingcenter",
            "username": "zoupingcenter"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "3098b5ed28a2a88f1e214a7de494f0a67193b70b",
          "message": "docs(store): correct signature lookup comments (#3484)\n\nSigned-off-by: zoupingcenter <zoupingcenter@outlook.com>",
          "timestamp": "2026-10-16T14:00:27+02:00",
          "tree_id": "575803627e742b6b233e1c9de48fa32b6c807b1f",
          "url": "https://github.com/evstack/ev-node/commit/3098b5ed28a2a88f1e214a7de494f0a67193b70b"
        },
        "date": 1792152470422,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 902020344,
            "unit": "ns/op\t 4192220 B/op\t   36175 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 902020344,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4192220,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36175,
            "unit": "allocs/op",
            "extra": "2 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "elonfliter@outlook.com",
            "name": "elonfliter",
            "username": "elonfliter"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "5b8180af88041ad8850b4cf6370ecf306477a2c6",
          "message": "docs(evm): stop naming a fixed Engine API version in ExecMeta comments (#3489)\n\nSigned-off-by: elonfliter <elonfliter@outlook.com>",
          "timestamp": "2026-10-19T08:24:02+02:00",
          "tree_id": "44e95a29f5f342c86776d65e8f93105db454f0a3",
          "url": "https://github.com/evstack/ev-node/commit/5b8180af88041ad8850b4cf6370ecf306477a2c6"
        },
        "date": 1792391413382,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkEvmContractRoundtrip",
            "value": 919709640,
            "unit": "ns/op\t 4276520 B/op\t   36975 allocs/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - ns/op",
            "value": 919709640,
            "unit": "ns/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - B/op",
            "value": 4276520,
            "unit": "B/op",
            "extra": "2 times\n4 procs"
          },
          {
            "name": "BenchmarkEvmContractRoundtrip - allocs/op",
            "value": 36975,
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
      },
      {
        "commit": {
          "author": {
            "email": "marko@baricevic.me",
            "name": "Marko",
            "username": "tac0turtle"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "cd339dcc81c9ceb98d26069cca0c16c765fefafa",
          "message": "fix(submitter): don't halt the node when SetFinal fails during DA backlog (#3473)\n\n* fix(submitter): don't halt the node when SetFinal fails during DA backlog\n\nWhen the DA-included height lags far behind the head, the DA inclusion\nloop finalizes historical heights one by one. EVM SetFinal resolves each\nheight through HeaderByNumber; once Reth has pruned that history, every\nretry fails and the submitter reports a critical error that shuts down an\notherwise healthy node.\n\n- EVM SetFinal: if the block lookup fails but the EL head is at or above\n  the requested height, the block is historical and unavailable, so skip\n  the finalized update with a warning. Lookups above the head, or when the\n  head cannot be read, still return an error.\n- Submitter: on exhausted SetFinal retries, log and retry on the next tick\n  instead of sending a critical error. The DA-included height does not\n  advance past a height that failed to finalize.\n\n* chore: add changelog entry for #3473\n\n* fix(evm): only skip SetFinal for pruned or missing history\n\nRestrict the historical-block skip in SetFinal to lookup errors that mean\nthe execution layer no longer serves the block: JSON-RPC code 4444\n(pruned history, EIP-4444, used by geth and reth) or ethereum.NotFound.\nTransient lookup failures are returned so the submitter retries the\nheight instead of advancing the DA-included height past it.",
          "timestamp": "2026-10-12T20:17:03+02:00",
          "tree_id": "589771274f6c86613ad057330346f761c0a6cef4",
          "url": "https://github.com/evstack/ev-node/commit/cd339dcc81c9ceb98d26069cca0c16c765fefafa"
        },
        "date": 1791829246418,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 37136,
            "unit": "ns/op\t    4871 B/op\t      51 allocs/op",
            "extra": "32466 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 37136,
            "unit": "ns/op",
            "extra": "32466 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4871,
            "unit": "B/op",
            "extra": "32466 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "32466 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 37631,
            "unit": "ns/op\t    5074 B/op\t      55 allocs/op",
            "extra": "31996 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 37631,
            "unit": "ns/op",
            "extra": "31996 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 5074,
            "unit": "B/op",
            "extra": "31996 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "31996 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 44704,
            "unit": "ns/op\t   10387 B/op\t      55 allocs/op",
            "extra": "27050 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 44704,
            "unit": "ns/op",
            "extra": "27050 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10387,
            "unit": "B/op",
            "extra": "27050 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "27050 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "committer": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "distinct": true,
          "id": "8053e8f0c570fdcf56509f3641cc4b76aea60efd",
          "message": "fix(build): compile current ev-node modules in apps and images\n\nPin EVM and testapp dependencies to cd339dcc81c9, which includes the pruning and DA-finalization fixes. Apply local module replacements inside Docker builds so later main and PR images compile the checked-out root, core, and EVM execution modules instead of stale release dependencies.",
          "timestamp": "2026-10-12T20:39:43+02:00",
          "tree_id": "ba1f3a8b8ec42cffc8f0da5ad8190cccf436493e",
          "url": "https://github.com/evstack/ev-node/commit/8053e8f0c570fdcf56509f3641cc4b76aea60efd"
        },
        "date": 1791830581921,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 30315,
            "unit": "ns/op\t    4722 B/op\t      51 allocs/op",
            "extra": "40150 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 30315,
            "unit": "ns/op",
            "extra": "40150 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4722,
            "unit": "B/op",
            "extra": "40150 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "40150 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 30496,
            "unit": "ns/op\t    4917 B/op\t      55 allocs/op",
            "extra": "40002 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 30496,
            "unit": "ns/op",
            "extra": "40002 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 4917,
            "unit": "B/op",
            "extra": "40002 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "40002 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 35771,
            "unit": "ns/op\t   10195 B/op\t      55 allocs/op",
            "extra": "34060 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 35771,
            "unit": "ns/op",
            "extra": "34060 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10195,
            "unit": "B/op",
            "extra": "34060 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "34060 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "committer": {
            "email": "marko@baricevic.me",
            "name": "tac0turtle",
            "username": "tac0turtle"
          },
          "distinct": true,
          "id": "fab7190c4e88cffc922d8e6e1218eef8c25b81b9",
          "message": "fix(build): honor pinned modules in Docker images",
          "timestamp": "2026-10-12T20:44:09+02:00",
          "tree_id": "d202f4998eaa6fdb1a29f141a05588458a819995",
          "url": "https://github.com/evstack/ev-node/commit/fab7190c4e88cffc922d8e6e1218eef8c25b81b9"
        },
        "date": 1791830830719,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 36673,
            "unit": "ns/op\t    4856 B/op\t      51 allocs/op",
            "extra": "33109 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 36673,
            "unit": "ns/op",
            "extra": "33109 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4856,
            "unit": "B/op",
            "extra": "33109 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "33109 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 37239,
            "unit": "ns/op\t    5059 B/op\t      55 allocs/op",
            "extra": "32612 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 37239,
            "unit": "ns/op",
            "extra": "32612 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 5059,
            "unit": "B/op",
            "extra": "32612 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "32612 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 44225,
            "unit": "ns/op\t   10381 B/op\t      55 allocs/op",
            "extra": "27229 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 44225,
            "unit": "ns/op",
            "extra": "27229 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10381,
            "unit": "B/op",
            "extra": "27229 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "27229 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "zerozebrazone@outlook.com",
            "name": "zerozebrazone",
            "username": "zerozebrazone"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "a64ea554e471f24458d65fa0c8a9ecbaa9fe23c2",
          "message": "Clarify public API and store comments (#3479)\n\nSigned-off-by: zerozebrazone <zerozebrazone@outlook.com>",
          "timestamp": "2026-10-13T14:36:59+02:00",
          "tree_id": "841b982af3a0baf61d6c2d83bc772b949624784b",
          "url": "https://github.com/evstack/ev-node/commit/a64ea554e471f24458d65fa0c8a9ecbaa9fe23c2"
        },
        "date": 1791895124901,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 20077,
            "unit": "ns/op\t    4914 B/op\t      51 allocs/op",
            "extra": "61540 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 20077,
            "unit": "ns/op",
            "extra": "61540 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4914,
            "unit": "B/op",
            "extra": "61540 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "61540 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 21685,
            "unit": "ns/op\t    5126 B/op\t      55 allocs/op",
            "extra": "60082 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 21685,
            "unit": "ns/op",
            "extra": "60082 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 5126,
            "unit": "B/op",
            "extra": "60082 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "60082 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 25325,
            "unit": "ns/op\t   10471 B/op\t      55 allocs/op",
            "extra": "49356 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 25325,
            "unit": "ns/op",
            "extra": "49356 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10471,
            "unit": "B/op",
            "extra": "49356 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "49356 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "luangucun@outlook.com",
            "name": "luangucun",
            "username": "luangucun"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "de0c64425d12f88083b26503b8ab75efa21d2a0f",
          "message": "docs: correct Data and FilterPostpone comments (#3482)\n\nSigned-off-by: luangucun <luangucun@outlook.com>",
          "timestamp": "2026-10-16T14:00:43+02:00",
          "tree_id": "a4f098749f282fd56964016a7d54b4c2486ccc12",
          "url": "https://github.com/evstack/ev-node/commit/de0c64425d12f88083b26503b8ab75efa21d2a0f"
        },
        "date": 1792152478170,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 31875,
            "unit": "ns/op\t    4757 B/op\t      51 allocs/op",
            "extra": "38078 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 31875,
            "unit": "ns/op",
            "extra": "38078 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4757,
            "unit": "B/op",
            "extra": "38078 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "38078 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 32382,
            "unit": "ns/op\t    4954 B/op\t      55 allocs/op",
            "extra": "37758 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 32382,
            "unit": "ns/op",
            "extra": "37758 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 4954,
            "unit": "B/op",
            "extra": "37758 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "37758 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 38456,
            "unit": "ns/op\t   10254 B/op\t      55 allocs/op",
            "extra": "31556 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 38456,
            "unit": "ns/op",
            "extra": "31556 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10254,
            "unit": "B/op",
            "extra": "31556 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "31556 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "zoupingcenter@outlook.com",
            "name": "zoupingcenter",
            "username": "zoupingcenter"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "3098b5ed28a2a88f1e214a7de494f0a67193b70b",
          "message": "docs(store): correct signature lookup comments (#3484)\n\nSigned-off-by: zoupingcenter <zoupingcenter@outlook.com>",
          "timestamp": "2026-10-16T14:00:27+02:00",
          "tree_id": "575803627e742b6b233e1c9de48fa32b6c807b1f",
          "url": "https://github.com/evstack/ev-node/commit/3098b5ed28a2a88f1e214a7de494f0a67193b70b"
        },
        "date": 1792152482474,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 35981,
            "unit": "ns/op\t    4848 B/op\t      51 allocs/op",
            "extra": "33447 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 35981,
            "unit": "ns/op",
            "extra": "33447 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 4848,
            "unit": "B/op",
            "extra": "33447 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "33447 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 36476,
            "unit": "ns/op\t    5047 B/op\t      55 allocs/op",
            "extra": "33152 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 36476,
            "unit": "ns/op",
            "extra": "33152 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 5047,
            "unit": "B/op",
            "extra": "33152 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "33152 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 43585,
            "unit": "ns/op\t   10366 B/op\t      55 allocs/op",
            "extra": "27682 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 43585,
            "unit": "ns/op",
            "extra": "27682 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10366,
            "unit": "B/op",
            "extra": "27682 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "27682 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "elonfliter@outlook.com",
            "name": "elonfliter",
            "username": "elonfliter"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "5b8180af88041ad8850b4cf6370ecf306477a2c6",
          "message": "docs(evm): stop naming a fixed Engine API version in ExecMeta comments (#3489)\n\nSigned-off-by: elonfliter <elonfliter@outlook.com>",
          "timestamp": "2026-10-19T08:24:02+02:00",
          "tree_id": "44e95a29f5f342c86776d65e8f93105db454f0a3",
          "url": "https://github.com/evstack/ev-node/commit/5b8180af88041ad8850b4cf6370ecf306477a2c6"
        },
        "date": 1792391421258,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkProduceBlock/empty_batch",
            "value": 24262,
            "unit": "ns/op\t    5077 B/op\t      51 allocs/op",
            "extra": "51310 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - ns/op",
            "value": 24262,
            "unit": "ns/op",
            "extra": "51310 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - B/op",
            "value": 5077,
            "unit": "B/op",
            "extra": "51310 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/empty_batch - allocs/op",
            "value": 51,
            "unit": "allocs/op",
            "extra": "51310 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx",
            "value": 23895,
            "unit": "ns/op\t    4933 B/op\t      55 allocs/op",
            "extra": "44500 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - ns/op",
            "value": 23895,
            "unit": "ns/op",
            "extra": "44500 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - B/op",
            "value": 4933,
            "unit": "B/op",
            "extra": "44500 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/single_tx - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "44500 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs",
            "value": 29056,
            "unit": "ns/op\t   10060 B/op\t      55 allocs/op",
            "extra": "42483 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - ns/op",
            "value": 29056,
            "unit": "ns/op",
            "extra": "42483 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - B/op",
            "value": 10060,
            "unit": "B/op",
            "extra": "42483 times\n4 procs"
          },
          {
            "name": "BenchmarkProduceBlock/100_txs - allocs/op",
            "value": 55,
            "unit": "allocs/op",
            "extra": "42483 times\n4 procs"
          }
        ]
      }
    ]
  }
}