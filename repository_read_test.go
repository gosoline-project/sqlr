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
	"github.com/stretchr/testify/suite"
)

// RepositoryReadTestSuite tests the Repository Read operations using sqlmock.
type RepositoryReadTestSuite struct {
	suite.Suite
	client                 sqlc.Client
	mock                   sqlmock.Sqlmock
	repo                   sqlr.Repository[int64, testUser]
	customPkRepo           sqlr.Repository[int64, testCustomPkUser]
	stringKeyRepo          sqlr.Repository[string, testStringKeyUser]
	boolKeyRepo            sqlr.Repository[bool, testBoolKeyUser]
	floatKeyRepo           sqlr.Repository[float64, testFloatKeyUser]
	authorAutoPreloadRepo  sqlr.Repository[int64, testAuthorAutoPreload]
	deepAuthorAutoRepo     sqlr.Repository[int64, testAuthorDeepAutoPreload]
	profileAutoPreloadRepo sqlr.Repository[int64, testAuthorWithProfileAutoPreload]
	articleAutoPreloadRepo sqlr.Repository[int64, testArticleAutoPreload]
	postWithAuthorAutoRepo sqlr.Repository[int64, testPostWithAuthorAutoPreload]
}

func TestRepositoryReadTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryReadTestSuite))
}

func (s *RepositoryReadTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.customPkRepo = mustNewRepo[int64, testCustomPkUser](s.T(), s.client)
	s.stringKeyRepo = mustNewRepo[string, testStringKeyUser](s.T(), s.client)
	s.boolKeyRepo = mustNewRepo[bool, testBoolKeyUser](s.T(), s.client)
	s.floatKeyRepo = mustNewRepo[float64, testFloatKeyUser](s.T(), s.client)
	s.authorAutoPreloadRepo = mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
	s.deepAuthorAutoRepo = mustNewRepo[int64, testAuthorDeepAutoPreload](s.T(), s.client)
	s.profileAutoPreloadRepo = mustNewRepo[int64, testAuthorWithProfileAutoPreload](s.T(), s.client)
	s.articleAutoPreloadRepo = mustNewRepo[int64, testArticleAutoPreload](s.T(), s.client)
	s.postWithAuthorAutoRepo = mustNewRepo[int64, testPostWithAuthorAutoPreload](s.T(), s.client)
}

func (s *RepositoryReadTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Basic Read Operations
// ==========================================================================

func (s *RepositoryReadTestSuite) TestRead_Success() {
	now := time.Now()

	// readEntity builds: SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(1, now, now, "Alice", "alice@test.com"))

	result, err := s.repo.Read(context.Background(), 1)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(1), result.GetId())
	s.Equal("Alice", result.Name)
	s.Equal("alice@test.com", result.Email)
}

func (s *RepositoryReadTestSuite) TestRead_NotFound() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(999), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns))

	result, err := s.repo.Read(context.Background(), 999)

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=999")
}

func (s *RepositoryReadTestSuite) TestRead_CustomPrimaryKey() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_id`, `created_at`, `updated_at`, `name` FROM `test_custom_pk_users` WHERE `user_id` = ? LIMIT ?")).
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows(testCustomPkUserColumns).
			AddRow(42, now, now, "Custom PK"))

	result, err := s.customPkRepo.Read(context.Background(), 42)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(42), result.Id)
	s.Equal("Custom PK", result.Name)
}

// ==========================================================================
// Key Type Variations
// ==========================================================================

func (s *RepositoryReadTestSuite) TestRead_StringPrimaryKey() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_string_key_users` WHERE `id` = ? LIMIT ?")).
		WithArgs("string-id", 1).
		WillReturnRows(sqlmock.NewRows(testKeyUserColumns).
			AddRow("string-id", now, now, "String User"))

	result, err := s.stringKeyRepo.Read(context.Background(), "string-id")

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("string-id", result.GetId())
	s.Equal("String User", result.Name)
}

func (s *RepositoryReadTestSuite) TestRead_BoolPrimaryKey() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_bool_key_users` WHERE `id` = ? LIMIT ?")).
		WithArgs(true, 1).
		WillReturnRows(sqlmock.NewRows(testKeyUserColumns).
			AddRow(true, now, now, "Bool User"))

	result, err := s.boolKeyRepo.Read(context.Background(), true)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.True(result.GetId())
	s.Equal("Bool User", result.Name)
}

func (s *RepositoryReadTestSuite) TestRead_FloatPrimaryKey() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_float_key_users` WHERE `id` = ? LIMIT ?")).
		WithArgs(float64(10), 1).
		WillReturnRows(sqlmock.NewRows(testKeyUserColumns).
			AddRow(float64(10), now, now, "Float User"))

	result, err := s.floatKeyRepo.Read(context.Background(), float64(10))

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(float64(10), result.GetId())
	s.Equal("Float User", result.Name)
}

// ==========================================================================
// Auto-Preloads on Read (preload tag)
// ==========================================================================

func (s *RepositoryReadTestSuite) TestRead_AutoPreloadHasMany() {
	now := time.Now()

	// Main read query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_author_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorAutoPreloadColumns).
			AddRow(1, now, now, "Alice"))

	// Auto-preload query for Posts (triggered by "preload" tag).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published").
			AddRow(11, now, now, 1, "Second Post", "draft"))

	result, err := s.authorAutoPreloadRepo.Read(context.Background(), 1)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthorAutoPreload(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
			{Title: ptr("Second Post")},
		},
		Comments: []expectedComment{},
	})
}

func (s *RepositoryReadTestSuite) TestRead_AutoPreloadHasOne() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_author_with_profile_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorWithProfileAutoPreloadColumns).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_profiles` WHERE `test_profiles`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(10, now, now, int64(1), "Read profile"))

	result, err := s.profileAutoPreloadRepo.Read(context.Background(), 1)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice", result.Name)
	s.Equal(int64(10), result.Profile.GetId())
	s.Equal("Read profile", result.Profile.Bio)
}

func (s *RepositoryReadTestSuite) TestRead_AutoPreloadBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `author_id`, `title` FROM `test_post_with_author_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(1), "First Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	result, err := s.postWithAuthorAutoRepo.Read(context.Background(), 10)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("First Post", result.Title)
	s.Equal("Alice", result.Author.Name)
}

func (s *RepositoryReadTestSuite) TestRead_AutoPreloadNested() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_author_deep_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorAutoPreloadColumns).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_comments_auto_preloads` WHERE `test_post_with_comments_auto_preloads`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, 1, "First Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`post_id` IN (?)")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "Nested Comment"))

	result, err := s.deepAuthorAutoRepo.Read(context.Background(), 1)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 1)
	s.Require().Len(result.Posts[0].Comments, 1)
	s.Equal("Nested Comment", result.Posts[0].Comments[0].Body)
}

func (s *RepositoryReadTestSuite) TestRead_NoAutoPreloadOnEntityWithoutTag() {
	now := time.Now()

	// testUser has no relationships, so Read should NOT issue any preload queries.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(1, now, now, "Alice", "alice@test.com"))

	result, err := s.repo.Read(context.Background(), 1)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice", result.Name)
	// sqlmock TearDownTest will verify no unexpected queries were executed.
}

func (s *RepositoryReadTestSuite) TestRead_AutoPreloadManyToMany() {
	now := time.Now()

	// Main read query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `title` FROM `test_article_auto_preloads` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(1, now, now, "My Article"))

	// Auto-preload M2M: first queries the join table.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `article_tags` WHERE `article_tags`.`test_article_auto_preload_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"test_article_auto_preload_id", "test_tag_id"}).
			AddRow(1, 100).
			AddRow(1, 200))

	// Then queries the related table for the matched IDs.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_tags` WHERE `test_tags`.`id` IN (?, ?)")).
		WithArgs(int64(100), int64(200)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(100, now, now, "Go").
			AddRow(200, now, now, "SQL"))

	result, err := s.articleAutoPreloadRepo.Read(context.Background(), 1)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertArticleAutoPreload(&s.Suite, *result, expectedArticle{
		Title: ptr("My Article"),
		Tags: []expectedTag{
			{Name: ptr("Go")},
			{Name: ptr("SQL")},
		},
	})
}

// ==========================================================================
// Prepared Statement Tests
// ==========================================================================

// RepositoryReadPreparedTestSuite tests the Repository Read operations with prepared statements.
type RepositoryReadPreparedTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
	repo   sqlr.Repository[int64, testUser]
}

func TestRepositoryReadPreparedTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryReadPreparedTestSuite))
}

func (s *RepositoryReadPreparedTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	settings := sqlr.Settings{PreparedStatements: true}
	s.repo = mustNewRepoWithSettings[int64, testUser](s.T(), s.client, settings)
}

func (s *RepositoryReadPreparedTestSuite) TearDownTest() {
	s.Require().NoError(s.repo.Close())
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func (s *RepositoryReadPreparedTestSuite) TestRead_PreparedStatement_Success() {
	now := time.Now()
	readSQL := "SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?"

	// Expect prepare on first call
	s.mock.ExpectPrepare(regexp.QuoteMeta(readSQL))

	s.mock.ExpectQuery(regexp.QuoteMeta(readSQL)).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(1, now, now, "Alice", "alice@test.com"))

	result1, err := s.repo.Read(context.Background(), 1)
	s.Require().NoError(err)
	s.Require().NotNil(result1)
	s.Equal(int64(1), result1.GetId())
	s.Equal("Alice", result1.Name)

	// Second call should reuse prepared statement (no ExpectPrepare)
	s.mock.ExpectQuery(regexp.QuoteMeta(readSQL)).
		WithArgs(int64(2), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(2, now, now, "Bob", "bob@test.com"))

	result2, err := s.repo.Read(context.Background(), 2)
	s.Require().NoError(err)
	s.Require().NotNil(result2)
	s.Equal(int64(2), result2.GetId())
	s.Equal("Bob", result2.Name)
}

func (s *RepositoryReadPreparedTestSuite) TestRead_PreparedStatement_NotFound() {
	readSQL := "SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `test_users` WHERE `id` = ? LIMIT ?"

	s.mock.ExpectPrepare(regexp.QuoteMeta(readSQL))

	s.mock.ExpectQuery(regexp.QuoteMeta(readSQL)).
		WithArgs(int64(999), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns))

	result, err := s.repo.Read(context.Background(), 999)
	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}

// ==========================================================================
// Read with Joins and Preloads
// ==========================================================================

// RepositoryReadWithRelationsTestSuite tests Read with explicit joins and preloads
// via the QueryBuilderRead options.
type RepositoryReadWithRelationsTestSuite struct {
	suite.Suite
	client                sqlc.Client
	mock                  sqlmock.Sqlmock
	authorRepo            sqlr.Repository[int64, testAuthor]
	authorWithProfileRepo sqlr.Repository[int64, testAuthorWithProfile]
	postRepo              sqlr.Repository[int64, testPost]
	articleRepo           sqlr.Repository[int64, testArticle]
	authorAutoPreloadRepo sqlr.Repository[int64, testAuthorAutoPreload]
}

func TestRepositoryReadWithRelationsTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryReadWithRelationsTestSuite))
}

func (s *RepositoryReadWithRelationsTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.client = client
	s.mock = mock

	s.authorRepo = mustNewRepo[int64, testAuthor](s.T(), s.client)
	s.authorWithProfileRepo = mustNewRepo[int64, testAuthorWithProfile](s.T(), s.client)
	s.postRepo = mustNewRepo[int64, testPost](s.T(), s.client)
	s.articleRepo = mustNewRepo[int64, testArticle](s.T(), s.client)
	s.authorAutoPreloadRepo = mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
}

func (s *RepositoryReadWithRelationsTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// --------------------------------------------------------------------------
// Preload Tests
// --------------------------------------------------------------------------

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithPreload() {
	now := time.Now()

	// Main query: simple SELECT with WHERE pk = ? LIMIT 1.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload query for Posts.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published").
			AddRow(11, now, now, 1, "Second Post", "draft"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Posts")
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

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithPreloadCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload with condition: only published posts.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?) AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Posts", sqlr.Condition("status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithNestedPreload() {
	now := time.Now()

	// Main query for author.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload posts.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	// Preload comments for post.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`post_id` IN (?)")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "Great post!"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Posts.Comments")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice", result.Name)
	s.Require().Len(result.Posts, 1)
	s.Require().Len(result.Posts[0].Comments, 1)
	s.Equal("Great post!", result.Posts[0].Comments[0].Body)
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithPreloadBelongsTo() {
	now := time.Now()

	// Main query for post.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`id` = ? LIMIT ?")).
		WithArgs(int64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, int64(1), "First Post", "published"))

	// Preload author (BelongsTo).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	result, err := s.postRepo.Read(context.Background(), 10, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Author")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("First Post", result.Title)
	s.Equal("Alice", result.Author.Name)
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithPreloadManyToMany() {
	now := time.Now()

	// Main query for article.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_articles` WHERE `test_articles`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(1, now, now, "My Article"))

	// M2M: query join table.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `article_tags` WHERE `article_tags`.`test_article_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"test_article_id", "test_tag_id"}).
			AddRow(1, 100).
			AddRow(1, 200))

	// M2M: query related tags.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_tags` WHERE `test_tags`.`id` IN (?, ?)")).
		WithArgs(int64(100), int64(200)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(100, now, now, "Go").
			AddRow(200, now, now, "SQL"))

	result, err := s.articleRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Tags")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertArticle(&s.Suite, *result, expectedArticle{
		Title: ptr("My Article"),
		Tags: []expectedTag{
			{Name: ptr("Go")},
			{Name: ptr("SQL")},
		},
	})
}

// --------------------------------------------------------------------------
// Join Tests
// --------------------------------------------------------------------------

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithLeftJoin() {
	now := time.Now()

	// Join query: SELECT with aliased columns + JOIN + WHERE pk = ? LIMIT 1.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL+" "+authorPostsLeftJoinSQL+" WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorPostsColumns).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), AuthorID: ptr(int64(1)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithLeftJoinCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL+" "+authorPostsLeftJoinSQL+" AND `test_posts`.`status` = ? WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs("published", int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorPostsColumns).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithInnerJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL+" "+authorPostsInnerJoinSQL+" WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorPostsColumns).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.InnerJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post")},
		},
	})
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithLeftJoinBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		postAuthorSelectSQL+" "+postAuthorLeftJoinSQL+" WHERE `test_posts`.`id` = ? LIMIT ?")).
		WithArgs(int64(10), 1).
		WillReturnRows(sqlmock.NewRows(testPostAuthorColumns).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	result, err := s.postRepo.Read(context.Background(), 10, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(10), result.GetId())
	s.Equal("First Post", result.Title)
	s.Equal("Alice", result.Author.Name)
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithLeftJoinHasOne() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorProfileSelectSQL+" "+authorProfileLeftJoinSQL+" WHERE `test_author_with_profiles`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorProfileColumns).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "A bio"))

	result, err := s.authorWithProfileRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Profile")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice", result.Name)
	s.Equal(int64(10), result.Profile.GetId())
	s.Equal("A bio", result.Profile.Bio)
}

// --------------------------------------------------------------------------
// Not Found with Relations
// --------------------------------------------------------------------------

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithLeftJoin_NotFound() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL+" "+authorPostsLeftJoinSQL+" WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(999), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorPostsColumns))

	result, err := s.authorRepo.Read(context.Background(), 999, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Posts")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=999")
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithPreload_NotFound() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(999), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns))

	result, err := s.authorRepo.Read(context.Background(), 999, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Posts")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
	s.Contains(err.Error(), "entity id=999")
}

// --------------------------------------------------------------------------
// Edge Cases
// --------------------------------------------------------------------------

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithEmptyOpts() {
	now := time.Now()

	// When opts are provided but don't add any relations, Read should use the fast path.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name` FROM `test_authors` WHERE `id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		// No joins or preloads added — fast path.
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice", result.Name)
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithAutoPreloadAndExplicitPreload() {
	now := time.Now()

	// Concurrent preloads at the same depth may execute in any order.
	s.mock.MatchExpectationsInOrder(false)

	// testAuthorAutoPreload has Posts auto-preloaded. Adding an explicit Preload("Comments")
	// should load both Posts (auto) and Comments (explicit), with deduplication.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads` WHERE `test_author_auto_preloads`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorAutoPreloadColumns).
			AddRow(1, now, now, "Alice"))

	// Explicit preload: Comments.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "A comment"))

	// Auto-preload: Posts (from "preload" tag on testAuthorAutoPreload).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	result, err := s.authorAutoPreloadRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Comments")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthorAutoPreload(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
		Comments: []expectedComment{
			{Body: ptr("A comment")},
		},
	})
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithLeftJoinMultiplePosts() {
	now := time.Now()

	// Author with multiple posts: JOIN returns multiple rows, deduplication should produce one author.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL+" "+authorPostsLeftJoinSQL+" WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorPostsColumns).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published").
			AddRow(1, now, now, "Alice", 11, now, now, int64(1), "Second Post", "draft"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
			{Id: ptr(int64(11)), Title: ptr("Second Post"), Status: ptr("draft")},
		},
	})
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithUnknownJoinRelation() {
	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.LeftJoin("Unknown")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), `join relation "Unknown" not found`)
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithUnknownPreloadRelation() {
	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Unknown")
	})

	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), `preload relation "Unknown" not found`)
}

func (s *RepositoryReadWithRelationsTestSuite) TestRead_WithMultiplePreloads() {
	now := time.Now()

	// Concurrent preloads at the same depth may execute in any order.
	s.mock.MatchExpectationsInOrder(false)

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` = ? LIMIT ?")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload Comments.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "A comment"))

	// Preload Posts.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	result, err := s.authorRepo.Read(context.Background(), 1, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Comments").Preload("Posts")
	})

	s.Require().NoError(err)
	s.Require().NotNil(result)
	assertAuthor(&s.Suite, *result, expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
		Comments: []expectedComment{
			{Body: ptr("A comment")},
		},
	})
}
