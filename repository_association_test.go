package sqlr_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

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

type assocAuthorWithPointerProfile struct {
	sqlr.Entity[int64]
	Name    string        `db:"name"`
	Profile *assocProfile `sqlr:"foreignKey:author_id"`
}

// assocPost is a post belonging to an author. Table: "assoc_posts".
type assocPost struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
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

// ==========================================================================
// RepositoryAssociationCreateTestSuite
// ==========================================================================

// RepositoryAssociationCreateTestSuite tests auto-save of associations on Create.
type RepositoryAssociationCreateTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

// TestRepositoryAssociationCreateTestSuite runs the repository association create test suite.
func TestRepositoryAssociationCreateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryAssociationCreateTestSuite))
}

func (s *RepositoryAssociationCreateTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryAssociationCreateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func syncCreatePosts(qb *sqlr.QueryBuilderCreate) {
	qb.SyncAssociation("Posts")
}

func syncCreatePostsComments(qb *sqlr.QueryBuilderCreate) {
	qb.SyncAssociation("Posts.Comments")
}

func omitCreatePosts(qb *sqlr.QueryBuilderCreate) {
	qb.OmitAssociation("Posts")
}

func disableCreateAutoUpdates(qb *sqlr.QueryBuilderCreate) {
	qb.DisableAutoUpdates()
}

// --------------------------------------------------------------------------
// No associations — no transaction overhead
// --------------------------------------------------------------------------

// TestCreate_NoAssociations_NoTransaction verifies that Create avoids starting a transaction when no associations are populated.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_NoAssociations_NoTransaction() {
	repo := mustNewRepo[int64, testUser](s.T(), s.client)

	// Expect a plain INSERT without a surrounding transaction.
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice", "alice@test.com").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity := testUser{Name: "Alice", Email: "alice@test.com"}
	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Equal(int64(1), entity.GetId())
}

// TestCreate_EmptyAssociationSlice_NoTransaction verifies that Create avoids starting a transaction for empty association slices.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_EmptyAssociationSlice_NoTransaction() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	// Posts and Profile fields are zero — no associations to save, so no transaction.
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob").
		WillReturnResult(sqlmock.NewResult(10, 1))

	entity := assocAuthor{Name: "Bob"}
	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Equal(int64(10), entity.GetId())
}

// TestCreate_OmitAssociation_SkipsOmittedRelationWithoutTransaction verifies that Create skips omitted relations without starting a transaction.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_OmitAssociation_SkipsOmittedRelationWithoutTransaction() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob").
		WillReturnResult(sqlmock.NewResult(10, 1))

	entity := assocAuthor{
		Name:  "Bob",
		Posts: []assocPost{{Title: "Skipped"}},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, omitCreatePosts))
	s.Equal(int64(10), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(0), entity.Posts[0].GetId())
	s.Equal(int64(0), entity.Posts[0].AuthorID)
}

// TestCreate_NilEntity_WithAssociationOptionsReturnsError verifies that Create returns an error for nil entities even when association sync is configured.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_NilEntity_WithAssociationOptionsReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	err := repo.Create(context.Background(), nil, syncCreatePosts)

	s.Require().Error(err)
	s.Require().ErrorIs(err, sqlr.ErrNilEntity)
}

// --------------------------------------------------------------------------
// HasMany: author with posts
// --------------------------------------------------------------------------

// TestCreate_HasMany_InsertsAssociations verifies that Create inserts associations for has-many relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_InsertsAssociations() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Phase 3: insert post 1 (author_id=1)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post A").
		WillReturnResult(sqlmock.NewResult(10, 1))

	// Phase 3: insert post 2 (author_id=1)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post B").
		WillReturnResult(sqlmock.NewResult(11, 1))

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Name: "Alice",
		Posts: []assocPost{
			{Title: "Post A"},
			{Title: "Post B"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 2)
	s.Equal(int64(10), entity.Posts[0].GetId())
	s.Equal(int64(1), entity.Posts[0].AuthorID)
	s.Equal(int64(11), entity.Posts[1].GetId())
	s.Equal(int64(1), entity.Posts[1].AuthorID)
}

// TestCreate_HasMany_PersistsExistingAssociations verifies that Create persists existing associations for has-many relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_PersistsExistingAssociations() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	authorNow := time.Now()
	postNow := time.Now().Add(-time.Hour)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Phase 3: existing post gets its FK persisted, new post gets inserted.
	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE `assoc_posts` SET `author_id` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(1), isTimestamp{}, int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "New Post").
		WillReturnResult(sqlmock.NewResult(20, 1))

	s.mock.ExpectCommit()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `assoc_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), authorNow, authorNow, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(99), postNow, postNow, int64(1), "Existing Post").
			AddRow(int64(20), authorNow, authorNow, int64(1), "New Post"))

	entity := assocAuthor{
		Name: "Alice",
		Posts: []assocPost{
			{Entity: sqlr.Entity[int64]{Id: 99, CreatedAt: postNow, UpdatedAt: postNow}, Title: "Existing Post"},
			{Title: "New Post"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	// Existing post keeps its ID unchanged.
	s.Equal(int64(99), entity.Posts[0].GetId())
	s.Equal(int64(1), entity.Posts[0].AuthorID)
	// New post gets its generated ID.
	s.Equal(int64(20), entity.Posts[1].GetId())
	s.Equal(int64(1), entity.Posts[1].AuthorID)

	stored, err := repo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(stored.Posts, 2)
	s.Equal(int64(99), stored.Posts[0].GetId())
	s.Equal(int64(1), stored.Posts[0].AuthorID)
	s.Equal("Existing Post", stored.Posts[0].Title)
}

// TestCreate_SyncAssociation_OnlyCreatesSelectedRelation verifies that Create only creates selected relation for explicit association sync.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_SyncAssociation_OnlyCreatesSelectedRelation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post A").
		WillReturnResult(sqlmock.NewResult(10, 1))
	s.mock.ExpectCommit()

	entity := assocAuthor{
		Name:    "Alice",
		Posts:   []assocPost{{Title: "Post A"}},
		Profile: assocProfile{Bio: "Skipped"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, syncCreatePosts))
	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(10), entity.Posts[0].GetId())
	s.Equal(int64(1), entity.Posts[0].AuthorID)
	s.Equal(int64(0), entity.Profile.GetId())
	s.Equal(int64(0), entity.Profile.AuthorID)
}

// TestCreate_SyncCreateTag_OnlyCreatesTaggedRelations verifies that sync:create
// tags narrow Create's default association synchronization to tagged paths.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_SyncCreateTag_OnlyCreatesTaggedRelations() {
	repo := mustNewRepo[int64, assocAuthorSyncCreateDefaults](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_author_sync_create_defaults` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post A").
		WillReturnResult(sqlmock.NewResult(10, 1))
	s.mock.ExpectCommit()

	entity := assocAuthorSyncCreateDefaults{
		Name:    "Alice",
		Posts:   []assocPost{{Title: "Post A"}},
		Profile: assocProfile{Bio: "Skipped"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(10), entity.Posts[0].GetId())
	s.Equal(int64(1), entity.Posts[0].AuthorID)
	s.Equal(int64(0), entity.Profile.GetId())
	s.Equal(int64(0), entity.Profile.AuthorID)
}

// --------------------------------------------------------------------------
// HasOne: author with profile
// --------------------------------------------------------------------------

// TestCreate_HasOne_InsertsAssociation verifies that Create inserts association for has-one relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasOne_InsertsAssociation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Carol").
		WillReturnResult(sqlmock.NewResult(5, 1))

	// Phase 3: insert profile with author_id=5
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_profiles` (`created_at`, `updated_at`, `author_id`, `bio`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(5), "My bio").
		WillReturnResult(sqlmock.NewResult(50, 1))

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Name:    "Carol",
		Profile: assocProfile{Bio: "My bio"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(5), entity.GetId())
	s.Equal(int64(50), entity.Profile.GetId())
	s.Equal(int64(5), entity.Profile.AuthorID)
}

// TestCreate_HasOnePointer_InsertsAssociation verifies that Create inserts association for pointer has-one relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasOnePointer_InsertsAssociation() {
	repo := mustNewRepo[int64, assocAuthorWithPointerProfile](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_author_with_pointer_profiles` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Carol").
		WillReturnResult(sqlmock.NewResult(5, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_profiles` (`created_at`, `updated_at`, `author_id`, `bio`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(5), "My bio").
		WillReturnResult(sqlmock.NewResult(50, 1))
	s.mock.ExpectCommit()

	entity := assocAuthorWithPointerProfile{
		Name:    "Carol",
		Profile: &assocProfile{Bio: "My bio"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Require().NotNil(entity.Profile)
	s.Equal(int64(50), entity.Profile.GetId())
	s.Equal(int64(5), entity.Profile.AuthorID)
}

// TestCreate_HasOne_PersistsExistingAssociation verifies that Create persists existing association for has-one relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasOne_PersistsExistingAssociation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	authorNow := time.Now()
	profileNow := time.Now().Add(-time.Hour)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Dave").
		WillReturnResult(sqlmock.NewResult(7, 1))

	// Profile has a non-zero PK and gets its FK persisted.
	s.mock.ExpectExec(regexp.QuoteMeta("UPDATE `assoc_profiles` SET `author_id` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(7), isTimestamp{}, int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `assoc_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(7), authorNow, authorNow, "Dave"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` IN (?)")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(int64(100), profileNow, profileNow, int64(7), "existing"))

	entity := assocAuthor{
		Name:    "Dave",
		Profile: assocProfile{Entity: sqlr.Entity[int64]{Id: 100, CreatedAt: profileNow, UpdatedAt: profileNow}, Bio: "existing"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(7), entity.GetId())
	// Profile ID unchanged.
	s.Equal(int64(100), entity.Profile.GetId())
	// FK on profile is still set (we set it even for existing associations).
	s.Equal(int64(7), entity.Profile.AuthorID)

	stored, err := repo.Read(context.Background(), 7, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Profile")
	})

	s.Require().NoError(err)
	s.Equal(int64(100), stored.Profile.GetId())
	s.Equal(int64(7), stored.Profile.AuthorID)
	s.Equal("existing", stored.Profile.Bio)
}

// --------------------------------------------------------------------------
// BelongsTo: post with author
// --------------------------------------------------------------------------

// TestCreate_BelongsTo_InsertsRelatedFirst verifies that Create inserts related first for belongs-to relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_InsertsRelatedFirst() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: insert author first (zero PK)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Eve").
		WillReturnResult(sqlmock.NewResult(3, 1))

	// Phase 2: insert post with author_id=3 (FK set from author PK)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_post_with_authors` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(3), "Eve's post").
		WillReturnResult(sqlmock.NewResult(30, 1))

	s.mock.ExpectCommit()

	entity := assocPostWithAuthor{
		Title:  "Eve's post",
		Author: assocAuthor{Name: "Eve"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(30), entity.GetId())
	s.Equal(int64(3), entity.AuthorID) // FK set on parent
	s.Equal(int64(3), entity.Author.GetId())
}

// TestCreate_BelongsToPointer_InsertsRelatedFirst verifies that Create inserts related first for pointer belongs-to relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsToPointer_InsertsRelatedFirst() {
	repo := mustNewRepo[int64, assocPostWithPointerAuthor](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Eve").
		WillReturnResult(sqlmock.NewResult(3, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_post_with_pointer_authors` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(3), "My Post").
		WillReturnResult(sqlmock.NewResult(8, 1))
	s.mock.ExpectCommit()

	entity := assocPostWithPointerAuthor{
		Title:  "My Post",
		Author: &assocAuthor{Name: "Eve"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Require().NotNil(entity.Author)
	s.Equal(int64(3), entity.Author.GetId())
	s.Equal(int64(3), entity.AuthorID)
	s.Equal(int64(8), entity.GetId())
}

// TestCreate_HasManyPointerElements_RejectsNilElement verifies that Create rejects nil element for pointer-based has-many collections.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasManyPointerElements_RejectsNilElement() {
	repo := mustNewRepo[int64, assocAuthorWithPointerPosts](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_author_with_pointer_posts` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectRollback()

	entity := assocAuthorWithPointerPosts{
		Name:  "Alice",
		Posts: []*assocPost{nil},
	}

	err := repo.Create(context.Background(), &entity)
	s.Require().Error(err)
	s.ErrorContains(err, "HasMany relation \"Posts\"[0] is nil")
}

// TestCreate_ManyToManyPointerElements_RejectsNilElement verifies that Create rejects nil element for pointer-based many-to-many collections.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToManyPointerElements_RejectsNilElement() {
	repo := mustNewRepo[int64, assocArticleWithPointerTags](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_article_with_pointer_tags` (`created_at`, `updated_at`, `title`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Go Tips").
		WillReturnResult(sqlmock.NewResult(2, 1))
	s.mock.ExpectRollback()

	entity := assocArticleWithPointerTags{
		Title: "Go Tips",
		Tags:  []*assocTag{nil},
	}

	err := repo.Create(context.Background(), &entity)
	s.Require().Error(err)
	s.ErrorContains(err, "ManyToMany relation \"Tags\"[0] is nil")
}

// TestCreate_BelongsTo_ExistingRelated_SetsFK verifies that Create reuses an existing belongs-to entity and sets the parent foreign key.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_ExistingRelated_SetsFK() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: author already has PK=7, no insert.
	// Phase 2: insert post with author_id=7
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_post_with_authors` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(7), "Frank's post").
		WillReturnResult(sqlmock.NewResult(40, 1))

	s.mock.ExpectCommit()

	entity := assocPostWithAuthor{
		Title:  "Frank's post",
		Author: assocAuthor{Entity: sqlr.Entity[int64]{Id: 7}, Name: "Frank"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(40), entity.GetId())
	s.Equal(int64(7), entity.AuthorID) // FK set from existing author PK
}

// TestCreate_DisableAutoUpdates_UsesPresetValuesAcrossGraph verifies that Create uses preset values across graph when auto-updates are disabled.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_DisableAutoUpdates_UsesPresetValuesAcrossGraph() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	authorCreatedAt := time.Now().Add(-3 * time.Hour)
	authorUpdatedAt := time.Now().Add(-2 * time.Hour)
	postCreatedAt := time.Now().Add(-90 * time.Minute)
	postUpdatedAt := time.Now().Add(-75 * time.Minute)

	authorInsertSQL := regexp.QuoteMeta("INSERT INTO `assoc_authors` (`id`, `created_at`, `updated_at`, `name`) VALUES (?, ?, ?, ?)")
	postUpdateSQL := regexp.QuoteMeta("UPDATE `assoc_posts` SET `author_id` = ? WHERE `id` = ?")

	s.mock.ExpectBegin()
	s.mock.ExpectExec(authorInsertSQL).
		WithArgs(int64(1), authorCreatedAt, authorUpdatedAt, "Alice").
		WillReturnResult(sqlmock.NewResult(999, 1))
	s.mock.ExpectExec(postUpdateSQL).
		WithArgs(int64(1), int64(10)).
		WillReturnResult(sqlmock.NewResult(999, 1))
	s.mock.ExpectCommit()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: authorCreatedAt,
			UpdatedAt: authorUpdatedAt,
		},
		Name: "Alice",
		Posts: []assocPost{{
			Entity: sqlr.Entity[int64]{
				Id:        10,
				CreatedAt: postCreatedAt,
				UpdatedAt: postUpdatedAt,
			},
			Title: "Post A",
		}},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, syncCreatePosts, disableCreateAutoUpdates))
	s.Equal(int64(1), entity.GetId())
	s.Equal(authorCreatedAt, entity.CreatedAt)
	s.Equal(authorUpdatedAt, entity.UpdatedAt)
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(10), entity.Posts[0].GetId())
	s.Equal(postCreatedAt, entity.Posts[0].CreatedAt)
	s.Equal(postUpdatedAt, entity.Posts[0].UpdatedAt)
	s.Equal(int64(1), entity.Posts[0].AuthorID)
}

// TestCreate_DisableAutoUpdates_FKMismatchReturnsError verifies that Create returns an error for foreign key mismatch when auto-updates are disabled.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_DisableAutoUpdates_FKMismatchReturnsError() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	s.mock.ExpectBegin()
	s.mock.ExpectRollback()

	entity := assocPostWithAuthor{
		Entity: sqlr.Entity[int64]{
			Id: 5,
		},
		AuthorID: 99,
		Title:    "Mismatch",
		Author: assocAuthor{
			Entity: sqlr.Entity[int64]{Id: 7},
			Name:   "Frank",
		},
	}

	err := repo.Create(context.Background(), &entity, disableCreateAutoUpdates)

	s.Require().Error(err)
	s.ErrorContains(err, "field assoc_post_with_authors.author_id value 99 does not match associated primary key 7")
}

// TestCreate_BelongsTo_ZeroRelated_Skipped verifies that Create skips zero-value belongs-to relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_ZeroRelated_Skipped() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	// Author field is zero (empty struct) — treated as no association.
	// No transaction expected because no associations to save.
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_post_with_authors` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(0), "Orphan post").
		WillReturnResult(sqlmock.NewResult(50, 1))

	entity := assocPostWithAuthor{
		Title: "Orphan post",
		// Author is zero value — not populated.
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(50), entity.GetId())
	s.Equal(int64(0), entity.AuthorID) // FK stays zero
}

// --------------------------------------------------------------------------
// ManyToMany: article with tags
// --------------------------------------------------------------------------

// TestCreate_ManyToMany_InsertsTagsAndJoinRows verifies that Create inserts tags and join rows for many-to-many relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToMany_InsertsTagsAndJoinRows() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent article
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_articles` (`created_at`, `updated_at`, `title`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Go Tips").
		WillReturnResult(sqlmock.NewResult(2, 1))

	// Phase 4: insert tag 1 (zero PK)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "golang").
		WillReturnResult(sqlmock.NewResult(100, 1))

	// Phase 4: insert tag 2 (zero PK)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "tips").
		WillReturnResult(sqlmock.NewResult(101, 1))

	// Phase 4: insert join table rows for both tags
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_tags` (`assoc_article_id`, `assoc_tag_id`) VALUES (?, ?), (?, ?)")).
		WithArgs(int64(2), int64(100), int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Title: "Go Tips",
		Tags: []assocTag{
			{Name: "golang"},
			{Name: "tips"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(2), entity.GetId())
	s.Require().Len(entity.Tags, 2)
	s.Equal(int64(100), entity.Tags[0].GetId())
	s.Equal(int64(101), entity.Tags[1].GetId())
}

// TestCreate_ManyToMany_SkipsExistingTags verifies that Create skips existing tags for many-to-many relations.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToMany_SkipsExistingTags() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent article
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_articles` (`created_at`, `updated_at`, `title`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Existing Tags").
		WillReturnResult(sqlmock.NewResult(3, 1))

	// Phase 4: only new tag gets inserted (existing tag with ID=200 is skipped)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "new-tag").
		WillReturnResult(sqlmock.NewResult(201, 1))

	// Phase 4: join table rows for both (existing + new)
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_tags` (`assoc_article_id`, `assoc_tag_id`) VALUES (?, ?), (?, ?)")).
		WithArgs(int64(3), int64(200), int64(3), int64(201)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Title: "Existing Tags",
		Tags: []assocTag{
			{Entity: sqlr.Entity[int64]{Id: 200}, Name: "existing-tag"},
			{Name: "new-tag"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(3), entity.GetId())
	s.Equal(int64(200), entity.Tags[0].GetId()) // unchanged
	s.Equal(int64(201), entity.Tags[1].GetId()) // newly generated
}

// --------------------------------------------------------------------------
// Mixed: post with BelongsTo author and HasMany comments
// --------------------------------------------------------------------------

// TestCreate_Mixed_BelongsToAndHasMany verifies that Create persists belongs-to and has-many relations within the same graph.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_Mixed_BelongsToAndHasMany() {
	repo := mustNewRepo[int64, assocPostWithAll](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: insert BelongsTo author first
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Grace").
		WillReturnResult(sqlmock.NewResult(4, 1))

	// Phase 2: insert parent post (author_id=4)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_post_with_alls` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(4), "Grace's post").
		WillReturnResult(sqlmock.NewResult(400, 1))

	// Phase 3: insert comments with post_id=400
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(400), "Great post!").
		WillReturnResult(sqlmock.NewResult(1000, 1))

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(400), "Thanks!").
		WillReturnResult(sqlmock.NewResult(1001, 1))

	s.mock.ExpectCommit()

	entity := assocPostWithAll{
		Title:  "Grace's post",
		Author: assocAuthor{Name: "Grace"},
		Comments: []assocComment{
			{Body: "Great post!"},
			{Body: "Thanks!"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(400), entity.GetId())
	s.Equal(int64(4), entity.AuthorID)
	s.Equal(int64(4), entity.Author.GetId())
	s.Require().Len(entity.Comments, 2)
	s.Equal(int64(1000), entity.Comments[0].GetId())
	s.Equal(int64(400), entity.Comments[0].PostID)
	s.Equal(int64(1001), entity.Comments[1].GetId())
	s.Equal(int64(400), entity.Comments[1].PostID)
}

// --------------------------------------------------------------------------
// Transaction rollback on error
// --------------------------------------------------------------------------

// TestCreate_HasMany_RollbackOnAssociationError verifies that Create rolls back parent and child state when a has-many association insert fails.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_RollbackOnAssociationError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: parent insert succeeds
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Henry").
		WillReturnResult(sqlmock.NewResult(5, 1))

	// Phase 3: association insert fails
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(5), "Bad Post").
		WillReturnError(fmt.Errorf("constraint violation"))

	s.mock.ExpectRollback()

	entity := assocAuthor{
		Name:  "Henry",
		Posts: []assocPost{{Title: "Bad Post"}},
	}

	err := repo.Create(context.Background(), &entity)
	s.Require().Error(err)
	s.Contains(err.Error(), "constraint violation")
	s.Zero(entity.GetId())
	s.Zero(entity.CreatedAt)
	s.Zero(entity.UpdatedAt)
	s.Require().Len(entity.Posts, 1)
	s.Zero(entity.Posts[0].GetId())
	s.Zero(entity.Posts[0].AuthorID)
	s.Zero(entity.Posts[0].CreatedAt)
	s.Zero(entity.Posts[0].UpdatedAt)
}

// TestCreate_BelongsTo_RollbackOnRelatedInsertError verifies that Create rolls back when inserting a belongs-to relation fails.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_RollbackOnRelatedInsertError() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: author insert fails
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Ivan").
		WillReturnError(fmt.Errorf("db error"))

	s.mock.ExpectRollback()

	entity := assocPostWithAuthor{
		Title:  "Ivan's post",
		Author: assocAuthor{Name: "Ivan"},
	}

	err := repo.Create(context.Background(), &entity)
	s.Require().Error(err)
	s.Contains(err.Error(), "db error")
	s.Zero(entity.GetId())
	s.Zero(entity.AuthorID)
	s.Zero(entity.CreatedAt)
	s.Zero(entity.UpdatedAt)
	s.Zero(entity.Author.GetId())
	s.Zero(entity.Author.CreatedAt)
	s.Zero(entity.Author.UpdatedAt)
}

// --------------------------------------------------------------------------
// Recursive: 3-level HasMany chain (deepAuthor → deepPost → deepComment)
// --------------------------------------------------------------------------

// TestCreate_HasMany_Recursive_InsertsNestedAssociations verifies that Create inserts nested associations for has-many recursive.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_Recursive_InsertsNestedAssociations() {
	repo := mustNewRepo[int64, deepAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert root author
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Root").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Phase 3: insert post 1 (author_id=1); post has two comments so post itself
	// is inserted first, then its comments recursively.
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post 1").
		WillReturnResult(sqlmock.NewResult(10, 1))

	// Recursive: comments for post 1
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(10), "Comment A").
		WillReturnResult(sqlmock.NewResult(100, 1))

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(10), "Comment B").
		WillReturnResult(sqlmock.NewResult(101, 1))

	// Phase 3: insert post 2 (author_id=1); no comments → just the post row.
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post 2").
		WillReturnResult(sqlmock.NewResult(11, 1))

	s.mock.ExpectCommit()

	entity := deepAuthor{
		Name: "Root",
		Posts: []deepPost{
			{
				Title: "Post 1",
				Comments: []deepComment{
					{Body: "Comment A"},
					{Body: "Comment B"},
				},
			},
			{Title: "Post 2"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(1), entity.GetId())

	s.Require().Len(entity.Posts, 2)
	s.Equal(int64(10), entity.Posts[0].GetId())
	s.Equal(int64(1), entity.Posts[0].AuthorID)

	s.Require().Len(entity.Posts[0].Comments, 2)
	s.Equal(int64(100), entity.Posts[0].Comments[0].GetId())
	s.Equal(int64(10), entity.Posts[0].Comments[0].PostID)
	s.Equal(int64(101), entity.Posts[0].Comments[1].GetId())
	s.Equal(int64(10), entity.Posts[0].Comments[1].PostID)

	s.Equal(int64(11), entity.Posts[1].GetId())
	s.Equal(int64(1), entity.Posts[1].AuthorID)
	s.Empty(entity.Posts[1].Comments)
}

// TestCreate_SyncAssociation_NestedPathSynchronizesAncestors verifies that Create nested path synchronizes ancestors for explicit association sync.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_SyncAssociation_NestedPathSynchronizesAncestors() {
	repo := mustNewRepo[int64, deepAuthor](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Root").
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post 1").
		WillReturnResult(sqlmock.NewResult(10, 1))
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(10), "Comment A").
		WillReturnResult(sqlmock.NewResult(100, 1))
	s.mock.ExpectCommit()

	entity := deepAuthor{
		Name: "Root",
		Posts: []deepPost{{
			Title: "Post 1",
			Comments: []deepComment{{
				Body: "Comment A",
			}},
		}},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, syncCreatePostsComments))
	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(10), entity.Posts[0].GetId())
	s.Equal(int64(1), entity.Posts[0].AuthorID)
	s.Require().Len(entity.Posts[0].Comments, 1)
	s.Equal(int64(100), entity.Posts[0].Comments[0].GetId())
	s.Equal(int64(10), entity.Posts[0].Comments[0].PostID)
}

// --------------------------------------------------------------------------
// Recursive: 3-level BelongsTo chain (deepLeafComment → deepLeafPost → deepLeafAuthor)
// --------------------------------------------------------------------------

// TestCreate_BelongsTo_Recursive_InsertsNestedChain verifies that Create inserts nested chain for belongs-to recursive.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_Recursive_InsertsNestedChain() {
	repo := mustNewRepo[int64, deepLeafComment](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1 (deepLeafComment): persist BelongsTo deepLeafPost
	//   Phase 1 (deepLeafPost): persist BelongsTo deepLeafAuthor
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_leaf_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Leaf Author").
		WillReturnResult(sqlmock.NewResult(5, 1))

	//   Phase 2 (deepLeafPost): insert post with author_id=5
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_leaf_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(5), "Leaf Post").
		WillReturnResult(sqlmock.NewResult(50, 1))

	// Phase 2 (deepLeafComment): insert comment with post_id=50
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_leaf_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(50), "Leaf Comment").
		WillReturnResult(sqlmock.NewResult(500, 1))

	s.mock.ExpectCommit()

	entity := deepLeafComment{
		Body: "Leaf Comment",
		Post: deepLeafPost{
			Title: "Leaf Post",
			Author: deepLeafAuthor{
				Name: "Leaf Author",
			},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(500), entity.GetId())
	s.Equal(int64(50), entity.PostID)
	s.Equal(int64(50), entity.Post.GetId())
	s.Equal(int64(5), entity.Post.AuthorID)
	s.Equal(int64(5), entity.Post.Author.GetId())
}

// TestCreate_SyncAssociation_InvalidPathReturnsError verifies that Create returns an error for invalid path for explicit association sync.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_SyncAssociation_InvalidPathReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	entity := assocAuthor{Name: "Alice"}

	err := repo.Create(context.Background(), &entity, func(qb *sqlr.QueryBuilderCreate) {
		qb.SyncAssociation("Unknown")
	})

	s.Require().Error(err)
	s.ErrorContains(err, "invalid sync association path \"Unknown\"")
}

// --------------------------------------------------------------------------
// Recursive: ManyToMany where each related entity has its own HasMany
// --------------------------------------------------------------------------

// TestCreate_ManyToMany_Recursive_InsertsNestedHasMany verifies that Create persists nested has-many relations inside recursive many-to-many graphs.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToMany_Recursive_InsertsNestedHasMany() {
	repo := mustNewRepo[int64, deepArticle](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert root article
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_articles` (`created_at`, `updated_at`, `title`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Deep Article").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Phase 4: insert tag 1 (has two sub-tags)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Tag A").
		WillReturnResult(sqlmock.NewResult(10, 1))

	// Recursive HasMany: sub-tags for tag 1
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_sub_tags` (`created_at`, `updated_at`, `tag_id`, `label`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(10), "Sub A1").
		WillReturnResult(sqlmock.NewResult(100, 1))

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_sub_tags` (`created_at`, `updated_at`, `tag_id`, `label`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(10), "Sub A2").
		WillReturnResult(sqlmock.NewResult(101, 1))

	// Phase 4: insert tag 2 (no sub-tags)
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `deep_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Tag B").
		WillReturnResult(sqlmock.NewResult(11, 1))

	// Phase 4: join table rows for both tags
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `deep_article_tags` (`deep_article_id`, `deep_tag_id`) VALUES (?, ?), (?, ?)")).
		WithArgs(int64(1), int64(10), int64(1), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	s.mock.ExpectCommit()

	entity := deepArticle{
		Title: "Deep Article",
		Tags: []deepTag{
			{
				Name: "Tag A",
				SubTags: []deepSubTag{
					{Label: "Sub A1"},
					{Label: "Sub A2"},
				},
			},
			{Name: "Tag B"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(1), entity.GetId())

	s.Require().Len(entity.Tags, 2)
	s.Equal(int64(10), entity.Tags[0].GetId())
	s.Require().Len(entity.Tags[0].SubTags, 2)
	s.Equal(int64(100), entity.Tags[0].SubTags[0].GetId())
	s.Equal(int64(10), entity.Tags[0].SubTags[0].TagID)
	s.Equal(int64(101), entity.Tags[0].SubTags[1].GetId())
	s.Equal(int64(10), entity.Tags[0].SubTags[1].TagID)
	s.Equal(int64(11), entity.Tags[1].GetId())
	s.Empty(entity.Tags[1].SubTags)
}

// --------------------------------------------------------------------------
// RepositoryTx: association save within an existing transaction
// --------------------------------------------------------------------------

// RepositoryTxAssociationTestSuite tests auto-save of associations in RepositoryTx.
type RepositoryTxAssociationTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

// TestRepositoryTxAssociationTestSuite runs the repository tx association test suite.
func TestRepositoryTxAssociationTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTxAssociationTestSuite))
}

func (s *RepositoryTxAssociationTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryTxAssociationTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// TestCreate_HasMany_UsesExistingTransaction verifies that Create uses existing transaction for has-many relations.
func (s *RepositoryTxAssociationTestSuite) TestCreate_HasMany_UsesExistingTransaction() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	// The caller manages the transaction; RepositoryTx uses it directly.
	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Judy").
		WillReturnResult(sqlmock.NewResult(6, 1))

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(6), "Judy's Post").
		WillReturnResult(sqlmock.NewResult(60, 1))

	s.mock.ExpectCommit()

	err = s.client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)

		entity := assocAuthor{
			Name:  "Judy",
			Posts: []assocPost{{Title: "Judy's Post"}},
		}

		return txRepo.Create(ttx, &entity)
	})
	s.Require().NoError(err)
}

// TestCreate_HasMany_NoAssociations_NoExtraQueries verifies that RepositoryTx Create avoids extra association queries when no has-many data is populated.
func (s *RepositoryTxAssociationTestSuite) TestCreate_HasMany_NoAssociations_NoExtraQueries() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	s.mock.ExpectBegin()

	// Only the author INSERT — no association INSERTs.
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Karl").
		WillReturnResult(sqlmock.NewResult(7, 1))

	s.mock.ExpectCommit()

	err = s.client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)
		entity := assocAuthor{Name: "Karl"} // no posts, no profile

		return txRepo.Create(ttx, &entity)
	})
	s.Require().NoError(err)
}

// TestCreate_EmptyRelationSlice_NoAssociationTransactionWork verifies that RepositoryTx Create performs no association work for empty relation slices.
func (s *RepositoryTxAssociationTestSuite) TestCreate_EmptyRelationSlice_NoAssociationTransactionWork() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Lena").
		WillReturnResult(sqlmock.NewResult(8, 1))
	s.mock.ExpectCommit()

	err = s.client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)
		entity := assocAuthor{Name: "Lena", Posts: []assocPost{}}

		return txRepo.Create(ttx, &entity)
	})
	s.Require().NoError(err)
}

// TestCreate_NilEntity_WithAssociationOptionsReturnsError verifies that Create returns an error for nil entities even when association sync is configured.
func (s *RepositoryTxAssociationTestSuite) TestCreate_NilEntity_WithAssociationOptionsReturnsError() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	err = txRepo.Create(sqlr.TTx{}, nil, syncCreatePosts)

	s.Require().Error(err)
	s.Require().ErrorIs(err, sqlr.ErrNilEntity)
}
