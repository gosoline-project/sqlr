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

// RepositoryUpdateTestSuite tests the Repository Update operations using sqlmock.
type RepositoryUpdateTestSuite struct {
	suite.Suite
	client        sqlc.Client
	mock          sqlmock.Sqlmock
	repo          sqlr.Repository[int64, testUser]
	customPkRepo  sqlr.Repository[int64, testCustomPkUser]
	stringKeyRepo sqlr.Repository[string, testStringKeyUser]
	boolKeyRepo   sqlr.Repository[bool, testBoolKeyUser]
	floatKeyRepo  sqlr.Repository[float64, testFloatKeyUser]
}

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
}

func (s *RepositoryUpdateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Success Cases
// ==========================================================================

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
}

// ==========================================================================
// Key Type Variations
// ==========================================================================

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
