# Rollkit Release Process

This document outlines the release process for all Go packages in the Rollkit repository. The packages must be released in a specific order due to inter-dependencies.

## Package Dependency Graph

```ascii
                        ┌──────────┐
                        │   core   │ (zero dependencies)
                        └────┬─────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
   ┌─────────┐         ┌─────────┐      ┌──────────────┐
   │   da    │         │ rollkit │      │execution/evm │
   └─────────┘         └────┬────┘      └──────────────┘
                            │
                ┌───────────┴───────────┐
                │                       │
                ▼                       ▼
      ┌─────────────────┐     ┌─────────────────┐
      │sequencers/based │     │sequencers/single│
      └────────┬────────┘     └────────┬────────┘
               │                       │
               ▼                       ▼
      ┌─────────────────┐     ┌─────────────────┐
      │apps/evm/based   │     │apps/evm/single  │
      └─────────────────┘     └─────────────────┘
```

## Release Order

Packages must be released in the following order to ensure dependencies are satisfied:

### Phase 1: Core Package

1. **github.com/rollkit/rollkit/core**
   - Path: `./core`
   - Dependencies: None (zero-dependency package)
   - This is the foundation package containing all interfaces and types

### Phase 2: First-Level Dependencies

These packages only depend on `core` and can be released in parallel after `core`:

2. **github.com/rollkit/rollkit/da**
   - Path: `./da`
   - Dependencies: `rollkit/core`

3. **github.com/rollkit/rollkit**
   - Path: `./` (root)
   - Dependencies: `rollkit/core`

4. **github.com/rollkit/rollkit/execution/evm**
   - Path: `./execution/evm`
   - Dependencies: `rollkit/core`

### Phase 3: Sequencer Packages

These packages depend on both `core` and the main `rollkit` package:

5. **github.com/rollkit/rollkit/sequencers/based**
   - Path: `./sequencers/based`
   - Dependencies: `rollkit/core`, `rollkit`

6. **github.com/rollkit/rollkit/sequencers/single**
   - Path: `./sequencers/single`
   - Dependencies: `rollkit/core`, `rollkit`

### Phase 4: Application Packages

These packages have the most dependencies and should be released last:

7. **github.com/rollkit/rollkit/apps/evm/based**
   - Path: `./apps/evm/based`
   - Dependencies: `rollkit/core`, `rollkit/da`, `rollkit/execution/evm`, `rollkit`, `rollkit/sequencers/based`

8. **github.com/rollkit/rollkit/apps/evm/single**
   - Path: `./apps/evm/single`
   - Dependencies: `rollkit/core`, `rollkit/da`, `rollkit/execution/evm`, `rollkit`, `rollkit/sequencers/single`

## Release Process Workflow

**IMPORTANT**: Each module must be fully released and available before updating dependencies in dependent modules.

**NOTE**: As you go through the release process, ensure that all go mods remove the replace tags and update the go.mod file to use the new version of the module.

When starting the release process make sure that a protected version branch is created. For major versions we will have `vX`, if we have a minor breaking changes that creates an icompatability with the major version we can create a `vX.Y` branch.

Changelogs should be kept up to date, when preparing the release, ensure that all changes are reflected in the changelog and that they are properly categorized.

### Phase 1: Release Core Package

#### 1. Release `core` module

Release `core` after all changes from linting, tests and go mod tidy has been merged.

```bash
cd core
# Create and push tag
git tag core/v<version>
git push origin core/v<version>

# Wait for Go proxy (5-10 minutes)
# Verify availability
go list -m github.com/rollkit/rollkit/core@v<version>
```

### Phase 2: Release First-Level Dependencies

After core is available, update and release modules that only depend on core:

#### 2. Update and release `da` module

```bash
cd da

# Update core dependency
go get github.com/rollkit/rollkit/core@v<version>
go mod tidy

# Create and push tag
git tag da/v<version>
git push origin da/v<version>

# Verify availability
go list -m github.com/rollkit/rollkit/da@v<version>
```

#### 3. Update and release main `rollkit` module

```bash
cd . # root directory

# Update core dependency
go get github.com/rollkit/rollkit/core@v<version>
go mod tidy

# Create and push tag
git tag v<version>
git push origin v<version>

# Verify availability
go list -m github.com/rollkit/rollkit@v<version>
```

#### 4. Update and release `execution/evm` module

```bash
cd execution/evm

# Update core dependency
go get github.com/rollkit/rollkit/core@v<version>
go mod tidy


# Create and push tag
git tag execution/evm/v<version>
git push origin execution/evm/v<version>

# Verify availability
go list -m github.com/rollkit/rollkit/execution/evm@v<version>
```

### Phase 3: Release Sequencer Packages

After core and main rollkit are available, update and release sequencers:

#### 5. Update and release `sequencers/*`

```bash
# Update dependencies
go get github.com/rollkit/rollkit/core@v<version>
go get github.com/rollkit/rollkit@v<version>
go mod tidy


# Create and push tag
git tag sequencers/based/v<version>
git push origin sequencers/based/v<version>

# Verify availability
go list -m github.com/rollkit/rollkit/sequencers/based@v<version>
```

#### 6. Update and release `sequencers/single`

```bash
cd sequencers/single

# Update dependencies
go get github.com/rollkit/rollkit/core@v<version>
go get github.com/rollkit/rollkit@v<version>
go mod tidy

# Create and push tag
git tag sequencers/single/v<version>
git push origin sequencers/single/v<version>

# Verify availability
go list -m github.com/rollkit/rollkit/sequencers/single@v<version>
```

### Phase 4: Release Application Packages

After all dependencies are available, update and release applications:

#### 7. Update and release `apps/evm/based`

```bash
cd apps/evm/based

# Update all dependencies
go get github.com/rollkit/rollkit/core@v<version>
go get github.com/rollkit/rollkit/da@v<version>
go get github.com/rollkit/rollkit/execution/evm@v<version>
go get github.com/rollkit/rollkit@v<version>
go get github.com/rollkit/rollkit/sequencers/based@v<version>
go mod tidy

# Create and push tag
git tag apps/evm/based/v<version>
git push origin apps/evm/based/v<version>

# Verify availability
go list -m github.com/rollkit/rollkit/apps/evm/based@v<version>
```

#### 8. Update and release `apps/evm/single`

```bash
cd apps/evm/single

# Update all dependencies
go get github.com/rollkit/rollkit/core@v<version>
go get github.com/rollkit/rollkit/da@v<version>
go get github.com/rollkit/rollkit/execution/evm@v<version>
go get github.com/rollkit/rollkit@v<version>
go get github.com/rollkit/rollkit/sequencers/single@v<version>
go mod tidy


# Create and push tag
git tag apps/evm/single/v<version>
git push origin apps/evm/single/v<version>

# Verify availability
go list -m github.com/rollkit/rollkit/apps/evm/single@v<version>
```

## Important Notes

1. **Version Synchronization**: While modules can have independent versions, all packages must keep major versions synchronized across related modules for easier dependency management.

2. **Breaking Changes**: If a module introduces breaking changes, all dependent modules must be updated and released with appropriate version bumps.

3. **Testing**: Always test the release process in a separate branch first, especially when updating multiple modules.

4. **Go Proxy Cache**: The Go module proxy may take up to 30 minutes to fully propagate new versions. Be patient and verify availability before proceeding to dependent modules.

5. **Rollback Plan**: If issues are discovered after tagging, you are required to create a new tag with a new version (e.g., a patch release). Replacing or deleting existing tags can cause issues with the Go module proxy and should be avoided.
