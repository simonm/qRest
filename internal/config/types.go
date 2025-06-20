package config

import "time"

// Config represents the complete qRest configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server" toml:"server"`
	APIs     []APIConfig    `mapstructure:"apis" toml:"apis"`
	Defaults DefaultConfig  `mapstructure:"defaults" toml:"defaults"`
	Logging  LoggingConfig  `mapstructure:"logging" toml:"logging"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string `mapstructure:"host" toml:"host"`
	Port int    `mapstructure:"port" toml:"port"`
	CORS CORSConfig `mapstructure:"cors" toml:"cors"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigins []string `mapstructure:"allow_origins" toml:"allow_origins"`
	AllowMethods []string `mapstructure:"allow_methods" toml:"allow_methods"`
	AllowHeaders []string `mapstructure:"allow_headers" toml:"allow_headers"`
}

// APIConfig represents a single API configuration
type APIConfig struct {
	Name        string     `mapstructure:"name" toml:"name"`
	Description string     `mapstructure:"description" toml:"description"`
	SpecURL     string     `mapstructure:"spec_url" toml:"spec_url"`
	BaseURL     string     `mapstructure:"base_url" toml:"base_url"`
	Auth        AuthConfig `mapstructure:"auth" toml:"auth"`
	Timeout     string     `mapstructure:"timeout" toml:"timeout"`
	Retry       RetryConfig `mapstructure:"retry" toml:"retry"`
	Cache       CacheConfig `mapstructure:"cache" toml:"cache"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Type   string            `mapstructure:"type" toml:"type"` // none, bearer, apikey, basic
	Token  string            `mapstructure:"token" toml:"token"`
	Header string            `mapstructure:"header" toml:"header"` // For apikey auth
	Params map[string]string `mapstructure:"params" toml:"params"` // For custom auth
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	Attempts int    `mapstructure:"attempts" toml:"attempts"`
	Delay    string `mapstructure:"delay" toml:"delay"`
}

// CacheConfig holds caching configuration
type CacheConfig struct {
	Enabled bool   `mapstructure:"enabled" toml:"enabled"`
	TTL     string `mapstructure:"ttl" toml:"ttl"`
}

// DefaultConfig holds global default settings
type DefaultConfig struct {
	MaxLimit    int    `mapstructure:"max_limit" toml:"max_limit"`
	DefaultLimit int   `mapstructure:"default_limit" toml:"default_limit"`
	Timeout     string `mapstructure:"timeout" toml:"timeout"`
	CacheTTL    string `mapstructure:"cache_ttl" toml:"cache_ttl"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level" toml:"level"`   // debug, info, warn, error
	Format string `mapstructure:"format" toml:"format"` // text, json
	File   string `mapstructure:"file" toml:"file"`     // empty for stdout
}

// GetDefaultConfig returns a configuration with sensible defaults
func GetDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
			CORS: CORSConfig{
				AllowOrigins: []string{"*"},
				AllowMethods: []string{"GET", "POST", "OPTIONS"},
				AllowHeaders: []string{"Content-Type", "Authorization"},
			},
		},
		APIs: []APIConfig{},
		Defaults: DefaultConfig{
			MaxLimit:     1000,
			DefaultLimit: 100,
			Timeout:      "30s",
			CacheTTL:     "5m",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			File:   "",
		},
	}
}

// GetTimeout returns the timeout as a time.Duration
func (a *APIConfig) GetTimeout() time.Duration {
	if a.Timeout == "" {
		return 30 * time.Second
	}
	if duration, err := time.ParseDuration(a.Timeout); err == nil {
		return duration
	}
	return 30 * time.Second
}

// GetRetryDelay returns the retry delay as a time.Duration
func (r *RetryConfig) GetRetryDelay() time.Duration {
	if r.Delay == "" {
		return 1 * time.Second
	}
	if duration, err := time.ParseDuration(r.Delay); err == nil {
		return duration
	}
	return 1 * time.Second
}

// GetCacheTTL returns the cache TTL as a time.Duration
func (c *CacheConfig) GetCacheTTL() time.Duration {
	if c.TTL == "" {
		return 5 * time.Minute
	}
	if duration, err := time.ParseDuration(c.TTL); err == nil {
		return duration
	}
	return 5 * time.Minute
}

// GetDefaultTimeout returns the default timeout as a time.Duration
func (d *DefaultConfig) GetDefaultTimeout() time.Duration {
	if d.Timeout == "" {
		return 30 * time.Second
	}
	if duration, err := time.ParseDuration(d.Timeout); err == nil {
		return duration
	}
	return 30 * time.Second
}

// GetDefaultCacheTTL returns the default cache TTL as a time.Duration
func (d *DefaultConfig) GetDefaultCacheTTL() time.Duration {
	if d.CacheTTL == "" {
		return 5 * time.Minute
	}
	if duration, err := time.ParseDuration(d.CacheTTL); err == nil {
		return duration
	}
	return 5 * time.Minute
}

// FindAPI returns the API configuration by name
func (c *Config) FindAPI(name string) *APIConfig {
	for i := range c.APIs {
		if c.APIs[i].Name == name {
			return &c.APIs[i]
		}
	}
	return nil
}

// ListAPINames returns a list of all configured API names
func (c *Config) ListAPINames() []string {
	names := make([]string, len(c.APIs))
	for i, api := range c.APIs {
		names[i] = api.Name
	}
	return names
}