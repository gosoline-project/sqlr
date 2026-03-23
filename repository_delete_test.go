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

// RepositoryDeleteTestSuite tests the Repository Delete operations using sqlmock.
type RepositoryDeleteTestSuite struct {
	suite.Suite
	client        sqlc.Client
	mock          sqlmock.Sqlmock
	repo          sqlr.Repository[int64, testUser]
	customPkRepo  sqlr.Repository[int64, testCustomPkUser]
	stringKeyRepo sqlr.Repository[string, testStringKeyUser]
	boolKeyRepo   sqlr.Repository[bool, testBoolKeyUser]
	floatKeyRepo  sqlr.Repository[float64, testFloatKeyUser]
}

// TestRepositoryDeleteTestSuite runs the repository delete test suite.
func TestRepositoryDeleteTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryDeleteTestSuite))
}

func (s *RepositoryDeleteTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.customPkRepo = mustNewRepo[int64, testCustomPkUser](s.T(), s.client)
	s.stringKeyRepo = mustNewRepo[string, testStringKeyUser](s.T(), s.client)
	s.boolKeyRepo = mustNewRepo[bool, testBoolKeyUser](s.T(), s.client)
	s.floatKeyRepo = mustNewRepo[float64, testFloatKeyUser](s.T(), s.client)
}

func (s *RepositoryDeleteTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Success Cases
// ==========================================================================

// TestDelete_Success verifies that Delete succeeds for the basic case.
func (s *RepositoryDeleteTestSuite) TestDelete_Success() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.Delete(context.Background(), 1)

	s.Require().NoError(err)
}

// TestDelete_CascadesOwnedRelationsByDefault verifies that Delete removes owned
// HasMany children before deleting the root row.
func (s *RepositoryDeleteTestSuite) TestDelete_CascadesOwnedRelationsByDefault() {
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
func (s *RepositoryDeleteTestSuite) TestDelete_SyncAssociation_OnlyDeletesSelectedRelation() {
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

// TestDelete_OmitAssociation_SkipsOmittedRelation verifies that Delete skips
// omitted relations while still cascading the remaining owned relations.
func (s *RepositoryDeleteTestSuite) TestDelete_OmitAssociation_SkipsOmittedRelation() {
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
func (s *RepositoryDeleteTestSuite) TestDelete_OmitAllAssociations_DeletesRootOnly() {
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
func (s *RepositoryDeleteTestSuite) TestDelete_ManyToManyDeletesLinksOnlyByDefault() {
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
func (s *RepositoryDeleteTestSuite) TestDelete_BelongsToIsPreservedByDefault() {
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
func (s *RepositoryDeleteTestSuite) TestDelete_NestedCascadeByDefault() {
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

// ==========================================================================
// Error/NotFound Cases
// ==========================================================================

// TestDelete_NotFound verifies that Delete returns ErrNotFound for missing rows.
func (s *RepositoryDeleteTestSuite) TestDelete_NotFound() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE `id` = ?")).
		WithArgs(int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.Delete(context.Background(), 999)

	s.Require().Error(err)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=999")
}

// TestDelete_MissingParentReturnsNotFound verifies that association-aware Delete
// returns ErrNotFound when the root entity is missing.
func (s *RepositoryDeleteTestSuite) TestDelete_MissingParentReturnsNotFound() {
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

// TestDelete_Error verifies that Delete propagates execution errors.
func (s *RepositoryDeleteTestSuite) TestDelete_Error() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE `id` = ?")).
		WithArgs(int64(1)).
		WillReturnError(fmt.Errorf("foreign key constraint"))

	err := s.repo.Delete(context.Background(), 1)

	s.Require().Error(err)
	s.Contains(err.Error(), "failed to delete entity")
}

// TestDelete_SyncAssociation_InvalidPathReturnsError verifies that Delete
// rejects invalid explicit association sync paths.
func (s *RepositoryDeleteTestSuite) TestDelete_SyncAssociation_InvalidPathReturnsError() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)

	err := repo.Delete(context.Background(), 1, func(qb *sqlr.QueryBuilderDelete) {
		qb.SyncAssociation("Unknown")
	})

	s.Require().Error(err)
	s.ErrorContains(err, "invalid sync association path \"Unknown\"")
}

// TestDelete_DeleteChildErrorRollsBack verifies that Delete rolls back the
// transaction when a cascaded child deletion fails.
func (s *RepositoryDeleteTestSuite) TestDelete_DeleteChildErrorRollsBack() {
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

// ==========================================================================
// Key Type Variations
// ==========================================================================

// TestDelete_CustomPrimaryKey verifies that Delete custom primary key.
func (s *RepositoryDeleteTestSuite) TestDelete_CustomPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_custom_pk_users` WHERE `user_id` = ?")).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.customPkRepo.Delete(context.Background(), 42)

	s.Require().NoError(err)
}

// TestDelete_StringPrimaryKey verifies that Delete string primary key.
func (s *RepositoryDeleteTestSuite) TestDelete_StringPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_string_key_users` WHERE `id` = ?")).
		WithArgs("id-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.stringKeyRepo.Delete(context.Background(), "id-1")

	s.Require().NoError(err)
}

// TestDelete_BoolPrimaryKey verifies that Delete bool primary key.
func (s *RepositoryDeleteTestSuite) TestDelete_BoolPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_bool_key_users` WHERE `id` = ?")).
		WithArgs(true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.boolKeyRepo.Delete(context.Background(), true)

	s.Require().NoError(err)
}

// TestDelete_FloatPrimaryKey verifies that Delete float primary key.
func (s *RepositoryDeleteTestSuite) TestDelete_FloatPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_float_key_users` WHERE `id` = ?")).
		WithArgs(float64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.floatKeyRepo.Delete(context.Background(), float64(5))

	s.Require().NoError(err)
}

// ==========================================================================
// Prepared Statement Tests
// ==========================================================================

// RepositoryDeletePreparedTestSuite tests the Repository Delete operations with prepared statements.
type RepositoryDeletePreparedTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
	repo   sqlr.Repository[int64, testUser]
}

// TestRepositoryDeletePreparedTestSuite runs the repository delete prepared test suite.
func TestRepositoryDeletePreparedTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryDeletePreparedTestSuite))
}

func (s *RepositoryDeletePreparedTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	settings := sqlr.Settings{PreparedStatements: true}
	s.repo = mustNewRepoWithSettings[int64, testUser](s.T(), s.client, settings)
}

func (s *RepositoryDeletePreparedTestSuite) TearDownTest() {
	s.Require().NoError(s.repo.Close())
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// TestDelete_PreparedStatement_Success verifies that Delete succeeds for the basic case in prepared-statement mode.
func (s *RepositoryDeletePreparedTestSuite) TestDelete_PreparedStatement_Success() {
	deleteSQL := "DELETE FROM `test_users` WHERE `id` = ?"

	// Expect prepare on first call
	s.mock.ExpectPrepare(regexp.QuoteMeta(deleteSQL))

	s.mock.ExpectExec(regexp.QuoteMeta(deleteSQL)).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.Delete(context.Background(), 1)
	s.Require().NoError(err)

	// Second call should reuse prepared statement (no ExpectPrepare)
	s.mock.ExpectExec(regexp.QuoteMeta(deleteSQL)).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.repo.Delete(context.Background(), 2)
	s.Require().NoError(err)
}

// TestDelete_PreparedStatement_NotFound verifies that Delete returns ErrNotFound for missing rows in prepared-statement mode.
func (s *RepositoryDeletePreparedTestSuite) TestDelete_PreparedStatement_NotFound() {
	deleteSQL := "DELETE FROM `test_users` WHERE `id` = ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(deleteSQL))

	s.mock.ExpectExec(regexp.QuoteMeta(deleteSQL)).
		WithArgs(int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.Delete(context.Background(), 999)
	s.Require().Error(err)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}

// TestDelete_PreparedStatement_Error verifies that Delete propagates execution errors in prepared-statement mode.
func (s *RepositoryDeletePreparedTestSuite) TestDelete_PreparedStatement_Error() {
	deleteSQL := "DELETE FROM `test_users` WHERE `id` = ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(deleteSQL))

	s.mock.ExpectExec(regexp.QuoteMeta(deleteSQL)).
		WithArgs(int64(1)).
		WillReturnError(fmt.Errorf("foreign key constraint"))

	err := s.repo.Delete(context.Background(), 1)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to delete entity")
}

// TestDelete_PreparedStatement_PrepareError verifies that Delete surfaces statement preparation errors in prepared-statement mode.
func (s *RepositoryDeletePreparedTestSuite) TestDelete_PreparedStatement_PrepareError() {
	deleteSQL := "DELETE FROM `test_users` WHERE `id` = ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(deleteSQL)).
		WillReturnError(fmt.Errorf("prepare failed"))

	err := s.repo.Delete(context.Background(), 1)

	s.Require().EqualError(err, "failed to delete entity: failed to prepare statement: prepare failed")
}

// TestDelete_PreparedStatement_CloseError verifies that Delete surfaces statement close errors in prepared-statement mode.
func (s *RepositoryDeletePreparedTestSuite) TestDelete_PreparedStatement_CloseError() {
	client, mock := newTestClient(s.T())
	repo := mustNewRepoWithSettings[int64, testUser](s.T(), client, sqlr.Settings{PreparedStatements: true})

	deleteSQL := "DELETE FROM `test_users` WHERE `id` = ?"

	mock.ExpectPrepare(regexp.QuoteMeta(deleteSQL))

	mock.ExpectExec(regexp.QuoteMeta(deleteSQL)).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Delete(context.Background(), 1)
	s.Require().NoError(err)

	forceFirstCachedStmtCloseError(s.T(), repo, fmt.Errorf("close failed"))

	err = repo.Close()
	s.Require().EqualError(err, "failed to close prepared statement: close failed")
	s.Require().NoError(mock.ExpectationsWereMet())
}
