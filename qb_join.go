package sqlr

import (
	"github.com/gosoline-project/sqlc"
	"gorm.io/gorm/clause"
)

// Condition creates a SqlerWhere clause that can be passed as a join condition.
// The condition argument can be a raw SQL string, an Expression, or an Eq map.
// Optional params are bound to placeholders in the condition string.
func Condition(condition any, params ...any) *sqlc.SqlerWhere {
	return sqlc.NewSqlerWhere().Where(condition, params...)
}

// joinEntry holds the configuration for a single JOIN clause, including the
// join type (LEFT, INNER, RIGHT, CROSS), the target relation name, and any
// additional WHERE conditions to apply within the join.
type joinEntry struct {
	joinType clause.JoinType
	relation string
	where    []*sqlc.SqlerWhere
}

// addJoin appends a join entry to the query builder and returns it for chaining.
func (s *QueryBuilderSelect) addJoin(joinType clause.JoinType, relation string, conditions []*sqlc.SqlerWhere) *QueryBuilderSelect {
	s.joins = append(s.joins, joinEntry{
		joinType: joinType,
		relation: relation,
		where:    conditions,
	})

	return s
}

// LeftJoin adds a LEFT JOIN on the named relation to the query. The relation must
// correspond to a relationship defined on the entity's GORM schema (e.g. "Profile"
// or "Company.Address" for nested relations). Optional conditions are applied as
// additional WHERE clauses within the join.
func (s *QueryBuilderSelect) LeftJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(clause.LeftJoin, relation, conditions)
}

// InnerJoin adds an INNER JOIN on the named relation to the query. Only rows with
// matching entries in the joined relation are included in the result set. Optional
// conditions further restrict the joined rows.
func (s *QueryBuilderSelect) InnerJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(clause.InnerJoin, relation, conditions)
}

// RightJoin adds a RIGHT JOIN on the named relation to the query. All rows from
// the joined relation are included, with NULL values for the primary entity's
// columns when no match exists. Optional conditions further restrict the join.
func (s *QueryBuilderSelect) RightJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(clause.RightJoin, relation, conditions)
}

// CrossJoin adds a CROSS JOIN on the named relation to the query, producing a
// Cartesian product of the primary entity and the joined relation. Cross joins
// do not accept conditions.
func (s *QueryBuilderSelect) CrossJoin(relation string) *QueryBuilderSelect {
	return s.addJoin(clause.CrossJoin, relation, nil)
}
