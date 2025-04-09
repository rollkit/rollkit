# 0001. Attestation System

Date: 2024-05-17

## Status

Proposed

## Context

The current IBC connection between the decentralized validator set and the centralized sequencer relies solely on the sequencer for block validity and propagation. This architecture presents a single point of failure and a potential attack vector, as the trust is placed entirely on one centralized entity. There is a need to enhance the security and robustness of this connection by introducing a mechanism for multi-party verification of the blocks produced by the sequencer before they are finalized and communicated via IBC.

## Decision

We propose the implementation of an Attester Network composed of N nodes that operate alongside the centralized sequencer. This network will utilize the RAFT consensus protocol, with the sequencer acting as the leader (but not mandatory), to agree upon the validity of blocks proposed by the sequencer.
Key aspects of the decision include:
1.  **RAFT Consensus:** Attester nodes (followers) and the sequencer (leader) form a RAFT cluster. The leader proposes blocks via `raft.Apply()`.
2.  **Block Attestation:** Upon reaching RAFT consensus and applying a block log entry to their local Finite State Machine (FSM), each attester node will sign the relevant block data using its unique private key.
3.  **Optional Execution:** Attester nodes can optionally connect to a full node to execute block content within their FSM `Apply` logic, providing an additional layer of verification.
4.  **Signature Submission:** A dedicated RPC protocol will be established. After signing a block, each attester node will actively push its signature (along with block identifiers) back to the sequencer via this RPC endpoint.
5.  **Signature Aggregation:** The sequencer will expose an RPC endpoint to receive signatures, aggregate them according to a defined policy (e.g., quorum), and embed the aggregated attestation information (e.g., multi-signature, list of signatures) into the final block header (e.g., as the validator hash).

## Consequences

**Easier:**
-   **Enhanced Security:** Reduces the single point of failure by requiring consensus and signatures from multiple independent attestors.
-   **Increased Trust:** Provides verifiable, multi-party proof of block validity beyond the sequencer's own assertion.
-   **Optional Deep Verification:** Allows for optional execution verification by attesters, ensuring not just block order but also correct transaction execution.

**More Difficult:**
-   **Increased Complexity:** Introduces a new distributed system (RAFT cluster) to manage, monitor, and maintain alongside the existing infrastructure.
-   **Network Overhead:** Adds network traffic due to RAFT consensus messages and the separate signature submission RPC calls.
-   **Latency:** Introduces potential latency in block finalization, as the sequencer must wait for RAFT consensus and subsequent signature aggregation.
-   **Configuration & Key Management:** Requires careful configuration of each attester node, including RAFT parameters, sequencer endpoint addresses, and secure management of individual attester private keys.
-   **RPC Security:** The signature submission RPC channel between attestors and the sequencer needs to be secured (e.g., TLS, authentication) to prevent attacks.
-   **Failure Handling:** Requires robust handling for failures in the RAFT consensus process and the signature submission/aggregation logic (e.g., timeouts, retries).

# Design Document for an Attester System with RAFT Consensus (v2)

This document describes a design for an attester system that adds an extra layer of security to an IBC connection between a decentralized validator set (using CometBFT, Cosmos SDK, and IBC) and a centralized sequencer (using Cosmos SDK and IBC). The attester network will use RAFT consensus to provide robust, verifiable signatures for blocks, and optionally, execute block content to further enhance security.

---

## Table of Contents

1. Introduction
2. System Overview
3. Architecture and Flow
4. Detailed Design
    - RAFT Consensus Integration
    - **Signature Collection Protocol**
    - Node Configuration and Deployment
    - Golang Implementation Details
5. Security Considerations
6. Failure Handling and Recovery
7. Future Enhancements
8. Conclusion
9. Appendix: Code Samples and Configurations

---

## Introduction

In our blockchain ecosystem, the centralized sequencer is responsible for block production and propagation. However, IBC (Inter-Blockchain Communication) currently trusts only the sequencer, which introduces a potential single point of failure or attack vector. To mitigate this risk, we introduce an attester network that works in parallel to the sequencer. This network leverages RAFT consensus among _N_ attester nodes to validate and sign blocks. Additionally, attester nodes can optionally execute the block contents by interfacing with a full node, ensuring that the block data is not only correctly ordered but also correctly executed.

---

## System Overview

- **Decentralized Validator Set:** Managed by CometBFT, Cosmos SDK, and IBC.
- **Centralized Sequencer:** Produces blocks using Cosmos SDK and IBC.
- **Attester Network:**
    - Consists of _N_ attester nodes running a dedicated binary.
    - Implements RAFT consensus (using a well-known Golang RAFT library such as [HashiCorp's RAFT](https://github.com/hashicorp/raft)).
    - The sequencer acts as the RAFT leader, while followers may optionally link to a full node to execute block contents.
- **Flow:**
    1. **DA Block Creation:** A Data Availability (DA) block is produced.
    2. **Sequencer Propagation:** The block is sent to the centralized sequencer.
    3. **Attester Verification & RAFT Proposal:** The sequencer (RAFT leader) forwards the block data to the attester network by proposing it via `raft.Apply()`.
    4. **Consensus, Signing, and Signature Return:** Attester nodes achieve RAFT consensus on the block proposal. Each node applies the committed log entry to its local Finite State Machine (FSM), signs the relevant block data using its private key. **Crucially, each attester node then actively pushes its generated signature (along with relevant block identifiers) back to the sequencer via a dedicated RPC call.**
    5. **Signature Aggregation & Block Finalization:** The sequencer receives signatures from the attestors via the dedicated RPC endpoint. It aggregates these signatures (e.g., creating a multi-signature or collecting them) and includes the aggregated signature data in the block header as a validator hash before finalizing the block.

---

## Architecture and Flow

Fragmento de código

```
graph LR
    A[DA Block] --> B(Centralized Sequencer / RAFT Leader)
    B -- 1. Propose Block (RAFT Apply) --> C{Attester Network (RAFT Cluster)}
    C -- 2. RAFT Consensus & FSM Apply --> C
    subgraph Attester Nodes (Followers)
        direction TB
        F1[Attester 1 FSM: Sign Block]
        F2[Attester 2 FSM: Sign Block]
        F3[...]
        F1 -- 3. Push Signature (RPC) --> B
        F2 -- 3. Push Signature (RPC) --> B
        F3 -- 3. Push Signature (RPC) --> B
    end
    B -- 4. Aggregate Signatures --> E[Final Block with Attester Signatures]

```

- **Step 1:** A DA block is created and submitted to the sequencer.
- **Step 2:** The sequencer, acting as the leader in a RAFT cluster, proposes the block data to all attester nodes using `raft.Apply()`.
- **Step 3:** Attester nodes participate in RAFT consensus. Once the block proposal is committed, each node applies it to its local FSM. Inside the FSM `Apply` function, the node validates the block (optionally executing content via a full node) and signs the relevant block data with its _local_ private key.
- **Step 4:** **Signature Return:** After successful signing within the FSM `Apply` function, each attester node initiates a separate RPC call (e.g., `SubmitSignature`) back to the sequencer, sending its generated signature and necessary block identification (height/hash).
- **Step 5:** **Signature Aggregation:** The sequencer listens for incoming signatures on its dedicated RPC endpoint. It collects signatures for the specific block from the attestors until a predefined condition is met (e.g., receiving signatures from a quorum or all nodes).
- **Step 6:** **Block Finalization:** The sequencer embeds the aggregated attester signatures (or a cryptographic representation like a multi-sig or signature hash) into the final block header as the validator hash. The block is then committed, providing an extra security layer over the centralized sequencer.

## Detailed Design

### RAFT Consensus Integration

#### Library Choice

We use the [HashiCorp RAFT library](https://github.com/hashicorp/raft) for its robustness and proven track record.

#### Leader and Follower Roles

- **Leader (Sequencer):**
    - Initiates the consensus process by proposing blocks (`raft.Apply()`) to be attested.
    - **Exposes an RPC endpoint to receive signatures back from attestors.** (Could also be p2p?)
    - Aggregates received signatures.
- **Followers (Attesters):**
    - Participate in RAFT consensus.
    - Apply committed log entries via their FSM.
    - Verify block contents within the FSM, optionally execute them via a connected full node (or using the `ExecutionVerifier`), and sign the block data using their local private key.
    - **Call the sequencer's dedicated RPC endpoint to submit their generated signature.**

#### State Machine (FSM)

The RAFT state machine (on each node) is responsible for:

- Receiving block proposals (as committed log entries).
- Validating the block data upon applying the log entry.
- **Optionally Verifying Execution:** The FSM will hold an `ExecutionVerifier` interface, injected during initialization. If this verifier is configured (`!= nil`), its `VerifyExecution` method is called *after* basic block validation (height, hash) but *before* signing. If `VerifyExecution` returns an error, the FSM logs the error and returns immediately for that log entry, preventing the signing and state update for that block on the current node.
- **Signing the appropriate block data using the node's local key** (only if optional execution verification passes or is disabled).
- **Recording processed block identifiers (height/hash) to prevent replays.**
- _(Triggering the subsequent signature submission RPC call back to the leader happens after the FSM `Apply` completes successfully and the block has been signed)._

### Signature Collection Protocol

To facilitate the collection of signatures by the sequencer (RAFT leader) from the attestors (RAFT followers), a dedicated RPC mechanism is required, operating alongside the standard RAFT log replication:

- **Attester Action:** After an attester node successfully applies a block proposal log entry via its FSM and generates its local signature, it **initiates an RPC call** to the sequencer's designated signature collection endpoint.
- **Sequencer Action:** The sequencer **exposes an RPC endpoint** (e.g., defined in a protobuf service) specifically for receiving these signatures. An example signature for such an RPC method could be `SubmitSignature(ctx context.Context, req *SubmitSignatureRequest) (*SubmitSignatureResponse, error)`, where `SubmitSignatureRequest` contains the attester's ID, block height, block hash, and the signature itself.
- **Aggregation Logic:** The sequencer implements logic to receive these incoming signatures. It should maintain a temporary store (e.g., a map keyed by block height/hash) to collect signatures for ongoing blocks. Once a sufficient number of signatures (based on policy - e.g., f+1, 2f+1, or all N) is received for a given block, the sequencer considers the attestation complete for that block and proceeds with incorporating the signatures into the final chain block. Timeout mechanisms should be included to handle non-responsive attestors.

### Node Configuration and Deployment

#### Configuration File (`config.toml`)

Each node is configured via a `config.toml` (or equivalent YAML, e.g., `attester.yaml`) file containing:

- **Node Identification:** Unique ID for the attester (`[node].id`).
- **RAFT Parameters:** Directory paths (`[raft].data_dir`), bind addresses (`[node].raft_bind_address`), timeouts (`[raft].election_timeout`, `[raft].heartbeat_timeout`), snapshot settings (`[raft].snapshot_interval`, `[raft].snapshot_threshold`), peers (`[raft].peers`), etc.
- **Network Settings:**
    - Address for the full node if direct execution verification is enabled (see Execution Verification section below, `[execution].fullnode_endpoint`).
    - Address of the sequencer's signature submission RPC endpoint for follower nodes (`[network].sequencer_sig_endpoint`).
- **Signing Configuration:** Path to the private key and the signing scheme (`[signing].private_key_path`, `[signing].scheme`).
- **Execution Verification Configuration (`[execution]` section):** Controls the optional execution verification.
    - `enabled: bool` (default: `false`): Globally enables or disables the feature.
    - `type: string` (e.g., `"noop"`, `"fullnode"`): Selects the `ExecutionVerifier` implementation. Required if `enabled` is `true`.
    - `fullnode_endpoint: string`: The URL/endpoint of the full node RPC/API. Required if `type` is `"fullnode"`.
    - `timeout: string` (e.g., `"15s"`): Specific timeout for verification calls.

#### Command Line Options

Nodes also accept command line flags to override or supplement configuration file settings, providing flexibility during deployment and testing.

### Golang Implementation Details

#### Main Components

- **Config Loader:** Reads the configuration file (`config.toml`/`attester.yaml`) and command line flags, handling defaults and parsing values, including the new `[execution]` section.
- **RAFT Node Initialization:** Sets up the RAFT consensus node using HashiCorp's RAFT library. Includes setting up the FSM and injecting dependencies like the logger, signer, and the configured `ExecutionVerifier` instance.
- **Execution Verifier Abstraction:**
    - An `ExecutionVerifier` interface defines the `VerifyExecution` method.
    - A `NoOpVerifier` implementation always returns success (nil error), used when `[execution].enabled` is false or `[execution].type` is `"noop"`.
    - Concrete implementations (e.g., `FullNodeVerifier`) encapsulate the logic to interact with external systems (like a full node RPC) for verification. The specific verifier is instantiated in `main.go` based on configuration and passed to the FSM.
- **Block Handling (Leader):** Receives blocks (e.g., from DA layer), proposes them to RAFT via `raft.Apply()`.
- **FSM Implementation:** Contains the `Apply` logic for validating block proposals, calling the injected `ExecutionVerifier` if present/enabled, signing block data (if verification passed), and recording processed block identifiers.
- **Signature Submission (Follower):** Logic triggered after successful FSM `Apply` (including signing) to call the sequencer's `SubmitSignature` RPC endpoint.
- **Signature Reception & Aggregation (Leader):** Implements the `SubmitSignature` RPC server endpoint and the logic to collect and aggregate signatures.
- **Optional Execution:** (Now primarily handled by the `ExecutionVerifier` implementations) For followers with a verifier like `FullNodeVerifier` configured, this component connects to a full node to verify the correctness of block execution, triggered via the `VerifyExecution` call from the FSM `Apply` logic.

#### Sample Workflow in Code

1. Load configuration from file and merge with command line flags.
2. Instantiate the appropriate `ExecutionVerifier` based on the `[execution]` config section (`NoOpVerifier` or a concrete one).
3. Initialize the RAFT node, FSM (injecting logger, signer, verifier), and transport.
4. **If Leader (Sequencer):**
    - Start the RPC server to listen for `SubmitSignature` calls.
    - Receive a new block.
    - Propose the block to the RAFT cluster: `raft.Apply(blockData, timeout)`.
    - Wait for the `Apply` to complete locally (confirming it's committed).
    - Wait for incoming signatures via the `SubmitSignature` RPC endpoint, aggregating them.
    - Once enough signatures are collected, finalize the block header.
5. **If Follower (Attester):**
    - Participate in RAFT consensus.
    - When a block log entry is applied via the FSM:
        - Perform basic validations.
        - Call `verifier.VerifyExecution(...)` if `verifier` is not nil. If it returns an error, stop processing this entry.
        - Sign the block data using the local key.
        - Record processed block identifiers locally.
    - **After FSM `Apply` returns successfully, call the sequencer's `SubmitSignature` RPC with the generated signature.**

---

## Security Considerations

- **Additional Attestation:** By requiring a consensus signature from multiple attestors (collected via the explicit submission protocol), the system ensures that block validation is not solely dependent on the centralized sequencer.
- **Execution Verification:** Optional block execution by follower nodes adds an extra layer of security, verifying that the block's transactions have been executed correctly.
- **Resilience:** RAFT consensus provides fault tolerance for the agreement on _what_ to sign. The signature collection protocol needs its own resilience (e.g., timeouts, retries on the follower side).
- **RPC Security:** The `SubmitSignature` RPC endpoint on the sequencer should be secured (e.g., TLS, authentication/authorization if necessary) to prevent spoofed signature submissions.

---

## Failure Handling and Recovery

- **RAFT Consensus Failures:** Handled by RAFT's built-in mechanisms (leader election, log repair).
- **Network Partitions:** RAFT handles partitions regarding consensus. The signature submission RPC calls might fail during a partition and should be retried by the attestor once connectivity is restored. The sequencer needs timeouts for signature collection.
- **Execution Errors:** Optional full node execution should include timeout and error handling, ensuring that execution failures within the FSM `Apply` prevent signing and potentially signal an error state.
- **Signature Submission Failures:** Attestors should implement retry logic for submitting signatures if the RPC call fails. The sequencer needs timeouts to avoid waiting indefinitely for signatures.

---

## Future Enhancements

- **Dynamic Attester Membership:** Explore RAFT's membership change features alongside updating the sequencer's signature aggregation logic.
- **Advanced Monitoring and Alerts:** Implement comprehensive monitoring for RAFT state, FSM execution, and the signature collection process (e.g., tracking missing signatures, RPC errors).
- **Integration Testing:** Further integration testing covering the end-to-end flow including DA layer, sequencer proposal, RAFT consensus, FSM execution, signature submission RPC, aggregation, and final block creation.

---

## Conclusion

This design document outlines a robust attester system that integrates RAFT consensus into an existing blockchain architecture to enhance security. By involving multiple attester nodes in block validation and optionally executing block contents, the system mitigates risks associated with a centralized sequencer. **The addition of an explicit signature submission protocol from attestors back to the sequencer ensures reliable collection of attestations.** The use of a well-known RAFT library in Golang, along with flexible configuration and a clear protocol for signature handling, provides a solid foundation for implementation.

---

## Appendix: Code Samples and Configurations

_(Existing samples remain relevant, but additional code for the `SubmitSignature` RPC client (follower) and server (leader) would be needed, along with Protobuf definitions for this new RPC call.)_

**Sample `config.toml`**

Ini, TOML

```
[node]
id = "attester-1" # or "sequencer-1"
raft_dir = "./raft"
raft_bind = "127.0.0.1:12000"

[network]
address = "127.0.0.1:26657" # General network address if needed
# Optional: Endpoint for block execution verification
fullnode_endpoint = "http://127.0.0.1:26657"
# Required for Followers: Sequencer's signature submission endpoint
sequencer_sig_endpoint = "127.0.0.1:13000" # Example endpoint

[raft]
election_timeout = "500ms"
heartbeat_timeout = "100ms"

# Add section for leader specific config if needed
# [leader]
# signature_listen_addr = "0.0.0.0:13000" # Address leader listens on for signatures
```

**Sample Golang Code Snippet (Conceptual additions)**

Go

```
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/hashicorp/raft"
	"github.com/pelletier/go-toml"
	// Assume protobuf definitions exist:
	// pb "path/to/your/protobuf/definitions"
	// Assume FSM implementation exists:
	// fsm "path/to/your/fsm"
	// Assume gRPC is used for signature submission:
	// "google.golang.org/grpc"
)

// ... (Config struct definition as before, potentially add SequencerSigEndpoint) ...
type Config struct {
	Node struct {
		ID       string `toml:"id"`
		RaftDir  string `toml:"raft_dir"`
		RaftBind string `toml:"raft_bind"`
	} `toml:"node"`
	Network struct {
		Address            string `toml:"address"`
		FullnodeEndpoint   string `toml:"fullnode_endpoint"`
		SequencerSigEndpoint string `toml:"sequencer_sig_endpoint"` // Added for followers
	} `toml:"network"`
	Raft struct {
		ElectionTimeout  string `toml:"election_timeout"`
		HeartbeatTimeout string `toml:"heartbeat_timeout"`
	} `toml:"raft"`
	Leader struct { // Optional: Leader specific config
		SignatureListenAddr string `toml:"signature_listen_addr"`
	} `toml:"leader"`
}


// ... (loadConfig function as before) ...

// Placeholder for the FSM type which includes signing logic
type YourFsm struct {
	// ... other FSM fields ...
	nodeID string
	signer Signer // Interface for signing
	// Function or channel to signal successful signing for RPC submission
	onSigned func(blockInfo BlockInfo) // Simplified example
}

// Placeholder - Implement raft.FSM interface (Apply, Snapshot, Restore)
// Inside Apply:
// 1. Decode log data (block proposal)
// 2. Validate/Execute
// 3. signature := fsm.signer.Sign(dataToSign)
// 4. blockInfo := BlockInfo{..., Signature: signature}
// 5. Store blockInfo in FSM state
// 6. **Call fsm.onSigned(blockInfo)** // Signal completion
// 7. Return result

// Placeholder interface for signing
type Signer interface {
	Sign([]byte) ([]byte, error)
	PublicKey() []byte
}

// Placeholder struct holding signature info
type BlockInfo struct {
	Height uint64
	Hash   []byte
	Data   []byte // The actual data that was signed
	Signature []byte
	NodeID string
}


// Placeholder for the signature submission client (used by followers)
type SignatureSubmitter struct {
	// gRPC client connection, etc.
	// submit func(ctx context.Context, req *pb.SubmitSignatureRequest) (*pb.SubmitSignatureResponse, error)
}

func (s *SignatureSubmitter) Submit(ctx context.Context, info BlockInfo) error {
	// Construct pb.SubmitSignatureRequest from info
	// Call s.submit(ctx, req)
	log.Printf("Node %s submitting signature for block %d\n", info.NodeID, info.Height)
	// Add retry logic here
	return nil // Placeholder
}

// Placeholder for the signature receiver service (run by leader)
type SignatureReceiver struct {
	// pb.UnimplementedAttesterServiceServer // If using gRPC proto
	aggregator *SignatureAggregator
	// gRPC server instance
}

// func (s *SignatureReceiver) SubmitSignature(ctx context.Context, req *pb.SubmitSignatureRequest) (*pb.SubmitSignatureResponse, error) {
// 	 // Validate request
//	 // Call s.aggregator.AddSignature(...)
//	 // Return response
// }

func (s *SignatureReceiver) Start(listenAddr string) error {
	log.Printf("Starting signature receiver on %s\n", listenAddr)
	// Setup gRPC server and register the service
	// lis, err := net.Listen("tcp", listenAddr)
	// ... start server ...
	return nil // Placeholder
}

// Placeholder for signature aggregation logic (used by leader)
type SignatureAggregator struct {
	// Mutex, map[uint64]map[string][]byte // e.g., map[height]map[nodeID]signature
	// requiredSignatures int
}

func (a *SignatureAggregator) AddSignature(height uint64, nodeID string, signature []byte) (bool, error) {
	// Add signature to internal store (thread-safe)
	// Check if enough signatures are collected for this height
	log.Printf("Received signature for block %d from %s\n", height, nodeID)
	// Return true if aggregation is complete for this block height
	return false, nil // Placeholder
}


func main() {
	// ... (Flag parsing, config loading, timeout parsing as before) ...
	config, err := loadConfig(*configPath)
	// ... handle error ...

	isLeader := flag.Bool("leader", false, "Set node as leader (sequencer)")
	flag.Parse() // Re-parse after adding the new flag


	// --- RAFT Setup --- (Simplified)
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.Node.ID)
	// ... setup transport, stores (log, stable, snapshot) ...
	// transport, _ := raft.NewTCPTransport(...)

	// --- FSM Setup ---
	var submitter *SignatureSubmitter // Only initialized for followers
	// This callback links FSM signing completion to RPC submission
	onSignedCallback := func(info BlockInfo) {
		if submitter != nil {
			go func() { // Run submission in a goroutine to avoid blocking FSM
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := submitter.Submit(ctx, info); err != nil {
					log.Printf("ERROR: Failed to submit signature for block %d: %v", info.Height, err)
				}
			}()
		}
	}
	// signer := InitializeLocalSigner() // Load node's private key
	// fsm := fsm.NewYourFsm(config.Node.ID, signer, onSignedCallback)
	fsm := &raft.MockFSM{} // Using mock FSM for placeholder

	// r, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
	// ... handle error ...

	// --- Leader/Follower Specific Setup ---
	if *isLeader {
		fmt.Println("Starting node as RAFT Leader (Sequencer)...")
		// Bootstrap cluster if needed: r.BootstrapCluster(...)
		aggregator := &SignatureAggregator{ /* ... initialize ... */ }
		receiver := &SignatureReceiver{aggregator: aggregator}
		go receiver.Start(config.Leader.SignatureListenAddr) // Start RPC listener in background

		// Leader logic: receive blocks, propose via r.Apply(...)
		// Monitor aggregator state to know when blocks are fully attested

	} else {
		fmt.Println("Starting node as RAFT Follower (Attester)...")
		if config.Network.SequencerSigEndpoint == "" {
			log.Fatalf("Follower requires sequencer_sig_endpoint in config")
		}
		// Setup gRPC client connection to sequencer's signature endpoint
		// conn, err := grpc.Dial(config.Network.SequencerSigEndpoint, ...)
		submitter = &SignatureSubmitter{ /* ... initialize gRPC client ... */ }

		// Follower logic: participate in RAFT, FSM handles signing, onSignedCallback triggers submission
	}

	// Keep process running
	select {}
}

```