package sqlr

import (
	"github.com/gosoline-project/sqlc"
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
	joinType sqlc.JoinType
	relation string
	where    []*sqlc.SqlerWhere
}

// addJoin appends a join entry to the query builder and returns it for chaining.
func (s *QueryBuilderSelect) addJoin(joinType sqlc.JoinType, relation string, conditions []*sqlc.SqlerWhere) *QueryBuilderSelect {
	appendJoin(&s.joins, joinType, relation, conditions)

	return s
}

// LeftJoin adds a LEFT JOIN on a direct relation defined on the entity's schema
// (for example "Profile", "Posts", or "Author"). Joins support HasOne, HasMany,
// and BelongsTo relations. For BelongsTo, the ON clause is generated as
// current.<foreign_key> = related.<primary_key>; for HasOne/HasMany it is
// current.<primary_key> = related.<foreign_key>.
//
// Dotted/nested join paths (for example "Posts.Comments") are not supported.
// Use [QueryBuilderSelect.Preload] for nested relation loading. ManyToMany joins are also not
// supported and should be loaded via Preload.
//
// Optional conditions are appended to the JOIN ON clause.
func (s *QueryBuilderSelect) LeftJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(sqlc.JoinLeft, relation, conditions)
}

// InnerJoin adds an INNER JOIN on the named relation to the query. Only rows with
// matching entries in the joined relation are included in the result set. Optional
// conditions further restrict the joined rows. See LeftJoin for supported relation
// types and limitations.
func (s *QueryBuilderSelect) InnerJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(sqlc.JoinInner, relation, conditions)
}

// RightJoin adds a RIGHT JOIN on the named relation to the query. All rows from
// the joined relation are included, with NULL values for the primary entity's
// columns when no match exists. Optional conditions further restrict the join.
// See LeftJoin for supported relation types and limitations.
func (s *QueryBuilderSelect) RightJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	return s.addJoin(sqlc.JoinRight, relation, conditions)
}

// CrossJoin adds a conditionless relation join on the named relation.
//
// Deprecated: CrossJoin does not produce a SQL Cartesian product. Repository
// joins are relation-aware, so this method behaves like InnerJoin(relation)
// using the relation key predicate. It is retained for compatibility; prefer
// InnerJoin for new code. See LeftJoin for supported relation types and
// limitations.
func (s *QueryBuilderSelect) CrossJoin(relation string) *QueryBuilderSelect {
	return s.addJoin(sqlc.JoinInner, relation, nil)
}
