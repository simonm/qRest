package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// LoadConfig loads configuration from file, environment variables, and defaults
// Priority order: CLI flags > Environment variables > Config file > Defaults
func LoadConfig(configPath string) (*Config, error) {
	// Initialize viper
	v := viper.New()
	
	// Set config name and type
	v.SetConfigName("qRest")
	v.SetConfigType("toml")
	
	// Add config search paths
	if configPath != "" {
		// Use specific config file path
		v.SetConfigFile(configPath)
	} else {
		// Search in standard locations following XDG Base Directory Specification
		v.AddConfigPath(".")                                 // Current directory
		v.AddConfigPath(getXDGConfigDir())                   // XDG config directory
		v.AddConfigPath("$HOME/.qRest")                      // Legacy fallback
		v.AddConfigPath("/etc/qRest")                        // System directory
		v.AddConfigPath("/usr/local/etc/qRest")              // Alternative system directory
	}
	
	// Setup environment variable support
	v.AutomaticEnv()
	v.SetEnvPrefix("QREST")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	
	// Set defaults
	setDefaults(v)
	
	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file found but has error
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found - continue with defaults and env vars
	}
	
	// Unmarshal config
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	
	// Expand environment variables in string fields
	if err := expandEnvVars(&config); err != nil {
		return nil, fmt.Errorf("error expanding environment variables: %w", err)
	}
	
	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// LoadConfigFromAPI creates a temporary config from CLI arguments
func LoadConfigFromAPI(specURL, baseURL, authType, authToken string) (*Config, error) {
	config := GetDefaultConfig()
	
	if specURL != "" && baseURL != "" {
		apiConfig := APIConfig{
			Name:    "cli-api",
			SpecURL: specURL,
			BaseURL: baseURL,
			Auth: AuthConfig{
				Type:  authType,
				Token: authToken,
			},
			Timeout: "30s",
		}
		config.APIs = []APIConfig{apiConfig}
	}
	
	return config, nil
}

// getXDGConfigDir returns the XDG config directory for qRest
func getXDGConfigDir() string {
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		homeDir := os.Getenv("HOME")
		if homeDir != "" {
			xdgConfigHome = filepath.Join(homeDir, ".config")
		}
	}
	return filepath.Join(xdgConfigHome, "qRest")
}

// setDefaults sets default values in viper
func setDefaults(v *viper.Viper) {
	defaults := GetDefaultConfig()
	
	// Server defaults
	v.SetDefault("server.host", defaults.Server.Host)
	v.SetDefault("server.port", defaults.Server.Port)
	v.SetDefault("server.cors.allow_origins", defaults.Server.CORS.AllowOrigins)
	v.SetDefault("server.cors.allow_methods", defaults.Server.CORS.AllowMethods)
	v.SetDefault("server.cors.allow_headers", defaults.Server.CORS.AllowHeaders)
	
	// Default settings
	v.SetDefault("defaults.max_limit", defaults.Defaults.MaxLimit)
	v.SetDefault("defaults.default_limit", defaults.Defaults.DefaultLimit)
	v.SetDefault("defaults.timeout", defaults.Defaults.Timeout)
	v.SetDefault("defaults.cache_ttl", defaults.Defaults.CacheTTL)
	
	// Logging defaults
	v.SetDefault("logging.level", defaults.Logging.Level)
	v.SetDefault("logging.format", defaults.Logging.Format)
	v.SetDefault("logging.file", defaults.Logging.File)
}

// expandEnvVars expands environment variables in configuration strings
func expandEnvVars(config *Config) error {
	// Expand auth tokens
	for i := range config.APIs {
		config.APIs[i].Auth.Token = os.ExpandEnv(config.APIs[i].Auth.Token)
		
		// Expand custom auth params
		for key, value := range config.APIs[i].Auth.Params {
			config.APIs[i].Auth.Params[key] = os.ExpandEnv(value)
		}
	}
	
	// Expand logging file path
	config.Logging.File = os.ExpandEnv(config.Logging.File)
	
	return nil
}

// validateConfig validates the configuration for common errors
func validateConfig(config *Config) error {
	// Validate server port
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}
	
	// Validate APIs
	apiNames := make(map[string]bool)
	for i, api := range config.APIs {
		// Check for duplicate names
		if apiNames[api.Name] {
			return fmt.Errorf("duplicate API name: %s", api.Name)
		}
		apiNames[api.Name] = true
		
		// Validate required fields
		if api.Name == "" {
			return fmt.Errorf("API at index %d missing name", i)
		}
		if api.SpecURL == "" {
			return fmt.Errorf("API '%s' missing spec_url", api.Name)
		}
		if api.BaseURL == "" {
			return fmt.Errorf("API '%s' missing base_url", api.Name)
		}
		
		// Validate auth type
		validAuthTypes := []string{"none", "bearer", "apikey", "basic"}
		if api.Auth.Type != "" && !contains(validAuthTypes, api.Auth.Type) {
			return fmt.Errorf("API '%s' has invalid auth type: %s", api.Name, api.Auth.Type)
		}
		
		// Validate auth requirements
		if api.Auth.Type == "bearer" || api.Auth.Type == "apikey" || api.Auth.Type == "basic" {
			if api.Auth.Token == "" {
				return fmt.Errorf("API '%s' with auth type '%s' missing token", api.Name, api.Auth.Type)
			}
		}
	}
	
	// Validate defaults
	if config.Defaults.MaxLimit <= 0 {
		return fmt.Errorf("max_limit must be positive, got: %d", config.Defaults.MaxLimit)
	}
	if config.Defaults.DefaultLimit <= 0 {
		return fmt.Errorf("default_limit must be positive, got: %d", config.Defaults.DefaultLimit)
	}
	if config.Defaults.DefaultLimit > config.Defaults.MaxLimit {
		return fmt.Errorf("default_limit (%d) cannot exceed max_limit (%d)", 
			config.Defaults.DefaultLimit, config.Defaults.MaxLimit)
	}
	
	// Validate logging level
	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLogLevels, config.Logging.Level) {
		return fmt.Errorf("invalid logging level: %s", config.Logging.Level)
	}
	
	// Validate logging format
	validLogFormats := []string{"text", "json"}
	if !contains(validLogFormats, config.Logging.Format) {
		return fmt.Errorf("invalid logging format: %s", config.Logging.Format)
	}
	
	return nil
}

// GetConfigPath returns the path to a config file if it exists
func GetConfigPath() string {
	configPaths := []string{
		"qRest.toml",                                              // Current directory
		filepath.Join(getXDGConfigDir(), "qRest.toml"),           // XDG config directory
		filepath.Join(os.Getenv("HOME"), ".qRest", "qRest.toml"), // Legacy fallback
		"/etc/qRest/qRest.toml",                                  // System directory
		"/usr/local/etc/qRest/qRest.toml",                        // Alternative system directory
	}
	
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	return ""
}

// CreateConfigDir creates the config directory following XDG standards
func CreateConfigDir() error {
	configDir := getXDGConfigDir()
	return os.MkdirAll(configDir, 0755)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}