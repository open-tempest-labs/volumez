package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the volumez configuration
type Config struct {
	// Mounts maps filesystem paths to backend configurations
	Mounts []MountConfig `json:"mounts"`

	// Cache configuration
	Cache CacheConfig `json:"cache"`

	// Global settings
	Debug bool `json:"debug"`
}

// MountConfig represents a single mount point configuration
type MountConfig struct {
	// Path is the filesystem path prefix to mount (e.g., "/data", "/images")
	Path string `json:"path"`

	// Backend is the backend type (e.g., "s3", "http")
	Backend string `json:"backend"`

	// Config contains backend-specific configuration
	Config map[string]interface{} `json:"config"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	// Enabled turns caching on/off
	Enabled bool `json:"enabled"`

	// MaxSize is the maximum cache size in bytes
	MaxSize int64 `json:"max_size"`

	// TTL is the time-to-live for cached items in seconds
	TTL int `json:"ttl"`

	// MetadataTTL is the TTL for metadata cache in seconds
	MetadataTTL int `json:"metadata_ttl"`
}

// Load reads and parses a configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Set defaults
	cfg.SetDefaults()

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if len(c.Mounts) == 0 {
		return fmt.Errorf("at least one mount is required")
	}

	// Check for duplicate paths
	seen := make(map[string]bool)
	for _, mount := range c.Mounts {
		if mount.Path == "" {
			return fmt.Errorf("mount path cannot be empty")
		}

		// Clean the path
		cleanPath := filepath.Clean(mount.Path)
		if seen[cleanPath] {
			return fmt.Errorf("duplicate mount path: %s", cleanPath)
		}
		seen[cleanPath] = true

		if mount.Backend == "" {
			return fmt.Errorf("backend type is required for mount %s", mount.Path)
		}

		if mount.Config == nil {
			return fmt.Errorf("backend config is required for mount %s", mount.Path)
		}
	}

	return nil
}

// SetDefaults sets default values for unspecified configuration
func (c *Config) SetDefaults() {
	// Cache defaults
	if !c.Cache.Enabled {
		return
	}

	if c.Cache.MaxSize == 0 {
		c.Cache.MaxSize = 1024 * 1024 * 1024 // 1GB default
	}

	if c.Cache.TTL == 0 {
		c.Cache.TTL = 300 // 5 minutes default
	}

	if c.Cache.MetadataTTL == 0 {
		c.Cache.MetadataTTL = 60 // 1 minute default
	}
}

// Save writes the configuration to a file
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Example generates an example configuration
func Example() *Config {
	return &Config{
		Mounts: []MountConfig{
			{
				Path:    "/s3-data",
				Backend: "s3",
				Config: map[string]interface{}{
					"bucket": "my-bucket",
					"region": "us-east-1",
					"prefix": "data",
				},
			},
			{
				Path:    "/api",
				Backend: "http",
				Config: map[string]interface{}{
					"base_url": "https://api.example.com/files",
					"headers": map[string]string{
						"Authorization": "Bearer YOUR_TOKEN_HERE",
					},
					"timeout": 30,
				},
			},
		},
		Cache: CacheConfig{
			Enabled:     true,
			MaxSize:     1073741824, // 1GB
			TTL:         300,
			MetadataTTL: 60,
		},
		Debug: false,
	}
}
