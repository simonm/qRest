# qRest - SQL to API Gateway

qRest bridges SQL and REST APIs through intelligent discovery. A proof-of-concept application that enables SQL queries against REST APIs using OpenAPI/Swagger specifications.

This allows us to bring decades of database querying patterns towards modern REST API architectures.

"Connecting the distant past with the not quite so distant past. Bridging the 1970s to the mid-2000s". Haha!

## tl;dr

- Run `qRest init` to generate a default toml config at `$XDG_CONFIG_HOME/qRest/config.toml`, if $XDG_CONFIG_HOME is not set, it defaults to: `~/.config/qRest/config.toml`
- Add whichever API you want to the config. There's a default there as an example.
- Add any auth if required to the config.
- Run it with a query. For example:

```
./qRest query  --api petstore  "SELECT * FROM findByStatus where status = 'available'"
```

- It grabs, parses the OpenAPI / Swagger docs for that API
- It then allows you to query the remote API using SQL via command line or using a local SQL flavoured HTTP API acting as a bridge.
- BOOM!

## Features

- **Auto-discovery**: Parses OpenAPI specs to understand API capabilities
- **SQL Grammar Generation**: Creates allowed SQL operations based on API parameters
- **Smart Validation**: Validates SQL queries against API constraints
- **Enhancement Suggestions**: Recommends API improvements for better SQL support
- **TOML Configuration**: Human-readable config files with multi-API support
- **Multiple Interfaces**: Both HTTP server and CLI tool
- **Authentication Support**: Bearer tokens, API keys, and basic auth

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   SQL Query     │───▶│     qRest        │───▶│   REST API      │
│ SELECT * FROM   │    │    Gateway       │    │   /users?       │
│ users WHERE     │    │                  │    │   age_gt=25     │
│ age > 25        │    │                  │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Quick Start

### 1. Initialize Configuration

```bash
# Build the CLI
go build -o qRest cmd/cli/main.go

# Generate sample configuration
./qRest init

# Edit qRest.toml to add your API credentials
```

### 2. Using TOML Configuration (Recommended)

```bash
# View available APIs and tables
./qRest capabilities --api petstore

# Execute SQL query
./qRest query --api petstore "SELECT * FROM findByStatus WHERE status = 'available' LIMIT 5"

# View SQL grammar for a table
./qRest grammar --api petstore --table findByStatus
```

### 3. HTTP Server with Configuration

```bash
# Start server with config file
go run cmd/server/main.go --config qRest.toml

# Query via HTTP
curl -X POST http://localhost:8080/query \
  -H "Content-Type" \
  -d '{"sql": "SELECT * FROM findByStatus WHERE status = \"available\" LIMIT 10"}'
```

### 4. CLI with Direct Parameters (No Config File)

```bash
# Build CLI
go build -o qRest cmd/cli/main.go

# Execute query
./qRest query \
  --spec "https://petstore.swagger.io/v2/swagger.json" \
  --base-url "https://petstore.swagger.io/v2" \
  --auth-type apikey \
  --auth-token "special-key" \
  "SELECT * FROM findByStatus WHERE status = 'available' LIMIT 5"

# View available grammar
./qRest grammar --spec <url> --base-url <url>

# View API capabilities
./qRest capabilities --spec <url> --base-url <url>
```

## Project Structure

```
├── cmd/
│   ├── server/          # HTTP server
│   └── cli/             # CLI tool
├── internal/
│   ├── parser/          # OpenAPI specification parsing
│   ├── grammar/         # SQL grammar generation
│   ├── translator/      # SQL parsing and validation
│   └── executor/        # REST API execution
├── go.mod
└── README.md
```

## Supported SQL Operations

### SELECT Queries

```sql
-- Basic SELECT
SELECT column1, column2 FROM table_name
SELECT * FROM users

-- WHERE clauses
SELECT * FROM users WHERE age = 25
SELECT * FROM users WHERE age > 21
SELECT * FROM users WHERE created_at >= '2024-01-01'
SELECT * FROM users WHERE name LIKE 'John%'
SELECT * FROM users WHERE age BETWEEN 21 AND 65

-- ORDER BY
SELECT * FROM users ORDER BY name ASC
SELECT * FROM users ORDER BY age DESC, name ASC

-- LIMIT and OFFSET
SELECT * FROM users LIMIT 10
SELECT * FROM users LIMIT 10 OFFSET 20
```

### INSERT Statements

```sql
-- Insert new record
INSERT INTO users (name, email, age) VALUES ('John Doe', 'john@example.com', 30)
INSERT INTO products (title, price) VALUES ('New Product', 29.99)
```

### UPDATE Statements

```sql
-- Update existing record (requires WHERE id clause)
UPDATE users SET email = 'newemail@example.com' WHERE id = 123
UPDATE users SET name = 'Jane Doe', age = 31 WHERE id = 456
UPDATE products SET price = 19.99 WHERE id = 789
```

### DELETE Statements

```sql
-- Delete record (requires WHERE clause for safety)
DELETE FROM users WHERE id = 123
DELETE FROM products WHERE id = 456
```

## SQL to REST API Mapping

### Query Operations

| SQL Statement                      | HTTP Method | REST Endpoint | Example                         |
| ---------------------------------- | ----------- | ------------- | ------------------------------- |
| `SELECT * FROM users`              | GET         | `/users`      | GET `/users`                    |
| `INSERT INTO users`                | POST        | `/users`      | POST `/users` with JSON body    |
| `UPDATE users WHERE id = 123`      | PUT/PATCH   | `/users/123`  | PUT `/users/123` with JSON body |
| `DELETE FROM users WHERE id = 123` | DELETE      | `/users/123`  | DELETE `/users/123`             |

### Query Parameter Mapping

| SQL Condition       | API Parameter                | Example                         |
| ------------------- | ---------------------------- | ------------------------------- |
| `age = 25`          | `age=25`                     | `/users?age=25`                 |
| `age > 25`          | `age_gt=25`                  | `/users?age_gt=25`              |
| `age >= 25`         | `age_gte=25` or `age_min=25` | `/users?age_gte=25`             |
| `name LIKE 'John%'` | `name_like=John`             | `/users?name_like=John`         |
| `ORDER BY name ASC` | `sort_by=name&order=asc`     | `/users?sort_by=name&order=asc` |
| `LIMIT 10`          | `limit=10`                   | `/users?limit=10`               |

## Configuration

### TOML Configuration File (Recommended)

qRest uses TOML configuration files for managing multiple APIs and settings. Generate a sample config:

```bash
./qRest init
```

Example `qRest.toml`:

```toml
[server]
host = "localhost"
port = 8080

[[apis]]
name = "petstore"
description = "Swagger Petstore API"
spec_url = "https://petstore.swagger.io/v2/swagger.json"
base_url = "https://petstore.swagger.io/v2"

[apis.auth]
type = "apikey"
token = "special-key"

[[apis]]
name = "github"
spec_url = "https://api.github.com/openapi.json"
base_url = "https://api.github.com"

[apis.auth]
type = "bearer"
token = "${GITHUB_TOKEN}"  # Environment variable expansion

[defaults]
max_limit = 1000
default_limit = 100
```

### Configuration Priority

1. **Command line flags** (highest priority)
2. **Configuration file** (`qRest.toml`)
3. **Environment variables**
4. **Default values** (lowest priority)

### Configuration Locations

qRest searches for configuration in:

- `./qRest.toml` (current directory)
- `~/.qRest/qRest.toml` (user home)
- `/etc/qRest/qRest.toml` (system-wide)

## HTTP Endpoints

- `POST /query` - Execute SQL queries
- `GET /grammar` - View allowed SQL grammar
- `GET /capabilities` - View API capabilities
- `GET /config` - View current configuration
- `GET /health` - Health check

## Example Usage

### Using Configuration File

```bash
# Create configuration
./qRest init

# Query via CLI
./qRest query --api petstore "SELECT * FROM findByStatus WHERE status = 'available' LIMIT 5"

# Start server with config
go run cmd/server/main.go --config qRest.toml

# Query via HTTP
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT * FROM findByStatus WHERE status = \"available\" LIMIT 5"}'
```

### Direct API Configuration

```bash
# Query with direct parameters
./qRest query \
  --spec "https://petstore.swagger.io/v2/swagger.json" \
  --base-url "https://petstore.swagger.io/v2" \
  --auth-type apikey \
  --auth-token "special-key" \
  "SELECT * FROM findByStatus WHERE status = 'available' LIMIT 5"
```

### Data Mutation Examples

```bash
# Insert a new pet (using pet_post table)
./qRest query --api petstore \
  "INSERT INTO pet (name, status) VALUES ('Fluffy', 'available')"

# Update a pet (using pet_put table)
./qRest query --api petstore \
  "UPDATE pet SET name = 'Fluffy Jr', status = 'sold' WHERE id = 123"

# Delete a pet (using pet_delete table)
./qRest query --api petstore \
  "DELETE FROM pet WHERE id = 123"
```

## Enhancement Suggestions

The gateway analyzes API capabilities and suggests improvements:

```json
{
  "suggestions": [
    "Add range filtering for 'age' (e.g., age_gt, age_lt parameters)",
    "Add partial text search for 'name' (e.g., name_like parameter)",
    "Add pagination support (e.g., 'limit' and 'offset' parameters)"
  ]
}
```

## Limitations

- No JOINs across different endpoints (yet)
- OR conditions not supported
- Subqueries not supported
- Limited to REST APIs with OpenAPI specs
- UPDATE/DELETE require WHERE id clause for safety
- Mutations depend on API endpoint structure

## Possible Future Enhancements

- JOIN support with caching
- GraphQL API support
- Advanced SQL features (subqueries, aggregations)
- Query optimization and caching
- WebSocket support for real-time data

## Development

```bash
# Install dependencies
go mod tidy

# Run tests
go test ./...

# Build server
go build -o qRest-server cmd/server/main.go

# Build CLI
go build -o qRest cmd/cli/main.go

# Generate configuration
./qRest init

# Run with configuration
./qRest-server --config qRest.toml
```

## Contributing

This is a proof-of-concept of a crazy idea. Contributions welcome for:

- Additional SQL features
- Better API parameter detection
- Performance optimizations
- More authentication methods
- GraphQL support

## License

MIT is perfect for this. See LICENCE.
