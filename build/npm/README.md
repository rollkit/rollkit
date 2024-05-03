## @rollkit/rollkit npm package

This module is for publishing the rollkit cli binary to npm under the `@rollkit/rollkit` package name.

### Installation

```bash
npm install -g @rollkit/rollkit
```

### Usage

```bash
rollkit start
```

### Development

The npm package is simplying distributing a binary. The binaries are built with `make npm-build` and are placed in the `./dist` directory. The `./dist` directory is then published to npm.

When the package is being installed, the `scripts/postinstall.js` script is run. This script will copy the binary to the local and global bin directories. As such, the only way to develop and test this is by publishing the package to npm.

NPM registry is considered immutable. It is very hard, if not impossible to unpublish a package after publishing. Even if you unpublish a package, for example rollkit@0.13.3, that name can never be used again, so you are forced to publish v0.13.4 or some other version bump.
As such, testing should be done on some sort of test package (learned the hard way 😅)

REF: https://docs.npmjs.com/policies/unpublish

When testing the publication of the package, you should publish to a test package.
This is done by changing the `name`, `version`, and `publishConfig.scope` in the `package.json` file.

i.e.
```json
{
  "name": "@rollkit/rollkit-test",
  "version": "0.0.1",
  "publishConfig": {
    "scope": "@rollkit/rollkit-test"
  },
```

You will need to use a version of the package that is not already published to npm. You can check this by searching for the package on the npm website.

REF: https://www.npmjs.com/package/@rollkit/rollkit-test

Once you've tested and verified the changes, you can then publish the package to the actual `@rollkit/rollkit` package with the new rollkit version.

i.e.
```json
{
  "name": "@rollkit/rollkit",
  "version": "0.13.4",
  "publishConfig": {
    "scope": "@rollkit/rollkit"
  },
  ```
