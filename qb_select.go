package sqlr

import (
	"fmt"
	"strings"

	"github.com/gosoline-project/sqlc"
)

// QueryBuilderSelect combines WHERE, GROUP BY, HAVING, ORDER BY, JOIN, and PRELOAD
// clauses into a reusable component for building SQL queries.
// It delegates to the individual Sqler components for each clause type.
type QueryBuilderSelect struct {
	joins    []joinEntry
	preloads []preloadEntry
	where    *sqlc.SqlerWhere
	groupBy  *sqlc.SqlerGroupBy
	having   *sqlc.SqlerHaving
	orderBy  *sqlc.SqlerOrderBy
	limit    *int
	offset   *int
}

// NewQueryBuilderSelect creates a new QueryBuilderSelect instance with all components initialized.
func NewQueryBuilderSelect() *QueryBuilderSelect {
	return &QueryBuilderSelect{
		where:   sqlc.NewSqlerWhere(),
		groupBy: sqlc.NewSqlerGroupBy(),
		having:  sqlc.NewSqlerHaving(),
		orderBy: sqlc.NewSqlerOrderBy(),
	}
}

// Where adds a WHERE condition to the query.
// Multiple Where() calls are combined with AND.
// Accepts either:
//   - A raw SQL string with placeholders and corresponding parameter values
//   - An *Expression object that encapsulates the condition and parameters
//   - An Eq map for creating equality conditions from column-value pairs
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	Where("status = ?", "active")                    // WHERE status = ?
//	Where(Col("age").Gt(18))                         // WHERE `age` > ?
//	Where(And(Col("a").Eq(1), Col("b").Eq(2)))       // WHERE (`a` = ? AND `b` = ?)
func (s *QueryBuilderSelect) Where(condition any, params ...any) *QueryBuilderSelect {
	s.where.Where(condition, params...)

	return s
}

// GroupBy sets the GROUP BY columns for the query.
// Accepts strings (column names) or *Expression objects.
// Replaces any previously set GROUP BY clause.
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	GroupBy("status")                           // GROUP BY `status`
//	GroupBy("country", "city")                  // GROUP BY `country`, `city`
func (s *QueryBuilderSelect) GroupBy(cols ...any) *QueryBuilderSelect {
	s.groupBy.GroupBy(cols...)

	return s
}

// Having adds a HAVING condition to the query (used with GROUP BY).
// Multiple Having() calls are combined with AND.
// Accepts either:
//   - A raw SQL string with placeholders and corresponding parameter values
//   - An *Expression object that encapsulates the condition and parameters
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	Having("COUNT(*) > ?", 10)                       // HAVING COUNT(*) > ?
//	Having(Col("*").Count().Gt(10))                  // HAVING COUNT(*) > ?
func (s *QueryBuilderSelect) Having(condition any, params ...any) *QueryBuilderSelect {
	s.having.Having(condition, params...)

	return s
}

// OrderBy sets the ORDER BY clause for the query.
// Accepts strings (column names with optional ASC/DESC) or *Expression objects.
// Replaces any previously set ORDER BY clause.
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	OrderBy("created_at DESC")                      // ORDER BY `created_at` DESC
//	OrderBy("name ASC", "created_at DESC")          // ORDER BY `name` ASC, `created_at` DESC
func (s *QueryBuilderSelect) OrderBy(cols ...any) *QueryBuilderSelect {
	s.orderBy.OrderBy(cols...)

	return s
}

// Limit sets the maximum number of rows to return.
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	Limit(10)                                        // LIMIT 10
func (s *QueryBuilderSelect) Limit(limit int) *QueryBuilderSelect {
	s.limit = &limit

	return s
}

// Offset sets the number of rows to skip before returning results.
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	Offset(20)                                       // OFFSET 20
func (s *QueryBuilderSelect) Offset(offset int) *QueryBuilderSelect {
	s.offset = &offset

	return s
}

// ToSql builds and returns the complete SQL query string and parameters.
// It combines all the clauses (WHERE, GROUP BY, HAVING, ORDER BY, LIMIT, OFFSET)
// in the correct order according to SQL syntax.
// Returns the SQL string, parameter values, and any error encountered during building.
//
// Example:
//
//	qb := NewQueryBuilderSelect()
//	qb.Where(Col("status").Eq("active")).
//	   GroupBy("country").
//	   Having(Col("*").Count().Gt(10)).
//	   OrderBy("country ASC").
//	   Limit(10).
//	   Offset(20)
//	sql, params, err := qb.ToSql()
//	// Returns: "WHERE `status` = ? GROUP BY `country` HAVING COUNT(*) > ? ORDER BY `country` ASC LIMIT 10 OFFSET 20"
func (s *QueryBuilderSelect) ToSql() (query string, params []any, err error) {
	var parts []string

	// WHERE clause
	whereSql, whereParams, err := s.where.ToSql()
	if err != nil {
		return "", nil, err
	}
	if whereSql != "" {
		parts = append(parts, "WHERE "+whereSql)
		params = append(params, whereParams...)
	}

	// GROUP BY clause
	groupBySql, err := s.groupBy.ToSql()
	if err != nil {
		return "", nil, err
	}
	if groupBySql != "" {
		parts = append(parts, "GROUP BY "+groupBySql)
	}

	// HAVING clause
	havingSql, havingParams, err := s.having.ToSql()
	if err != nil {
		return "", nil, err
	}
	if havingSql != "" {
		parts = append(parts, "HAVING "+havingSql)
		params = append(params, havingParams...)
	}

	// ORDER BY clause
	orderBySql, err := s.orderBy.ToSql()
	if err != nil {
		return "", nil, err
	}
	if orderBySql != "" {
		parts = append(parts, "ORDER BY "+orderBySql)
	}

	// LIMIT clause
	if s.limit != nil {
		parts = append(parts, fmt.Sprintf("LIMIT %d", *s.limit))
	}

	// OFFSET clause
	if s.offset != nil {
		parts = append(parts, fmt.Sprintf("OFFSET %d", *s.offset))
	}

	return strings.Join(parts, " "), params, nil
}
