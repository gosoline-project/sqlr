package sqlr_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func mustNewTxRepo[K sqlr.KeyTypes, E sqlr.Entitier[K]](t *testing.T, client sqlc.Client) sqlr.RepositoryTx[K, E] {
	t.Helper()

	repo, err := sqlr.NewRepositoryTxWithSettings[K, E](client, sqlr.DefaultSettings())
	require.NoError(t, err)

	return repo
}

func mustNewTxRepoWithSettings[K sqlr.KeyTypes, E sqlr.Entitier[K]](t *testing.T, client sqlc.Client, settings sqlr.Settings) sqlr.RepositoryTx[K, E] {
	t.Helper()

	repo, err := sqlr.NewRepositoryTxWithSettings[K, E](client, settings)
	require.NoError(t, err)

	return repo
}

func runWithTx(ctx context.Context, client sqlc.Client, fn func(ttx sqlr.TTx) error) error {
	return client.WithTx(ctx, func(tx sqlc.Tx) error {
		return fn(sqlr.NewTx(tx))
	})
}

// TestNewRepositoryTxWithSettings_PreparedStatementsWithoutClient verifies that
// transactional repositories reject prepared statement mode when no client is
// available for preparing statements.
func TestNewRepositoryTxWithSettings_PreparedStatementsWithoutClient(t *testing.T) {
	t.Parallel()

	_, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.Settings{PreparedStatements: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "client")
}

func TestRepositoryTxCreate_NilEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Create(sqlr.TTx{}, nil)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
}

func TestRepositoryTxUpdate_NilEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	result, err := repo.Update(sqlr.TTx{}, nil)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
	require.Nil(t, result)
}

func TestRepositoryTxCreate_NilAssociationEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Create(sqlr.TTx{}, nil, syncCreatePosts)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
}

func TestRepositoryTxUpdate_NilAssociationEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	result, err := repo.Update(sqlr.TTx{}, nil, syncAllAssociations)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
	require.Nil(t, result)
}

func TestRepositoryTxCreate_AutoIncrementPrimaryKey_UsesReflectionWithoutSetId(t *testing.T) {
	client, mock := newTestClient(t)

	repo := mustNewTxRepo[int64, testNoSetterUser](t, client)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `test_no_setter_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(51, 1))
	mock.ExpectCommit()

	err := runWithTx(context.Background(), client, func(ttx sqlr.TTx) error {
		entity := testNoSetterUser{Name: "Alice"}

		if err := repo.Create(ttx, &entity); err != nil {
			return err
		}

		require.Equal(t, int64(51), entity.GetId())

		return nil
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

type RepositoryTxCrudTestSuite struct {
	suite.Suite
	client      sqlc.Client
	mock        sqlmock.Sqlmock
	userRepo    sqlr.RepositoryTx[int64, testUser]
	authorRepo  sqlr.RepositoryTx[int64, testAuthor]
	preparedCtx context.Context
}

func TestRepositoryTxCrudTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTxCrudTestSuite))
}

func (s *RepositoryTxCrudTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock
	s.userRepo = mustNewTxRepo[int64, testUser](s.T(), s.client)
	s.authorRepo = mustNewTxRepo[int64, testAuthor](s.T(), s.client)
	s.preparedCtx = context.Background()
}

func (s *RepositoryTxCrudTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func (s *RepositoryTxCrudTestSuite) TestCreate_Success() {
	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)"),
	).WithArgs(isTimestamp{}, isTimestamp{}, "Alice", "alice@test.com").WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()

	entity := testUser{
		Name:  "Alice",
		Email: "alice@test.com",
	}

	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		return s.userRepo.Create(ttx, &entity)
	})

	s.Require().NoError(err)
	s.Equal(int64(1), entity.GetId())
}

func (s *RepositoryTxCrudTestSuite) TestRead_Success() {
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?"),
	).WithArgs(int64(1), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
		AddRow(1, now, now, "Alice", "alice@test.com"))
	s.mock.ExpectCommit()

	var result *testUser
	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		var err error
		result, err = s.userRepo.Read(ttx, 1)

		return err
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(1), result.GetId())
	s.Equal("Alice", result.Name)
	s.Equal("alice@test.com", result.Email)
}

func (s *RepositoryTxCrudTestSuite) TestRead_NotFound() {
	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?"),
	).WithArgs(int64(999), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}))
	s.mock.ExpectRollback()

	var result *testUser
	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		var err error
		result, err = s.userRepo.Read(ttx, 999)

		return err
	})

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=999")
}

func (s *RepositoryTxCrudTestSuite) TestRead_WithPreload() {
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` = ? LIMIT ?"),
	).WithArgs(int64(1), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
		AddRow(1, now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)"),
	).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
		AddRow(10, now, now, 1, "First Post", "published").
		AddRow(11, now, now, 1, "Second Post", "draft"))
	s.mock.ExpectCommit()

	var result *testAuthor
	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		var err error
		result, err = s.authorRepo.Read(ttx, 1, func(qb *sqlr.QueryBuilderRead) {
			qb.Preload("Posts")
		})

		return err
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post"), Status: ptr("published")},
			{Title: ptr("Second Post"), Status: ptr("draft")},
		},
	})
}

func (s *RepositoryTxCrudTestSuite) TestQuery_Success() {
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?"),
	).WithArgs("Alice").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
		AddRow(1, now, now, "Alice", "alice@test.com").
		AddRow(2, now, now, "Alice", "alice2@test.com"))
	s.mock.ExpectCommit()

	var results []testUser
	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		var err error
		results, err = s.userRepo.Query(ttx, func(qb *sqlr.QueryBuilderSelect) {
			qb.Where("name = ?", "Alice")
		})

		return err
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.Equal("Alice", results[0].Name)
	s.Equal("alice@test.com", results[0].Email)
	s.Equal("Alice", results[1].Name)
	s.Equal("alice2@test.com", results[1].Email)
}

func (s *RepositoryTxCrudTestSuite) TestUpdate_Success() {
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

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"),
	).WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	var result *testUser
	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		var err error
		result, err = s.userRepo.Update(ttx, &entity)

		return err
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice Updated", result.Name)
	s.Equal("alice-updated@test.com", result.Email)
}

func (s *RepositoryTxCrudTestSuite) TestUpdate_NotFound() {
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

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?"),
	).WithArgs(isTimestamp{}, entity.Email, entity.Name, isTimestamp{}, entity.Id).WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectRollback()

	var result *testUser
	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		var err error
		result, err = s.userRepo.Update(ttx, &entity)

		return err
	})

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=99")
}

func (s *RepositoryTxCrudTestSuite) TestDelete_Success() {
	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE `id` = ?"),
	).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		return s.userRepo.Delete(ttx, 1)
	})

	s.Require().NoError(err)
}

func (s *RepositoryTxCrudTestSuite) TestDelete_NotFound() {
	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE `id` = ?"),
	).WithArgs(int64(999)).WillReturnResult(sqlmock.NewResult(0, 0))
	s.mock.ExpectRollback()

	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		return s.userRepo.Delete(ttx, 999)
	})

	s.Require().Error(err)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=999")
}

type RepositoryTxPreparedTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
	repo   sqlr.RepositoryTx[int64, testUser]
}

func TestRepositoryTxPreparedTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTxPreparedTestSuite))
}

func (s *RepositoryTxPreparedTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock
	s.repo = mustNewTxRepoWithSettings[int64, testUser](s.T(), s.client, sqlr.Settings{PreparedStatements: true})
}

func (s *RepositoryTxPreparedTestSuite) TearDownTest() {
	s.Require().NoError(s.repo.Close())
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func (s *RepositoryTxPreparedTestSuite) TestRead_PreparedStatement_ReusedAcrossTransactions() {
	now := time.Now()
	readSQL := "SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?"

	s.mock.ExpectBegin()
	s.mock.ExpectPrepare(regexp.QuoteMeta(readSQL))
	s.mock.ExpectPrepare(regexp.QuoteMeta(readSQL))
	s.mock.ExpectQuery(regexp.QuoteMeta(readSQL)).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(1, now, now, "Alice", "alice@test.com"))
	s.mock.ExpectCommit()

	var result1 *testUser
	err := runWithTx(context.Background(), s.client, func(ttx sqlr.TTx) error {
		var err error
		result1, err = s.repo.Read(ttx, 1)

		return err
	})
	s.Require().NoError(err)
	s.Require().NotNil(result1)
	s.Equal(int64(1), result1.GetId())

	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(readSQL)).
		WithArgs(int64(2), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(2, now, now, "Bob", "bob@test.com"))
	s.mock.ExpectCommit()

	var result2 *testUser
	err = runWithTx(context.Background(), s.client, func(ttx sqlr.TTx) error {
		var err error
		result2, err = s.repo.Read(ttx, 2)

		return err
	})
	s.Require().NoError(err)
	s.Require().NotNil(result2)
	s.Equal(int64(2), result2.GetId())
	s.Equal("Bob", result2.Name)
}
