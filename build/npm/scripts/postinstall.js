const os = require("os");
const path = require("path");
const fs = require("fs");
const { execSync } = require("child_process");

const platform = os.platform();
let binaryName;
const appName = "rollkit";

switch (platform) {
  case "win32":
    binaryName = "rollkit-windows-amd64";
    break;
  case "darwin":
    binaryName = "rollkit-darwin-amd64";
    break;
  case "linux":
    binaryName = "rollkit-linux-amd64";
    break;
  default:
    console.error("Unsupported platform:", platform);
    process.exit(1);
}

const sourcePath = path.join(__dirname, "..", "dist", binaryName);
const destDir = path.join(__dirname, "..", "bin");
const destPath = path.join(destDir, appName);

// ** Bin Installation at Download Location **
//
// This installs the binary into a bin directory where the npm package was downloaded.
// This is useful for development and testing, but not for production use.

// Ensure the destination directory exists
if (!fs.existsSync(destDir)) {
  fs.mkdirSync(destDir);
}

// Copy the binary to the destination directory
fs.copyFileSync(sourcePath, destPath);

// Make the binary executable (for macOS and Linux)
if (platform !== "win32") {
  fs.chmodSync(destPath, "755");
}

console.log(`Installed ${binaryName} to ${destPath}`);

// ** Local Bin Installation **
//
// This installs the binary into the node_modules/.bin directory of the package in the local project folder.

// Handle local installation
const localBinDir = path.join(__dirname, "..", "node_modules", ".bin");
const localBinPath = path.join(localBinDir, appName);

// Ensure the local bin directory exists
if (!fs.existsSync(localBinDir)) {
  fs.mkdirSync(localBinDir, { recursive: true });
}

fs.copyFileSync(destPath, localBinPath);

if (platform !== "win32") {
  fs.chmodSync(localBinPath, "755");
}

console.log(`Installed ${binaryName} to ${destPath} and ${localBinPath}`);

// ** Global Bin Installation **
//
// This installs the binary into the global npm bin directory.

// Handle global installation
if (process.env.npm_config_global) {
  const globalPrefix = execSync("npm config get prefix").toString().trim();
  const globalBinDir = path.join(globalPrefix, "bin");
  const globalBinPath = path.join(globalBinDir, appName);

  fs.copyFileSync(destPath, globalBinPath);
  if (platform !== "win32") {
    fs.chmodSync(globalBinPath, "755");
  }
  console.log(`Installed ${binaryName} globally to ${globalBinPath}`);
}
