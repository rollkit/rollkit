package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/goccy/go-yaml"
)

// DurationWrapper is a wrapper for time.Duration that implements encoding.TextMarshaler and encoding.TextUnmarshaler
// needed for YAML marshalling/unmarshalling especially for time.Duration
type DurationWrapper struct {
	time.Duration
}

// MarshalText implements encoding.TextMarshaler to format the duration as text
func (d DurationWrapper) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler to parse the duration from text
func (d *DurationWrapper) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// ErrReadYaml is the error returned when reading the rollkit.yaml file fails.
var ErrReadYaml = fmt.Errorf("reading %s", ConfigName)

// SaveAsYaml saves the current configuration to a YAML file.
func (c *Config) SaveAsYaml() error {
	configPath := filepath.Join(DefaultConfig.RootDir, AppConfigDir, ConfigName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return fmt.Errorf("could not create directory %q: %w", filepath.Dir(configPath), err)
	}

	// helper function to add comments
	yamlCommentMap := yaml.CommentMap{}
	addComment := func(path string, comment string) {
		yamlCommentMap[path] = []*yaml.Comment{
			yaml.HeadComment(comment),
		}
	}

	// helper function to process struct fields recursively
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

	// process structs fields and comments
	processFields(reflect.TypeOf(Config{}), "")

	data, err := yaml.MarshalWithOptions(c, yaml.WithComment(yamlCommentMap))
	if err != nil {
		return fmt.Errorf("error marshaling YAML data: %w", err)
	}

	return os.WriteFile(configPath, data, 0o600)
}
