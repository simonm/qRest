package executor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/simonm/qRest/internal/parser"
	"github.com/simonm/qRest/internal/translator"
)

type RESTExecutor struct {
	client      *http.Client
	authType    string
	authToken   string
}

type QueryResult struct {
	Data     []map[string]interface{} `json:"data"`
	Total    int                      `json:"total"`
	Error    string                   `json:"error,omitempty"`
	Warnings []string                 `json:"warnings,omitempty"`
}

func NewRESTExecutor(authType, authToken string) *RESTExecutor {
	return &RESTExecutor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		authType:  authType,
		authToken: authToken,
	}
}

func (e *RESTExecutor) ExecuteQuery(capability parser.APICapability, query *translator.ParsedQuery) (*QueryResult, error) {
	var resp *http.Response
	var err error
	
	switch query.QueryType {
	case "SELECT":
		// Build REST API URL with query parameters
		apiURL, err := e.buildAPIURL(capability, query)
		if err != nil {
			return nil, fmt.Errorf("failed to build API URL: %w", err)
		}
		
		// Make GET request
		resp, err = e.makeRequest("GET", apiURL, nil)
		
	case "INSERT":
		// Build base URL without query parameters
		apiURL := capability.BaseURL + capability.Path
		
		// Build request body from columns and values
		body := make(map[string]interface{})
		for i, col := range query.Columns {
			body[col] = query.Values[i]
		}
		
		// Make POST request
		resp, err = e.makeRequest("POST", apiURL, body)
		
	case "UPDATE":
		// Build URL with ID from WHERE clause
		apiURL, err := e.buildMutationURL(capability, query)
		if err != nil {
			return nil, fmt.Errorf("failed to build API URL: %w", err)
		}
		
		// Make PUT/PATCH request
		method := capability.Method // Use the method from capability (PUT or PATCH)
		resp, err = e.makeRequest(method, apiURL, query.Updates)
		
	case "DELETE":
		// Build URL with ID from WHERE clause
		apiURL, err := e.buildMutationURL(capability, query)
		if err != nil {
			return nil, fmt.Errorf("failed to build API URL: %w", err)
		}
		
		// Make DELETE request
		resp, err = e.makeRequest("DELETE", apiURL, nil)
		
	default:
		return nil, fmt.Errorf("unsupported query type: %s", query.QueryType)
	}
	
	if err != nil {
		return &QueryResult{
			Error: fmt.Sprintf("API request failed: %v", err),
		}, nil
	}

	// Check if response is nil
	if resp == nil {
		return &QueryResult{
			Error: "No response received from API",
		}, nil
	}

	// Parse response
	result, err := e.parseResponse(resp, query)
	if err != nil {
		return &QueryResult{
			Error: fmt.Sprintf("Failed to parse API response: %v", err),
		}, nil
	}

	return result, nil
}

func (e *RESTExecutor) buildAPIURL(capability parser.APICapability, query *translator.ParsedQuery) (string, error) {
	baseURL := capability.BaseURL + capability.Path
	params := url.Values{}

	// Convert SQL conditions to REST API query parameters
	for _, condition := range query.Conditions {
		paramName, paramValue, err := e.convertConditionToParam(capability, condition)
		if err != nil {
			return "", err
		}
		
		if paramName != "" {
			params.Add(paramName, paramValue)
		}
	}

	// Add ORDER BY
	if len(query.OrderBy) > 0 {
		// Try to find a sort parameter in the API
		sortParam := e.findSortParameter(capability)
		if sortParam != "" {
			sortValue := e.buildSortValue(query.OrderBy)
			params.Add(sortParam, sortValue)
		}
	}

	// Add LIMIT (pagination)
	if query.Limit > 0 {
		limitParam := e.findLimitParameter(capability)
		if limitParam != "" {
			params.Add(limitParam, strconv.Itoa(query.Limit))
		}
	}

	// Add OFFSET (pagination)
	if query.Offset > 0 {
		offsetParam := e.findOffsetParameter(capability)
		if offsetParam != "" {
			params.Add(offsetParam, strconv.Itoa(query.Offset))
		}
	}

	// Build final URL
	if len(params) > 0 {
		baseURL += "?" + params.Encode()
	}

	return baseURL, nil
}

func (e *RESTExecutor) convertConditionToParam(capability parser.APICapability, condition translator.Condition) (string, string, error) {
	// Find the parameter that matches this condition
	for _, param := range capability.Parameters {
		columnName := strings.ToLower(param.Name)
		conditionColumn := strings.ToLower(condition.Column)
		
		// Direct match
		if columnName == conditionColumn {
			if contains(param.Operators, condition.Operator) {
				return param.Name, fmt.Sprintf("%v", condition.Value), nil
			}
		}
		
		// Pattern-based match (e.g., "age_gt" for "age > 25")
		if e.matchesOperatorPattern(param.Name, condition.Column, condition.Operator) {
			return param.Name, fmt.Sprintf("%v", condition.Value), nil
		}
	}

	return "", "", fmt.Errorf("no API parameter found for condition: %s %s %v", 
		condition.Column, condition.Operator, condition.Value)
}

func (e *RESTExecutor) matchesOperatorPattern(paramName, column, operator string) bool {
	paramLower := strings.ToLower(paramName)
	columnLower := strings.ToLower(column)
	
	// Remove the column name from parameter name to check suffix
	if !strings.HasPrefix(paramLower, columnLower) {
		return false
	}
	
	suffix := strings.TrimPrefix(paramLower, columnLower+"_")
	
	switch operator {
	case ">":
		return suffix == "gt" || suffix == "greater"
	case ">=":
		return suffix == "gte" || suffix == "min"
	case "<":
		return suffix == "lt" || suffix == "less"
	case "<=":
		return suffix == "lte" || suffix == "max"
	case "!=":
		return suffix == "ne" || suffix == "not"
	case "LIKE":
		return suffix == "like" || suffix == "search" || suffix == "contains"
	case "IN":
		return suffix == "in"
	}
	
	return false
}

func (e *RESTExecutor) findSortParameter(capability parser.APICapability) string {
	for _, param := range capability.Parameters {
		name := strings.ToLower(param.Name)
		if strings.Contains(name, "sort") || strings.Contains(name, "order") {
			return param.Name
		}
	}
	return ""
}

func (e *RESTExecutor) findLimitParameter(capability parser.APICapability) string {
	for _, param := range capability.Parameters {
		name := strings.ToLower(param.Name)
		if strings.Contains(name, "limit") || strings.Contains(name, "size") || 
		   strings.Contains(name, "per_page") {
			return param.Name
		}
	}
	return ""
}

func (e *RESTExecutor) findOffsetParameter(capability parser.APICapability) string {
	for _, param := range capability.Parameters {
		name := strings.ToLower(param.Name)
		if strings.Contains(name, "offset") || strings.Contains(name, "page") {
			return param.Name
		}
	}
	return ""
}

func (e *RESTExecutor) buildSortValue(orderBy []translator.OrderByField) string {
	var parts []string
	for _, field := range orderBy {
		if field.Order == "DESC" {
			parts = append(parts, "-"+field.Column)
		} else {
			parts = append(parts, field.Column)
		}
	}
	return strings.Join(parts, ",")
}

func (e *RESTExecutor) makeRequest(method string, url string, body interface{}) (*http.Response, error) {
	var req *http.Request
	var err error
	
	// Create request with body if provided
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		req, err = http.NewRequest(method, url, strings.NewReader(string(jsonBody)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return nil, err
		}
	}

	// Add authentication
	switch strings.ToLower(e.authType) {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+e.authToken)
	case "apikey":
		req.Header.Set("X-API-Key", e.authToken)
	case "basic":
		req.Header.Set("Authorization", "Basic "+e.authToken)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "qRest/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

func (e *RESTExecutor) buildMutationURL(capability parser.APICapability, query *translator.ParsedQuery) (string, error) {
	baseURL := capability.BaseURL + capability.Path
	
	// For UPDATE and DELETE, we need to add the ID to the path
	// Look for an ID condition in WHERE clause
	for _, condition := range query.Conditions {
		if condition.Column == "id" && condition.Operator == "=" {
			// Replace {id} placeholder in path if exists
			if strings.Contains(capability.Path, "{id}") {
				baseURL = strings.Replace(baseURL, "{id}", fmt.Sprintf("%v", condition.Value), 1)
			} else {
				// Otherwise append ID to path
				baseURL = fmt.Sprintf("%s/%v", baseURL, condition.Value)
			}
			return baseURL, nil
		}
	}
	
	// If no ID found, return error
	return "", fmt.Errorf("UPDATE/DELETE requires WHERE id = value clause")
}

func (e *RESTExecutor) parseResponse(resp *http.Response, query *translator.ParsedQuery) (*QueryResult, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Try to parse as JSON
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}

	// Extract data array
	data, err := e.extractDataArray(jsonData)
	if err != nil {
		return nil, err
	}

	// Filter columns based on SELECT clause
	filteredData := e.filterColumns(data, query.Columns)

	result := &QueryResult{
		Data:  filteredData,
		Total: len(filteredData),
	}

	return result, nil
}

func (e *RESTExecutor) extractDataArray(jsonData interface{}) ([]map[string]interface{}, error) {
	switch data := jsonData.(type) {
	case []interface{}:
		// Direct array response
		var result []map[string]interface{}
		for _, item := range data {
			if itemMap, ok := item.(map[string]interface{}); ok {
				result = append(result, itemMap)
			}
		}
		return result, nil
		
	case map[string]interface{}:
		// Wrapped response - look for common data field names
		dataFields := []string{"data", "results", "items", "records", "list"}
		for _, field := range dataFields {
			if dataArray, exists := data[field]; exists {
				if array, ok := dataArray.([]interface{}); ok {
					var result []map[string]interface{}
					for _, item := range array {
						if itemMap, ok := item.(map[string]interface{}); ok {
							result = append(result, itemMap)
						}
					}
					return result, nil
				}
			}
		}
		
		// If no data field found, treat the whole object as a single record
		return []map[string]interface{}{data}, nil
		
	default:
		return nil, fmt.Errorf("unsupported JSON response format")
	}
}

func (e *RESTExecutor) filterColumns(data []map[string]interface{}, columns []string) []map[string]interface{} {
	if len(columns) == 0 {
		return data
	}

	var result []map[string]interface{}
	for _, record := range data {
		filteredRecord := make(map[string]interface{})
		
		for _, column := range columns {
			if value, exists := record[column]; exists {
				filteredRecord[column] = value
			}
		}
		
		result = append(result, filteredRecord)
	}
	
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}