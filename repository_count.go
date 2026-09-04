package sqlr

import (
	"context"
	"fmt"

	"github.com/gosoline-project/sqlc"
)

// CountingRepository extends Repository with an entity count operation.
type CountingRepository[K KeyTypes, E Entitier[K]] interface {
	Repository[K, E]
	Count(ctx context.Context, qb *QueryBuilderSelect) (int, error)
}

// CountingRepositoryTx extends RepositoryTx with a transaction-aware entity
// count operation.
type CountingRepositoryTx[K KeyTypes, E Entitier[K]] interface {
	RepositoryTx[K, E]
	Count(ttx TTx, qb *QueryBuilderSelect) (int, error)
}

type repositoryCountQuery struct {
	query  string
	params []any
}

func (q repositoryCountQuery) ToSql() (query string, params []any, err error) {
	return q.query, q.params, nil
}

func (r *repositoryCommon[K, E]) countEntities(q sqlc.Querier, ctx context.Context, qb *QueryBuilderSelect) (int, error) {
	if qb == nil {
		qb = NewQueryBuilderSelect()
	}

	countScope := *qb
	countScope.orderBy = sqlc.NewSqlerOrderBy()
	countScope.limit = nil
	countScope.offset = nil
	countScope.forUpdate = false

	column := "1"
	if countScope.groupBy.IsEmpty() && countScope.having.IsEmpty() {
		if r.schema.PrimaryKey == nil {
			return 0, fmt.Errorf("primary key not defined for %s", r.schema.TableName)
		}

		column = fmt.Sprintf("DISTINCT `%s`.`%s`", r.schema.TableName, r.schema.PrimaryKey.Name)
	}
	innerQuery := sqlc.From(r.schema.TableName).Columns(sqlc.Literal(column))

	for _, join := range sortJoinEntries(countScope.joins) {
		relation, ok := r.schema.Relationships[join.relation]
		if !ok {
			return 0, fmt.Errorf("join relation %q not found", join.relation)
		}

		relatedSchema, err := relation.ResolveRelatedSchema()
		if err != nil {
			return 0, fmt.Errorf("failed to resolve schema for relation %q: %w", join.relation, err)
		}
		if innerQuery, err = r.applyJoinClause(innerQuery, join, relation, relatedSchema); err != nil {
			return 0, err
		}
	}

	var err error
	if innerQuery, err = countScope.applyToSqlcBuilder(innerQuery); err != nil {
		return 0, fmt.Errorf("failed to apply count query clauses: %w", err)
	}

	innerSQL, params, err := innerQuery.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}
	query := repositoryCountQuery{
		query:  fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS `sqlr_count`", innerSQL),
		params: params,
	}

	var total int
	if err := r.statementCache.Get(ctx, query, q, &total); err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	return total, nil
}
