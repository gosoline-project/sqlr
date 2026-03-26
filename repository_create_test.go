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
	client              sqlc.Client
	mock                sqlmock.Sqlmock
	repo                sqlr.Repository[int64, testUser]
	customPkRepo        sqlr.Repository[int64, testNoSetterCustomPkUser]
	noSetterRepo        sqlr.Repository[int64, testNoSetterUser]
	noSetterPointerRepo sqlr.Repository[*int64, testNoSetterPointerKeyUser]
	stringKeyRepo       sqlr.Repository[string, testStringKeyUser]
	boolKeyRepo         sqlr.Repository[bool, testBoolKeyUser]
	floatKeyRepo        sqlr.Repository[float64, testFloatKeyUser]
	pointerKeyRepo      sqlr.Repository[*int64, testPointerKeyUser]
	pointerTimeRepo     sqlr.Repository[int64, testPointerTimestampUser]
	nullableAuthorRepo  sqlr.Repository[int64, testPostWithPointerAuthorID]
}

// TestRepositoryCreateTestSuite runs the repository create test suite.
func TestRepositoryCreateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryCreateTestSuite))
}

func (s *RepositoryCreateTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.customPkRepo = mustNewRepo[int64, testNoSetterCustomPkUser](s.T(), s.client)
	s.noSetterRepo = mustNewRepo[int64, testNoSetterUser](s.T(), s.client)
	s.noSetterPointerRepo = mustNewRepo[*int64, testNoSetterPointerKeyUser](s.T(), s.client)
	s.stringKeyRepo = mustNewRepo[string, testStringKeyUser](s.T(), s.client)
	s.boolKeyRepo = mustNewRepo[bool, testBoolKeyUser](s.T(), s.client)
	s.floatKeyRepo = mustNewRepo[float64, testFloatKeyUser](s.T(), s.client)
	s.pointerKeyRepo = mustNewRepo[*int64, testPointerKeyUser](s.T(), s.client)
	s.pointerTimeRepo = mustNewRepo[int64, testPointerTimestampUser](s.T(), s.client)
	s.nullableAuthorRepo = mustNewRepo[int64, testPostWithPointerAuthorID](s.T(), s.client)
}

func (s *RepositoryCreateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Success Cases
// ==========================================================================

// TestCreate_Success verifies that Create succeeds for the basic case.
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

// TestCreate_PointerPrimaryKey_AutoIncrementSetsPointer verifies that Create sets pointer values from auto-increment IDs for pointer primary keys.
func (s *RepositoryCreateTestSuite) TestCreate_PointerPrimaryKey_AutoIncrementSetsPointer() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_pointer_key_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(42, 1))

	entity := testPointerKeyUser{Name: "Alice"}
	err := s.pointerKeyRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(entity.GetId())
	s.Equal(int64(42), *entity.GetId())
}

// TestCreate_AutoIncrementPrimaryKey_UsesReflectionWithoutSetId verifies that Create uses reflection without SetId for auto-increment primary keys.
func (s *RepositoryCreateTestSuite) TestCreate_AutoIncrementPrimaryKey_UsesReflectionWithoutSetId() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_no_setter_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(24, 1))

	entity := testNoSetterUser{Name: "Alice"}
	err := s.noSetterRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal(int64(24), entity.GetId())
}

// TestCreate_PointerAutoIncrementPrimaryKey_UsesReflectionWithoutSetId verifies that Create uses reflection without SetId for pointer auto-increment primary keys.
func (s *RepositoryCreateTestSuite) TestCreate_PointerAutoIncrementPrimaryKey_UsesReflectionWithoutSetId() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_no_setter_pointer_key_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(43, 1))

	entity := testNoSetterPointerKeyUser{Name: "Alice"}
	err := s.noSetterPointerRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(entity.GetId())
	s.Equal(int64(43), *entity.GetId())
}

// TestCreate_PointerTimestamps_AreSet verifies that Create sets the expected values for pointer timestamps.
func (s *RepositoryCreateTestSuite) TestCreate_PointerTimestamps_AreSet() {
	insertSQL := regexp.QuoteMeta("INSERT INTO `test_pointer_timestamp_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")

	s.mock.ExpectExec(insertSQL).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(12, 1))

	entity := testPointerTimestampUser{Name: "Alice"}
	err := s.pointerTimeRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal(int64(12), entity.GetId())
	s.Require().NotNil(entity.CreatedAt)
	s.Require().NotNil(entity.UpdatedAt)
	s.False(entity.CreatedAt.IsZero())
	s.False(entity.UpdatedAt.IsZero())
}

// TestCreate_DisableAutoUpdates_UsesPresetValues verifies that Create uses preset values when auto-updates are disabled.
func (s *RepositoryCreateTestSuite) TestCreate_DisableAutoUpdates_UsesPresetValues() {
	createdAt := time.Now().Add(-2 * time.Hour)
	updatedAt := time.Now().Add(-time.Hour)

	insertSQL := regexp.QuoteMeta("INSERT INTO `test_users` (`id`, `created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?, ?)")

	s.mock.ExpectExec(insertSQL).
		WithArgs(int64(55), createdAt, updatedAt, "Alice", "alice@test.com").
		WillReturnResult(sqlmock.NewResult(999, 1))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			Id:        55,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Name:  "Alice",
		Email: "alice@test.com",
	}

	err := s.repo.Create(context.Background(), &entity, func(qb *sqlr.QueryBuilderCreate) {
		qb.DisableAutoUpdates()
	})

	s.Require().NoError(err)
	s.Equal(int64(55), entity.GetId())
	s.Equal(createdAt, entity.CreatedAt)
	s.Equal(updatedAt, entity.UpdatedAt)
}

// TestCreate_WithExplicitPreload verifies that Create can reload and hydrate
// caller-requested relations after inserting the root row.
func (s *RepositoryCreateTestSuite) TestCreate_WithExplicitPreload() {
	now := time.Now()
	postNow := now.Add(-time.Hour)

	repo := mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
	entity := testAuthorAutoPreload{Name: "Alice"}

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_author_auto_preloads` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads` WHERE `test_author_auto_preloads`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(int64(1), now, now, "Alice"))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?) AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Published Post", "published"))

	err := repo.Create(context.Background(), &entity, func(qb *sqlr.QueryBuilderCreate) {
		qb.Preload("Posts", sqlr.Condition("status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Equal(int64(1), entity.GetId())
	s.Require().Len(entity.Posts, 1)
	s.Equal("Published Post", entity.Posts[0].Title)
	s.Equal("published", entity.Posts[0].Status)
}

// TestCreate_WithInvalidPreloadReturnsError verifies that Create rejects invalid
// post-create preload paths before issuing any SQL.
func (s *RepositoryCreateTestSuite) TestCreate_WithInvalidPreloadReturnsError() {
	repo := mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
	entity := testAuthorAutoPreload{Name: "Alice"}

	err := repo.Create(context.Background(), &entity, func(qb *sqlr.QueryBuilderCreate) {
		qb.Preload("Unknown")
	})

	s.Require().Error(err)
	s.ErrorContains(err, `preload relation "Unknown" not found`)
}

// TestCreate_DisableAutoUpdates_MissingPrimaryKeyReturnsError verifies that Create returns an error when auto-updates are disabled without a preset primary key.
func (s *RepositoryCreateTestSuite) TestCreate_DisableAutoUpdates_MissingPrimaryKeyReturnsError() {
	entity := testUser{Name: "Alice", Email: "alice@test.com"}

	err := s.repo.Create(context.Background(), &entity, func(qb *sqlr.QueryBuilderCreate) {
		qb.DisableAutoUpdates()
	})

	s.Require().Error(err)
	s.ErrorIs(err, sqlr.ErrAutoUpdatesRequirePresetPrimaryKey)
}

// TestCreate_CustomPrimaryKeyColumn_UsesReflectionWithoutSetId verifies that Create uses reflection without SetId for custom primary key columns.
func (s *RepositoryCreateTestSuite) TestCreate_CustomPrimaryKeyColumn_UsesReflectionWithoutSetId() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_no_setter_custom_pk_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(17, 1))

	entity := testNoSetterCustomPkUser{Name: "Alice"}
	err := s.customPkRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Equal(int64(17), entity.GetId())
}

// TestCreate_BelongsTo_SetsNullableForeignKey verifies that Create sets nullable foreign keys for belongs-to relations.
func (s *RepositoryCreateTestSuite) TestCreate_BelongsTo_SetsNullableForeignKey() {
	createdAt := time.Now().Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Author").
		WillReturnResult(sqlmock.NewResult(7, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_post_with_pointer_author_ids` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(7), "Post").
		WillReturnResult(sqlmock.NewResult(11, 1))
	s.mock.ExpectCommit()

	entity := testPostWithPointerAuthorID{
		Entity: sqlr.Entity[int64]{
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		Title:  "Post",
		Author: testAuthor{Name: "Author"},
	}

	err := s.nullableAuthorRepo.Create(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(entity.AuthorID)
	s.Equal(int64(7), *entity.AuthorID)
}

// ==========================================================================
// Error Cases
// ==========================================================================

// TestCreate_Error verifies that Create propagates execution errors.
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

// TestCreate_NilEntityReturnsError verifies that Create returns an error for nil entity.
func (s *RepositoryCreateTestSuite) TestCreate_NilEntityReturnsError() {
	err := s.repo.Create(context.Background(), nil)

	s.Require().Error(err)
	s.Require().ErrorIs(err, sqlr.ErrNilEntity)
}

// TestCreate_LastInsertIDError verifies that Create surfaces last-insert-ID errors.
func (s *RepositoryCreateTestSuite) TestCreate_LastInsertIDError() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob", "bob@test.com").
		WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("last insert id unavailable")))

	entity := testUser{Name: "Bob", Email: "bob@test.com"}
	err := s.repo.Create(context.Background(), &entity)

	s.Require().Error(err)
	s.Contains(err.Error(), "failed to get last insert id")
}

// TestCreate_Error_RestoresInMemoryState verifies that Create restores in-memory state for error.
func (s *RepositoryCreateTestSuite) TestCreate_Error_RestoresInMemoryState() {
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob", "bob@test.com").
		WillReturnError(fmt.Errorf("duplicate entry"))

	entity := testUser{
		Entity: sqlr.Entity[int64]{
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Bob",
		Email: "bob@test.com",
	}

	err := s.repo.Create(context.Background(), &entity)

	s.Require().Error(err)
	s.Zero(entity.GetId())
	s.Equal(now, entity.CreatedAt)
	s.Equal(now, entity.UpdatedAt)
}

// ==========================================================================
// Non-Standard Key Types
// ==========================================================================

// TestCreate_StringPrimaryKey verifies that Create string primary key.
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

// TestCreate_BoolPrimaryKey verifies that Create bool primary key.
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

// TestCreate_FloatPrimaryKey verifies that Create float primary key.
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

// TestCreate_NonAutoIncrementIncludesPrimaryKey verifies that Create nonauto-increment includesprimary key.
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

// TestRepositoryCreatePreparedTestSuite runs the repository create prepared test suite.
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

// TestCreate_PreparedStatement_Success verifies that Create succeeds for the basic case in prepared-statement mode.
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

// TestCreate_PreparedStatement_Error verifies that Create propagates execution errors in prepared-statement mode.
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

// TestCreate_PreparedStatement_PrepareError verifies that Create surfaces statement preparation errors in prepared-statement mode.
func (s *RepositoryCreatePreparedTestSuite) TestCreate_PreparedStatement_PrepareError() {
	createSQL := "INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)"

	s.mock.ExpectPrepare(regexp.QuoteMeta(createSQL)).
		WillReturnError(fmt.Errorf("prepare failed"))

	entity := testUser{
		Name:  "Charlie",
		Email: "charlie@test.com",
	}

	err := s.repo.Create(context.Background(), &entity)

	s.Require().EqualError(err, "failed to create entity: failed to prepare statement: prepare failed")
}

// TestCreate_PreparedStatement_CloseError verifies that Create surfaces statement close errors in prepared-statement mode.
func (s *RepositoryCreatePreparedTestSuite) TestCreate_PreparedStatement_CloseError() {
	client, mock := newTestClient(s.T())
	repo := mustNewRepoWithSettings[int64, testUser](s.T(), client, sqlr.Settings{PreparedStatements: true})

	createSQL := "INSERT INTO `test_users` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)"

	mock.ExpectPrepare(regexp.QuoteMeta(createSQL))

	mock.ExpectExec(regexp.QuoteMeta(createSQL)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Charlie", "charlie@test.com").
		WillReturnResult(sqlmock.NewResult(1, 1))

	entity := testUser{
		Name:  "Charlie",
		Email: "charlie@test.com",
	}

	err := repo.Create(context.Background(), &entity)
	s.Require().NoError(err)

	forceFirstCachedStmtCloseError(s.T(), repo, fmt.Errorf("close failed"))

	err = repo.Close()
	s.Require().EqualError(err, "failed to close prepared statement: close failed")
	s.Require().NoError(mock.ExpectationsWereMet())
}
