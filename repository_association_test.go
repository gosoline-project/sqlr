package sqlr_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

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
	Posts   []assocPost  `db:"-,foreignKey:author_id"`
	Profile assocProfile `db:"-,foreignKey:author_id"`
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
	Author   assocAuthor `db:"-,belongsTo:author_id"`
}

// assocArticle has a ManyToMany relationship with assocTag. Table: "assoc_articles".
type assocArticle struct {
	sqlr.Entity[int64]
	Title string     `db:"title"`
	Tags  []assocTag `db:"-,many2many:assoc_article_tags"`
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
	Author   assocAuthor    `db:"-,belongsTo:author_id"`
	Comments []assocComment `db:"-,foreignKey:post_id"`
}

// assocComment is a comment on a post. Table: "assoc_comments".
type assocComment struct {
	sqlr.Entity[int64]
	PostID int64  `db:"post_id"`
	Body   string `db:"body"`
}

// ==========================================================================
// SQL constants for association tests
// ==========================================================================

const (
	assocAuthorInsertSQL         = "INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)"
	assocPostInsertSQL           = "INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)"
	assocProfileInsertSQL        = "INSERT INTO `assoc_profiles` (`created_at`, `updated_at`, `author_id`, `bio`) VALUES (?, ?, ?, ?)"
	assocPostWithAuthorInsertSQL = "INSERT INTO `assoc_post_with_authors` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)"
	assocArticleInsertSQL        = "INSERT INTO `assoc_articles` (`created_at`, `updated_at`, `title`) VALUES (?, ?, ?)"
	assocTagInsertSQL            = "INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)"
	assocCommentInsertSQL        = "INSERT INTO `assoc_comments` (`created_at`, `updated_at`, `post_id`, `body`) VALUES (?, ?, ?, ?)"
	assocPostWithAllInsertSQL    = "INSERT INTO `assoc_post_with_alls` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)"
)

// ==========================================================================
// RepositoryAssociationCreateTestSuite
// ==========================================================================

// RepositoryAssociationCreateTestSuite tests auto-save of associations on Create.
type RepositoryAssociationCreateTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

func TestRepositoryAssociationCreateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryAssociationCreateTestSuite))
}

func (s *RepositoryAssociationCreateTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryAssociationCreateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// --------------------------------------------------------------------------
// No associations — no transaction overhead
// --------------------------------------------------------------------------

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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_EmptyAssociationSlice_NoTransaction() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	// Posts and Profile fields are zero — no associations to save, so no transaction.
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob").
		WillReturnResult(sqlmock.NewResult(10, 1))

	entity := assocAuthor{Name: "Bob"}
	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Equal(int64(10), entity.GetId())
}

// --------------------------------------------------------------------------
// HasMany: author with posts
// --------------------------------------------------------------------------

func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_InsertsAssociations() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Phase 3: insert post 1 (author_id=1)
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Post A").
		WillReturnResult(sqlmock.NewResult(10, 1))

	// Phase 3: insert post 2 (author_id=1)
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_SkipsExistingAssociations() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Phase 3: only the post without an ID gets inserted (post with ID=99 is skipped).
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "New Post").
		WillReturnResult(sqlmock.NewResult(20, 1))

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Name: "Alice",
		Posts: []assocPost{
			{Entity: sqlr.Entity[int64]{Id: 99}, AuthorID: 1, Title: "Existing Post"},
			{Title: "New Post"},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	// Existing post keeps its ID unchanged.
	s.Equal(int64(99), entity.Posts[0].GetId())
	// New post gets its generated ID.
	s.Equal(int64(20), entity.Posts[1].GetId())
}

// --------------------------------------------------------------------------
// HasOne: author with profile
// --------------------------------------------------------------------------

func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasOne_InsertsAssociation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Carol").
		WillReturnResult(sqlmock.NewResult(5, 1))

	// Phase 3: insert profile with author_id=5
	s.mock.ExpectExec(regexp.QuoteMeta(assocProfileInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasOne_SkipsExistingAssociation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent author
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Dave").
		WillReturnResult(sqlmock.NewResult(7, 1))

	// Profile has a non-zero PK → no insert expected.

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Name:    "Dave",
		Profile: assocProfile{Entity: sqlr.Entity[int64]{Id: 100}, AuthorID: 7, Bio: "existing"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))

	s.Equal(int64(7), entity.GetId())
	// Profile ID unchanged.
	s.Equal(int64(100), entity.Profile.GetId())
	// FK on profile is still set (we set it even for existing associations).
	s.Equal(int64(7), entity.Profile.AuthorID)
}

// --------------------------------------------------------------------------
// BelongsTo: post with author
// --------------------------------------------------------------------------

func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_InsertsRelatedFirst() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: insert author first (zero PK)
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Eve").
		WillReturnResult(sqlmock.NewResult(3, 1))

	// Phase 2: insert post with author_id=3 (FK set from author PK)
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostWithAuthorInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_ExistingRelated_SetsFK() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: author already has PK=7, no insert.
	// Phase 2: insert post with author_id=7
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostWithAuthorInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_ZeroRelated_Skipped() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	// Author field is zero (empty struct) — treated as no association.
	// No transaction expected because no associations to save.
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostWithAuthorInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToMany_InsertsTagsAndJoinRows() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent article
	s.mock.ExpectExec(regexp.QuoteMeta(assocArticleInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Go Tips").
		WillReturnResult(sqlmock.NewResult(2, 1))

	// Phase 4: insert tag 1 (zero PK)
	s.mock.ExpectExec(regexp.QuoteMeta(assocTagInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "golang").
		WillReturnResult(sqlmock.NewResult(100, 1))

	// Phase 4: insert tag 2 (zero PK)
	s.mock.ExpectExec(regexp.QuoteMeta(assocTagInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToMany_SkipsExistingTags() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: insert parent article
	s.mock.ExpectExec(regexp.QuoteMeta(assocArticleInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Existing Tags").
		WillReturnResult(sqlmock.NewResult(3, 1))

	// Phase 4: only new tag gets inserted (existing tag with ID=200 is skipped)
	s.mock.ExpectExec(regexp.QuoteMeta(assocTagInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_Mixed_BelongsToAndHasMany() {
	repo := mustNewRepo[int64, assocPostWithAll](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: insert BelongsTo author first
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Grace").
		WillReturnResult(sqlmock.NewResult(4, 1))

	// Phase 2: insert parent post (author_id=4)
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostWithAllInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(4), "Grace's post").
		WillReturnResult(sqlmock.NewResult(400, 1))

	// Phase 3: insert comments with post_id=400
	s.mock.ExpectExec(regexp.QuoteMeta(assocCommentInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(400), "Great post!").
		WillReturnResult(sqlmock.NewResult(1000, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(assocCommentInsertSQL)).
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

func (s *RepositoryAssociationCreateTestSuite) TestCreate_HasMany_RollbackOnAssociationError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 2: parent insert succeeds
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Henry").
		WillReturnResult(sqlmock.NewResult(5, 1))

	// Phase 3: association insert fails
	s.mock.ExpectExec(regexp.QuoteMeta(assocPostInsertSQL)).
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
}

func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_RollbackOnRelatedInsertError() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)

	s.mock.ExpectBegin()

	// Phase 1: author insert fails
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
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

func TestRepositoryTxAssociationTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTxAssociationTestSuite))
}

func (s *RepositoryTxAssociationTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryTxAssociationTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func (s *RepositoryTxAssociationTestSuite) TestCreate_HasMany_UsesExistingTransaction() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	// The caller manages the transaction; RepositoryTx uses it directly.
	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Judy").
		WillReturnResult(sqlmock.NewResult(6, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(assocPostInsertSQL)).
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

func (s *RepositoryTxAssociationTestSuite) TestCreate_HasMany_NoAssociations_NoExtraQueries() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	s.mock.ExpectBegin()

	// Only the author INSERT — no association INSERTs.
	s.mock.ExpectExec(regexp.QuoteMeta(assocAuthorInsertSQL)).
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
