package sqlr_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

func omitDeleteAllAssociations(qb *sqlr.QueryBuilderDelete) {
	qb.OmitAllAssociations()
}

func syncDeletePostsAssociation(qb *sqlr.QueryBuilderDelete) {
	qb.SyncAssociation("Posts")
}

func omitDeletePostsAssociation(qb *sqlr.QueryBuilderDelete) {
	qb.OmitAssociation("Posts")
}

// RepositoryAssociationDeleteTestSuite tests association-aware Delete behavior.
type RepositoryAssociationDeleteTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

// TestRepositoryAssociationDeleteTestSuite runs the repository association delete test suite.
func TestRepositoryAssociationDeleteTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryAssociationDeleteTestSuite))
}

func (s *RepositoryAssociationDeleteTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryAssociationDeleteTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// TestDelete_CascadesOwnedRelationsByDefault verifies that Delete removes owned
// HasMany children before deleting the root row.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_CascadesOwnedRelationsByDefault() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), now, now, int64(1), "Post A").
			AddRow(int64(11), now, now, int64(1), "Post B"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(int64(20), now, now, int64(1), "bio"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_profiles` WHERE `id` = ?")).
		WithArgs(int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_authors` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 1)

	s.Require().NoError(err)
}

// TestDelete_SyncAssociation_OnlyDeletesSelectedRelation verifies that Delete
// limits cascade cleanup to the explicitly selected relation path.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_SyncAssociation_OnlyDeletesSelectedRelation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), now, now, int64(1), "Post A"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_authors` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 1, syncDeletePostsAssociation)

	s.Require().NoError(err)
}

// TestDelete_SyncDeleteTag_OnlyDeletesTaggedRelation verifies that sync:delete
// tags narrow Delete's default cascade cleanup to tagged branches.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_SyncDeleteTag_OnlyDeletesTaggedRelation() {
	repo := mustNewRepo[int64, assocAuthorSyncDeleteDefaults](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_author_sync_delete_defaults` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), now, now, int64(1), "Post A"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_author_sync_delete_defaults` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 1)

	s.Require().NoError(err)
}

// TestDelete_OmitAssociation_SkipsOmittedRelation verifies that Delete skips
// omitted relations while still cascading the remaining owned relations.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_OmitAssociation_SkipsOmittedRelation() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_profiles` WHERE `assoc_profiles`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(int64(20), now, now, int64(1), "bio"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_profiles` WHERE `id` = ?")).
		WithArgs(int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_authors` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 1, omitDeletePostsAssociation)

	s.Require().NoError(err)
}

// TestDelete_OmitAllAssociations_DeletesRootOnly verifies that Delete can skip
// owned-association cleanup entirely for a single call.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_OmitAllAssociations_DeletesRootOnly() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_authors` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), 1, omitDeleteAllAssociations)

	s.Require().NoError(err)
}

// TestDelete_ManyToManyDeletesLinksOnlyByDefault verifies that Delete removes
// join-table rows but leaves related many-to-many target rows intact.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_ManyToManyDeletesLinksOnlyByDefault() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_articles` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(int64(2), now, now, "Go Tips"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ?")).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_articles` WHERE `id` = ?")).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 2)

	s.Require().NoError(err)
}

// TestDelete_BelongsToIsPreservedByDefault verifies that Delete does not delete
// belongs-to target rows during cascade cleanup.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_BelongsToIsPreservedByDefault() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_post_with_authors` WHERE `id` = ?")).
		WithArgs(int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), 3)

	s.Require().NoError(err)
}

// TestDelete_NestedCascadeByDefault verifies that Delete cascades nested owned
// relations by default.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_NestedCascadeByDefault() {
	repo := mustNewRepo[int64, deepAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `deep_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `deep_posts` WHERE `deep_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), now, now, int64(1), "Post"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `deep_comments` WHERE `deep_comments`.`post_id` = ?")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "post_id", "body"}).
			AddRow(int64(100), now, now, int64(10), "Comment"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `deep_comments` WHERE `id` = ?")).
		WithArgs(int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `deep_posts` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `deep_authors` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 1)

	s.Require().NoError(err)
}

// TestDelete_NestedSyncDeleteTag_DeletesSelectedBranch verifies that a nested
// sync:delete tag includes the ancestor branch while skipping unrelated paths.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_NestedSyncDeleteTag_DeletesSelectedBranch() {
	repo := mustNewRepo[int64, deepAuthorNestedSyncDeleteDefaults](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `deep_author_nested_sync_delete_defaults` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `deep_post_nested_sync_delete_defaults` WHERE `deep_post_nested_sync_delete_defaults`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), now, now, int64(1), "Post"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `deep_comment_nested_sync_delete_defaults` WHERE `deep_comment_nested_sync_delete_defaults`.`post_id` = ?")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "post_id", "body"}).
			AddRow(int64(100), now, now, int64(10), "Comment"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `deep_comment_nested_sync_delete_defaults` WHERE `id` = ?")).
		WithArgs(int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `deep_post_nested_sync_delete_defaults` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `deep_author_nested_sync_delete_defaults` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := repo.Delete(context.Background(), 1)

	s.Require().NoError(err)
}

// TestDelete_MissingParentReturnsNotFound verifies that association-aware Delete
// returns ErrNotFound when the root entity is missing.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_MissingParentReturnsNotFound() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(999), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}))
	s.mock.ExpectRollback()

	err := repo.Delete(context.Background(), 999)

	s.Require().Error(err)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity assoc_authors id=999")
}

// TestDelete_SyncAssociation_InvalidPathReturnsError verifies that Delete
// rejects invalid explicit association sync paths.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_SyncAssociation_InvalidPathReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	err := repo.Delete(context.Background(), 1, func(qb *sqlr.QueryBuilderDelete) {
		qb.SyncAssociation("Unknown")
	})

	s.Require().Error(err)
	s.ErrorContains(err, "invalid sync association path \"Unknown\"")
}

// TestDelete_DeleteChildErrorRollsBack verifies that Delete rolls back the
// transaction when a cascaded child deletion fails.
func (s *RepositoryAssociationDeleteTestSuite) TestDelete_DeleteChildErrorRollsBack() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), now, now, int64(1), "Post A"))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(10)).
		WillReturnError(fmt.Errorf("delete child failed"))
	s.mock.ExpectRollback()

	err := repo.Delete(context.Background(), 1)

	s.Require().Error(err)
	s.Contains(err.Error(), "failed to cascade delete relation \"Posts\"")
}
