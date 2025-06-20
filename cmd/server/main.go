package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/simonm/qRest/internal/config"
	"github.com/simonm/qRest/internal/executor"
	"github.com/simonm/qRest/internal/grammar"
	"github.com/simonm/qRest/internal/parser"
	"github.com/simonm/qRest/internal/translator"
)

type SQLGateway struct {
	config       *config.Config
	capabilities map[string]parser.APICapability
	grammars     map[string]grammar.SQLGrammar
	executor     *executor.RESTExecutor
}

type QueryRequest struct {
	SQL string `json:"sql" binding:"required"`
}

type QueryResponse struct {
	Data         []map[string]interface{} `json:"data,omitempty"`
	Total        int                      `json:"total"`
	Error        string                   `json:"error,omitempty"`
	Warnings     []string                 `json:"warnings,omitempty"`
	Suggestions  []string                 `json:"suggestions,omitempty"`
	Grammar      interface{}              `json:"grammar,omitempty"`
}

var (
	configPath string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qRest-server",
		Short: "qRest HTTP Server",
		Long:  `qRest HTTP Server - Bridging SQL and REST APIs through intelligent discovery`,
		RunE:  runServer,
	}

	rootCmd.Flags().StringVar(&configPath, "config", "", "Path to configuration file")

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize the SQL Gateway
	gateway, err := initializeGateway(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize gateway: %w", err)
	}

	// Setup Gin router
	r := gin.Default()

	// Add CORS middleware from config
	r.Use(func(c *gin.Context) {
		if len(cfg.Server.CORS.AllowOrigins) > 0 {
			c.Header("Access-Control-Allow-Origin", strings.Join(cfg.Server.CORS.AllowOrigins, ", "))
		}
		if len(cfg.Server.CORS.AllowMethods) > 0 {
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.Server.CORS.AllowMethods, ", "))
		}
		if len(cfg.Server.CORS.AllowHeaders) > 0 {
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.Server.CORS.AllowHeaders, ", "))
		}
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	})

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"version": "1.0.0",
			"apis":    len(cfg.APIs),
		})
	})

	// Main query endpoint
	r.POST("/query", gateway.handleQuery)

	// Grammar information endpoint
	r.GET("/grammar", gateway.handleGrammar)

	// Capabilities endpoint
	r.GET("/capabilities", gateway.handleCapabilities)

	// Configuration endpoint
	r.GET("/config", gateway.handleConfig)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	
	fmt.Printf("qRest Server starting on %s\n", addr)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST /query - Execute SQL queries\n")
	fmt.Printf("  GET /grammar - View allowed SQL grammar\n")
	fmt.Printf("  GET /capabilities - View API capabilities\n")
	fmt.Printf("  GET /config - View configuration\n")
	fmt.Printf("  GET /health - Health check\n")
	fmt.Printf("\nConfigured APIs: %d\n", len(cfg.APIs))
	for _, api := range cfg.APIs {
		fmt.Printf("  - %s (%s)\n", api.Name, api.BaseURL)
	}

	if err := r.Run(addr); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	
	return nil
}

func initializeGateway(cfg *config.Config) (*SQLGateway, error) {
	gateway := &SQLGateway{
		config:       cfg,
		capabilities: make(map[string]parser.APICapability),
		grammars:     make(map[string]grammar.SQLGrammar),
	}

	if len(cfg.APIs) == 0 {
		log.Println("Warning: No API configurations found in config file.")
		log.Println("Create a qRest.toml config file or use environment variables.")
		return gateway, nil
	}

	// Initialize parsers and grammars for each API
	grammarGen := grammar.NewGrammarGenerator()
	
	for _, apiCfg := range cfg.APIs {
		// Parse OpenAPI spec
		apiParser, err := parser.NewOpenAPIParser(
			apiCfg.SpecURL,
			apiCfg.BaseURL,
			apiCfg.Auth.Type,
			apiCfg.Auth.Token,
		)
		if err != nil {
			log.Printf("Warning: Failed to load API spec for '%s': %v", apiCfg.Name, err)
			continue
		}

		// Extract capabilities
		capabilities, err := apiParser.ParseCapabilities()
		if err != nil {
			log.Printf("Warning: Failed to parse capabilities for '%s': %v", apiCfg.Name, err)
			continue
		}

		// Generate grammars
		for _, capability := range capabilities {
			// Prefix table name with API name to avoid conflicts
			tableName := capability.TableName
			if len(cfg.APIs) > 1 {
				tableName = apiCfg.Name + "_" + capability.TableName
			}
			
			gateway.capabilities[tableName] = capability
			gateway.grammars[tableName] = grammarGen.GenerateGrammar(capability)
			
			log.Printf("Loaded table '%s' from API '%s'", tableName, apiCfg.Name)
		}
	}

	// Initialize REST executor (will be created per-request with appropriate auth)
	gateway.executor = executor.NewRESTExecutor("", "")

	return gateway, nil
}

func (g *SQLGateway) handleQuery(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, QueryResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Parse SQL to extract table name
	tableName, err := g.extractTableName(req.SQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, QueryResponse{
			Error: fmt.Sprintf("Failed to parse SQL: %v", err),
		})
		return
	}

	// Find corresponding capability and grammar
	capability, exists := g.capabilities[tableName]
	if !exists {
		available := make([]string, 0, len(g.capabilities))
		for table := range g.capabilities {
			available = append(available, table)
		}
		
		c.JSON(http.StatusBadRequest, QueryResponse{
			Error: fmt.Sprintf("Table '%s' not found. Available tables: %v", tableName, available),
		})
		return
	}

	grammar := g.grammars[tableName]

	// Parse and validate SQL
	sqlTranslator := translator.NewSimpleSQLTranslator(grammar)
	parsedQuery, err := sqlTranslator.ParseSQL(req.SQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, QueryResponse{
			Error: err.Error(),
			Suggestions: grammar.WhereClause.Suggestions,
		})
		return
	}

	// Execute query
	result, err := g.executor.ExecuteQuery(capability, parsedQuery)
	if err != nil {
		c.JSON(http.StatusInternalServerError, QueryResponse{
			Error: fmt.Sprintf("Query execution failed: %v", err),
		})
		return
	}

	// Return results
	response := QueryResponse{
		Data:     result.Data,
		Total:    result.Total,
		Warnings: result.Warnings,
	}

	if result.Error != "" {
		response.Error = result.Error
	}

	c.JSON(http.StatusOK, response)
}

func (g *SQLGateway) handleGrammar(c *gin.Context) {
	tableName := c.Query("table")
	
	if tableName != "" {
		// Return grammar for specific table
		if grammarData, exists := g.grammars[tableName]; exists {
			grammarGen := grammar.NewGrammarGenerator()
			c.JSON(http.StatusOK, grammarGen.GetAllowedOperations(grammarData))
			return
		}
		
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Table '%s' not found", tableName),
		})
		return
	}

	// Return grammar for all tables
	allGrammars := make(map[string]interface{})
	grammarGen := grammar.NewGrammarGenerator()
	
	for tableName, grammarData := range g.grammars {
		allGrammars[tableName] = grammarGen.GetAllowedOperations(grammarData)
	}

	c.JSON(http.StatusOK, allGrammars)
}

func (g *SQLGateway) handleCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, g.capabilities)
}

func (g *SQLGateway) handleConfig(c *gin.Context) {
	// Return sanitized config (without sensitive tokens)
	sanitizedAPIs := make([]map[string]interface{}, len(g.config.APIs))
	
	for i, api := range g.config.APIs {
		sanitizedAPIs[i] = map[string]interface{}{
			"name":        api.Name,
			"description": api.Description,
			"spec_url":    api.SpecURL,
			"base_url":    api.BaseURL,
			"auth_type":   api.Auth.Type,
			"timeout":     api.Timeout,
		}
	}
	
	configInfo := map[string]interface{}{
		"server": map[string]interface{}{
			"host": g.config.Server.Host,
			"port": g.config.Server.Port,
		},
		"apis":     sanitizedAPIs,
		"defaults": g.config.Defaults,
		"logging":  g.config.Logging,
	}
	
	c.JSON(http.StatusOK, configInfo)
}

func (g *SQLGateway) extractTableName(sql string) (string, error) {
	// Quick extraction - for a more robust solution, we'd use the full SQL parser
	// This is a simplified version for demo purposes
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

