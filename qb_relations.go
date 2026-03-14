package sqlr

import "github.com/gosoline-project/sqlc"

func appendPreload(preloads *[]preloadEntry, relation string, conditions []*sqlc.SqlerWhere) {
	*preloads = append(*preloads, preloadEntry{
		relation: relation,
		where:    filterNilWhereConditions(conditions),
	})
}

func appendJoin(joins *[]joinEntry, joinType sqlc.JoinType, relation string, conditions []*sqlc.SqlerWhere) {
	*joins = append(*joins, joinEntry{
		joinType: joinType,
		relation: relation,
		where:    filterNilWhereConditions(conditions),
	})
}

func filterNilWhereConditions(conditions []*sqlc.SqlerWhere) []*sqlc.SqlerWhere {
	filtered := make([]*sqlc.SqlerWhere, 0, len(conditions))
	for _, condition := range conditions {
		if condition == nil {
			continue
		}

		filtered = append(filtered, condition)
	}

	return filtered
}
