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
	journal         *mutationJournal
	mutationOptions mutationOptions
}

func newAssociationMutationContext(cache *statementCache, q sqlc.Querier, journal *mutationJournal, options mutationOptions) associationMutationContext {
	return associationMutationContext{
		associationCallContext: newAssociationCallContext(cache, q),
		journal:                journal,
		mutationOptions:        options,
	}
}

type associationCreateContext struct {
	associationMutationContext
	policy *associationSyncPolicy
}

func newAssociationCreateContext(cache *statementCache, q sqlc.Querier, policy *associationSyncPolicy, journal *mutationJournal, options mutationOptions) *associationCreateContext {
	return &associationCreateContext{
		associationMutationContext: newAssociationMutationContext(cache, q, journal, options),
		policy:                     policy,
	}
}

type associationSyncContext struct {
	associationMutationContext
	state  *associationSyncState
	policy *associationSyncPolicy
}

func newAssociationSyncContext(cache *statementCache, q sqlc.Querier, policy *associationSyncPolicy, journal *mutationJournal, options mutationOptions) *associationSyncContext {
	return &associationSyncContext{
		associationMutationContext: newAssociationMutationContext(cache, q, journal, options),
		state:                      newAssociationSyncState(),
		policy:                     policy,
	}
}
