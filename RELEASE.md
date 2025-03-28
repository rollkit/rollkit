# Release

## Release Steps

* Update config version in [config/defaults.go](https://github.com/rollkit/rollkit/blob/main/config/defaults.go)
* Release new Rollkit version
* Update [Rollkit/Cosmos-SDK](https://github.com/rollkit/cosmos-sdk) with the newly released Rollkit version
* Release new Rollkit/Cosmos-SDK version

## Releasing a new Rollkit version

### Two ways

* [CI and Release Github Actions Workflows](https://github.com/rollkit/rollkit/actions/workflows/ci_release.yml)
* [Manual](https://github.com/rollkit/rollkit/releases)

#### CI and Release Actions Workflows

The CI and Release workflow involves using GitHub Actions to automate the release process. This automated process runs tests, builds the project, and handles versioning and release creation. To use this method, navigate to the GitHub Actions tab in the repository, select the CI and Release workflow, and trigger it with appropriate parameters.

#### Manual

To manually create a release, navigate to the Releases section in the GitHub repository. Click on "Draft a new release", fill in the tag version, write a title and description for your release, and select the target branch. You can also attach binaries or other assets to the release.

The manual release process also allows for creating release candidates and pre-releases by selecting the appropriate options in the release form. This gives you full control over the release process, allowing you to add detailed release notes and customize all aspects of the release.

## Update Rollkit/Cosmos-SDK

### Update Steps

* Navigate to the branch that you want to update. e.g., [release/v0.47.x](https://github.com/rollkit/cosmos-sdk/tree/release/v0.47.x) or [release/v0.47.x](https://github.com/rollkit/cosmos-sdk/tree/release/v0.50.x)
* Modify go.mod with the newly released rollkit version. e.g., `github.com/rollkit/rollkit v0.10.4`
* Run `go mod tidy` for updating the dependencies for the newly added rollkit version
* Make a pull request/commit the changes

## Release new Rollkit/Cosmos-SDK version

Make a new release of the Rollkit/Cosmos-SDK by [drafting new release](https://github.com/rollkit/cosmos-sdk/releases)
