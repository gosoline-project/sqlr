package sqlr_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

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
