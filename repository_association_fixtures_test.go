package sqlr_test

import "github.com/gosoline-project/sqlr"

// ==========================================================================
// Test Entity Types (association-specific)
// ==========================================================================

// assocAuthor is an author entity with HasMany posts and a HasOne profile.
// Table: "assoc_authors".
type assocAuthor struct {
	sqlr.Entity[int64]
	Name    string       `db:"name"`
	Posts   []assocPost  `sqlr:"foreignKey:author_id"`
	Profile assocProfile `sqlr:"foreignKey:author_id"`
}

type assocAuthorSyncCreateDefaults struct {
	sqlr.Entity[int64]
	Name    string       `db:"name"`
	Posts   []assocPost  `sqlr:"foreignKey:author_id;sync:create"`
	Profile assocProfile `sqlr:"foreignKey:author_id"`
}

func (assocAuthorSyncCreateDefaults) TableName() string { return "assoc_author_sync_create_defaults" }

type assocAuthorSyncUpdateDefaults struct {
	sqlr.Entity[int64]
	Name    string       `db:"name"`
	Posts   []assocPost  `sqlr:"foreignKey:author_id;sync:update"`
	Profile assocProfile `sqlr:"foreignKey:author_id"`
}

func (assocAuthorSyncUpdateDefaults) TableName() string { return "assoc_author_sync_update_defaults" }

type assocAuthorSyncDeleteDefaults struct {
	sqlr.Entity[int64]
	Name    string       `db:"name"`
	Posts   []assocPost  `sqlr:"foreignKey:author_id;sync:delete"`
	Profile assocProfile `sqlr:"foreignKey:author_id"`
}

func (assocAuthorSyncDeleteDefaults) TableName() string { return "assoc_author_sync_delete_defaults" }

type assocAuthorWithPointerProfile struct {
	sqlr.Entity[int64]
	Name    string        `db:"name"`
	Profile *assocProfile `sqlr:"foreignKey:author_id"`
}

type assocAuthorAutoPreload struct {
	sqlr.Entity[int64]
	Name   string                             `db:"name"`
	Parent uint                               `db:"-"`
	Posts  []assocPostWithCommentsAutoPreload `sqlr:"foreignKey:author_id;preload"`
}

// assocPost is a post belonging to an author. Table: "assoc_posts".
type assocPost struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
}

type assocPostWithCommentsAutoPreload struct {
	sqlr.Entity[int64]
	AuthorID int64          `db:"author_id"`
	Title    string         `db:"title"`
	CacheKey string         `db:"-"`
	Comments []assocComment `sqlr:"foreignKey:post_id;preload"`
}

// assocProfile is a profile belonging to an author (HasOne). Table: "assoc_profiles".
type assocProfile struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Bio      string `db:"bio"`
}

// assocPostWithAuthor is a post that has a BelongsTo author. Table: "assoc_post_with_authors".
type assocPostWithAuthor struct {
	sqlr.Entity[int64]
	AuthorID int64       `db:"author_id"`
	Title    string      `db:"title"`
	Author   assocAuthor `sqlr:"belongsTo:author_id"`
}

type assocPostWithPointerAuthor struct {
	sqlr.Entity[int64]
	AuthorID int64        `db:"author_id"`
	Title    string       `db:"title"`
	Author   *assocAuthor `sqlr:"belongsTo:author_id"`
}

// assocArticle has a ManyToMany relationship with assocTag. Table: "assoc_articles".
type assocArticle struct {
	sqlr.Entity[int64]
	Title string     `db:"title"`
	Tags  []assocTag `sqlr:"many2many:assoc_article_tags"`
}

type assocArticleSyncUpdateDefaults struct {
	sqlr.Entity[int64]
	Title string     `db:"title"`
	Tags  []assocTag `sqlr:"many2many:assoc_article_sync_update_default_tags;sync:update;syncMode:many2many"`
}

func (assocArticleSyncUpdateDefaults) TableName() string { return "assoc_article_sync_update_defaults" }

type assocArticleWithPointerTags struct {
	sqlr.Entity[int64]
	Title string      `db:"title"`
	Tags  []*assocTag `sqlr:"many2many:assoc_article_tags"`
}

type assocAuthorWithPointerPosts struct {
	sqlr.Entity[int64]
	Name  string       `db:"name"`
	Posts []*assocPost `sqlr:"foreignKey:author_id"`
}

// assocTag is a tag used in many-to-many tests. Table: "assoc_tags".
type assocTag struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

// assocPostWithAll has HasMany comments and BelongsTo author (mixed relations).
// Table: "assoc_post_with_alls".
type assocPostWithAll struct {
	sqlr.Entity[int64]
	AuthorID int64          `db:"author_id"`
	Title    string         `db:"title"`
	Author   assocAuthor    `sqlr:"belongsTo:author_id"`
	Comments []assocComment `sqlr:"foreignKey:post_id"`
}

// assocComment is a comment on a post. Table: "assoc_comments".
type assocComment struct {
	sqlr.Entity[int64]
	PostID int64  `db:"post_id"`
	Body   string `db:"body"`
}

// ==========================================================================
// Test Entity Types (recursive / deeply-nested associations)
// ==========================================================================

// deepAuthor is the root entity for 3-level HasMany recursion tests.
// Table: "deep_authors".
type deepAuthor struct {
	sqlr.Entity[int64]
	Name  string     `db:"name"`
	Posts []deepPost `sqlr:"foreignKey:author_id"`
}

// deepPost belongs to deepAuthor and has many deepComments.
// Table: "deep_posts".
type deepPost struct {
	sqlr.Entity[int64]
	AuthorID int64         `db:"author_id"`
	Title    string        `db:"title"`
	Comments []deepComment `sqlr:"foreignKey:post_id"`
}

// deepComment belongs to deepPost. Table: "deep_comments".
type deepComment struct {
	sqlr.Entity[int64]
	PostID int64  `db:"post_id"`
	Body   string `db:"body"`
}

type deepAuthorNestedSyncDeleteDefaults struct {
	sqlr.Entity[int64]
	Name  string                            `db:"name"`
	Posts []deepPostNestedSyncDeleteDefault `sqlr:"foreignKey:author_id"`
}

func (deepAuthorNestedSyncDeleteDefaults) TableName() string {
	return "deep_author_nested_sync_delete_defaults"
}

type deepPostNestedSyncDeleteDefault struct {
	sqlr.Entity[int64]
	AuthorID int64                                `db:"author_id"`
	Title    string                               `db:"title"`
	Comments []deepCommentNestedSyncDeleteDefault `sqlr:"foreignKey:post_id;sync:delete"`
}

type deepCommentNestedSyncDeleteDefault struct {
	sqlr.Entity[int64]
	PostID int64  `db:"post_id"`
	Body   string `db:"body"`
}

// deepTag is used for recursive ManyToMany tests. Table: "deep_tags".
type deepTag struct {
	sqlr.Entity[int64]
	Name    string       `db:"name"`
	SubTags []deepSubTag `sqlr:"foreignKey:tag_id"`
}

// deepSubTag belongs to deepTag. Table: "deep_sub_tags".
type deepSubTag struct {
	sqlr.Entity[int64]
	TagID int64  `db:"tag_id"`
	Label string `db:"label"`
}

// deepArticle has ManyToMany deepTags (which themselves have HasMany subTags).
// Table: "deep_articles".
type deepArticle struct {
	sqlr.Entity[int64]
	Title string    `db:"title"`
	Tags  []deepTag `sqlr:"many2many:deep_article_tags"`
}

// deepLeafComment is used for a recursive BelongsTo chain test.
// It belongs to deepLeafPost, which itself belongs to deepLeafAuthor.
// Table: "deep_leaf_comments".
type deepLeafComment struct {
	sqlr.Entity[int64]
	PostID int64        `db:"post_id"`
	Body   string       `db:"body"`
	Post   deepLeafPost `sqlr:"belongsTo:post_id"`
}

// deepLeafPost belongs to deepLeafAuthor. Table: "deep_leaf_posts".
type deepLeafPost struct {
	sqlr.Entity[int64]
	AuthorID int64          `db:"author_id"`
	Title    string         `db:"title"`
	Author   deepLeafAuthor `sqlr:"belongsTo:author_id"`
}

// deepLeafAuthor is the leaf of the BelongsTo chain. Table: "deep_leaf_authors".
type deepLeafAuthor struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}
