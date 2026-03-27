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

func omitCreateAllAssociations(qb *sqlr.QueryBuilderCreate) {
	qb.OmitAllAssociations()
}

func disableCreateAutoUpdates(qb *sqlr.QueryBuilderCreate) {
	qb.DisableAutoUpdates()
}

func preloadCreateAuthor(qb *sqlr.QueryBuilderCreate) {
	qb.Preload("Author")
}

func preloadCreateTags(qb *sqlr.QueryBuilderCreate) {
	qb.Preload("Tags")
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

// TestCreate_OmitAllAssociations_InsertsRootOnly verifies that Create can skip
// association persistence entirely for a single call.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_OmitAllAssociations_InsertsRootOnly() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob").
		WillReturnResult(sqlmock.NewResult(10, 1))

	entity := assocAuthor{
		Name:    "Bob",
		Posts:   []assocPost{{Title: "Skipped"}},
		Profile: assocProfile{Bio: "Skipped"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, omitCreateAllAssociations))
	s.Equal(int64(10), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(0), entity.Posts[0].GetId())
	s.Equal(int64(0), entity.Posts[0].AuthorID)
	s.Equal(int64(0), entity.Profile.GetId())
	s.Equal(int64(0), entity.Profile.AuthorID)
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

// TestCreate_AssociationSync_AutoPreloadRehydratesNewAssociations verifies that
// Create reloads auto-preloaded relations after association persistence.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_AssociationSync_AutoPreloadRehydratesNewAssociations() {
	repo := mustNewRepo[int64, assocAuthorAutoPreload](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)
	commentNow := now.Add(-30 * time.Minute)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_author_auto_preloads` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_post_with_comments_auto_preloads` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Brand New").
		WillReturnResult(sqlmock.NewResult(12, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `assoc_author_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_comments_auto_preloads` WHERE `assoc_post_with_comments_auto_preloads`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(12), postNow, postNow, int64(1), "Brand New"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_comments` WHERE `assoc_comments`.`post_id` IN (?)")).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "post_id", "body"}).
			AddRow(int64(100), commentNow, commentNow, int64(12), "Hydrated Comment"))
	s.mock.ExpectCommit()

	entity := assocAuthorAutoPreload{
		Name:   "Alice",
		Parent: 5,
		Posts: []assocPostWithCommentsAutoPreload{{
			Title:    "Brand New",
			CacheKey: "brand-new",
		}},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity))
	s.Equal(int64(1), entity.GetId())
	s.Equal(uint(5), entity.Parent)
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(12), entity.Posts[0].GetId())
	s.Equal("brand-new", entity.Posts[0].CacheKey)
	s.Require().Len(entity.Posts[0].Comments, 1)
	s.Equal("Hydrated Comment", entity.Posts[0].Comments[0].Body)
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

// TestCreate_OmitAllAssociations_OverridesSyncAssociation verifies that
// OmitAllAssociations disables Create association persistence even when
// SyncAssociation selects a relation explicitly.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_OmitAllAssociations_OverridesSyncAssociation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity := assocAuthor{
		Name:    "Alice",
		Posts:   []assocPost{{Title: "Post A"}},
		Profile: assocProfile{Bio: "Skipped"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, syncCreatePosts, omitCreateAllAssociations))
	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(0), entity.Posts[0].GetId())
	s.Equal(int64(0), entity.Posts[0].AuthorID)
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

// TestCreate_OmitAllAssociations_OverridesSyncCreateDefaults verifies that
// OmitAllAssociations disables schema-level sync:create defaults for one call.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_OmitAllAssociations_OverridesSyncCreateDefaults() {
	repo := mustNewRepo[int64, assocAuthorSyncCreateDefaults](s.T(), s.client)

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_author_sync_create_defaults` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity := assocAuthorSyncCreateDefaults{
		Name:    "Alice",
		Posts:   []assocPost{{Title: "Post A"}},
		Profile: assocProfile{Bio: "Skipped"},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, omitCreateAllAssociations))
	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal(int64(0), entity.Posts[0].GetId())
	s.Equal(int64(0), entity.Posts[0].AuthorID)
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

// TestCreate_BelongsTo_ExplicitPreloadRehydratesExistingRelated verifies that
// Create can hydrate an existing belongs-to relation provided with only a
// primary key when an explicit preload is requested.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_BelongsTo_ExplicitPreloadRehydratesExistingRelated() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_post_with_authors` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(7), "Frank's post").
		WillReturnResult(sqlmock.NewResult(40, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_authors` WHERE `assoc_post_with_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(40), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(40), now, now, int64(7), "Frank's post"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `assoc_authors`.`id` IN (?)")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(7), authorNow, authorNow, "Frank"))
	s.mock.ExpectCommit()

	entity := assocPostWithAuthor{
		Title:  "Frank's post",
		Author: assocAuthor{Entity: sqlr.Entity[int64]{Id: 7}},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, preloadCreateAuthor))
	s.Equal(int64(40), entity.GetId())
	s.Equal(int64(7), entity.AuthorID)
	s.Equal(int64(7), entity.Author.GetId())
	s.Equal("Frank", entity.Author.Name)
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

// TestCreate_ManyToMany_ExplicitPreloadRehydratesExistingRelated verifies that
// Create can hydrate existing many-to-many targets provided with primary keys
// only when an explicit preload is requested.
func (s *RepositoryAssociationCreateTestSuite) TestCreate_ManyToMany_ExplicitPreloadRehydratesExistingRelated() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()
	tagNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_articles` (`created_at`, `updated_at`, `title`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Existing Tags").
		WillReturnResult(sqlmock.NewResult(3, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_tags` (`assoc_article_id`, `assoc_tag_id`) VALUES (?, ?), (?, ?)")).
		WithArgs(int64(3), int64(200), int64(3), int64(201)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_articles` WHERE `assoc_articles`.`id` = ? LIMIT ?")).
		WithArgs(int64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(int64(3), now, now, "Existing Tags"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_tags` WHERE `assoc_article_tags`.`assoc_article_id` IN (?)")).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_id", "assoc_tag_id"}).
			AddRow(int64(3), int64(200)).
			AddRow(int64(3), int64(201)))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_tags` WHERE `assoc_tags`.`id` IN (?, ?)")).
		WithArgs(int64(200), int64(201)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(200), tagNow, tagNow, "existing-tag").
			AddRow(int64(201), now, now, "new-tag"))
	s.mock.ExpectCommit()

	entity := assocArticle{
		Title: "Existing Tags",
		Tags: []assocTag{
			{Entity: sqlr.Entity[int64]{Id: 200}},
			{Entity: sqlr.Entity[int64]{Id: 201}},
		},
	}

	s.Require().NoError(repo.Create(context.Background(), &entity, preloadCreateTags))
	s.Equal(int64(3), entity.GetId())
	s.Require().Len(entity.Tags, 2)
	s.Equal("existing-tag", entity.Tags[0].Name)
	s.Equal("new-tag", entity.Tags[1].Name)
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
