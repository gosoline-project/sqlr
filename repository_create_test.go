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

// RepositoryCreateTestSuite tests the Repository Create operations using sqlmock.
type RepositoryCreateTestSuite struct {
	suite.Suite
	client        sqlc.Client
	mock          sqlmock.Sqlmock
	repo          sqlr.Repository[int64, testUser]
	stringKeyRepo sqlr.Repository[string, testStringKeyUser]
	boolKeyRepo   sqlr.Repository[bool, testBoolKeyUser]
	floatKeyRepo  sqlr.Repository[float64, testFloatKeyUser]
}

func TestRepositoryCreateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryCreateTestSuite))
}

func (s *RepositoryCreateTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.stringKeyRepo = mustNewRepo[string, testStringKeyUser](s.T(), s.client)
	s.boolKeyRepo = mustNewRepo[bool, testBoolKeyUser](s.T(), s.client)
	s.floatKeyRepo = mustNewRepo[float64, testFloatKeyUser](s.T(), s.client)
}

func (s *RepositoryCreateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Success Cases
// ==========================================================================

func (s *RepositoryCreateTestSuite) TestCreate_Success() {
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice", "alice@test.com").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	err := s.repo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal(int64(1), entity.GetId())
}

// ==========================================================================
// Error Cases
// ==========================================================================

func (s *RepositoryCreateTestSuite) TestCreate_Error() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob", "bob@test.com").
		WillReturnError(fmt.Errorf("duplicate entry"))

	entity := testUser{
		Name:  "Bob",
		Email: "bob@test.com",
	}

	err := s.repo.Create(context.Background(), &entity)

	s.Require().Error(err)
	s.Contains(err.Error(), "failed to create entity")
}

// ==========================================================================
// Non-Standard Key Types
// ==========================================================================

func (s *RepositoryCreateTestSuite) TestCreate_StringPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_string_key_users` (`id`, `created_at`, `updated_at`, `name`) VALUES (?, ?, ?, ?)")).
		WithArgs("", isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(21, 1))

	entity := testStringKeyUser{Name: "Alice"}
	err := s.stringKeyRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal("", entity.GetId())
}

func (s *RepositoryCreateTestSuite) TestCreate_BoolPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_bool_key_users` (`id`, `created_at`, `updated_at`, `name`) VALUES (?, ?, ?, ?)")).
		WithArgs(true, isTimestamp{}, isTimestamp{}, "Bob").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity := testBoolKeyUser{
		Entity: sqlr.Entity[bool]{Id: true},
		Name:   "Bob",
	}
	err := s.boolKeyRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.True(entity.GetId())
}

func (s *RepositoryCreateTestSuite) TestCreate_FloatPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_float_key_users` (`id`, `created_at`, `updated_at`, `name`) VALUES (?, ?, ?, ?)")).
		WithArgs(float64(7.5), isTimestamp{}, isTimestamp{}, "Carol").
		WillReturnResult(sqlmock.NewResult(7, 1))

	entity := testFloatKeyUser{
		Entity: sqlr.Entity[float64]{Id: 7.5},
		Name:   "Carol",
	}
	err := s.floatKeyRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal(float64(7.5), entity.GetId())
}

func (s *RepositoryCreateTestSuite) TestCreate_NonAutoIncrementIncludesPrimaryKey() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_string_key_users` (`id`, `created_at`, `updated_at`, `name`) VALUES (?, ?, ?, ?)")).
		WithArgs("user-abc", isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(33, 1))

	entity := testStringKeyUser{
		Entity: sqlr.Entity[string]{Id: "user-abc"},
		Name:   "Alice",
	}
	err := s.stringKeyRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal("user-abc", entity.GetId())
}

// ==========================================================================
// Prepared Statement Tests
// ==========================================================================

// RepositoryCreatePreparedTestSuite tests the Repository Create operations with prepared statements.
type RepositoryCreatePreparedTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
	repo   sqlr.Repository[int64, testUser]
}

func TestRepositoryCreatePreparedTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryCreatePreparedTestSuite))
}

func (s *RepositoryCreatePreparedTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	settings := sqlr.Settings{PreparedStatements: true}
	s.repo = mustNewRepoWithSettings[int64, testUser](s.T(), s.client, settings)
}

func (s *RepositoryCreatePreparedTestSuite) TearDownTest() {
	s.Require().NoError(s.repo.Close())
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func (s *RepositoryCreatePreparedTestSuite) TestCreate_PreparedStatement_Success() {
	createSQL := "INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)"

	// Expect prepare on first call
	s.mock.ExpectPrepare(regexp.QuoteMeta(createSQL))

	s.mock.ExpectExec(regexp.QuoteMeta(createSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice", "alice@test.com").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity1 := testUser{
		Name:  "Alice",
		Email: "alice@test.com",
	}

	err := s.repo.Create(context.Background(), &entity1)
	s.Require().NoError(err)
	s.Equal(int64(1), entity1.GetId())

	// Second call should reuse prepared statement (no ExpectPrepare)
	s.mock.ExpectExec(regexp.QuoteMeta(createSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob", "bob@test.com").
		WillReturnResult(sqlmock.NewResult(2, 1))

	entity2 := testUser{
		Name:  "Bob",
		Email: "bob@test.com",
	}

	err = s.repo.Create(context.Background(), &entity2)
	s.Require().NoError(err)
	s.Equal(int64(2), entity2.GetId())
}

func (s *RepositoryCreatePreparedTestSuite) TestCreate_PreparedStatement_Error() {
	createSQL := "INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)"

	s.mock.ExpectPrepare(regexp.QuoteMeta(createSQL))

	s.mock.ExpectExec(regexp.QuoteMeta(createSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Charlie", "charlie@test.com").
		WillReturnError(fmt.Errorf("duplicate entry"))

	entity := testUser{
		Name:  "Charlie",
		Email: "charlie@test.com",
	}

	err := s.repo.Create(context.Background(), &entity)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to create entity")
}
