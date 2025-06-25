# CLAUDE.md - AI Assistant Guidelines for Rollkit

## Project Overview

Rollkit is the first sovereign application framework that allows developers to launch sovereign, customizable blockchains as easily as smart contracts. It's built with a modular architecture using Go and leverages libp2p for networking.

### Key Architecture Principles
- **Modularity**: Rollkit uses a zero-dependency core package that defines interfaces for execution, sequencing, and data availability
- **Sovereignty**: Applications built with Rollkit are sovereign, meaning they have full control over their execution environment
- **Pluggable Components**: DA layers, execution environments, and sequencing mechanisms are all pluggable

## Project Structure

### Core Directories

#### `/core` - Zero-Dependency Foundation
- Contains interface definitions for all major components
- Must remain dependency-free - never add external modules to core/go.mod
- Key interfaces: `Executor`, `Sequencer`, `DA`
- Changes here affect the entire system - be extremely careful

#### `/block` - Block Management
- Handles block creation, validation, and synchronization
- Key components: Manager, Store, Submitter, Retriever
- Uses data availability layer for block data storage

#### `/node` - Node Types
- `full.go` - Full node implementation
- `light.go` - Light client implementation
- Node setup and configuration logic

#### `/pkg` - Shared Packages
- `/p2p` - P2P networking using libp2p
- `/store` - Storage abstractions
- `/config` - Configuration management
- `/cmd` - CLI commands
- `/rpc` - RPC server implementation
- `/signer` - Transaction signing interfaces

#### `/apps` - Example Applications
- `/testapp` - Reference implementation for testing
- `/evm` - EVM-based applications (single and based sequencing)

#### `/sequencers` - Sequencing Implementations
- `/single` - Single sequencer mode
- `/based` - Based sequencing (decentralized)

#### `/da` - Data Availability
- Local DA implementation for testing
- JSON-RPC client/server for DA communication

#### `/execution` - Execution Environments
- `/evm` - EVM execution environment integration

#### `/types` - Core Types
- Block, transaction, and state types
- Protobuf generated code in `/pb`

#### `/proto` - Protocol Buffers
- Protobuf definitions for all messages
- Generated code goes to `/types/pb`

## Development Guidelines

### Go Version and Dependencies
- Requires Go >= 1.22
- Uses Go modules with workspace support
- Multiple go.mod files for modular builds
- Main module: `github.com/rollkit/rollkit`

### Building and Testing

#### Main Commands
```bash
# Build TestApp CLI
make build

# Install TestApp
make install

# Run all tests
make test

# Run tests with coverage
make test-cover

# Run integration tests
make test-integration

# Run E2E tests
make test-e2e

# Run Docker E2E tests
make test-docker-e2e

# Build all binaries
make build-all
```

#### Linting
```bash
# Run all linters
make lint

# Lint proto files
make proto-lint
```

#### Proto Generation
```bash
# Generate protobuf files (requires Docker)
make proto-gen
```

### Code Style and Conventions

#### Import Ordering (enforced by gci)
1. Standard library imports
2. Third-party imports  
3. `github.com/rollkit` imports
4. `github.com/rollkit/rollkit` imports

#### Testing Patterns
- Unit tests: `*_test.go` files alongside implementation
- Integration tests: Use build tag `integration`
- E2E tests: Use build tag `e2e`
- Mock generation: Uses uber/mock for interfaces

#### Error Handling
- Always check and handle errors explicitly
- Use wrapped errors with context
- Common error variables defined in `errors.go` files

### Working with Interfaces

#### Core Package Rules
1. Never import anything into the core package
2. All other packages should depend on core interfaces
3. Use dependency injection for implementations

#### Adding New Interfaces
1. Define in appropriate core subpackage
2. Create an ADR (Architecture Decision Record) for significant changes
3. Ensure backward compatibility

### P2P and Networking

#### Key Concepts
- Uses libp2p for all P2P communication
- GossipSub for transaction gossiping
- Kademlia DHT for peer discovery
- Topics namespaced by chain ID

#### Configuration
```go
type P2PConfig struct {
    ListenAddress string // e.g., "/ip4/0.0.0.0/tcp/26656"
    Seeds         string // Comma-separated multiaddrs
    BlockedPeers  string // Comma-separated peer IDs
    AllowedPeers  string // Comma-separated peer IDs
}
```

### Data Availability Layer

#### Interface Pattern
- DA interface in `/core/da/da.go`
- Implementations should handle blob submission and retrieval
- Support for multiple blobs per submission
- Namespace-based operations

### State Management

#### Key Principles
- State stored in configurable backend (default: BadgerDB)
- Merkle tree for state commitments
- Support for state fraud proofs

### Testing Guidelines

#### Unit Tests
- Test files should be in same package
- Use table-driven tests where appropriate
- Mock external dependencies

#### Integration Tests
```bash
# Run with integration tag
go test -tags='integration' ./...
```

#### E2E Tests
- Require built binaries
- Test full node scenarios
- Include DA layer interactions

## Common Tasks

### Adding a New Command
1. Create command in `/pkg/cmd/`
2. Add to root command in the app
3. Include proper flag parsing and validation

### Implementing a New DA Layer
1. Implement the `DA` interface from `/core/da/`
2. Add configuration options
3. Create integration tests
4. Document the implementation

### Adding RPC Endpoints
1. Define proto messages in `/proto/rollkit/v1/`
2. Generate code with `make proto-gen`
3. Implement server handlers
4. Add client methods

## Debugging Tips

### Common Issues
1. **Import cycles**: Check that core package has no imports
2. **Proto generation failures**: Ensure Docker is running
3. **Test failures**: Check for proper build tags
4. **P2P connection issues**: Verify multiaddr format

### Useful Debug Commands
```bash
# Check module dependencies
go mod graph

# Verify proto files
buf lint

# Run specific test
go test -v -run TestName ./path/to/package

# Check for race conditions
go test -race ./...
```

## Security Considerations

1. Never commit private keys or sensitive data
2. Use secure randomness for cryptographic operations
3. Validate all external inputs
4. Be careful with goroutine leaks
5. Always use context for cancellation

## Performance Guidelines

1. Use sync.Pool for frequently allocated objects
2. Minimize allocations in hot paths
3. Profile before optimizing
4. Consider concurrent operations carefully
5. Cache computed values when appropriate

## Contributing

1. Read CONTRIBUTING.md first
2. Open an issue before starting work
3. Follow the code review process
4. Ensure all tests pass
5. Update documentation as needed

## Resources

- [Rollkit Documentation](https://rollkit.dev)
- [Architecture Decision Records](./specs/lazy-adr/)
- [Dependency Graph](./specs/src/specs/rollkit-dependency-graph.md)
- [Discord Community](https://discord.com/invite/YsnTPcSfWQ)