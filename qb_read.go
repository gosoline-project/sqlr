package sqlr

import (
	"github.com/gosoline-project/sqlc"
)

// QueryBuilderRead provides relation-loading options for single-entity reads.
// It supports Preload and Join methods but intentionally omits Where, OrderBy,
// Limit, Offset, GroupBy, and Having since Read always queries by primary key.
type QueryBuilderRead struct {
	joins    []joinEntry
	preloads []preloadEntry
}

// NewQueryBuilderRead creates a new QueryBuilderRead instance.
func NewQueryBuilderRead() *QueryBuilderRead {
	return &QueryBuilderRead{}
}

// Preload adds an eager-loading directive for the named relation. The relation
// must correspond to a relationship defined on the entity's schema (e.g.
// "Posts", "Profile", "Author", "Tags", or nested paths such as
// "Posts.Comments"). Unlike joins, preloads execute separate queries to load
// related entities and support HasOne, HasMany, BelongsTo, and ManyToMany.
//
// Optional conditions are applied as WHERE clauses when loading the related
// entities. For nested preload paths, conditions are applied to the leaf
// relation only.
//
// Returns the same QueryBuilderRead instance for method chaining.
func (r *QueryBuilderRead) Preload(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderRead {
	r.preloads = append(r.preloads, preloadEntry{
		relation: relation,
		where:    conditions,
	})

	return r
}

// LeftJoin adds a LEFT JOIN on a direct relation defined on the entity's schema.
// Joins support HasOne, HasMany, and BelongsTo relations. Dotted/nested join
// paths and ManyToMany joins are not supported; use Preload for those.
//
// Optional conditions are appended to the JOIN ON clause.
//
// Returns the same QueryBuilderRead instance for method chaining.
func (r *QueryBuilderRead) LeftJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderRead {
	return r.addJoin(sqlc.JoinLeft, relation, conditions)
}

// InnerJoin adds an INNER JOIN on the named relation. Only rows with matching
// entries in the joined relation are included. Optional conditions further
// restrict the joined rows. See LeftJoin for supported relation types.
//
// Returns the same QueryBuilderRead instance for method chaining.
func (r *QueryBuilderRead) InnerJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderRead {
	return r.addJoin(sqlc.JoinInner, relation, conditions)
}

// RightJoin adds a RIGHT JOIN on the named relation. All rows from the joined
// relation are included, with NULL values for the primary entity's columns when
// no match exists. Optional conditions further restrict the join. See LeftJoin
// for supported relation types.
//
// Returns the same QueryBuilderRead instance for method chaining.
func (r *QueryBuilderRead) RightJoin(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderRead {
	return r.addJoin(sqlc.JoinRight, relation, conditions)
}

// CrossJoin adds a CROSS JOIN on the named relation, producing a Cartesian
// product of the primary entity and the joined relation. Cross joins do not
// accept conditions. See LeftJoin for supported relation types.
//
// Returns the same QueryBuilderRead instance for method chaining.
func (r *QueryBuilderRead) CrossJoin(relation string) *QueryBuilderRead {
	return r.addJoin(sqlc.JoinCross, relation, nil)
}

// addJoin appends a join entry and returns the builder for chaining.
func (r *QueryBuilderRead) addJoin(joinType sqlc.JoinType, relation string, conditions []*sqlc.SqlerWhere) *QueryBuilderRead {
	r.joins = append(r.joins, joinEntry{
		joinType: joinType,
		relation: relation,
		where:    conditions,
	})

	return r
}

// hasRelations reports whether any explicit joins or preloads have been added.
func (r *QueryBuilderRead) hasRelations() bool {
	return len(r.joins) > 0 || len(r.preloads) > 0
}

// toQueryBuilderSelect converts the read builder into a full QueryBuilderSelect
// by copying joins and preloads and adding a WHERE pk = ? LIMIT 1 constraint.
// This allows reuse of the existing queryEntities infrastructure.
func (r *QueryBuilderRead) toQueryBuilderSelect(pkColumn string, pkValue any) *QueryBuilderSelect {
	qb := NewQueryBuilderSelect()
	qb.joins = r.joins
	qb.preloads = r.preloads
	qb.Where(sqlc.Col(pkColumn).Eq(pkValue))
	qb.Limit(1)

	return qb
}
