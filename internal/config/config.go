// Package config loads opcli settings with Viper.
//
// Precedence: flags > environment (OPENPROJECT_*) > .env in the working
// directory > config file (~/.openproject.yaml, or --config).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
	"go.yaml.in/yaml/v3"
)

// DefaultFile is the config path used when --config is not given.
func DefaultFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".openproject.yaml"
	}
	return filepath.Join(home, ".openproject.yaml")
}

// Config holds the opcli configuration.
type Config struct {
	URL     string `mapstructure:"url" yaml:"url"`
	APIKey  string `mapstructure:"api_key" yaml:"api_key"`
	Project string `mapstructure:"project" yaml:"project,omitempty"`
	// DefaultType is the type of new work packages when none is given;
	// empty means the project's default type.
	DefaultType string `mapstructure:"default_type" yaml:"default_type,omitempty"`
	// Language of text opcli writes to the server (en, es); default en.
	Language string `mapstructure:"language" yaml:"language,omitempty"`
	// File is the config file actually read (empty when none).
	File string `mapstructure:"-" yaml:"-"`
}

// Load reads the configuration. Missing credentials are not an error here:
// commands that talk to the server call Validate.
func Load(v *viper.Viper, configFile string) (*Config, error) {
	_ = gotenv.Load()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigFile(DefaultFile())
	}
	v.SetConfigType("yaml")

	_ = v.BindEnv("url", "OPENPROJECT_URL")
	_ = v.BindEnv("api_key", "OPENPROJECT_API_KEY")
	_ = v.BindEnv("project", "OPENPROJECT_PROJECT")
	_ = v.BindEnv("default_type", "OPENPROJECT_DEFAULT_TYPE")
	_ = v.BindEnv("language", "OPENPROJECT_LANGUAGE")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		if configFile != "" {
			return nil, fmt.Errorf("config file %s not found", configFile)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.File = v.ConfigFileUsed()
	if _, err := os.Stat(cfg.File); err != nil {
		cfg.File = ""
	}
	return &cfg, nil
}

// Validate checks that the settings needed to reach the server are present.
func (c *Config) Validate() error {
	if c.URL == "" || c.APIKey == "" {
		return errors.New("not configured: run `opcli config init --url <url> --api-key <key>` " +
			"or set OPENPROJECT_URL and OPENPROJECT_API_KEY (see `opcli help auth`)")
	}
	return nil
}

// Save writes c to path with 0600 permissions (it contains a secret).
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// MaskedKey shows only the first and last characters of the API key.
func (c *Config) MaskedKey() string {
	k := c.APIKey
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "…" + k[len(k)-4:]
}
