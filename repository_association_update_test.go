package sqlr_test

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

// RepositoryAssociationUpdateTestSuite tests association-aware graph synchronization on Update.
type RepositoryAssociationUpdateTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

// TestRepositoryAssociationUpdateTestSuite runs the repository association update test suite.
func TestRepositoryAssociationUpdateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryAssociationUpdateTestSuite))
}

func (s *RepositoryAssociationUpdateTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryAssociationUpdateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func syncAllAssociations(qb *sqlr.QueryBuilderUpdate) {
	qb.SyncAllAssociations()
}

func syncPostsAssociation(qb *sqlr.QueryBuilderUpdate) {
	qb.SyncAssociation("Posts")
}

func syncAllAssociationsOmitPosts(qb *sqlr.QueryBuilderUpdate) {
	qb.SyncAllAssociations().OmitAssociation("Posts")
}

func syncAllAssociationsDisableAutoUpdates(qb *sqlr.QueryBuilderUpdate) {
	qb.SyncAllAssociations().DisableAutoUpdates()
}

func syncAllAssociationsWithMany2many(qb *sqlr.QueryBuilderUpdate) {
	qb.SyncAllAssociations().SyncMany2many("Tags")
}

// TestUpdate_Default_DoesNotSynchronizePopulatedAssociations verifies that Update does not synchronize populated associations by default.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_Default_DoesNotSynchronizePopulatedAssociations() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{
			{Title: "In Memory Only"},
		},
	}

	result, err := repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 1)
	s.Equal(int64(0), result.Posts[0].AuthorID)
	s.Equal(int64(0), result.Posts[0].GetId())
}

// TestUpdate_NilEntity_WithAssociationOptionsReturnsError verifies that Update returns an error for nil entities even when association sync is configured.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_NilEntity_WithAssociationOptionsReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	result, err := repo.Update(context.Background(), nil, syncAllAssociations)

	s.Require().Error(err)
	s.Require().ErrorIs(err, sqlr.ErrNilEntity)
	s.Nil(result)
}

// TestUpdate_HasMany_NilSlice_UntouchedNoTransaction verifies that Update leaves nil has-many slices untouched without starting a transaction.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasMany_NilSlice_UntouchedNoTransaction() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Nil(result.Posts)
}

// TestUpdate_BelongsTo_UpdatesRelatedAndParentFK verifies that Update syncs belongs-to entities and the parent foreign key together.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_BelongsTo_UpdatesRelatedAndParentFK() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorNow, "Alice Updated", isTimestamp{}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_post_with_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(7), now, "Updated Post", isTimestamp{}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocPostWithAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Updated Post",
		Author: assocAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorNow,
				UpdatedAt: authorNow,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(7), result.AuthorID)
	s.Equal("Alice Updated", result.Author.Name)
}

// TestUpdate_BelongsToPointer_UpdatesRelatedAndParentFK verifies that Update syncs pointer belongs-to entities and the parent foreign key together.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_BelongsToPointer_UpdatesRelatedAndParentFK() {
	repo := mustNewRepo[int64, assocPostWithPointerAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorNow, "Alice Updated", isTimestamp{}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_post_with_pointer_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(7), now, "Updated Post", isTimestamp{}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	entity := assocPostWithPointerAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Updated Post",
		Author: &assocAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorNow,
				UpdatedAt: authorNow,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().NotNil(result.Author)
	s.Equal(int64(7), result.AuthorID)
	s.Equal("Alice Updated", result.Author.Name)
}

// TestUpdate_DisableAutoUpdates_UsesPresetValuesAcrossGraph verifies that Update uses preset values across graph when auto-updates are disabled.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_DisableAutoUpdates_UsesPresetValuesAcrossGraph() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	postCreatedAt := time.Now().Add(-4 * time.Hour)
	postUpdatedAt := time.Now().Add(-3 * time.Hour)
	authorCreatedAt := time.Now().Add(-2 * time.Hour)
	authorUpdatedAt := time.Now().Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorCreatedAt, "Alice Updated", authorUpdatedAt, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_post_with_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(7), postCreatedAt, "Updated Post", postUpdatedAt, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	entity := assocPostWithAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: postCreatedAt,
			UpdatedAt: postUpdatedAt,
		},
		Title: "Updated Post",
		Author: assocAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorCreatedAt,
				UpdatedAt: authorUpdatedAt,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociationsDisableAutoUpdates)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(postUpdatedAt, result.UpdatedAt)
	s.Equal(authorUpdatedAt, result.Author.UpdatedAt)
	s.Equal(int64(7), result.AuthorID)
}

// TestUpdate_DisableAutoUpdates_FKMismatchReturnsError verifies that Update returns an error for foreign key mismatch when auto-updates are disabled.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_DisableAutoUpdates_FKMismatchReturnsError() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	authorCreatedAt := time.Now().Add(-2 * time.Hour)
	authorUpdatedAt := time.Now().Add(-90 * time.Minute)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorCreatedAt, "Alice Updated", authorUpdatedAt, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))
	s.mock.ExpectRollback()

	entity := assocPostWithAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: time.Now().Add(-time.Hour),
			UpdatedAt: time.Now().Add(-30 * time.Minute),
		},
		AuthorID: 99,
		Title:    "Mismatch",
		Author: assocAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorCreatedAt,
				UpdatedAt: authorUpdatedAt,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociationsDisableAutoUpdates)

	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "field assoc_post_with_authors.author_id value 99 does not match associated primary key 7")
}

// TestUpdate_HasMany_SynchronizesAndDeletesMissingChildren verifies that Update synchronizes and deletes missing children for has-many relations.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasMany_SynchronizesAndDeletesMissingChildren() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post").
			AddRow(int64(11), postNow, postNow, int64(1), "Delete Me"))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_posts` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(1), postNow, "Updated Post", isTimestamp{}, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Brand New").
		WillReturnResult(sqlmock.NewResult(12, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{
			{
				Entity: sqlr.Entity[int64]{
					Id:        10,
					CreatedAt: postNow,
					UpdatedAt: postNow,
				},
				Title: "Updated Post",
			},
			{Title: "Brand New"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 2)
	s.Equal(int64(1), result.Posts[0].AuthorID)
	s.Equal(int64(10), result.Posts[0].GetId())
	s.Equal(int64(1), result.Posts[1].AuthorID)
	s.Equal(int64(12), result.Posts[1].GetId())
}

// TestUpdate_AssociationSync_AutoPreloadRehydratesNewAssociations verifies that
// Update reloads the entity graph when association sync is active and the root
// schema defines auto-preloads, so newly added associations are returned fully
// hydrated.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_AssociationSync_AutoPreloadRehydratesNewAssociations() {
	repo := mustNewRepo[int64, assocAuthorAutoPreload](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)
	commentNow := now.Add(-30 * time.Minute)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_auto_preloads` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_comments_auto_preloads` WHERE `assoc_post_with_comments_auto_preloads`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_post_with_comments_auto_preloads` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Brand New").
		WillReturnResult(sqlmock.NewResult(12, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `assoc_author_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice Updated"))

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
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:   "Alice Updated",
		Parent: 8,
		Posts: []assocPostWithCommentsAutoPreload{{
			Title:    "Brand New",
			CacheKey: "brand-new",
		}},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().Same(&entity, result)
	s.Equal(uint(8), result.Parent)
	s.Require().Len(result.Posts, 1)
	s.Equal(int64(12), result.Posts[0].GetId())
	s.Equal(int64(1), result.Posts[0].AuthorID)
	s.Equal("brand-new", result.Posts[0].CacheKey)
	s.Require().Len(result.Posts[0].Comments, 1)
	s.Equal("Hydrated Comment", result.Posts[0].Comments[0].Body)
}

// TestUpdate_HasOne_ZeroValueClearsExistingChild verifies that syncing a cleared
// value-form HasOne relation deletes the existing related row instead of silently
// leaving it linked.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasOne_ZeroValueClearsExistingChild() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	profileNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(int64(10), profileNow, profileNow, int64(1), "Old Profile"))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_profiles` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:    "Alice Updated",
		Profile: assocProfile{},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(0), result.Profile.GetId())
	s.Equal(int64(0), result.Profile.AuthorID)
	s.Equal("", result.Profile.Bio)
}

// TestUpdate_HasOne_NilPointerClearsExistingChild verifies that syncing a cleared
// pointer-form HasOne relation deletes the existing related row instead of
// silently leaving it linked.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasOne_NilPointerClearsExistingChild() {
	repo := mustNewRepo[int64, assocAuthorWithPointerProfile](s.T(), s.client)
	now := time.Now()
	profileNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_with_pointer_profiles` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(int64(10), profileNow, profileNow, int64(1), "Old Profile"))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_profiles` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocAuthorWithPointerProfile{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:    "Alice Updated",
		Profile: nil,
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Nil(result.Profile)
}

// TestUpdate_ManyToMany_DefaultSync_SynchronizesLinksWithoutUpdatingExistingRows verifies that Update synchronizes links without updating existing rows for many-to-many default sync.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToMany_DefaultSync_SynchronizesLinksWithoutUpdatingExistingRows() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()
	tagNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_articles` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id` FROM `assoc_tags` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(100), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(int64(100)))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "new-tag").
		WillReturnResult(sqlmock.NewResult(102, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_tags` WHERE `assoc_article_tags`.`assoc_article_id` = ?")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_id", "assoc_tag_id"}).
			AddRow(int64(2), int64(100)).
			AddRow(int64(2), int64(101)))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_tags` (`assoc_article_id`, `assoc_tag_id`) VALUES (?, ?)")).
		WithArgs(int64(2), int64(102)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags: []assocTag{
			{
				Entity: sqlr.Entity[int64]{
					Id:        100,
					CreatedAt: tagNow,
					UpdatedAt: tagNow,
				},
				Name: "golang-updated",
			},
			{Name: "new-tag"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Tags, 2)
	s.Equal(int64(100), result.Tags[0].GetId())
	s.Equal(int64(102), result.Tags[1].GetId())
	s.Equal(tagNow, result.Tags[0].UpdatedAt)
}

// TestUpdate_ManyToMany_FullEntitySync_UpdatesExistingRowsWhenOptedIn verifies that Update updates existing rows when opted in for many-to-many full entity sync.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToMany_FullEntitySync_UpdatesExistingRowsWhenOptedIn() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()
	tagNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_articles` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_tags` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(tagNow, "golang-updated", isTimestamp{}, int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "new-tag").
		WillReturnResult(sqlmock.NewResult(102, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_tags` WHERE `assoc_article_tags`.`assoc_article_id` = ?")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_id", "assoc_tag_id"}).
			AddRow(int64(2), int64(100)).
			AddRow(int64(2), int64(101)))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_tags` (`assoc_article_id`, `assoc_tag_id`) VALUES (?, ?)")).
		WithArgs(int64(2), int64(102)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags: []assocTag{
			{
				Entity: sqlr.Entity[int64]{
					Id:        100,
					CreatedAt: tagNow,
					UpdatedAt: tagNow,
				},
				Name: "golang-updated",
			},
			{Name: "new-tag"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociationsWithMany2many)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Tags, 2)
	s.Equal(int64(100), result.Tags[0].GetId())
	s.Equal(int64(102), result.Tags[1].GetId())
}

// TestUpdate_ManyToMany_DefaultSync_MissingRelatedEntityReturnsNotFound verifies that Update returns ErrNotFound for missing related entity for many-to-many default sync.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToMany_DefaultSync_MissingRelatedEntityReturnsNotFound() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()
	tagNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_articles` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id` FROM `assoc_tags` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(100), 1).
		WillReturnError(sql.ErrNoRows)

	s.mock.ExpectRollback()

	entity := assocArticle{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags: []assocTag{{
			Entity: sqlr.Entity[int64]{
				Id:        100,
				CreatedAt: tagNow,
				UpdatedAt: tagNow,
			},
			Name: "ignored",
		}},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}

// TestUpdate_ManyToMany_EmptySlice_RemovesAllLinks verifies that Update removes all links for many-to-many empty slice.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToMany_EmptySlice_RemovesAllLinks() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_articles` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Clear Tags", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_tags` WHERE `assoc_article_tags`.`assoc_article_id` = ?")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_id", "assoc_tag_id"}).
			AddRow(int64(2), int64(100)).
			AddRow(int64(2), int64(101)))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Clear Tags",
		Tags:  []assocTag{},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Empty(result.Tags)
}

// TestUpdate_HasManyPointerElements_RejectsNilElement verifies that Update rejects nil element for pointer-based has-many collections.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasManyPointerElements_RejectsNilElement() {
	repo := mustNewRepo[int64, assocAuthorWithPointerPosts](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_with_pointer_posts` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post"))
	s.mock.ExpectRollback()

	entity := assocAuthorWithPointerPosts{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice Updated",
		Posts: []*assocPost{nil},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)
	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "HasMany relation \"Posts\"[0] is nil")
}

// TestUpdate_ManyToManyPointerElements_RejectsNilElement verifies that Update rejects nil element for pointer-based many-to-many collections.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToManyPointerElements_RejectsNilElement() {
	repo := mustNewRepo[int64, assocArticleWithPointerTags](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_article_with_pointer_tags` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectRollback()

	entity := assocArticleWithPointerTags{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags:  []*assocTag{nil},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)
	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "ManyToMany relation \"Tags\"[0] is nil")
	s.Equal(now, entity.UpdatedAt)
}

// TestUpdate_HasMany_RollbackRestoresParentAndChildState verifies that Update rollback restores parent and child state for has-many relations.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasMany_RollbackRestoresParentAndChildState() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_posts` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(1), postNow, "Updated Post", isTimestamp{}, int64(10)).
		WillReturnError(errors.New("child update failed"))
	s.mock.ExpectRollback()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{{
			Entity: sqlr.Entity[int64]{
				Id:        10,
				CreatedAt: postNow,
				UpdatedAt: postNow,
			},
			Title: "Updated Post",
		}},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "child update failed")
	s.Equal(now, entity.UpdatedAt)
	s.Require().Len(entity.Posts, 1)
	s.Equal(postNow, entity.Posts[0].UpdatedAt)
	s.Equal(int64(0), entity.Posts[0].AuthorID)
}

// TestUpdate_SyncAllAssociations_BelongsToWithNullableFK verifies that Update syncs belongs-to relations with nullable foreign keys when all associations are enabled.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAllAssociations_BelongsToWithNullableFK() {
	repo := mustNewRepo[int64, testPostWithNullableAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorNow, "Alice Updated", isTimestamp{}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_post_with_nullable_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(sqlmock.AnyArg(), now, "Updated Post", isTimestamp{}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := testPostWithNullableAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Updated Post",
		Author: testAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorNow,
				UpdatedAt: authorNow,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().NotNil(result.AuthorID)
	s.Equal(int64(7), *result.AuthorID)
	s.Equal("Alice Updated", result.Author.Name)
}

// TestUpdate_SyncAssociation_OnlySynchronizesSelectedRelation verifies that Update only synchronizes selected relation for explicit association sync.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAssociation_OnlySynchronizesSelectedRelation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_posts` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(1), postNow, "Updated Post", isTimestamp{}, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{{
			Entity: sqlr.Entity[int64]{
				Id:        10,
				CreatedAt: postNow,
				UpdatedAt: postNow,
			},
			Title: "Updated Post",
		}},
		Profile: assocProfile{Bio: "ignore me"},
	}

	result, err := repo.Update(context.Background(), &entity, syncPostsAssociation)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 1)
	s.Equal(int64(10), result.Posts[0].GetId())
	s.Equal(int64(1), result.Posts[0].AuthorID)
	s.Equal(int64(0), result.Profile.GetId())
	s.Equal(int64(0), result.Profile.AuthorID)
}

// TestUpdate_SyncUpdateTag_OnlySynchronizesTaggedRelation verifies that
// sync:update tags enable association sync by default for tagged paths.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncUpdateTag_OnlySynchronizesTaggedRelation() {
	repo := mustNewRepo[int64, assocAuthorSyncUpdateDefaults](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_sync_update_defaults` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_posts` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(1), postNow, "Updated Post", isTimestamp{}, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	entity := assocAuthorSyncUpdateDefaults{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{{
			Entity: sqlr.Entity[int64]{
				Id:        10,
				CreatedAt: postNow,
				UpdatedAt: postNow,
			},
			Title: "Updated Post",
		}},
		Profile: assocProfile{Bio: "ignore me"},
	}

	result, err := repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 1)
	s.Equal(int64(10), result.Posts[0].GetId())
	s.Equal(int64(1), result.Posts[0].AuthorID)
	s.Equal(int64(0), result.Profile.GetId())
	s.Equal(int64(0), result.Profile.AuthorID)
}

// TestUpdate_SyncUpdateTag_ManyToManyDefaultsFullEntitySync verifies that
// sync:update and syncMode:many2many tags activate full many-to-many entity
// synchronization without per-call query-builder options.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncUpdateTag_ManyToManyDefaultsFullEntitySync() {
	repo := mustNewRepo[int64, assocArticleSyncUpdateDefaults](s.T(), s.client)
	now := time.Now()
	tagNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_article_sync_update_defaults` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_tags` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(tagNow, "golang-updated", isTimestamp{}, int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "new-tag").
		WillReturnResult(sqlmock.NewResult(102, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_sync_update_default_tags` WHERE `assoc_article_sync_update_default_tags`.`assoc_article_sync_update_defaults_id` = ?")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_sync_update_defaults_id", "assoc_tag_id"}).
			AddRow(int64(2), int64(100)).
			AddRow(int64(2), int64(101)))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_sync_update_default_tags` WHERE `assoc_article_sync_update_defaults_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_sync_update_default_tags` (`assoc_article_sync_update_defaults_id`, `assoc_tag_id`) VALUES (?, ?)")).
		WithArgs(int64(2), int64(102)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	entity := assocArticleSyncUpdateDefaults{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags: []assocTag{
			{
				Entity: sqlr.Entity[int64]{
					Id:        100,
					CreatedAt: tagNow,
					UpdatedAt: tagNow,
				},
				Name: "golang-updated",
			},
			{Name: "new-tag"},
		},
	}

	result, err := repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Tags, 2)
	s.Equal(int64(100), result.Tags[0].GetId())
	s.Equal(int64(102), result.Tags[1].GetId())
}

// TestUpdate_SyncAllAssociations_OmitAssociation_SkipsOmittedRelation verifies that Update skips omitted relations even when syncing all associations.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAllAssociations_OmitAssociation_SkipsOmittedRelation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))
	s.mock.ExpectCommit()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice Updated",
		Posts: []assocPost{{Title: "Ignored"}},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociationsOmitPosts)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 1)
	s.Equal(int64(0), result.Posts[0].GetId())
	s.Equal(int64(0), result.Posts[0].AuthorID)
}

// TestUpdate_SyncAssociation_InvalidPathReturnsError verifies that Update returns an error for invalid path for explicit association sync.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAssociation_InvalidPathReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(context.Background(), &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.SyncAssociation("Unknown")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "invalid sync association path \"Unknown\"")
}

// TestUpdate_AssociationSync_ExplicitPreloadTakesPrecedence verifies that when
// Update requests an explicit preload for a relation that is also auto-preloaded,
// the explicit preload conditions are used during the post-update reload.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_AssociationSync_ExplicitPreloadTakesPrecedence() {
	repo := mustNewRepo[int64, assocAuthorAutoPreload](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_auto_preloads` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_comments_auto_preloads` WHERE `assoc_post_with_comments_auto_preloads`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_post_with_comments_auto_preloads` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Brand New").
		WillReturnResult(sqlmock.NewResult(12, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_author_auto_preloads` WHERE `assoc_author_auto_preloads`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice Updated"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_comments_auto_preloads` WHERE `assoc_post_with_comments_auto_preloads`.`author_id` IN (?) AND title = ?")).
		WithArgs(int64(1), "Brand New").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(12), postNow, postNow, int64(1), "Brand New"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_comments` WHERE `assoc_comments`.`post_id` IN (?)")).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "post_id", "body"}))

	s.mock.ExpectCommit()

	entity := assocAuthorAutoPreload{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPostWithCommentsAutoPreload{{
			Title: "Brand New",
		}},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations, func(qb *sqlr.QueryBuilderUpdate) {
		qb.Preload("Posts", sqlr.Condition("title = ?", "Brand New"))
	})

	s.Require().NoError(err)
	s.Require().Same(&entity, result)
	s.Require().Len(result.Posts, 1)
	s.Equal("Brand New", result.Posts[0].Title)
}

// TestUpdate_SyncMany2many_InvalidPathReturnsError verifies that Update rejects invalid many2many sync paths.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncMany2many_InvalidPathReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(context.Background(), &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.SyncMany2many("Unknown")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "invalid many2many sync association path \"Unknown\"")
}

// TestUpdate_SyncMany2many_NonManyToManyPathReturnsError verifies that Update rejects many2many sync paths for non-many-to-many relations.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncMany2many_NonManyToManyPathReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(context.Background(), &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.SyncMany2many("Posts")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "invalid many2many sync association path \"Posts\": relation is not many-to-many")
}

// TestUpdate_SyncAllAssociations_MissingParentReturnsNotFound verifies that Update returns ErrNotFound for missing parent with sync-all-associations enabled.
func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAllAssociations_MissingParentReturnsNotFound() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Missing", isTimestamp{}, int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	s.mock.ExpectRollback()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        99,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Missing",
		Posts: []assocPost{
			{Title: "Should Not Be Saved"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAllAssociations)

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}
