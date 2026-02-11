package sqlr

import (
	"github.com/gosoline-project/sqlc"
	"gorm.io/gorm/clause"
)

// Condition is a helper function that creates a *SqlerWhere condition for use with join methods.
// It accepts the same flexible input types as Where():
//   - A raw SQL string with placeholders and corresponding parameter values
//   - An *Expression object that encapsulates the condition and parameters
//   - An Eq map for creating equality conditions from column-value pairs
//
// This helper makes join syntax more readable and explicit.
//
// Example:
//
//	Condition("orders.user_id = users.id")
//	Condition("orders.status = ?", "active")
//	Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id")))
//	Condition(sqlc.Eq{"addresses.user_id": "users.id"})
func Condition(condition any, params ...any) *sqlc.SqlerWhere {
	return sqlc.NewSqlerWhere().Where(condition, params...)
}

// joinEntry stores metadata for a single join clause.
// It contains the join type, table name, and the conditions for the ON clause.
type joinEntry struct {
	joinType clause.JoinType
	table    string
	where    []*sqlc.SqlerWhere
}

// addJoin is a shared helper to add a join with conditions.
// It creates a joinEntry with the specified join type, table, and conditions,
// then appends it to the joins slice.
func (s *QueryBuilderSelect) addJoin(joinType clause.JoinType, table string, conditions []*sqlc.SqlerWhere) *QueryBuilderSelect {
	s.joins = append(s.joins, joinEntry{
		joinType: joinType,
		table:    table,
		where:    conditions,
	})

	return s
}

// LeftJoin adds a LEFT JOIN clause to the query.
// Multiple LeftJoin() calls are appended (not replaced).
//
// Conditions are optional. Accepts zero or more *SqlerWhere conditions that will be combined with AND in the ON clause.
// Providing no conditions creates a join without an ON clause (useful when the join condition is handled elsewhere).
// Use the Condition() helper to create SqlerWhere instances from various input types:
//   - Raw SQL strings with placeholders: Condition("orders.user_id = users.id")
//   - Expression objects: Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id")))
//   - Eq maps: Condition(sqlc.Eq{"addresses.user_id": "users.id"})
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	LeftJoin("orders", Condition("orders.user_id = users.id"))
//	LeftJoin("payments", Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id"))))
//	LeftJoin("addresses", Condition(sqlc.Eq{"addresses.user_id": "users.id"}))
//	LeftJoin("products", Condition("p.active = ?", true), Condition("p.deleted_at IS NULL"))
//	LeftJoin("categories") // No conditions
func (s *QueryBuilderSelect) LeftJoin(table string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(clause.LeftJoin, table, conditions)
}

// InnerJoin adds an INNER JOIN clause to the query.
// Multiple InnerJoin() calls are appended (not replaced).
//
// Conditions are optional. Accepts zero or more *SqlerWhere conditions that will be combined with AND in the ON clause.
// Providing no conditions creates a join without an ON clause (useful when the join condition is handled elsewhere).
// Use the Condition() helper to create SqlerWhere instances from various input types:
//   - Raw SQL strings with placeholders: Condition("orders.user_id = users.id")
//   - Expression objects: Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id")))
//   - Eq maps: Condition(sqlc.Eq{"addresses.user_id": "users.id"})
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	InnerJoin("orders", Condition("orders.user_id = users.id"))
//	InnerJoin("payments", Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id"))))
//	InnerJoin("addresses", Condition(sqlc.Eq{"addresses.user_id": "users.id"}))
//	InnerJoin("products", Condition("p.active = ?", true), Condition("p.deleted_at IS NULL"))
//	InnerJoin("categories") // No conditions
func (s *QueryBuilderSelect) InnerJoin(table string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(clause.InnerJoin, table, conditions)
}

// RightJoin adds a RIGHT JOIN clause to the query.
// Multiple RightJoin() calls are appended (not replaced).
//
// Conditions are optional. Accepts zero or more *SqlerWhere conditions that will be combined with AND in the ON clause.
// Providing no conditions creates a join without an ON clause (useful when the join condition is handled elsewhere).
// Use the Condition() helper to create SqlerWhere instances from various input types:
//   - Raw SQL strings with placeholders: Condition("orders.user_id = users.id")
//   - Expression objects: Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id")))
//   - Eq maps: Condition(sqlc.Eq{"addresses.user_id": "users.id"})
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	RightJoin("orders", Condition("orders.user_id = users.id"))
//	RightJoin("payments", Condition(sqlc.Col("payments.order_id").Eq(sqlc.Col("orders.id"))))
//	RightJoin("addresses", Condition(sqlc.Eq{"addresses.user_id": "users.id"}))
//	RightJoin("products", Condition("p.active = ?", true), Condition("p.deleted_at IS NULL"))
//	RightJoin("categories") // No conditions
func (s *QueryBuilderSelect) RightJoin(table string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(clause.RightJoin, table, conditions)
}

// CrossJoin adds a CROSS JOIN clause to the query.
// A CROSS JOIN produces a cartesian product of the two tables and has no ON clause.
// Multiple CrossJoin() calls are appended (not replaced).
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	CrossJoin("categories")
func (s *QueryBuilderSelect) CrossJoin(table string) *QueryBuilderSelect {
	s.joins = append(s.joins, joinEntry{
		joinType: clause.CrossJoin,
		table:    table,
		where:    nil,
	})

	return s
}
