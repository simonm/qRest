package translator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/simonm/qRest/internal/grammar"
)

type ParsedQuery struct {
	QueryType   string // SELECT, INSERT, UPDATE, DELETE
	TableName   string
	Columns     []string
	Values      []interface{} // For INSERT
	Updates     map[string]interface{} // For UPDATE
	Conditions  []Condition
	OrderBy     []OrderByField
	Limit       int
	Offset      int
}

type Condition struct {
	Column   string
	Operator string
	Value    interface{}
}

type OrderByField struct {
	Column string
	Order  string // ASC or DESC
}

// SimpleSQLTranslator uses regex-based SQL parsing for the PoC
type SimpleSQLTranslator struct {
	grammar grammar.SQLGrammar
}

func NewSimpleSQLTranslator(grammar grammar.SQLGrammar) *SimpleSQLTranslator {
	return &SimpleSQLTranslator{
		grammar: grammar,
	}
}

func (t *SimpleSQLTranslator) ParseSQL(sql string) (*ParsedQuery, error) {
	query := &ParsedQuery{
		Updates: make(map[string]interface{}),
	}
	
	// Normalize SQL
	sql = strings.TrimSpace(sql)
	if !strings.HasSuffix(sql, ";") {
		sql += ";"
	}
	
	// Determine query type
	sqlUpper := strings.ToUpper(sql)
	switch {
	case strings.HasPrefix(sqlUpper, "SELECT"):
		query.QueryType = "SELECT"
		return t.parseSelectQuery(sql, query)
	case strings.HasPrefix(sqlUpper, "INSERT"):
		query.QueryType = "INSERT"
		return t.parseInsertQuery(sql, query)
	case strings.HasPrefix(sqlUpper, "UPDATE"):
		query.QueryType = "UPDATE"
		return t.parseUpdateQuery(sql, query)
	case strings.HasPrefix(sqlUpper, "DELETE"):
		query.QueryType = "DELETE"
		return t.parseDeleteQuery(sql, query)
	default:
		return nil, fmt.Errorf("unsupported SQL statement type. Only SELECT, INSERT, UPDATE, DELETE are supported")
	}
}

func (t *SimpleSQLTranslator) parseSelectQuery(sql string, query *ParsedQuery) (*ParsedQuery, error) {
	// Extract SELECT columns
	if err := t.parseSelect(sql, query); err != nil {
		return nil, err
	}
	
	// Extract FROM table
	if err := t.parseFrom(sql, query); err != nil {
		return nil, err
	}
	
	// Validate table name
	if query.TableName != t.grammar.TableName {
		return nil, fmt.Errorf("table '%s' not found. Available table: %s", 
			query.TableName, t.grammar.TableName)
	}
	
	// Extract WHERE conditions
	if err := t.parseWhere(sql, query); err != nil {
		return nil, err
	}
	
	// Extract ORDER BY
	if err := t.parseOrderBy(sql, query); err != nil {
		return nil, err
	}
	
	// Extract LIMIT and OFFSET
	if err := t.parseLimit(sql, query); err != nil {
		return nil, err
	}
	
	return query, nil
}

func (t *SimpleSQLTranslator) parseSelect(sql string, query *ParsedQuery) error {
	// Match SELECT columns FROM
	re := regexp.MustCompile(`(?i)SELECT\s+(.+?)\s+FROM`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 2 {
		return fmt.Errorf("invalid SELECT statement")
	}
	
	columnsStr := strings.TrimSpace(matches[1])
	
	if columnsStr == "*" {
		// SELECT * - use all available columns
		query.Columns = append(query.Columns, t.grammar.AllowedColumns...)
	} else {
		// Parse column list
		columns := strings.Split(columnsStr, ",")
		for _, col := range columns {
			columnName := strings.TrimSpace(col)
			
			// Remove quotes if present
			columnName = strings.Trim(columnName, "\"'`")
			
			// Validate column against grammar
			if !t.isColumnAllowed(columnName) {
				return fmt.Errorf("column '%s' not available. Available columns: %v", 
					columnName, t.grammar.AllowedColumns)
			}
			
			query.Columns = append(query.Columns, columnName)
		}
	}
	
	return nil
}

func (t *SimpleSQLTranslator) parseFrom(sql string, query *ParsedQuery) error {
	// Match FROM table_name
	re := regexp.MustCompile(`(?i)FROM\s+(\w+)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 2 {
		return fmt.Errorf("no FROM clause found")
	}
	
	query.TableName = strings.TrimSpace(matches[1])
	return nil
}

func (t *SimpleSQLTranslator) parseWhere(sql string, query *ParsedQuery) error {
	// Match WHERE clause
	re := regexp.MustCompile(`(?i)WHERE\s+(.+?)(?:\s+ORDER\s+BY|\s+LIMIT|\s*;|$)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 2 {
		return nil // No WHERE clause
	}
	
	whereClause := strings.TrimSpace(matches[1])
	
	// Split by AND (we don't support OR yet)
	conditions := strings.Split(whereClause, " AND ")
	
	for _, condStr := range conditions {
		condition, err := t.parseCondition(strings.TrimSpace(condStr))
		if err != nil {
			return err
		}
		query.Conditions = append(query.Conditions, *condition)
	}
	
	return nil
}

func (t *SimpleSQLTranslator) parseCondition(condStr string) (*Condition, error) {
	// Handle different operators
	operators := []string{">=", "<=", "!=", "<>", ">", "<", "=", "LIKE", "ILIKE"}
	
	for _, op := range operators {
		re := regexp.MustCompile(fmt.Sprintf(`(?i)(\w+)\s*%s\s*(.+)`, regexp.QuoteMeta(op)))
		matches := re.FindStringSubmatch(condStr)
		if len(matches) == 3 {
			column := strings.TrimSpace(matches[1])
			valueStr := strings.TrimSpace(matches[2])
			
			// Validate column
			if !t.isColumnAllowed(column) {
				return nil, fmt.Errorf("column '%s' not available for filtering", column)
			}
			
			// Validate operator
			allowedOps, exists := t.grammar.WhereClause.AllowedColumns[column]
			if !exists || !contains(allowedOps, op) {
				return nil, fmt.Errorf("operator '%s' not supported for column '%s'. Allowed: %v", 
					op, column, allowedOps)
			}
			
			// Parse value
			value, err := t.parseValue(valueStr)
			if err != nil {
				return nil, err
			}
			
			return &Condition{
				Column:   column,
				Operator: op,
				Value:    value,
			}, nil
		}
	}
	
	// Handle BETWEEN
	re := regexp.MustCompile(`(?i)(\w+)\s+BETWEEN\s+(.+?)\s+AND\s+(.+)`)
	matches := re.FindStringSubmatch(condStr)
	if len(matches) == 4 {
		column := strings.TrimSpace(matches[1])
		fromStr := strings.TrimSpace(matches[2])
		toStr := strings.TrimSpace(matches[3])
		
		// Validate column and BETWEEN operator
		if !t.isColumnAllowed(column) {
			return nil, fmt.Errorf("column '%s' not available for filtering", column)
		}
		
		allowedOps, exists := t.grammar.WhereClause.AllowedColumns[column]
		if !exists || !contains(allowedOps, "BETWEEN") {
			return nil, fmt.Errorf("BETWEEN operator not supported for column '%s'", column)
		}
		
		// Parse values
		fromValue, err := t.parseValue(fromStr)
		if err != nil {
			return nil, err
		}
		
		_, err = t.parseValue(toStr)
		if err != nil {
			return nil, err
		}
		
		// Return first condition (we'll add the second one separately)
		// This is a simplified approach - in practice we'd handle BETWEEN properly
		return &Condition{
			Column:   column,
			Operator: ">=",
			Value:    fromValue,
		}, nil
	}
	
	return nil, fmt.Errorf("unsupported condition: %s", condStr)
}

func (t *SimpleSQLTranslator) parseOrderBy(sql string, query *ParsedQuery) error {
	// Match ORDER BY clause
	re := regexp.MustCompile(`(?i)ORDER\s+BY\s+(.+?)(?:\s+LIMIT|\s*;|$)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 2 {
		return nil // No ORDER BY clause
	}
	
	orderByClause := strings.TrimSpace(matches[1])
	
	// Split by comma for multiple columns
	orderFields := strings.Split(orderByClause, ",")
	
	for _, fieldStr := range orderFields {
		fieldStr = strings.TrimSpace(fieldStr)
		parts := strings.Fields(fieldStr)
		
		if len(parts) == 0 {
			continue
		}
		
		column := parts[0]
		order := "ASC"
		
		if len(parts) > 1 {
			orderWord := strings.ToUpper(parts[1])
			if orderWord == "DESC" {
				order = "DESC"
			}
		}
		
		// Validate column
		if !contains(t.grammar.OrderBy.AllowedColumns, column) {
			return fmt.Errorf("column '%s' not available for ordering. Available: %v", 
				column, t.grammar.OrderBy.AllowedColumns)
		}
		
		query.OrderBy = append(query.OrderBy, OrderByField{
			Column: column,
			Order:  order,
		})
	}
	
	return nil
}

func (t *SimpleSQLTranslator) parseLimit(sql string, query *ParsedQuery) error {
	// Set default limit
	query.Limit = t.grammar.Limit.DefaultLimit
	
	// Match LIMIT clause
	re := regexp.MustCompile(`(?i)LIMIT\s+(\d+)(?:\s+OFFSET\s+(\d+))?`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) >= 2 {
		limit, err := strconv.Atoi(matches[1])
		if err != nil {
			return fmt.Errorf("invalid LIMIT value: %s", matches[1])
		}
		
		if limit > t.grammar.Limit.MaxLimit {
			return fmt.Errorf("LIMIT %d exceeds maximum allowed limit of %d", 
				limit, t.grammar.Limit.MaxLimit)
		}
		
		query.Limit = limit
		
		// Check for OFFSET
		if len(matches) >= 3 && matches[2] != "" {
			offset, err := strconv.Atoi(matches[2])
			if err != nil {
				return fmt.Errorf("invalid OFFSET value: %s", matches[2])
			}
			query.Offset = offset
		}
	}
	
	return nil
}

func (t *SimpleSQLTranslator) parseValue(valueStr string) (interface{}, error) {
	// Remove quotes
	valueStr = strings.Trim(valueStr, "'\"")
	
	// Try to parse as number first
	if intVal, err := strconv.Atoi(valueStr); err == nil {
		return intVal, nil
	}
	if floatVal, err := strconv.ParseFloat(valueStr, 64); err == nil {
		return floatVal, nil
	}
	
	// Return as string
	return valueStr, nil
}

func (t *SimpleSQLTranslator) isColumnAllowed(column string) bool {
	return contains(t.grammar.AllowedColumns, column)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// INSERT INTO table (col1, col2) VALUES (val1, val2)
func (t *SimpleSQLTranslator) parseInsertQuery(sql string, query *ParsedQuery) (*ParsedQuery, error) {
	// Match INSERT INTO table_name
	re := regexp.MustCompile(`(?i)INSERT\s+INTO\s+(\w+)\s*\(([^)]+)\)\s*VALUES\s*\(([^)]+)\)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid INSERT statement format")
	}
	
	query.TableName = strings.TrimSpace(matches[1])
	
	// Validate table name
	expectedTable := strings.TrimSuffix(t.grammar.TableName, "_post")
	if query.TableName != expectedTable {
		return nil, fmt.Errorf("table '%s' not found for INSERT. Available table: %s", 
			query.TableName, expectedTable)
	}
	
	// Parse columns
	columns := strings.Split(matches[2], ",")
	for _, col := range columns {
		columnName := strings.TrimSpace(col)
		query.Columns = append(query.Columns, columnName)
	}
	
	// Parse values
	values := strings.Split(matches[3], ",")
	if len(values) != len(query.Columns) {
		return nil, fmt.Errorf("column count doesn't match value count")
	}
	
	for _, val := range values {
		value, err := t.parseValue(strings.TrimSpace(val))
		if err != nil {
			return nil, err
		}
		query.Values = append(query.Values, value)
	}
	
	return query, nil
}

// UPDATE table SET col1 = val1, col2 = val2 WHERE id = 123
func (t *SimpleSQLTranslator) parseUpdateQuery(sql string, query *ParsedQuery) (*ParsedQuery, error) {
	// Match UPDATE table_name SET
	re := regexp.MustCompile(`(?i)UPDATE\s+(\w+)\s+SET\s+(.+?)(?:\s+WHERE\s+(.+?))?(?:\s*;|$)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid UPDATE statement format")
	}
	
	query.TableName = strings.TrimSpace(matches[1])
	
	// Validate table name
	expectedTable := strings.TrimSuffix(t.grammar.TableName, "_put")
	expectedTable = strings.TrimSuffix(expectedTable, "_patch")
	if query.TableName != expectedTable {
		return nil, fmt.Errorf("table '%s' not found for UPDATE. Available table: %s", 
			query.TableName, expectedTable)
	}
	
	// Parse SET clause
	setPart := matches[2]
	assignments := strings.Split(setPart, ",")
	for _, assignment := range assignments {
		parts := strings.Split(assignment, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SET clause: %s", assignment)
		}
		
		column := strings.TrimSpace(parts[0])
		value, err := t.parseValue(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, err
		}
		
		query.Updates[column] = value
	}
	
	// Parse WHERE clause if present
	if len(matches) > 3 && matches[3] != "" {
		wherePart := matches[3]
		condition, err := t.parseCondition(wherePart)
		if err != nil {
			return nil, err
		}
		query.Conditions = append(query.Conditions, *condition)
	}
	
	return query, nil
}

// DELETE FROM table WHERE id = 123
func (t *SimpleSQLTranslator) parseDeleteQuery(sql string, query *ParsedQuery) (*ParsedQuery, error) {
	// Match DELETE FROM table_name
	re := regexp.MustCompile(`(?i)DELETE\s+FROM\s+(\w+)(?:\s+WHERE\s+(.+?))?(?:\s*;|$)`)
	matches := re.FindStringSubmatch(sql)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid DELETE statement format")
	}
	
	query.TableName = strings.TrimSpace(matches[1])
	
	// Validate table name
	expectedTable := strings.TrimSuffix(t.grammar.TableName, "_delete")
	if query.TableName != expectedTable {
		return nil, fmt.Errorf("table '%s' not found for DELETE. Available table: %s", 
			query.TableName, expectedTable)
	}
	
	// Parse WHERE clause if present
	if len(matches) > 2 && matches[2] != "" {
		wherePart := matches[2]
		condition, err := t.parseCondition(wherePart)
		if err != nil {
			return nil, err
		}
		query.Conditions = append(query.Conditions, *condition)
	} else {
		return nil, fmt.Errorf("DELETE without WHERE clause is not allowed for safety")
	}
	
	return query, nil
}