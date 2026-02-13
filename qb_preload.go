package sqlr

import (
	"github.com/gosoline-project/sqlc"
)

// preloadEntry holds the configuration for a single PRELOAD directive, including
// the target relation name and any additional WHERE conditions to apply when
// loading the related entities.
type preloadEntry struct {
	relation string
	where    []*sqlc.SqlerWhere
}

// Preload adds an eager-loading directive for the named relation to the query.
// The relation must correspond to a relationship defined on the entity's schema
// (e.g. "Posts", "Profile", "Author", "Tags", or nested paths such as
// "Posts.Comments"). Unlike joins, preloads execute separate queries to load
// related entities and support HasOne, HasMany, BelongsTo, and ManyToMany.
//
// Optional conditions are applied as WHERE clauses when loading the related
// entities, allowing you to filter which related records are loaded. For nested
// preload paths, conditions are applied to the leaf relation only.
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	Preload("Posts")                                                        // HasMany
//	Preload("Profile")                                                      // HasOne
//	Preload("Author")                                                       // BelongsTo
//	Preload("Tags")                                                         // ManyToMany
//	Preload("Posts.Comments")                                               // nested preload
//	Preload("Posts.Comments", Condition("body = ?", "keep"))               // condition applies to Comments only
func (s *QueryBuilderSelect) Preload(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	s.preloads = append(s.preloads, preloadEntry{
		relation: relation,
		where:    conditions,
	})

	return s
}
