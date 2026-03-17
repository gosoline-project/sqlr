package sqlr

import "github.com/gosoline-project/sqlc"

type associationCallContext struct {
	cache *statementCache
	q     sqlc.Querier
}

func newAssociationCallContext(cache *statementCache, q sqlc.Querier) associationCallContext {
	return associationCallContext{
		cache: cache,
		q:     q,
	}
}

type associationMutationContext struct {
	associationCallContext
	journal *mutationJournal
}

func newAssociationMutationContext(cache *statementCache, q sqlc.Querier, journal *mutationJournal) associationMutationContext {
	return associationMutationContext{
		associationCallContext: newAssociationCallContext(cache, q),
		journal:                journal,
	}
}

type associationCreateContext struct {
	associationMutationContext
	policy *associationSyncPolicy
}

func newAssociationCreateContext(cache *statementCache, q sqlc.Querier, policy *associationSyncPolicy, journal *mutationJournal) *associationCreateContext {
	return &associationCreateContext{
		associationMutationContext: newAssociationMutationContext(cache, q, journal),
		policy:                     policy,
	}
}

type associationSyncContext struct {
	associationMutationContext
	state  *associationSyncState
	policy *associationSyncPolicy
}

func newAssociationSyncContext(cache *statementCache, q sqlc.Querier, policy *associationSyncPolicy, journal *mutationJournal) *associationSyncContext {
	return &associationSyncContext{
		associationMutationContext: newAssociationMutationContext(cache, q, journal),
		state:                      newAssociationSyncState(),
		policy:                     policy,
	}
}
