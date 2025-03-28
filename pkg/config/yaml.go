package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// ConfigBaseName is the base name of the rollkit configuration file without extension.
const ConfigBaseName = "rollkit"

// ConfigExtension is the file extension for the configuration file without the leading dot.
const ConfigExtension = "yaml"

// RollkitConfigYaml is the filename for the rollkit configuration file.
const RollkitConfigYaml = ConfigBaseName + "." + ConfigExtension

// ErrReadYaml is the error returned when reading the rollkit.yaml file fails.
var ErrReadYaml = fmt.Errorf("reading %s", RollkitConfigYaml)

// ReadYaml reads the YAML configuration from the rollkit.yaml file and returns the parsed Config.
// If dir is provided, it will look for the config file in that directory.
func ReadYaml(dir string) (config Config, err error) {
	// Configure Viper to search for the configuration file
	v := viper.New()
	v.SetConfigName(ConfigBaseName)
	v.SetConfigType(ConfigExtension)

	if dir != "" {
		// If a directory is provided, look for the config file there
		v.AddConfigPath(dir)
	} else {
		// Otherwise, search for the configuration file in the current directory and its parents
		startDir, err := os.Getwd()
		if err != nil {
			err = fmt.Errorf("%w: getting current dir: %w", ErrReadYaml, err)
			return config, err
		}

		configPath, err := findConfigFile(startDir)
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrReadYaml, err)
			return config, err
		}

		v.SetConfigFile(configPath)
	}

	// Set default values
	config = DefaultNodeConfig

	// Read the configuration file
	if err = v.ReadInConfig(); err != nil {
		err = fmt.Errorf("%w decoding file: %w", ErrReadYaml, err)
		return
	}

	// Unmarshal directly into Config
	if err = v.Unmarshal(&config, func(c *mapstructure.DecoderConfig) {
		c.TagName = "mapstructure"
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
				if t == reflect.TypeOf(DurationWrapper{}) && f.Kind() == reflect.String {
					if str, ok := data.(string); ok {
						duration, err := time.ParseDuration(str)
						if err != nil {
							return nil, err
						}
						return DurationWrapper{Duration: duration}, nil
					}
				}
				return data, nil
			},
		)
	}); err != nil {
		err = fmt.Errorf("%w unmarshaling config: %w", ErrReadYaml, err)
		return
	}

	// Set the root directory
	if dir != "" {
		config.RootDir = dir
	} else {
		config.RootDir = filepath.Dir(v.ConfigFileUsed())
	}

	// Add configPath to ConfigDir if it is a relative path
	if config.ConfigDir != "" && !filepath.IsAbs(config.ConfigDir) {
		config.ConfigDir = filepath.Join(config.RootDir, config.ConfigDir)
	}

	return
}

// findConfigFile searches for the rollkit.yaml file starting from the given
// directory and moving up the directory tree. It returns the full path to
// the rollkit.yaml file or an error if it was not found.
func findConfigFile(startDir string) (string, error) {
	dir := startDir
	for {
		configPath := filepath.Join(dir, RollkitConfigYaml)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return "", fmt.Errorf("no %s found", RollkitConfigYaml)
}

// WriteYamlConfig writes the YAML configuration to the rollkit.yaml file.
// It ensures the directory exists and writes the configuration with proper permissions.
func WriteYamlConfig(config Config) error {
	// Configure the output file
	configPath := filepath.Join(config.RootDir, RollkitConfigYaml)

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), DefaultDirPerm); err != nil {
		return err
	}

	// Marshal the config to YAML with comments
	yamlCommentMap := yaml.CommentMap{}

	// Helper function to add comments
	addComment := func(path string, comment string) {
		yamlCommentMap[path] = []*yaml.Comment{
			yaml.HeadComment(comment),
		}
	}

	// Helper function to process struct fields recursively
	var processFields func(t reflect.Type, prefix string)
	processFields = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			// Skip unexported fields
			if !field.IsExported() {
				continue
			}

			yamlTag := field.Tag.Get("yaml")
			comment := field.Tag.Get("comment")

			// Skip fields without yaml tag
			if yamlTag == "" || yamlTag == "-" {
				continue
			}

			// Handle embedded structs
			if field.Type.Kind() == reflect.Struct && field.Anonymous {
				processFields(field.Type, prefix)
				continue
			}

			// Get the field path
			fieldPath := yamlTag
			if prefix != "" {
				fieldPath = prefix + "." + fieldPath
			}
			fieldPath = "$." + fieldPath

			// Add comment if present
			if comment != "" {
				addComment(fieldPath, comment)
			}

			// Process nested structs (non-anonymous)
			if field.Type.Kind() == reflect.Struct && !field.Anonymous {
				processFields(field.Type, yamlTag)
			}
		}
	}

	// Process the Config struct
	processFields(reflect.TypeOf(Config{}), "")

	data, err := yaml.MarshalWithOptions(config, yaml.WithComment(yamlCommentMap))
	if err != nil {
		return fmt.Errorf("error marshaling YAML data: %w", err)
	}

	// Write the YAML data to the file
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("error writing %s file: %w", RollkitConfigYaml, err)
	}

	return nil
}

// EnsureRoot ensures that the root directory exists.
func EnsureRoot(rootDir string) error {
	if rootDir == "" {
		return fmt.Errorf("root directory cannot be empty")
	}

	if err := os.MkdirAll(rootDir, DefaultDirPerm); err != nil {
		return fmt.Errorf("could not create directory %q: %w", rootDir, err)
	}

	return nil
}
