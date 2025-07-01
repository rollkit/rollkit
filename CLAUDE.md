# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Rollkit is a sovereign rollup framework built in Go that allows developers to build rollups on any DA layer. It provides a modular architecture where components like the data availability layer, executor, and sequencer can be plugged in.

## Build and Development Commands

### Building

- `make build` - Builds the Testapp CLI to `./build/testapp`
- `make install` - Installs Testapp CLI to your Go bin directory
- `make build-all` - Builds all Rollkit binaries
- `make docker-build` - Builds Docker image tagged as `rollkit:local-dev`

### Testing

- `make test` - Runs unit tests for all go.mod files
- `make test-integration` - Runs integration tests (15m timeout)
- `make test-e2e` - Runs end-to-end tests (requires building binaries first)
- `make test-cover` - Generates code coverage report
- `make test-all` - Runs all tests including Docker E2E tests

### Linting and Code Quality

- `make lint` - Runs all linters (golangci-lint, markdownlint, hadolint, yamllint, goreleaser check, actionlint)
- `make lint-fix` - Auto-fixes linting issues where possible
- `make vet` - Runs go vet

### Development Utilities

- `make deps` - Downloads dependencies and runs go mod tidy for all modules
- `make proto-gen` - Generates protobuf files (requires Docker)
- `make mock-gen` - Generates mocks using mockery
- `make run-n NODES=3` - Run multiple nodes locally (default: 1)

## Code Architecture

### Core Package Structure

The project uses a zero-dependency core package pattern:

- **core/** - Contains only interfaces and types, no external dependencies
- **block/** - Block management, creation, validation, and synchronization
- **p2p/** - Networking layer built on libp2p
- **sequencing/** - Modular sequencer implementations
- **testapp/** - Reference implementation for testing

### Key Interfaces

- **Executor** (core/executor.go) - Handles state transitions
- **Sequencer** (core/sequencer.go) - Orders transactions
- **DA** (core/da.go) - Data availability layer abstraction

### Modular Design

- Each component has an interface in the core package
- Implementations are in separate packages
- Components are wired together via dependency injection
- Multiple go.mod files enable modular builds

### P2P Architecture

- Built on libp2p with GossipSub and Kademlia DHT
- Nodes advertise capabilities (full/light, DA layers)
- Automatic peer discovery with rendezvous points

## Testing Patterns

### Test Organization

- Unit tests: `*_test.go` files alongside code
- Integration tests: `test/integration/`
- E2E tests: `test/e2e/`

### Running Specific Tests

```bash
# Run a single test
go test -run TestSpecificFunction ./package/...

# Run with verbose output
go test -v ./package/...

# Run with race detection
go test -race ./package/...
```

### Mock Generation

- Mocks are defined in `.mockery.yaml`
- Generate with `make mock-gen`
- Mocks are placed in `mocks/` directories

## Code Style Guidelines

### Go Conventions

- Follow standard Go formatting (enforced by golangci-lint)
- Use meaningful variable names
- Keep functions small and focused
- Document exported types and functions
- Use context.Context for cancellation

### Error Handling

- Wrap errors with context using `fmt.Errorf`
- Return errors early
- Use custom error types for domain-specific errors

### Logging

- Use structured logging (look for existing patterns)
- Include relevant context in log messages
- Use appropriate log levels

## Common Development Tasks

### Adding a New DA Layer

1. Implement the `DA` interface from `core/da.go`
2. Add configuration in the appropriate config package
3. Wire it up in the initialization code
4. Add tests following existing patterns

### Modifying Protobuf Definitions

1. Edit `.proto` files in `types/pb/`
2. Run `make proto-gen` to regenerate Go code
3. Update any affected code
4. Run tests to ensure compatibility

### Adding New Tests

1. Place unit tests next to the code being tested
2. Use table-driven tests where appropriate
3. Mock external dependencies using mockery
4. Ensure tests are deterministic

## Security Considerations

- Never expose private keys in logs or errors
- Validate all inputs from external sources
- Use secure random number generation
- Follow the principle of least privilege
- Be careful with concurrent access to shared state

## Performance Considerations

- The project uses concurrent processing extensively
- Be mindful of goroutine leaks
- Use buffered channels appropriately
- Profile before optimizing
- Consider memory allocation in hot paths

## Debugging Tips

- Use `make run-n NODES=2` to test multi-node scenarios locally
- Check logs for error messages and stack traces
- Use Go's built-in profiling tools for performance issues
- The testapp provides a simple way to test changes

## Contributing Guidelines

- All code must pass linting (`make lint`)
- All tests must pass (`make test-all`)
- Follow the existing code patterns
- Update tests when changing functionality
- Keep commits focused and atomic
