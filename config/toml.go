package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// RollkitToml is the filename for the rollkit configuration file.
const RollkitToml = "rollkit.toml"

// DefaultDirPerm is the default permissions used when creating directories.
const DefaultDirPerm = 0700

// DefaultConfigDir is the default directory for configuration files.
const DefaultConfigDir = "config"

// DefaultDataDir is the default directory for data files.
const DefaultDataDir = "data"

// ErrReadToml is the error returned when reading the rollkit.toml file fails.
var ErrReadToml = fmt.Errorf("reading %s", RollkitToml)

// ReadToml reads the TOML configuration from the rollkit.toml file and returns the parsed NodeConfig.
// Only the TOML-specific fields are populated.
func ReadToml() (config NodeConfig, err error) {
	startDir, err := os.Getwd()
	if err != nil {
		err = fmt.Errorf("%w: getting current dir: %w", ErrReadToml, err)
		return
	}

	// Configure Viper to search for the configuration file
	v := viper.New()
	v.SetConfigName(RollkitToml[:len(RollkitToml)-5]) // Remove the .toml extension
	v.SetConfigType("toml")

	// Search for the configuration file in the current directory and its parents
	configPath, err := findConfigFile(startDir)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrReadToml, err)
		return
	}

	v.SetConfigFile(configPath)

	// Set default values
	config = DefaultNodeConfig

	// Read the configuration file
	if err = v.ReadInConfig(); err != nil {
		err = fmt.Errorf("%w decoding file %s: %w", ErrReadToml, configPath, err)
		return
	}

	// Unmarshal directly into NodeConfig
	if err = v.Unmarshal(&config); err != nil {
		err = fmt.Errorf("%w unmarshaling config: %w", ErrReadToml, err)
		return
	}

	// Set the root directory
	config.RootDir = filepath.Dir(configPath)

	// Add configPath to chain.ConfigDir if it is a relative path
	if config.Chain.ConfigDir != "" && !filepath.IsAbs(config.Chain.ConfigDir) {
		config.Chain.ConfigDir = filepath.Join(config.RootDir, config.Chain.ConfigDir)
	}

	return
}

// findConfigFile searches for the rollkit.toml file starting from the given
// directory and moving up the directory tree. It returns the full path to
// the rollkit.toml file or an error if it was not found.
func findConfigFile(startDir string) (string, error) {
	dir := startDir
	for {
		configPath := filepath.Join(dir, RollkitToml)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return "", fmt.Errorf("no %s found", RollkitToml)
}

// FindEntrypoint searches for a main.go file in the current directory and its
// subdirectories. It returns the directory name of the main.go file and the full
// path to the main.go file.
func FindEntrypoint() (string, string) {
	startDir, err := os.Getwd()
	if err != nil {
		return "", ""
	}

	return findDefaultEntrypoint(startDir)
}

func findDefaultEntrypoint(dir string) (string, string) {
	// Check if there is a main.go file in the current directory
	mainPath := filepath.Join(dir, "main.go")
	if _, err := os.Stat(mainPath); err == nil && !os.IsNotExist(err) {
		//dirName := filepath.Dir(dir)
		return dir, mainPath
	}

	// Check subdirectories for a main.go file
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", ""
	}

	for _, file := range files {
		if file.IsDir() {
			subdir := filepath.Join(dir, file.Name())
			dirName, entrypoint := findDefaultEntrypoint(subdir)
			if entrypoint != "" {
				return dirName, entrypoint
			}
		}
	}

	return "", ""
}

// FindConfigDir checks if there is a ~/.{dir} directory and returns the full path to it or an empty string.
// This is used to find the default config directory for cosmos-sdk chains.
func FindConfigDir(dir string) (string, bool) {
	dir = filepath.Base(dir)
	// trim last 'd' from dir if it exists
	if dir[len(dir)-1] == 'd' {
		dir = dir[:len(dir)-1]
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return dir, false
	}

	configDir := filepath.Join(home, "."+dir)
	if _, err := os.Stat(configDir); err == nil {
		return configDir, true
	}

	return dir, false
}

// WriteTomlConfig writes the TOML-specific fields of the given NodeConfig to the rollkit.toml file.
func WriteTomlConfig(config NodeConfig) error {
	// Configure Viper
	v := viper.New()

	// Set values in Viper directly from NodeConfig
	v.Set("entrypoint", config.Entrypoint)
	v.Set("chain", config.Chain)
	v.Set("rollkit", config.Rollkit)

	// Configure the output file
	configPath := filepath.Join(config.RootDir, RollkitToml)
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), DefaultDirPerm); err != nil {
		return err
	}

	// Write the configuration file
	if err := v.WriteConfig(); err != nil {
		return err
	}

	return nil
}

// EnsureRoot creates the root, config, and data directories if they don't exist,
// and panics if it fails.
func EnsureRoot(rootDir string) {
	if err := ensureDir(rootDir, DefaultDirPerm); err != nil {
		panic(err.Error())
	}
	if err := ensureDir(filepath.Join(rootDir, DefaultConfigDir), DefaultDirPerm); err != nil {
		panic(err.Error())
	}
	if err := ensureDir(filepath.Join(rootDir, DefaultDataDir), DefaultDirPerm); err != nil {
		panic(err.Error())
	}
}

// ensureDir ensures the directory exists, creating it if necessary.
func ensureDir(dirPath string, mode os.FileMode) error {
	err := os.MkdirAll(dirPath, mode)
	if err != nil {
		return fmt.Errorf("could not create directory %q: %w", dirPath, err)
	}
	return nil
}

// ReadTomlFromDir reads the TOML configuration from the rollkit.toml file in the specified directory
// and returns the parsed NodeConfig. Only the TOML-specific fields are populated.
func ReadTomlFromDir(startDir string) (config NodeConfig, err error) {
	configPath, err := findConfigFile(startDir)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrReadToml, err)
		return
	}

	// Configure Viper to read the TOML file
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Set default values
	config = DefaultNodeConfig

	// Read the configuration file
	if err = v.ReadInConfig(); err != nil {
		err = fmt.Errorf("%w decoding file %s: %w", ErrReadToml, configPath, err)
		return
	}

	// Unmarshal directly into NodeConfig
	if err = v.Unmarshal(&config); err != nil {
		err = fmt.Errorf("%w unmarshaling config: %w", ErrReadToml, err)
		return
	}

	// Set the root directory
	config.RootDir = filepath.Dir(configPath)

	// Add configPath to chain.ConfigDir if it is a relative path
	if config.Chain.ConfigDir != "" && !filepath.IsAbs(config.Chain.ConfigDir) {
		config.Chain.ConfigDir = filepath.Join(config.RootDir, config.Chain.ConfigDir)
	}

	return
}
