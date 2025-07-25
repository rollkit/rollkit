# ADR 016: Genesis File

## Changelog

- 2025-03-21: Initial draft

## Context

Rollkit currently uses a simple genesis state structure (`RollkitGenesis`) that is embedded within the block manager code. This structure defines the initial state of a Rollkit chain but lacks a formal specification and a standardized file format.

Currently, the genesis information is passed directly to the `NewManager` function as a parameter, without a clear process for how this information should be serialized, validated, or shared between nodes. This creates challenges for initializing new chains and ensuring all nodes start from the same state.

The current `RollkitGenesis` structure contains basic fields like `GenesisTime`, `InitialHeight`, `ChainID`, and `ProposerAddress`. This structure is used to initialize the chain state when no existing state is found in the store.

When a full node is created, a `RollkitGenesis` instance is populated from a CometBFT GenesisDoc, and the genesis file is loaded from disk using a function that expects the CometBFT genesis format.

However, this approach has several limitations:

1. It relies on CometBFT's genesis format, which may include fields that are not relevant to Rollkit
2. There's no explicit validation of the genesis file specific to Rollkit's needs
3. The conversion from CometBFT's GenesisDoc to RollkitGenesis is implicit and not well-documented
4. There's no standardized way to create, share, or modify the genesis file
5. The use of `GenesisTime` is problematic for chains that are a function of a DA layer, as the chain's starting point should be defined by a DA block height rather than a timestamp
6. The hardcoded `ProposerAddress` field doesn't allow for flexibility in different sequencing mechanisms

## Alternative Approaches

### 1. Continue Using CometBFT Genesis Format

We could continue using the CometBFT genesis format and enhance the conversion logic to extract Rollkit-specific values. This approach has the advantage of compatibility with existing tools but means Rollkit is constrained by the structure of the CometBFT genesis file.

### 2. Create a Completely Custom Genesis Format (Chosen)

We will define a completely new genesis file format specific to Rollkit with no dependency on CometBFT's format. This gives us maximum flexibility to define fields that are relevant to chains that rely on a DA layer.

### 3. Hybrid Approach

Define a Rollkit-specific genesis file format that:

1. Contains only the fields needed by Rollkit
2. Supports importing/exporting to/from CometBFT genesis format for compatibility
3. Includes validation specific to Rollkit's requirements

## Decision

We will implement a dedicated Rollkit genesis file format that is completely decoupled from CometBFT's format. This will allow us to define fields that are specifically relevant to chains of this type, such as `genesisDAStartHeight` instead of `genesisTime`, and a flexible `extraData` field instead of hardcoding a `proposerAddress`.

The new genesis format will be defined in its own package (`genesis`) and will include validation and serialization methods.

## Detailed Design

### Genesis File Structure

The new genesis file structure will contain the following key fields:

1. **GenesisDAStartHeight**: The DA layer height at which the chain starts, replacing the traditional `GenesisTime` field. This provides a more accurate starting point for chains built on a DA layer.

2. **InitialHeight**: The initial block height of the chain.

3. **ChainID**: A unique identifier for the chain.

4. **ExtraData**: A flexible field that can contain chain-specific configuration, such as proposer/sequencer information or consensus parameters. This replaces the hardcoded `ProposerAddress` field.

5. **AppState**: Application-specific genesis state as a JSON object.

The `ExtraData` field is particularly important as it allows different node types (sequencer, full node) to store type-specific information. For example, a single sequencer setup might include the sequencer's address in this field, while a decentralized setup might include validator set information.

The `AppState` field should contain the genesis file specific to the execution layer as a JSON Object. It will be passed on the execution layer during initialization.

### Genesis Validation

The genesis structure will include validation logic to ensure all required fields are present and valid. This would include checks that:

- The DA start height is non-zero
- The chain ID is present and properly formatted
- The initial height is at least 1
- Other format-specific validations

### ExtraData Structure

Since `ExtraData` is a flexible field that can contain different types of information, we'll define a common configuration structure that can be serialized into this field. This configuration would include:

- Proposer address for single sequencer mode
- Validator information for multiple sequencers or validator-based consensus
- Consensus parameters as needed

The package will provide helper functions for encoding and decoding this information to and from the `ExtraData` field.

### Genesis File I/O

The genesis package will provide functions to:

- Load a genesis file from disk
- Save a genesis file to disk
- Parse a genesis document from JSON
- Serialize a genesis document to JSON
- Validate a genesis document

### Command-Line Tools

We will provide CLI commands to initialize, validate, and inspect genesis files. These would include:

- `init`: Initialize a new genesis file with a specified chain ID
- `validate`: Validate an existing genesis file
- `add-proposer`: Set the proposer address in the extra data field

The commands would use flags to set various genesis parameters like the initial height, DA start height, and proposer address.

### Integration with Node Types

#### Sequencer Node

Sequencer nodes need to interpret the `ExtraData` field to determine if they are authorized to create blocks. In a single sequencer model, this would involve:

1. Loading the genesis file
2. Decoding the extra data to extract the proposer address
3. Checking if the node's signing key matches the proposer address
4. Initializing the node based on its role (sequencer or non-sequencer)

#### Full Node

Full nodes use the genesis file to initialize their state and validate incoming blocks. This process would involve:

1. Loading the genesis file
2. Decoding the extra data to access consensus parameters and proposer information
3. Initializing the node with the genesis information

### Initial State Creation

The block manager's `getInitialState` function will be updated to work with the new genesis format. This would involve:

1. Loading the state from the store if available
2. If no state is found, initializing a new state based on the genesis information:
   - Decoding the extra data to get the proposer address
   - Determining the genesis time based on the DA start height (by querying the DA layer)
   - Initializing the chain with the appropriate parameters
   - Creating and saving the genesis block
3. Performing sanity checks when resuming from an existing state

## Status

Proposed

## Consequences

### Positive

1. **DA-Centric Design**: Using `genesisDAStartHeight` instead of `genesisTime` provides a more accurate starting point for chains built on a DA layer.
2. **Flexibility**: The `extraData` field allows for different sequencing mechanisms and chain-specific configurations.
3. **Simplicity**: A custom genesis format allows us to include only what's needed for Rollkit chains.
4. **Independence**: No dependency on CometBFT's genesis format allows Rollkit to evolve independently.
5. **Better Semantics**: The structure more accurately reflects how chains initialize and operate.

### Negative

1. **Breaking Change**: Existing code that directly uses `RollkitGenesis` will need to be updated.
2. **Tooling Requirements**: New tools will need to be developed for genesis file creation and management.
3. **Compatibility Issue**: No direct compatibility with existing Cosmos ecosystem tools that expect CometBFT genesis format.

### Neutral

1. **Documentation Requirements**: The new genesis format will need to be documented for users.
2. **Testing Requirements**: Comprehensive tests will be needed for the new functionality.

## References

- [Block Manager Implementation](https://github.com/evstack/ev-node/blob/main/block/manager.go)
