# rollkit-types

Proto-generated types for Rollkit.

## Building

The proto files are automatically generated during the build process:

```bash
cargo build
```

## Proto Generation

The generated code is committed to the repository. If you modify the proto files, you need to regenerate:

```bash
# From the repository root
make rust-proto-gen

# Or directly
cd client/crates/rollkit-types
cargo build
```

## Version Consistency

**Important**: The CI uses protoc version 25.1. If your local protoc version differs, you may see formatting differences in the generated files.

To check your protoc version:

```bash
protoc --version
```

To ensure consistency with CI:

1. Install protoc version 25.1
2. Use the provided script: `./client/scripts/generate-protos.sh`
3. Or use the Makefile: `make rust-proto-gen`

### Common Issues

If you see differences in generated files between local and CI:

- It's usually due to protoc version differences
- Different versions may format the generated code slightly differently
- The functionality remains the same, only formatting changes

To avoid this:

- Use the same protoc version as CI (25.1)
- Or accept the formatting from your version and update CI if needed

## Dependencies

This crate requires:

- `protoc` (Protocol Buffers compiler)
- `tonic-build` for code generation
- `prost` for runtime support

The build dependencies are specified in `Cargo.toml` and use workspace versions for consistency.
