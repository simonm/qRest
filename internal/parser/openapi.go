package parser

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
)

type APICapability struct {
	Path            string
	Method          string
	Parameters      []Parameter
	ResponseColumns []string // Available columns from response schema
	TableName       string
	BaseURL         string
	MaxResults      int
	HasPaging       bool
	PageParam       string
	LimitParam      string
}

type Parameter struct {
	Name        string
	Type        string
	Location    string // query, path, header
	Required    bool
	Format      string
	Enum        []string
	Operators   []string // derived from name patterns like "age_gt", "created_at_gte"
}

type OpenAPIParser struct {
	spec     *spec.Swagger
	baseURL  string
	authType string
	authToken string
}

func NewOpenAPIParser(specURL, baseURL, authType, authToken string) (*OpenAPIParser, error) {
	doc, err := loads.Spec(specURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	return &OpenAPIParser{
		spec:      doc.Spec(),
		baseURL:   baseURL,
		authType:  authType,
		authToken: authToken,
	}, nil
}

func (p *OpenAPIParser) ParseCapabilities() ([]APICapability, error) {
	var capabilities []APICapability

	for path, pathItem := range p.spec.Paths.Paths {
		// Parse all HTTP methods for comprehensive capability discovery
		
		// GET operations - for SELECT queries
		if pathItem.Get != nil {
			capability := p.parseOperation(path, "GET", pathItem.Get)
			if capability != nil {
				capabilities = append(capabilities, *capability)
			}
		}
		
		// POST operations - for INSERT queries
		if pathItem.Post != nil {
			capability := p.parseOperation(path, "POST", pathItem.Post)
			if capability != nil {
				capabilities = append(capabilities, *capability)
			}
		}
		
		// PUT operations - for UPDATE queries (full replacement)
		if pathItem.Put != nil {
			capability := p.parseOperation(path, "PUT", pathItem.Put)
			if capability != nil {
				capabilities = append(capabilities, *capability)
			}
		}
		
		// PATCH operations - for UPDATE queries (partial update)
		if pathItem.Patch != nil {
			capability := p.parseOperation(path, "PATCH", pathItem.Patch)
			if capability != nil {
				capabilities = append(capabilities, *capability)
			}
		}
		
		// DELETE operations - for DELETE queries
		if pathItem.Delete != nil {
			capability := p.parseOperation(path, "DELETE", pathItem.Delete)
			if capability != nil {
				capabilities = append(capabilities, *capability)
			}
		}
	}

	return capabilities, nil
}

func (p *OpenAPIParser) parseOperation(path string, method string, operation *spec.Operation) *APICapability {
	// Extract table name from path (e.g., "/users" -> "users", "/api/v1/users" -> "users")
	tableName := p.extractTableName(path)
	if tableName == "" {
		return nil
	}

	// Create unique table name for different operations on same resource
	// e.g., users_get, users_post, users_update, users_delete
	operationTableName := tableName
	if method != "GET" {
		operationTableName = fmt.Sprintf("%s_%s", tableName, strings.ToLower(method))
	}

	capability := &APICapability{
		Path:      path,
		Method:    method,
		TableName: operationTableName,
		BaseURL:   p.baseURL,
	}

	// Parse parameters
	for _, param := range operation.Parameters {
		if param.In == "query" {
			parameter := Parameter{
				Name:     param.Name,
				Type:     param.Type,
				Location: param.In,
				Required: param.Required,
				Format:   param.Format,
			}

			// Detect operators from parameter naming patterns
			parameter.Operators = p.detectOperators(param.Name, param.Type)

			// Handle enum values
			if param.Enum != nil {
				for _, v := range param.Enum {
					if s, ok := v.(string); ok {
						parameter.Enum = append(parameter.Enum, s)
					}
				}
			}

			capability.Parameters = append(capability.Parameters, parameter)

			// Detect pagination parameters
			if strings.Contains(strings.ToLower(param.Name), "limit") || 
			   strings.Contains(strings.ToLower(param.Name), "size") ||
			   strings.Contains(strings.ToLower(param.Name), "per_page") {
				capability.HasPaging = true
				capability.LimitParam = param.Name
				if param.Maximum != nil {
					capability.MaxResults = int(*param.Maximum)
				}
			}

			if strings.Contains(strings.ToLower(param.Name), "page") ||
			   strings.Contains(strings.ToLower(param.Name), "offset") {
				capability.HasPaging = true
				capability.PageParam = param.Name
			}
		}
	}

	// Parse response schema to extract available columns
	capability.ResponseColumns = p.extractResponseColumns(operation)

	return capability
}

func (p *OpenAPIParser) extractTableName(path string) string {
	// Remove leading slash and split by slash
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	
	// Look for the last part that doesn't contain parameters
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		// Skip path parameters (e.g., "{id}")
		if !strings.HasPrefix(part, "{") && !strings.HasSuffix(part, "}") {
			// Skip common API prefixes
			if part != "api" && part != "v1" && part != "v2" && part != "v3" {
				return part
			}
		}
	}
	
	return ""
}

func (p *OpenAPIParser) detectOperators(paramName, paramType string) []string {
	var operators []string
	
	// Common REST API parameter patterns
	name := strings.ToLower(paramName)
	
	// Exact match (default)
	operators = append(operators, "=")
	
	// Numeric/date operators
	if paramType == "integer" || paramType == "number" || 
	   strings.Contains(name, "date") || strings.Contains(name, "time") {
		
		if strings.HasSuffix(name, "_gt") || strings.HasSuffix(name, "_greater") {
			operators = append(operators, ">")
		}
		if strings.HasSuffix(name, "_gte") || strings.HasSuffix(name, "_min") {
			operators = append(operators, ">=")
		}
		if strings.HasSuffix(name, "_lt") || strings.HasSuffix(name, "_less") {
			operators = append(operators, "<")
		}
		if strings.HasSuffix(name, "_lte") || strings.HasSuffix(name, "_max") {
			operators = append(operators, "<=")
		}
		if strings.HasSuffix(name, "_between") {
			operators = append(operators, "BETWEEN")
		}
		
		// If no specific pattern, assume basic numeric comparisons are possible
		if len(operators) == 1 {
			operators = append(operators, ">", ">=", "<", "<=")
		}
	}
	
	// String operators
	if paramType == "string" {
		if strings.Contains(name, "search") || strings.Contains(name, "query") ||
		   strings.Contains(name, "filter") || strings.Contains(name, "name") {
			operators = append(operators, "LIKE", "ILIKE")
		}
		if strings.HasSuffix(name, "_in") {
			operators = append(operators, "IN")
		}
	}
	
	// NOT operator (common pattern)
	if strings.HasSuffix(name, "_not") || strings.HasSuffix(name, "_ne") {
		operators = append(operators, "!=", "<>")
	}
	
	return operators
}

func (p *OpenAPIParser) BuildURL(capability APICapability, filters map[string]interface{}) (string, error) {
	baseURL := capability.BaseURL + capability.Path
	
	if len(filters) == 0 {
		return baseURL, nil
	}
	
	values := url.Values{}
	for key, value := range filters {
		values.Add(key, fmt.Sprintf("%v", value))
	}
	
	if len(values) > 0 {
		baseURL += "?" + values.Encode()
	}
	
	return baseURL, nil
}

func (p *OpenAPIParser) extractResponseColumns(operation *spec.Operation) []string {
	var columns []string
	
	// Look for 200 response
	if operation.Responses == nil || operation.Responses.StatusCodeResponses == nil {
		return columns
	}
	
	response, exists := operation.Responses.StatusCodeResponses[200]
	if !exists || response.Schema == nil {
		return columns
	}
	
	// Extract columns from response schema
	columns = p.extractColumnsFromSchema(response.Schema)
	
	return columns
}

func (p *OpenAPIParser) extractColumnsFromSchema(schema *spec.Schema) []string {
	var columns []string
	
	if schema == nil {
		return columns
	}
	
	// Handle array responses (most common for list endpoints)
	if schema.Type.Contains("array") && schema.Items != nil && schema.Items.Schema != nil {
		// Extract from array item schema
		return p.extractColumnsFromSchema(schema.Items.Schema)
	}
	
	// Handle object responses
	if schema.Type.Contains("object") && schema.Properties != nil {
		// Extract property names
		for propName := range schema.Properties {
			columns = append(columns, propName)
		}
	}
	
	// Handle referenced schemas
	if schema.Ref.String() != "" {
		// Try to resolve the reference
		if resolved := p.resolveSchemaRef(schema.Ref.String()); resolved != nil {
			return p.extractColumnsFromSchema(resolved)
		}
	}
	
	return columns
}

func (p *OpenAPIParser) resolveSchemaRef(ref string) *spec.Schema {
	// Simple reference resolution - look in definitions
	if p.spec.Definitions != nil {
		// Extract definition name from ref (e.g., "#/definitions/Pet" -> "Pet")
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			defName := parts[len(parts)-1]
			if schema, exists := p.spec.Definitions[defName]; exists {
				return &schema
			}
		}
	}
	return nil
}