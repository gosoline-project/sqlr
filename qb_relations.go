package sqlr

import "github.com/gosoline-project/sqlc"

func appendPreload(preloads *[]preloadEntry, relation string, conditions []*sqlc.SqlerWhere) {
	*preloads = append(*preloads, preloadEntry{
		relation: relation,
		where:    conditions,
	})
}

func appendJoin(joins *[]joinEntry, joinType sqlc.JoinType, relation string, conditions []*sqlc.SqlerWhere) {
	*joins = append(*joins, joinEntry{
		joinType: joinType,
		relation: relation,
		where:    conditions,
	})
}
