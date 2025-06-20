package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/simonm/qRest/internal/config"
	"github.com/simonm/qRest/internal/executor"
	"github.com/simonm/qRest/internal/grammar"
	"github.com/simonm/qRest/internal/parser"
	"github.com/simonm/qRest/internal/translator"
)

var (
	configPath string
	specURL    string
	baseURL    string
	authType   string
	authToken  string
	tableName  string
	verbose    bool
	apiName    string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qRest",
		Short: "qRest CLI - Bridging SQL and REST APIs",
		Long:  `qRest - A CLI tool to execute SQL queries against REST APIs using OpenAPI specifications.
		
Bridging the gap between SQL and modern REST APIs through intelligent discovery.`,
	}

	// Query command
	var queryCmd = &cobra.Command{
		Use:   "query [SQL]",
		Short: "Execute SQL query against REST API",
		Long:  `Execute a SQL query against a REST API endpoint using OpenAPI specification.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runQuery,
	}

	// Grammar command
	var grammarCmd = &cobra.Command{
		Use:   "grammar",
		Short: "Show allowed SQL grammar for API",
		Long:  `Display the allowed SQL grammar and operations for the configured API.`,
		RunE:  runGrammar,
	}

	// Capabilities command
	var capabilitiesCmd = &cobra.Command{
		Use:   "capabilities",
		Short: "Show API capabilities",
		Long:  `Display the discovered API capabilities from the OpenAPI specification.`,
		RunE:  runCapabilities,
	}

	// Init command
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Generate sample configuration file",
		Long:  `Generate a sample qRest.toml configuration file with examples.`,
		RunE:  runInit,
	}

	// Add flags
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to configuration file")
	rootCmd.PersistentFlags().StringVar(&specURL, "spec", "", "OpenAPI specification URL (overrides config)")
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "API base URL (overrides config)")
	rootCmd.PersistentFlags().StringVar(&authType, "auth-type", "bearer", "Authentication type (bearer, apikey, basic)")
	rootCmd.PersistentFlags().StringVar(&authToken, "auth-token", "", "Authentication token")
	rootCmd.PersistentFlags().StringVar(&apiName, "api", "", "API name from config to use")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")

	grammarCmd.Flags().StringVar(&tableName, "table", "", "Show grammar for specific table")

	// Add commands
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(grammarCmd)
	rootCmd.AddCommand(capabilitiesCmd)
	rootCmd.AddCommand(initCmd)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runQuery(cmd *cobra.Command, args []string) error {
	sql := args[0]

	// Load configuration
	cfg, apiConfig, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Load and parse API specification
	capabilities, grammars, err := loadAPICapabilities(cfg, apiConfig)
	if err != nil {
		return fmt.Errorf("failed to load API capabilities: %w", err)
	}

	// Extract table name from SQL
	tableNameFromSQL, err := extractTableName(sql)
	if err != nil {
		return fmt.Errorf("failed to parse SQL: %w", err)
	}

	// Find corresponding capability and grammar
	capability, exists := capabilities[tableNameFromSQL]
	if !exists {
		available := make([]string, 0, len(capabilities))
		for table := range capabilities {
			available = append(available, table)
		}
		return fmt.Errorf("table '%s' not found. Available tables: %v", tableNameFromSQL, available)
	}

	grammar := grammars[tableNameFromSQL]

	// Parse and validate SQL
	sqlTranslator := translator.NewSimpleSQLTranslator(grammar)
	parsedQuery, err := sqlTranslator.ParseSQL(sql)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SQL Error: %v\n", err)
		
		// Show suggestions if available
		if len(grammar.WhereClause.Suggestions) > 0 {
			fmt.Fprintf(os.Stderr, "\nSuggestions to improve the API:\n")
			for _, suggestion := range grammar.WhereClause.Suggestions {
				fmt.Fprintf(os.Stderr, "  - %s\n", suggestion)
			}
		}
		
		return err
	}

	if verbose {
		fmt.Printf("Parsed Query: %+v\n", parsedQuery)
	}

	// Execute query
	executor := executor.NewRESTExecutor(apiConfig.Auth.Type, apiConfig.Auth.Token)
	result, err := executor.ExecuteQuery(capability, parsedQuery)
	if err != nil {
		return fmt.Errorf("query execution failed: %w", err)
	}

	// Handle API error
	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "API Error: %s\n", result.Error)
		return fmt.Errorf("API returned error")
	}

	// Display warnings
	if len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
		}
	}

	// Output results
	if len(result.Data) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	// Pretty print JSON results
	jsonData, err := json.MarshalIndent(result.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format results: %w", err)
	}

	fmt.Printf("Results (%d records):\n%s\n", result.Total, string(jsonData))
	return nil
}

func runGrammar(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, apiConfig, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Load API capabilities
	_, grammars, err := loadAPICapabilities(cfg, apiConfig)
	if err != nil {
		return fmt.Errorf("failed to load API capabilities: %w", err)
	}

	grammarGen := grammar.NewGrammarGenerator()

	if tableName != "" {
		// Show grammar for specific table
		if grammar, exists := grammars[tableName]; exists {
			operations := grammarGen.GetAllowedOperations(grammar)
			jsonData, err := json.MarshalIndent(operations, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to format grammar: %w", err)
			}
			fmt.Printf("Grammar for table '%s':\n%s\n", tableName, string(jsonData))
		} else {
			return fmt.Errorf("table '%s' not found", tableName)
		}
	} else {
		// Show grammar for all tables
		allGrammars := make(map[string]interface{})
		for tableName, grammar := range grammars {
			allGrammars[tableName] = grammarGen.GetAllowedOperations(grammar)
		}

		jsonData, err := json.MarshalIndent(allGrammars, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format grammar: %w", err)
		}
		fmt.Printf("Available SQL Grammar:\n%s\n", string(jsonData))
	}

	return nil
}

func runCapabilities(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, apiConfig, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Load API capabilities
	capabilities, _, err := loadAPICapabilities(cfg, apiConfig)
	if err != nil {
		return fmt.Errorf("failed to load API capabilities: %w", err)
	}

	jsonData, err := json.MarshalIndent(capabilities, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format capabilities: %w", err)
	}

	fmt.Printf("API Capabilities:\n%s\n", string(jsonData))
	return nil
}

func loadConfig() (*config.Config, *config.APIConfig, error) {
	var cfg *config.Config
	var err error
	
	// If CLI flags are provided, use them directly
	if specURL != "" && baseURL != "" {
		cfg, err = config.LoadConfigFromAPI(specURL, baseURL, authType, authToken)
		if err != nil {
			return nil, nil, err
		}
		return cfg, &cfg.APIs[0], nil
	}
	
	// Load from config file
	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		return nil, nil, err
	}
	
	// Find specific API if name provided
	if apiName != "" {
		apiConfig := cfg.FindAPI(apiName)
		if apiConfig == nil {
			return nil, nil, fmt.Errorf("API '%s' not found in configuration", apiName)
		}
		return cfg, apiConfig, nil
	}
	
	// Use first API if available
	if len(cfg.APIs) == 0 {
		return nil, nil, fmt.Errorf("no APIs configured")
	}
	
	return cfg, &cfg.APIs[0], nil
}

func loadAPICapabilities(cfg *config.Config, apiConfig *config.APIConfig) (map[string]parser.APICapability, map[string]grammar.SQLGrammar, error) {
	// Parse OpenAPI spec
	apiParser, err := parser.NewOpenAPIParser(
		apiConfig.SpecURL, 
		apiConfig.BaseURL, 
		apiConfig.Auth.Type, 
		apiConfig.Auth.Token,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	// Extract capabilities
	capabilities, err := apiParser.ParseCapabilities()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse capabilities: %w", err)
	}

	if len(capabilities) == 0 {
		return nil, nil, fmt.Errorf("no API capabilities found in specification")
	}

	// Generate grammars
	grammarGen := grammar.NewGrammarGenerator()
	capabilityMap := make(map[string]parser.APICapability)
	grammarMap := make(map[string]grammar.SQLGrammar)

	for _, capability := range capabilities {
		capabilityMap[capability.TableName] = capability
		grammarMap[capability.TableName] = grammarGen.GenerateGrammar(capability)

		if verbose {
			fmt.Printf("Loaded table '%s' from %s\n", capability.TableName, capability.Path)
		}
	}

	return capabilityMap, grammarMap, nil
}

func runInit(cmd *cobra.Command, args []string) error {
	var configFile string
	if configPath != "" {
		configFile = configPath
	} else {
		// Create XDG config directory if it doesn't exist
		if err := config.CreateConfigDir(); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		
		// Use XDG config directory
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			homeDir := os.Getenv("HOME")
			if homeDir != "" {
				xdgConfigHome = filepath.Join(homeDir, ".config")
			}
		}
		configFile = filepath.Join(xdgConfigHome, "qRest", "qRest.toml")
	}

	// Check if file already exists
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("Configuration file '%s' already exists.\n", configFile)
		fmt.Print("Overwrite? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Generate sample config
	sampleConfig := `# qRest Configuration File
# Bridging SQL and REST APIs through intelligent discovery

[server]
host = "localhost"
port = 8080

[server.cors]
allow_origins = ["*"]
allow_methods = ["GET", "POST", "OPTIONS"]
allow_headers = ["Content-Type", "Authorization"]

# Example API configurations
[[apis]]
name = "petstore"
description = "Swagger Petstore API"
spec_url = "https://petstore.swagger.io/v2/swagger.json"
base_url = "https://petstore.swagger.io/v2"
timeout = "30s"

[apis.auth]
type = "apikey"
token = "special-key"
header = "X-API-Key"

[apis.retry]
attempts = 3
delay = "1s"

[apis.cache]
enabled = true
ttl = "5m"

[[apis]]
name = "jsonplaceholder"
description = "JSONPlaceholder REST API"
spec_url = "https://jsonplaceholder.typicode.com/openapi.json"
base_url = "https://jsonplaceholder.typicode.com"
timeout = "15s"

[apis.auth]
type = "none"

[[apis]]
name = "github"
description = "GitHub REST API"
spec_url = "https://api.github.com/openapi.json"
base_url = "https://api.github.com"
timeout = "30s"

[apis.auth]
type = "bearer"
token = "${GITHUB_TOKEN}"  # Environment variable

# Global defaults
[defaults]
max_limit = 1000
default_limit = 100
timeout = "30s"
cache_ttl = "5m"

# Logging configuration
[logging]
level = "info"    # debug, info, warn, error
format = "text"   # text, json
file = ""         # empty for stdout
`

	// Write config file
	if err := os.WriteFile(configFile, []byte(sampleConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Generated sample configuration file: %s\n", configFile)
	fmt.Println("\nNext steps:")
	fmt.Println("1. Edit the configuration file to add your API credentials")
	fmt.Println("2. Test your configuration: qRest capabilities")
	fmt.Println("3. Run SQL queries: qRest query \"SELECT * FROM table_name LIMIT 5\"")
	
	return nil
}

func extractTableName(sql string) (string, error) {
	// Simple extraction - for demo purposes
	// In production, we'd use the full SQL parser
	sqlLower := strings.ToLower(sql)
	fromIndex := strings.Index(sqlLower, "from ")
	if fromIndex == -1 {
		return "", fmt.Errorf("no FROM clause found in SQL")
	}

	// Extract table name after FROM
	remaining := sql[fromIndex+5:]
	words := strings.Fields(remaining)
	if len(words) == 0 {
		return "", fmt.Errorf("no table name found after FROM")
	}

	return strings.TrimSpace(words[0]), nil
}