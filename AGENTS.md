# AGENTS.md

## Project Overview

This is `sqlr`, a Go library providing a generic, type-safe SQL repository layer built on top of `github.com/gosoline-project/sqlc`. It provides CRUD operations, query building with joins/preloads, and transaction support using Go generics for type safety.

- **Module**: `github.com/gosoline-project/sqlr`
- **Go Version**: 1.25.0+
- **Structure**: Single-package library (no subdirectories, all files in root)
- **Key Dependencies**: `gosoline-project/sqlc`, `justtrackio/gosoline`, `jmoiron/sqlx`, `stretchr/testify`

## Build, Test & Lint Commands

### Building
```bash
go build ./...              # Build all code
```

### Testing
```bash
go test ./...                                    # Run all tests
go test -v ./...                                 # Run all tests with verbose output
go test -run TestName                            # Run a specific test function
go test -run TestRepositoryCreateTestSuite       # Run all tests in a testify suite
go test -run TestRepositoryCreateTestSuite/TestCreate_Success  # Run specific test method in suite
go test -short ./...                             # Run short tests only
go test -race ./...                              # Run tests with race detector
```

### Linting
```bash
golangci-lint run                                # Run all linters (see .golangci.yml)
golangci-lint run --fix                          # Run linters and auto-fix issues
```

### Formatting
```bash
gofumpt -w .                                     # Format all files (stricter than gofmt)
gofumpt -l .                                     # List files that need formatting
```

### Mocking
```bash
mockery                                          # Generate mocks (config in .mockery.yml)
```

### Tool Versions
Tool versions are managed via `.tool-versions` (asdf):
- `gofumpt 0.9.2`
- `golang 1.25.4`
- `golangci-lint 2.6.2`
- `mockery 3.6.1`

## Code Style Guidelines

### Formatting
- Use **gofumpt** (stricter than gofmt) for all formatting
- Maximum line length: **240 characters** (configured in `.golangci.yml`)
- Use tabs for indentation
- Always run `gofumpt -w .` before committing

### Imports
Organize imports in three groups, separated by blank lines:
1. Standard library
2. Third-party packages
3. Project packages (sqlc, gosoline)

Example:
```go
import (
	"context"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)
```

Test imports follow the same pattern but include testing frameworks:
```go
import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)
```

### Naming Conventions

#### Types
- **Exported types**: PascalCase (e.g., `Repository`, `QueryBuilderSelect`, `Entity`)
- **Unexported types**: camelCase (e.g., `repository`, `repositoryCommon`, `joinEntry`)
- **Generic types**: Use descriptive single-letter type params with constraints:
  - `K` for key types
  - `E` for entity types
  - Example: `Repository[K KeyTypes, E Entitier[K]]`

#### Test Types
- **Test suites**: Suffix with `TestSuite` (e.g., `RepositoryCreateTestSuite`)
- **Test entities**: Prefix with `test` (e.g., `testUser`, `testPost`, `testAuthor`)
- **Test variables**: Use descriptive snake_case for SQL strings (e.g., `authorPostsSelectSQL`)
- **Entry point**: Test suite runner named `TestXxxTestSuite` (e.g., `TestRepositoryCreateTestSuite`)

#### Functions and Methods
- **Exported functions**: PascalCase (e.g., `NewRepository`, `NewQueryBuilderSelect`)
- **Unexported functions**: camelCase (e.g., `newRepositoryCommon`, `parseSchema`)
- **Test functions**: Start with `Test` (e.g., `TestCreate_Success`, `TestQuery_WithLimitAndOffset`)
- **Test helper functions**: camelCase (e.g., `newTestClient`)

#### Variables
- Use full words, avoid abbreviations except for common cases:
  - `ctx` for context
  - `err` for errors
  - `qb` for query builder
  - `rv` for reflect.Value
  - `pk` for primary key
  - `tx` for transactions
- Receiver names: single letter or short abbreviation of type (e.g., `r` for repository, `s` for suite, `e` for entity)

### Type Definitions and Interface Compliance

Always verify interface compliance at package level with compile-time checks:
```go
var _ Repository[int64, Entitier[int64]] = (*repository[int64, Entitier[int64]])(nil)
var _ RepositoryTx[int64, Entitier[int64]] = (*repositoryTx[int64, Entitier[int64]])(nil)
```

### Error Handling

- **Always** check errors (enforced by `errcheck` linter)
- **Always** wrap errors with context using `fmt.Errorf("descriptive context: %w", err)`
- Error messages should be lowercase and not end with punctuation
- Error context should explain what operation failed, not just restate the error

Example:
```go
if err := q.Select(ctx, &results, fullSQL, params...); err != nil {
    return nil, fmt.Errorf("failed to execute query: %w", err)
}
```

### Comments and Documentation

- **All exported** types, functions, methods, and constants **must** have doc comments
- Doc comments start with the name of the item being documented
- Use complete sentences with proper punctuation
- Explain **why** and **what**, not just restate the code
- For complex functions, include usage examples in comments

Example:
```go
// NewRepository creates a new Repository instance backed by the sqlc client
// configured from the provided config and logger. The name parameter identifies
// the database connection configuration to use. Returns an error if the sqlc
// client cannot be initialized.
func NewRepository[K KeyTypes, E Entitier[K]](ctx context.Context, config cfg.Config, logger log.Logger, name string) (Repository[K, E], error) {
```

### Struct Tags

Column names are expressed via the `db` struct tag, while sqlr metadata is expressed via the `sqlr` struct tag:
- **DB column**: `db:"column_name"`
- **Ignored field**: `db:"-"`
- **Primary key**: `db:"column_name" sqlr:"primaryKey"`
- **Auto-increment inference**: integer primary key field types (`int*`/`uint*`, including pointers) are treated as auto-increment at runtime; non-integer primary keys are inserted as provided
- **Auto timestamps**: `db:"column_name" sqlr:"autoCreateTime"` or `db:"column_name" sqlr:"autoUpdateTime"`
- **Relationships**:
  - `sqlr:"foreignKey:column_name"` for HasOne/HasMany (non-slice field => HasOne, slice field => HasMany). The foreign key column lives on the related table.
  - `sqlr:"belongsTo:column_name"` for BelongsTo. The foreign key column lives on the current entity table.
  - `sqlr:"many2many:join_table_name"` for ManyToMany
  - Add `preload` to any relationship tag option list for auto-preloading (for example: `sqlr:"foreignKey:author_id,preload"`).
- **Loading strategies**:
  - `Preload("Posts")` loads a single relation
  - `Preload("Posts.Comments")` loads nested relations; optional conditions apply to the leaf relation (`Comments` in this example)
  - Join methods (`LeftJoin`, `InnerJoin`, `RightJoin`, `CrossJoin`) support direct HasOne/HasMany/BelongsTo relations
  - ManyToMany is supported by preload (including auto-preload), not by join methods
  - Auto-preload (`preload` tag) applies to HasOne, HasMany, BelongsTo, and ManyToMany, including nested paths discovered from tagged relations

Example:
```go
type Entity[K KeyTypes] struct {
    Id        K         `db:"id" sqlr:"primaryKey"`
    CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
    UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
}

type Author struct {
    Entity[int64]
    Name  string    `db:"name"`
    Posts []Post    `sqlr:"foreignKey:author_id"`
    Profile Profile `sqlr:"foreignKey:author_id"`
}

type Post struct {
    Entity[int64]
    AuthorID int64  `db:"author_id"`
    Author   Author `sqlr:"belongsTo:author_id"`
}

type Article struct {
    Entity[int64]
    Tags  []Tag     `sqlr:"many2many:article_tags"`
}
```

### Testing Patterns

#### Test Organization
- Use **testify/suite** for integration-style repository tests (CRUD operations)
- Use standalone test functions for unit tests (query builder)
- Group related tests in suites
- Name test methods with underscore separators: `TestOperation_Condition`

#### Test Suite Structure
```go
type RepositoryCreateTestSuite struct {
    suite.Suite
    client sqlc.Client
    mock   sqlmock.Sqlmock
    repo   sqlr.Repository[int64, testUser]
}

func TestRepositoryCreateTestSuite(t *testing.T) {
    suite.Run(t, new(RepositoryCreateTestSuite))
}

func (s *RepositoryCreateTestSuite) SetupTest() {
    // Initialize resources for each test
}

func (s *RepositoryCreateTestSuite) TearDownTest() {
    // Verify mock expectations were met
    s.Require().NoError(s.mock.ExpectationsWereMet())
}
```

#### Assertions
- Use `s.Require()` for critical assertions that should fail immediately
- Use `s.Assert()` or `s.Equal()` for non-critical assertions
- Always verify mock expectations in `TearDownTest()`

#### sqlmock Usage
- Use `regexp.QuoteMeta()` for exact SQL matching
- Create custom matchers for dynamic values (e.g., `isTimestamp{}` for time.Time)
- Verify both SQL and arguments with `WithArgs()`

### Variable Declarations

Prefer explicit variable declaration followed by assignment for clarity:
```go
var err error
var client sqlc.Client

if client, err = sqlc.ProvideClient(ctx, config, logger, name); err != nil {
    return nil, fmt.Errorf("failed to initialize sqlc client: %w", err)
}
```

Use `make()` with capacity hints when size is known:
```go
vals := make([]any, 0, len(insertCols))
scanDests := make([]any, len(columns))
```

### Linter Configuration

Key linters enabled in `.golangci.yml`:
- `errcheck` — catch unchecked errors
- `govet` — suspicious constructs
- `staticcheck` — comprehensive static analysis
- `unused` — unused code detection
- `revive` — style and best practices
- `gocritic` — performance and style
- `nlreturn` — blank line before returns
- `whitespace` — unnecessary blank lines
- `godox` — TODO/FIXME/BUG detection
- `lll` — line length (240 chars max)

## Commit Message Style

Based on repository history, commit messages follow a simple, lowercase style:
- Use lowercase
- Be descriptive but concise
- End with semicolon
- Examples: `added preloading;`, `join fixes, code improvements, linting;`

## Additional Notes

- This is a library package; there is no `main` package or executable
- All files are in the root directory; no subdirectories
- Heavy use of Go generics for type safety
- Reflection is used internally for schema parsing and struct hydration
- Table names are derived from type names: PascalCase → snake_case + pluralization
