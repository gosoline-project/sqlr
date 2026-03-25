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

// TestRepositoryTxCreate_NilEntityReturnsError verifies that RepositoryTx Create returns an error for nil entity.
func TestRepositoryTxCreate_NilEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Create(sqlr.TTx{}, nil)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
}

// TestRepositoryTxUpdate_NilEntityReturnsError verifies that RepositoryTx Update returns an error for nil entity.
func TestRepositoryTxUpdate_NilEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	result, err := repo.Update(sqlr.TTx{}, nil)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
	require.Nil(t, result)
}

// TestRepositoryTxCreate_NilAssociationEntityReturnsError verifies that RepositoryTx Create returns an error for nil association entity.
func TestRepositoryTxCreate_NilAssociationEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Create(sqlr.TTx{}, nil, syncCreatePosts)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
}

// TestRepositoryTxUpdate_NilAssociationEntityReturnsError verifies that RepositoryTx Update returns an error for nil association entity.
func TestRepositoryTxUpdate_NilAssociationEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	result, err := repo.Update(sqlr.TTx{}, nil, syncAllAssociations)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
	require.Nil(t, result)
}

// TestRepositoryTxDelete_InvalidAssociationPathReturnsError verifies that
// RepositoryTx Delete validates association paths before executing SQL.
func TestRepositoryTxDelete_InvalidAssociationPathReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Delete(sqlr.TTx{}, 1, func(qb *sqlr.QueryBuilderDelete) {
		qb.SyncAssociation("Unknown")
	})
	require.ErrorContains(t, err, `invalid sync association path "Unknown"`)
}

// TestRepositoryTxCreate_AutoIncrementPrimaryKey_UsesReflectionWithoutSetId verifies that RepositoryTx Create uses reflection without SetId for auto-increment primary keys.
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

// TestRepositoryTxCreate_Error_RestoresInMemoryState verifies that RepositoryTx Create restores in-memory state for error.
func TestRepositoryTxCreate_Error_RestoresInMemoryState(t *testing.T) {
	client, mock := newTestClient(t)

	repo := mustNewTxRepo[int64, testUser](t, client)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice", "alice@test.com").
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	err := runWithTx(context.Background(), client, func(ttx sqlr.TTx) error {
		return repo.Create(ttx, &entity)
	})
	require.Error(t, err)
	require.Equal(t, now, entity.CreatedAt)
	require.Equal(t, now, entity.UpdatedAt)
	require.Zero(t, entity.GetId())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepositoryTxCreate_DisableAutoUpdates_UsesPresetValues verifies that RepositoryTx Create uses preset values when auto-updates are disabled.
func TestRepositoryTxCreate_DisableAutoUpdates_UsesPresetValues(t *testing.T) {
	client, mock := newTestClient(t)
	repo := mustNewTxRepo[int64, testUser](t, client)
	createdAt := time.Now().Add(-2 * time.Hour)
	updatedAt := time.Now().Add(-time.Hour)

	insertSQL := regexp.QuoteMeta("INSERT INTO `test_users` (`id`, `created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?, ?)")

	mock.ExpectBegin()
	mock.ExpectExec(insertSQL).
		WithArgs(int64(55), createdAt, updatedAt, "Alice", "alice@test.com").
		WillReturnResult(sqlmock.NewResult(999, 1))
	mock.ExpectCommit()

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        55,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	err := runWithTx(context.Background(), client, func(ttx sqlr.TTx) error {
		return repo.Create(ttx, &entity, func(qb *sqlr.QueryBuilderCreate) {
			qb.DisableAutoUpdates()
		})
	})
	require.NoError(t, err)
	require.Equal(t, int64(55), entity.GetId())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepositoryTxUpdate_AssociationSync_AutoPreloadRehydratesNewAssociations verifies that
// transactional Update reloads auto-preloaded relations after association sync.
func TestRepositoryTxUpdate_AssociationSync_AutoPreloadRehydratesNewAssociations(t *testing.T) {
	client, mock := newTestClient(t)
	repo := mustNewTxRepo[int64, assocAuthorAutoPreload](t, client)
	now := time.Now()
	postNow := now.Add(-time.Hour)
	commentNow := now.Add(-30 * time.Minute)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_auto_preloads` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_comments_auto_preloads` WHERE `assoc_post_with_comments_auto_preloads`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}))
	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_post_with_comments_auto_preloads` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Brand New").
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `assoc_author_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice Updated"))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_post_with_comments_auto_preloads` WHERE `assoc_post_with_comments_auto_preloads`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(12), postNow, postNow, int64(1), "Brand New"))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_comments` WHERE `assoc_comments`.`post_id` IN (?)")).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "post_id", "body"}).
			AddRow(int64(100), commentNow, commentNow, int64(12), "Hydrated Comment"))
	mock.ExpectCommit()

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

	var result *assocAuthorAutoPreload
	err := runWithTx(context.Background(), client, func(ttx sqlr.TTx) error {
		var err error
		result, err = repo.Update(ttx, &entity, syncAllAssociations)

		return err
	})
	require.NoError(t, err)
	require.Same(t, &entity, result)
	require.Len(t, result.Posts, 1)
	require.Len(t, result.Posts[0].Comments, 1)
	require.Equal(t, "Hydrated Comment", result.Posts[0].Comments[0].Body)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepositoryTxUpdate_WithExplicitPreload verifies that transactional Update
// honors caller-requested post-update preloads without association sync.
func TestRepositoryTxUpdate_WithExplicitPreload(t *testing.T) {
	client, mock := newTestClient(t)
	repo := mustNewTxRepo[int64, testAuthorAutoPreload](t, client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_author_auto_preloads` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads` WHERE `test_author_auto_preloads`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice Updated"))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?) AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Published Post", "published"))
	mock.ExpectCommit()

	entity := testAuthorAutoPreload{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	var result *testAuthorAutoPreload
	err := runWithTx(context.Background(), client, func(ttx sqlr.TTx) error {
		var err error
		result, err = repo.Update(ttx, &entity, func(qb *sqlr.QueryBuilderUpdate) {
			qb.Preload("Posts", sqlr.Condition("status = ?", "published"))
		})

		return err
	})
	require.NoError(t, err)
	require.Same(t, &entity, result)
	require.Len(t, result.Posts, 1)
	require.Equal(t, "Published Post", result.Posts[0].Title)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepositoryTxUpdate_WithInvalidPreloadReturnsError verifies that
// transactional Update rejects invalid preload paths before mutating state.
func TestRepositoryTxUpdate_WithInvalidPreloadReturnsError(t *testing.T) {
	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testAuthorAutoPreload](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	now := time.Now()
	entity := testAuthorAutoPreload{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(sqlr.TTx{}, &entity, func(qb *sqlr.QueryBuilderUpdate) {
		qb.Preload("Unknown")
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorContains(t, err, `preload relation "Unknown" not found`)
}

type RepositoryTxCrudTestSuite struct {
	suite.Suite
	client      sqlc.Client
	mock        sqlmock.Sqlmock
	userRepo    sqlr.RepositoryTx[int64, testUser]
	authorRepo  sqlr.RepositoryTx[int64, testAuthor]
	preparedCtx context.Context
}

// TestRepositoryTxCrudTestSuite runs the repository tx crud test suite.
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

// TestCreate_Success verifies that Create succeeds for the basic case.
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

// TestRead_Success verifies that Read succeeds for the basic case.
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

// TestRead_NotFound verifies that Read returns ErrNotFound for missing rows.
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

// TestRead_WithPreload verifies that Read with preload.
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

// TestQuery_Success verifies that Query succeeds for the basic case.
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

// TestUpdate_Success verifies that Update succeeds for the basic case.
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

// TestUpdate_NotFound verifies that Update returns ErrNotFound for missing rows.
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

// TestDelete_Success verifies that Delete succeeds for the basic case.
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

// TestDelete_NotFound verifies that Delete returns ErrNotFound for missing rows.
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

// TestDelete_CascadesOwnedRelationsByDefault verifies that transactional Delete
// cascades owned relations inside the provided transaction.
func (s *RepositoryTxCrudTestSuite) TestDelete_CascadesOwnedRelationsByDefault() {
	repo := mustNewTxRepo[int64, assocAuthor](s.T(), s.client)
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

	err := runWithTx(s.preparedCtx, s.client, func(ttx sqlr.TTx) error {
		return repo.Delete(ttx, 1)
	})

	s.Require().NoError(err)
}

type RepositoryTxPreparedTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
	repo   sqlr.RepositoryTx[int64, testUser]
}

// TestRepositoryTxPreparedTestSuite runs the repository tx prepared test suite.
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

// TestRead_PreparedStatement_ReusedAcrossTransactions verifies that Read reused across transactions in prepared-statement mode.
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
