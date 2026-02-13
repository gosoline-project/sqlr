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
// The relation must correspond to a relationship defined on the entity's GORM
// schema (e.g. "Posts" or "Posts.Comments" for nested relations). Unlike joins,
// preloads execute separate queries to load related entities and support
// many-to-many associations.
//
// Optional conditions are applied as WHERE clauses when loading the related
// entities, allowing you to filter which related records are loaded.
//
// Returns the same QueryBuilderSelect instance for method chaining.
//
// Example:
//
//	Preload("Posts")                                              // load all posts
//	Preload("Posts", Condition("published = ?", true))            // load only published posts
//	Preload("Posts.Comments")                                     // load posts and their comments
func (s *QueryBuilderSelect) Preload(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderSelect {
	s.preloads = append(s.preloads, preloadEntry{
		relation: relation,
		where:    conditions,
	})

	return s
}
