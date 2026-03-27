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

// RepositoryUpdateTestSuite tests the Repository Update operations using sqlmock.
type RepositoryUpdateTestSuite struct {
	suite.Suite
	client          sqlc.Client
	mock            sqlmock.Sqlmock
	repo            sqlr.Repository[int64, testUser]
	customPkRepo    sqlr.Repository[int64, testCustomPkUser]
	stringKeyRepo   sqlr.Repository[string, testStringKeyUser]
	boolKeyRepo     sqlr.Repository[bool, testBoolKeyUser]
	floatKeyRepo    sqlr.Repository[float64, testFloatKeyUser]
	pointerTimeRepo sqlr.Repository[int64, testPointerTimestampUser]
}

// TestRepositoryUpdateTestSuite runs the repository update test suite.
func TestRepositoryUpdateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryUpdateTestSuite))
}

func (s *RepositoryUpdateTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.customPkRepo = mustNewRepo[int64, testCustomPkUser](s.T(), s.client)
	s.stringKeyRepo = mustNewRepo[string, testStringKeyUser](s.T(), s.client)
	s.boolKeyRepo = mustNewRepo[bool, testBoolKeyUser](s.T(), s.client)
	s.floatKeyRepo = mustNewRepo[float64, testFloatKeyUser](s.T(), s.client)
	s.pointerTimeRepo = mustNewRepo[int64, testPointerTimestampUser](s.T(), s.client)
}

func (s *RepositoryUpdateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Success Cases
// ==========================================================================

// TestUpdate_Success verifies that Update succeeds for the basic case.
func (s *RepositoryUpdateTestSuite) TestUpdate_Success() {
	now := time.Now()

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice Updated",
		Email: "alice-updated@test.com",
	}

	// SetMap sorts columns alphabetically: created_at, email, name, updated_at
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice Updated", result.Name)
	s.Equal("alice-updated@test.com", result.Email)
}

// TestUpdate_Error verifies that Update propagates execution errors.
func (s *RepositoryUpdateTestSuite) TestUpdate_Error() {
	now := time.Now()

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnError(fmt.Errorf("deadlock"))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to update entity")
	s.Equal(now, entity.UpdatedAt)
}

// TestUpdate_NilEntityReturnsError verifies that Update returns an error for nil entity.
func (s *RepositoryUpdateTestSuite) TestUpdate_NilEntityReturnsError() {
	result, err := s.repo.Update(context.Background(), nil)

	s.Require().Error(err)
	s.Require().ErrorIs(err, sqlr.ErrNilEntity)
	s.Nil(result)
}

// TestUpdate_NotFound verifies that Update returns ErrNotFound for missing rows.
func (s *RepositoryUpdateTestSuite) TestUpdate_NotFound() {
	now := time.Now()

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        99,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Missing",
		Email: "missing@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}

// TestUpdate_RowsAffectedError verifies that Update surfaces rows-affected errors.
func (s *RepositoryUpdateTestSuite) TestUpdate_RowsAffectedError() {
	now := time.Now()

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        99,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Missing",
		Email: "missing@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected unavailable")))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to get rows affected")
}

// ==========================================================================
// Key Type Variations
// ==========================================================================

// TestUpdate_CustomPrimaryKey verifies that Update custom primary key.
func (s *RepositoryUpdateTestSuite) TestUpdate_CustomPrimaryKey() {
	now := time.Now()

	entity := testCustomPkUser{
		Id:        7,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "Custom PK",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_custom_pk_users` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `user_id` = ?")).
		WithArgs(isTimestamp{}, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.customPkRepo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(7), result.Id)
	s.Equal("Custom PK", result.Name)
}

// TestUpdate_StringPrimaryKey verifies that Update string primary key.
func (s *RepositoryUpdateTestSuite) TestUpdate_StringPrimaryKey() {
	now := time.Now()

	entity := testStringKeyUser{
		Entity: sqlr.Entity[string]{
			Id:        "user-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "String Updated",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_string_key_users` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.stringKeyRepo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("user-1", result.GetId())
}

// TestUpdate_BoolPrimaryKey verifies that Update bool primary key.
func (s *RepositoryUpdateTestSuite) TestUpdate_BoolPrimaryKey() {
	now := time.Now()

	entity := testBoolKeyUser{
		Entity: sqlr.Entity[bool]{
			Id:        true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Bool Updated",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_bool_key_users` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.boolKeyRepo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.True(result.GetId())
}

// TestUpdate_FloatPrimaryKey verifies that Update float primary key.
func (s *RepositoryUpdateTestSuite) TestUpdate_FloatPrimaryKey() {
	now := time.Now()

	entity := testFloatKeyUser{
		Entity: sqlr.Entity[float64]{
			Id:        float64(5),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Float Updated",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_float_key_users` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.floatKeyRepo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(float64(5), result.GetId())
}

// TestUpdate_PointerTimestamps_AreSet verifies that Update sets the expected values for pointer timestamps.
func (s *RepositoryUpdateTestSuite) TestUpdate_PointerTimestamps_AreSet() {
	createdAt := time.Now().Add(-time.Hour)
	updatedAt := time.Now().Add(-30 * time.Minute)

	entity := testPointerTimestampUser{
		Id:        5,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
		Name:      "Alice Updated",
	}

	updateSQL := regexp.QuoteMeta("UPDATE `test_pointer_timestamp_users` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")

	s.mock.ExpectExec(updateSQL).
		WithArgs(isTimestamp{}, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.pointerTimeRepo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().NotNil(result.CreatedAt)
	s.Require().NotNil(result.UpdatedAt)
	s.False(result.CreatedAt.IsZero())
	s.False(result.UpdatedAt.IsZero())
}

// TestUpdate_DisableAutoUpdates_UsesPresetValues verifies that Update uses preset values when auto-updates are disabled.
func (s *RepositoryUpdateTestSuite) TestUpdate_DisableAutoUpdates_UsesPresetValues() {
	createdAt := time.Now().Add(-2 * time.Hour)
	updatedAt := time.Now().Add(-time.Hour)

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Name:  "Alice Updated",
		Email: "alice-updated@test.com",
	}

	updateSQL := regexp.QuoteMeta("UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")

	s.mock.ExpectExec(updateSQL).
		WithArgs(createdAt, entity.Email, entity.Name, updatedAt, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.repo.Update(context.Background(), &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.DisableAutoUpdates()
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(createdAt, result.CreatedAt)
	s.Equal(updatedAt, result.UpdatedAt)
}

// TestUpdate_WithExplicitPreload verifies that Update can reload and hydrate
// caller-requested relations even without association sync.
func (s *RepositoryUpdateTestSuite) TestUpdate_WithExplicitPreload() {
	now := time.Now()
	postNow := now.Add(-time.Hour)

	repo := mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
	entity := testAuthorAutoPreload{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:   "Alice Updated",
		Parent: 7,
	}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_author_auto_preloads` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads` WHERE `test_author_auto_preloads`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice Updated"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?) AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Published Post", "published"))

	result, err := repo.Update(context.Background(), &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.Preload("Posts", sqlr.Condition("status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().Same(&entity, result)
	s.Equal(uint(7), result.Parent)
	s.Require().Len(result.Posts, 1)
	s.Equal("Published Post", result.Posts[0].Title)
	s.Equal("published", result.Posts[0].Status)
}

// TestUpdate_WithInvalidPreloadReturnsError verifies that Update rejects invalid
// post-update preload paths before issuing any SQL.
func (s *RepositoryUpdateTestSuite) TestUpdate_WithInvalidPreloadReturnsError() {
	now := time.Now()
	repo := mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
	entity := testAuthorAutoPreload{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(context.Background(), &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.Preload("Unknown")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, `preload relation "Unknown" not found`)
}

// ==========================================================================
// Prepared Statement Tests
// ==========================================================================

// RepositoryUpdatePreparedTestSuite tests the Repository Update operations with prepared statements.
type RepositoryUpdatePreparedTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
	repo   sqlr.Repository[int64, testUser]
}

// TestRepositoryUpdatePreparedTestSuite runs the repository update prepared test suite.
func TestRepositoryUpdatePreparedTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryUpdatePreparedTestSuite))
}

func (s *RepositoryUpdatePreparedTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	settings := sqlr.Settings{PreparedStatements: true}
	s.repo = mustNewRepoWithSettings[int64, testUser](s.T(), s.client, settings)
}

func (s *RepositoryUpdatePreparedTestSuite) TearDownTest() {
	s.Require().NoError(s.repo.Close())
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// TestUpdate_PreparedStatement_Success verifies that Update succeeds for the basic case in prepared-statement mode.
func (s *RepositoryUpdatePreparedTestSuite) TestUpdate_PreparedStatement_Success() {
	now := time.Now()
	updateSQL := "UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"

	// Expect prepare on first call
	s.mock.ExpectPrepare(regexp.QuoteMeta(updateSQL))

	entity1 := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice Updated",
		Email: "alice-updated@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(updateSQL)).
		WithArgs(isTimestamp{}, entity1.Email, entity1.Name, isTimestamp{}, entity1.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result1, err := s.repo.Update(context.Background(), &entity1)
	s.Require().NoError(err)
	s.Require().NotNil(result1)
	s.Equal("Alice Updated", result1.Name)

	// Second call should reuse prepared statement (no ExpectPrepare)
	entity2 := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Bob Updated",
		Email: "bob-updated@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(updateSQL)).
		WithArgs(isTimestamp{}, entity2.Email, entity2.Name, isTimestamp{}, entity2.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result2, err := s.repo.Update(context.Background(), &entity2)
	s.Require().NoError(err)
	s.Require().NotNil(result2)
	s.Equal("Bob Updated", result2.Name)
}

// TestUpdate_PreparedStatement_Error verifies that Update propagates execution errors in prepared-statement mode.
func (s *RepositoryUpdatePreparedTestSuite) TestUpdate_PreparedStatement_Error() {
	now := time.Now()
	updateSQL := "UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(updateSQL))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(updateSQL)).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnError(fmt.Errorf("deadlock"))

	result, err := s.repo.Update(context.Background(), &entity)
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to update entity")
}

// TestUpdate_PreparedStatement_NotFound verifies that Update returns ErrNotFound for missing rows in prepared-statement mode.
func (s *RepositoryUpdatePreparedTestSuite) TestUpdate_PreparedStatement_NotFound() {
	now := time.Now()
	updateSQL := "UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(updateSQL))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        99,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Missing",
		Email: "missing@test.com",
	}

	s.mock.ExpectExec(regexp.QuoteMeta(updateSQL)).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}

// TestUpdate_PreparedStatement_PrepareError verifies that Update surfaces statement preparation errors in prepared-statement mode.
func (s *RepositoryUpdatePreparedTestSuite) TestUpdate_PreparedStatement_PrepareError() {
	now := time.Now()
	updateSQL := "UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(updateSQL)).
		WillReturnError(fmt.Errorf("prepare failed"))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	result, err := s.repo.Update(context.Background(), &entity)

	s.Nil(result)
	s.Require().EqualError(err, "failed to update entity: failed to prepare statement: prepare failed")
}

// TestUpdate_PreparedStatement_CloseError verifies that Update surfaces statement close errors in prepared-statement mode.
func (s *RepositoryUpdatePreparedTestSuite) TestUpdate_PreparedStatement_CloseError() {
	client, mock := newTestClient(s.T())
	repo := mustNewRepoWithSettings[int64, testUser](s.T(), client, sqlr.Settings{PreparedStatements: true})

	now := time.Now()
	updateSQL := "UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"

	mock.ExpectPrepare(regexp.QuoteMeta(updateSQL))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice Updated",
		Email: "alice-updated@test.com",
	}

	mock.ExpectExec(regexp.QuoteMeta(updateSQL)).
		WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := repo.Update(context.Background(), &entity)
	s.Require().NoError(err)
	s.Require().NotNil(result)

	forceFirstCachedStmtCloseError(s.T(), repo, fmt.Errorf("close failed"))

	err = repo.Close()
	s.Require().EqualError(err, "failed to close prepared statement: close failed")
	s.Require().NoError(mock.ExpectationsWereMet())
}
