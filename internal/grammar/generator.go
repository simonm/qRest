package grammar

import (
	"fmt"
	"strings"

	"github.com/simonm/qRest/internal/parser"
)

type SQLGrammar struct {
	TableName      string
	AllowedColumns []string
	WhereClause    WhereGrammar
	OrderBy        OrderByGrammar
	Limit          LimitGrammar
}

type WhereGrammar struct {
	AllowedColumns map[string][]string // column -> allowed operators
	Suggestions    []string
}

type OrderByGrammar struct {
	AllowedColumns []string
	DefaultOrder   string
}

type LimitGrammar struct {
	MaxLimit     int
	DefaultLimit int
	HasPaging    bool
}

type GrammarGenerator struct{}

func NewGrammarGenerator() *GrammarGenerator {
	return &GrammarGenerator{}
}

func (g *GrammarGenerator) GenerateGrammar(capability parser.APICapability) SQLGrammar {
	grammar := SQLGrammar{
		TableName: capability.TableName,
		WhereClause: WhereGrammar{
			AllowedColumns: make(map[string][]string),
		},
		OrderBy: OrderByGrammar{
			AllowedColumns: []string{},
		},
		Limit: LimitGrammar{
			MaxLimit:     capability.MaxResults,
			DefaultLimit: 100,
			HasPaging:    capability.HasPaging,
		},
	}

	// Use response columns for SELECT clause (what can be retrieved)
	if len(capability.ResponseColumns) > 0 {
		grammar.AllowedColumns = capability.ResponseColumns
		grammar.OrderBy.AllowedColumns = capability.ResponseColumns
	} else {
		// Fallback to parameter-based columns if no response schema
		grammar.AllowedColumns = []string{}
		grammar.OrderBy.AllowedColumns = []string{}
	}

	// Process parameters to build WHERE clause operations (what can be filtered)
	for _, param := range capability.Parameters {
		// Skip pagination parameters from WHERE clause
		if isPaginationParam(param.Name) {
			continue
		}

		columnName := g.extractColumnName(param.Name)
		if columnName == "" {
			continue
		}

		// Add to WHERE clause operators
		if len(param.Operators) > 0 {
			grammar.WhereClause.AllowedColumns[columnName] = param.Operators
		} else {
			grammar.WhereClause.AllowedColumns[columnName] = []string{"="}
		}

		// If no response columns available, add parameter columns to allowed columns
		if len(capability.ResponseColumns) == 0 {
			grammar.AllowedColumns = append(grammar.AllowedColumns, columnName)
			grammar.OrderBy.AllowedColumns = append(grammar.OrderBy.AllowedColumns, columnName)
		}
	}

	// Generate suggestions for API improvements
	grammar.WhereClause.Suggestions = g.generateSuggestions(capability)

	// Set default limit if not specified
	if grammar.Limit.MaxLimit == 0 {
		grammar.Limit.MaxLimit = 1000
	}

	return grammar
}

func (g *GrammarGenerator) extractColumnName(paramName string) string {
	// Remove common suffixes that indicate operators
	suffixes := []string{"_gt", "_gte", "_lt", "_lte", "_ne", "_not", "_in", "_like", "_between", "_min", "_max"}
	
	name := strings.ToLower(paramName)
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	
	return paramName
}

func (g *GrammarGenerator) generateSuggestions(capability parser.APICapability) []string {
	var suggestions []string
	
	// Analyze missing common operators
	for _, param := range capability.Parameters {
		columnName := g.extractColumnName(param.Name)
		
		// Suggest range operators for numeric/date fields
		if param.Type == "integer" || param.Type == "number" || 
		   strings.Contains(strings.ToLower(param.Name), "date") {
			
			hasRange := false
			for _, op := range param.Operators {
				if op == ">" || op == ">=" || op == "<" || op == "<=" {
					hasRange = true
					break
				}
			}
			
			if !hasRange {
				suggestions = append(suggestions, 
					fmt.Sprintf("Add range filtering for '%s' (e.g., %s_gt, %s_lt parameters)", 
						columnName, columnName, columnName))
			}
		}
		
		// Suggest LIKE operator for string fields
		if param.Type == "string" && !contains(param.Operators, "LIKE") {
			suggestions = append(suggestions, 
				fmt.Sprintf("Add partial text search for '%s' (e.g., %s_like parameter)", 
					columnName, columnName))
		}
		
		// Suggest IN operator for enum fields
		if len(param.Enum) > 0 && !contains(param.Operators, "IN") {
			suggestions = append(suggestions, 
				fmt.Sprintf("Add multiple value filtering for '%s' (e.g., %s_in parameter)", 
					columnName, columnName))
		}
	}
	
	// Suggest pagination if not available
	if !capability.HasPaging {
		suggestions = append(suggestions, 
			"Add pagination support (e.g., 'limit' and 'offset' or 'page' parameters)")
	}
	
	// Suggest ordering if no sortable parameters found
	if len(capability.Parameters) > 0 {
		hasSorting := false
		for _, param := range capability.Parameters {
			if strings.Contains(strings.ToLower(param.Name), "sort") || 
			   strings.Contains(strings.ToLower(param.Name), "order") {
				hasSorting = true
				break
			}
		}
		
		if !hasSorting {
			suggestions = append(suggestions, 
				"Add sorting support (e.g., 'sort_by' and 'order' parameters)")
		}
	}
	
	return suggestions
}

func (g *GrammarGenerator) ValidateSQL(grammar SQLGrammar, sql string) error {
	// This would integrate with the SQL parser to validate
	// For now, return a placeholder
	return nil
}

func (g *GrammarGenerator) GetAllowedOperations(grammar SQLGrammar) map[string]interface{} {
	operations := map[string]interface{}{
		"table":   grammar.TableName,
		"columns": grammar.AllowedColumns,
		"where":   grammar.WhereClause.AllowedColumns,
		"order_by": grammar.OrderBy.AllowedColumns,
		"limit": map[string]interface{}{
			"max":     grammar.Limit.MaxLimit,
			"default": grammar.Limit.DefaultLimit,
			"paging":  grammar.Limit.HasPaging,
		},
		"suggestions": grammar.WhereClause.Suggestions,
	}
	
	return operations
}

func isPaginationParam(paramName string) bool {
	name := strings.ToLower(paramName)
	pagingParams := []string{"limit", "offset", "page", "per_page", "page_size", "size"}
	
	for _, param := range pagingParams {
		if strings.Contains(name, param) {
			return true
		}
	}
	
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}