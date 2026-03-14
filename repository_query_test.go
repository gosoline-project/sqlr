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

// RepositoryQueryTestSuite tests the Repository Query operations using sqlmock,
// including simple queries and joins.
type RepositoryQueryTestSuite struct {
	suite.Suite
	ctx                   context.Context
	client                sqlc.Client
	mock                  sqlmock.Sqlmock
	repo                  sqlr.Repository[int64, testUser]
	postRepo              sqlr.Repository[int64, testPost]
	postPointerRepo       sqlr.Repository[int64, testPostWithAuthorPointer]
	authorRepo            sqlr.Repository[int64, testAuthor]
	authorWithProfileRepo sqlr.Repository[int64, testAuthorWithProfile]
	authorProfilePtrRepo  sqlr.Repository[int64, testAuthorWithProfilePointer]
	articleRepo           sqlr.Repository[int64, testArticle]
	articlePointerRepo    sqlr.Repository[int64, testArticleWithPointerTags]
	authorAutoPreloadRepo sqlr.Repository[int64, testAuthorAutoPreload]
}

func TestRepositoryQueryTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryQueryTestSuite))
}

func (s *RepositoryQueryTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.ctx = s.T().Context()
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.postRepo = mustNewRepo[int64, testPost](s.T(), s.client)
	s.postPointerRepo = mustNewRepo[int64, testPostWithAuthorPointer](s.T(), s.client)
	s.authorRepo = mustNewRepo[int64, testAuthor](s.T(), s.client)
	s.authorWithProfileRepo = mustNewRepo[int64, testAuthorWithProfile](s.T(), s.client)
	s.authorProfilePtrRepo = mustNewRepo[int64, testAuthorWithProfilePointer](s.T(), s.client)
	s.articleRepo = mustNewRepo[int64, testArticle](s.T(), s.client)
	s.articlePointerRepo = mustNewRepo[int64, testArticleWithPointerTags](s.T(), s.client)
	s.authorAutoPreloadRepo = mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
}

func (s *RepositoryQueryTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Simple Queries
// ==========================================================================

// TestQuery_Success verifies that a basic WHERE query returns all matching rows
// and maps each column to the correct struct field.
func (s *RepositoryQueryTestSuite) TestQuery_Success() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(1, now, now, "Alice", "alice@test.com").
			AddRow(2, now, now, "Alice", "alice2@test.com"))

	results, err := s.repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Alice")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.Equal("Alice", results[0].Name)
	s.Equal("alice@test.com", results[0].Email)
	s.Equal("Alice", results[1].Name)
	s.Equal("alice2@test.com", results[1].Email)
}

// TestQuery_ReusedQueryBuilderDoesNotAccumulateAutoPreloads verifies that
// reusing the same query-builder closure across two different repositories does
// not cause auto-preload state from the first call to leak into the second call.
func (s *RepositoryQueryTestSuite) TestQuery_ReusedQueryBuilderDoesNotAccumulateAutoPreloads() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, int64(1), "First Post", "published"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(1, now, now, "Alice", "alice@test.com"))

	whereAlice := func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Alice")
	}

	authors, err := s.authorAutoPreloadRepo.Query(s.ctx, whereAlice)
	s.Require().NoError(err)
	s.Require().Len(authors, 1)
	s.Require().Len(authors[0].Posts, 1)

	users, err := s.repo.Query(s.ctx, whereAlice)
	s.Require().NoError(err)
	s.Require().Len(users, 1)
	s.Equal("Alice", users[0].Name)
}

// TestQuery_EmptyResult verifies that a query matching no rows returns an empty
// slice and no error.
func (s *RepositoryQueryTestSuite) TestQuery_EmptyResult() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Nobody").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}))

	results, err := s.repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Nobody")
	})

	s.Require().NoError(err)
	s.Empty(results)
}

// TestQuery_WithLimitAndOffset verifies that LIMIT and OFFSET clauses are
// appended to the SQL query in the correct order and with the correct values.
func (s *RepositoryQueryTestSuite) TestQuery_WithLimitAndOffset() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ? LIMIT ? OFFSET ?")).
		WithArgs("Alice", 10, 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(1, now, now, "Alice", "alice@test.com"))

	results, err := s.repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Alice").
			Limit(10).
			Offset(5)
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
}

// TestQuery_WithOrderBy verifies that an ORDER BY clause is appended to the
// query and that the result set preserves the returned row order.
func (s *RepositoryQueryTestSuite) TestQuery_WithOrderBy() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ? ORDER BY `created_at` DESC")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(2, now, now, "Alice", "alice2@test.com").
			AddRow(1, now, now, "Alice", "alice@test.com"))

	results, err := s.repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Alice").
			OrderBy("created_at DESC")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
}

// TestQuery_Error verifies that a database error is propagated as a wrapped
// error containing a descriptive message.
func (s *RepositoryQueryTestSuite) TestQuery_Error() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnError(fmt.Errorf("connection lost"))

	results, err := s.repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Alice")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "failed to execute query")
}

// ==========================================================================
// Joins
// ==========================================================================

// TestQuery_LeftJoinWithoutCondition verifies that a LEFT JOIN without any extra
// ON condition generates the correct SELECT and FROM clauses and maps the joined
// columns into the relation field.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithoutCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), AuthorID: ptr(int64(1)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

// TestQuery_LeftJoinWithUnknownRelation verifies that referencing a relation
// name that does not exist on the entity returns a descriptive error.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithUnknownRelation() {
	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Unknown")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `join relation "Unknown" not found`)
}

// TestQuery_LeftJoinWithUnexpectedJoinedColumn verifies that when the database
// returns columns whose prefixes reference an unknown relation or don't match
// any known column, a descriptive mapping error is returned without a panic.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithUnexpectedJoinedColumn() {
	now := time.Now()
	columns := []string{
		"id", "created_at", "updated_at", "name",
		"Posts__id", "Posts__created_at", "Posts__updated_at",
		"Posts__author_id", "Posts__title", "Posts__status",
		"Broken__id", "unmapped_col",
	}

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`")).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published", 999, "ignored"))

	var results []testAuthor
	var err error
	s.NotPanics(func() {
		results, err = s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
			qb.LeftJoin("Posts")
		})
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `failed to map joined columns`)
	s.Contains(err.Error(), `references unknown relation "Broken"`)
	s.Contains(err.Error(), `column "unmapped_col" does not map to base entity columns or joined relations`)
}

// TestQuery_LeftJoinWithRelatedEntityWithoutPrimaryKey verifies that attempting
// to join a relation whose related entity type has no primary key field returns
// a descriptive error during schema resolution.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithRelatedEntityWithoutPrimaryKey() {
	repo, err := sqlr.NewRepositoryWithInterfaces[int64, testAuthorWithPostWithoutPrimaryKey](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	results, err := repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `failed to resolve schema for join relation "Posts"`)
	s.Contains(err.Error(), "related entity type testPostWithoutPrimaryKey has no primary key")
}

// TestQuery_LeftJoinWithCondition verifies that a single ON condition is
// appended to the LEFT JOIN clause and its argument is passed correctly.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`" +
			" AND `test_posts`.`status` = ?")).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

// TestQuery_LeftJoinWithMultipleConditions verifies that multiple ON conditions
// are each appended as separate AND clauses in the JOIN clause.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithMultipleConditions() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`" +
			" AND `test_posts`.`status` = ? AND `test_posts`.`title` IS NOT NULL")).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts",
			sqlr.Condition("test_posts.status = ?", "published"),
			sqlr.Condition("test_posts.title IS NOT NULL"),
		)
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

// TestQuery_LeftJoinWithParameterizedCondition verifies that a parameterized ON
// condition correctly binds its argument in the JOIN clause.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinWithParameterizedCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`" +
			" AND `test_posts`.`author_id` = ?")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(42), "First Post", "draft"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts", sqlr.Condition("test_posts.author_id = ?", int64(42)))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), AuthorID: ptr(int64(42)), Title: ptr("First Post"), Status: ptr("draft")},
		},
	})
}

// TestQuery_InnerJoin verifies that InnerJoin generates a JOIN (INNER JOIN)
// clause and maps the joined columns correctly.
func (s *RepositoryQueryTestSuite) TestQuery_InnerJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`" +
			" AND `test_posts`.`status` = ?")).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.InnerJoin("Posts", sqlr.Condition("test_posts.status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

// TestQuery_RightJoin verifies that RightJoin generates a RIGHT JOIN clause and
// maps the joined columns correctly.
func (s *RepositoryQueryTestSuite) TestQuery_RightJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` RIGHT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`" +
			" AND `test_posts`.`status` = ?")).
		WithArgs("draft").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Bob", 20, now, now, int64(1), "Draft Post", "draft"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.RightJoin("Posts", sqlr.Condition("test_posts.status = ?", "draft"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Bob", results[0].Name)
	s.Require().Len(results[0].Posts, 1)
	s.Equal(int64(20), results[0].Posts[0].Id)
	s.Equal("Draft Post", results[0].Posts[0].Title)
	s.Equal("draft", results[0].Posts[0].Status)
}

func (s *RepositoryQueryTestSuite) TestQuery_LeftJoin_DuplicateRelationReturnsError() {
	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts").LeftJoin("Posts")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `join relation "Posts" specified multiple times`)
}

func (s *RepositoryQueryTestSuite) TestQuery_LeftJoin_PointerPrimaryKeyDeduplicatesRows() {
	pointerRepo := mustNewRepo[*int64, testAuthorWithPointerKeyProfile](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_author_with_pointer_key_profiles`.`id`, `test_author_with_pointer_key_profiles`.`created_at`, `test_author_with_pointer_key_profiles`.`updated_at`, `test_author_with_pointer_key_profiles`.`name`, " +
			"`Profiles`.`id` AS `Profiles__id`, `Profiles`.`created_at` AS `Profiles__created_at`, `Profiles`.`updated_at` AS `Profiles__updated_at`, " +
			"`Profiles`.`author_id` AS `Profiles__author_id`, `Profiles`.`title` AS `Profiles__title`" +
			" FROM `test_author_with_pointer_key_profiles` LEFT JOIN `test_pointer_children` AS Profiles ON `test_author_with_pointer_key_profiles`.`id` = `Profiles`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Profiles__id", "Profiles__created_at", "Profiles__updated_at", "Profiles__author_id", "Profiles__title"}).
			AddRow(int64(1), now, now, "Alice", int64(10), now, now, int64(1), "One").
			AddRow(int64(1), now, now, "Alice", int64(11), now, now, int64(1), "Two"))

	results, err := pointerRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Profiles")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().NotNil(results[0].Id)
	s.Equal(int64(1), *results[0].Id)
	s.Len(results[0].Profiles, 2)
}

// TestQuery_CrossJoin verifies the current CrossJoin behavior: it reuses the
// relation-aware JOIN implementation and therefore emits an inner JOIN with the
// relation key predicate instead of a Cartesian product.
func (s *RepositoryQueryTestSuite) TestQuery_CrossJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "Post A", "published").
			AddRow(2, now, now, "Bob", 20, now, now, int64(2), "Post B", "draft"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.CrossJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("Post A")},
		},
	})
	assertAuthor(&s.Suite, results[1], expectedAuthor{
		Name: ptr("Bob"),
		Posts: []expectedPost{
			{Id: ptr(int64(20)), Title: ptr("Post B")},
		},
	})
}

// TestQuery_JoinWithWhere verifies that a WHERE clause on the parent query is
// correctly placed after the JOIN clause when both are combined.
func (s *RepositoryQueryTestSuite) TestQuery_JoinWithWhere() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, "+
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, "+
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`"+
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`"+
			" AND `test_posts`.`status` = ? WHERE name = ?")).
		WithArgs("published", "Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published")).
			Where("name = ?", "Alice")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

// TestQuery_JoinWithOrderBy verifies that an ORDER BY clause is correctly placed
// after the JOIN clause and that the result set reflects the returned row order.
func (s *RepositoryQueryTestSuite) TestQuery_JoinWithOrderBy() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`" +
			" ORDER BY `created_at` DESC")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(2, now, now, "Bob", 20, now, now, int64(2), "Bob's Post", "draft").
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "Alice's Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts").
			OrderBy("created_at DESC")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Bob"),
		Posts: []expectedPost{
			{Id: ptr(int64(20)), Title: ptr("Bob's Post")},
		},
	})
	assertAuthor(&s.Suite, results[1], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("Alice's Post")},
		},
	})
}

// TestQuery_MultipleJoins verifies that two simultaneous LEFT JOINs are sorted
// alphabetically in the SQL and that both joined relations are mapped correctly
// into the same parent entity.
func (s *RepositoryQueryTestSuite) TestQuery_MultipleJoins() {
	now := time.Now()

	// With multiple joins, joins are sorted alphabetically by name.
	// Comments comes before Posts.
	// SELECT clause: base author columns + Comments columns + Posts columns
	commentsSelectSQL := "`Comments`.`id` AS `Comments__id`, `Comments`.`created_at` AS `Comments__created_at`, " +
		"`Comments`.`updated_at` AS `Comments__updated_at`, `Comments`.`author_id` AS `Comments__author_id`, " +
		"`Comments`.`post_id` AS `Comments__post_id`, `Comments`.`body` AS `Comments__body`"

	postsSelectSQL := "`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, " +
		"`Posts`.`updated_at` AS `Posts__updated_at`, `Posts`.`author_id` AS `Posts__author_id`, " +
		"`Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`"

	fullSelectSQL := "SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
		commentsSelectSQL + ", " + postsSelectSQL

	// FROM + JOIN clauses: base table + LEFT JOIN Comments + LEFT JOIN Posts (with condition)
	fullFromSQL := "FROM `test_authors` LEFT JOIN `test_comments` AS Comments ON `test_authors`.`id` = `Comments`.`author_id`" +
		" LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id` AND `test_posts`.`status` = ?"

	s.mock.ExpectQuery(regexp.QuoteMeta(fullSelectSQL + " " + fullFromSQL)).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Comments__id", "Comments__created_at", "Comments__updated_at", "Comments__author_id", "Comments__post_id", "Comments__body", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 100, now, now, int64(1), int64(10), "Great work!", 10, now, now, int64(1), "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published")).
			LeftJoin("Comments")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Comments: []expectedComment{
			{Id: ptr(int64(100)), AuthorID: ptr(int64(1)), Body: ptr("Great work!")},
		},
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post"), Status: ptr("published")},
		},
	})
}

// TestQuery_LeftJoinMultipleRelated verifies that multiple rows sharing the same
// parent primary key are collected into a single parent entity's relation slice.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinMultipleRelated() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 10, now, now, int64(1), "First Post", "published").
			AddRow(1, now, now, "Alice", 11, now, now, int64(1), "Second Post", "draft").
			AddRow(1, now, now, "Alice", 12, now, now, int64(1), "Third Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("First Post")},
			{Id: ptr(int64(11)), Title: ptr("Second Post")},
			{Id: ptr(int64(12)), Title: ptr("Third Post")},
		},
	})
}

// TestQuery_LeftJoinMultipleRelations verifies that joining two relations at
// once produces a Cartesian product in the result set and that duplicate rows
// are deduplicated correctly into distinct slices on the parent entity.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinMultipleRelations() {
	now := time.Now()

	// SELECT clause: base author columns + Comments columns + Posts columns
	commentsSelectSQL := "`Comments`.`id` AS `Comments__id`, `Comments`.`created_at` AS `Comments__created_at`, " +
		"`Comments`.`updated_at` AS `Comments__updated_at`, `Comments`.`author_id` AS `Comments__author_id`, " +
		"`Comments`.`post_id` AS `Comments__post_id`, `Comments`.`body` AS `Comments__body`"

	postsSelectSQL := "`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, " +
		"`Posts`.`updated_at` AS `Posts__updated_at`, `Posts`.`author_id` AS `Posts__author_id`, " +
		"`Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`"

	fullSelectSQL := "SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
		commentsSelectSQL + ", " + postsSelectSQL

	// FROM + JOIN clauses: base table + LEFT JOIN Comments + LEFT JOIN Posts
	fullFromSQL := "FROM `test_authors` LEFT JOIN `test_comments` AS Comments ON `test_authors`.`id` = `Comments`.`author_id`" +
		" LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`"

	s.mock.ExpectQuery(regexp.QuoteMeta(fullSelectSQL + " " + fullFromSQL)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Comments__id", "Comments__created_at", "Comments__updated_at", "Comments__author_id", "Comments__post_id", "Comments__body", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, now, now, "Alice", 100, now, now, int64(1), int64(10), "First Comment", 10, now, now, int64(1), "Post A", "published").
			AddRow(1, now, now, "Alice", 100, now, now, int64(1), int64(10), "First Comment", 11, now, now, int64(1), "Post B", "draft").
			AddRow(1, now, now, "Alice", 101, now, now, int64(1), int64(11), "Second Comment", 10, now, now, int64(1), "Post A", "published").
			AddRow(1, now, now, "Alice", 101, now, now, int64(1), int64(11), "Second Comment", 11, now, now, int64(1), "Post B", "draft"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts").
			LeftJoin("Comments")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Comments: []expectedComment{
			{Id: ptr(int64(100)), Body: ptr("First Comment")},
			{Id: ptr(int64(101)), Body: ptr("Second Comment")},
		},
		Posts: []expectedPost{
			{Id: ptr(int64(10)), Title: ptr("Post A")},
			{Id: ptr(int64(11)), Title: ptr("Post B")},
		},
	})
}

// TestQuery_LeftJoinNoRelated verifies that when all joined columns contain
// zero values (i.e. the LEFT JOIN found no match), the relation slice on the
// parent entity is left empty rather than containing a zero-value element.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinNoRelated() {
	zeroTime := time.Time{}

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(1, zeroTime, zeroTime, "Alice", int64(0), zeroTime, zeroTime, int64(0), "", ""))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Empty(results[0].Posts)
}

// TestQuery_LeftJoinHasOne verifies that a HasOne relation is correctly mapped
// from joined columns into a non-slice struct field.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinHasOne() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_author_with_profiles`.`id`, `test_author_with_profiles`.`created_at`, `test_author_with_profiles`.`updated_at`, `test_author_with_profiles`.`name`, " +
			"`Profile`.`id` AS `Profile__id`, `Profile`.`created_at` AS `Profile__created_at`, `Profile`.`updated_at` AS `Profile__updated_at`, " +
			"`Profile`.`author_id` AS `Profile__author_id`, `Profile`.`bio` AS `Profile__bio`" +
			" FROM `test_author_with_profiles` LEFT JOIN `test_profiles` AS Profile ON `test_author_with_profiles`.`id` = `Profile`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Profile__id", "Profile__created_at", "Profile__updated_at", "Profile__author_id", "Profile__bio"}).
			AddRow(1, now, now, "Alice", 100, now, now, int64(1), "Go engineer"))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Profile")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(int64(100), results[0].Profile.GetId())
	s.Equal(int64(1), results[0].Profile.AuthorID)
	s.Equal("Go engineer", results[0].Profile.Bio)
}

// TestQuery_LeftJoinHasOneNoRelated verifies that when all joined columns for a
// HasOne relation are zero values, the field is left as its zero value rather
// than being populated with an empty struct.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinHasOneNoRelated() {
	zeroTime := time.Time{}

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_author_with_profiles`.`id`, `test_author_with_profiles`.`created_at`, `test_author_with_profiles`.`updated_at`, `test_author_with_profiles`.`name`, " +
			"`Profile`.`id` AS `Profile__id`, `Profile`.`created_at` AS `Profile__created_at`, `Profile`.`updated_at` AS `Profile__updated_at`, " +
			"`Profile`.`author_id` AS `Profile__author_id`, `Profile`.`bio` AS `Profile__bio`" +
			" FROM `test_author_with_profiles` LEFT JOIN `test_profiles` AS Profile ON `test_author_with_profiles`.`id` = `Profile`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Profile__id", "Profile__created_at", "Profile__updated_at", "Profile__author_id", "Profile__bio"}).
			AddRow(1, zeroTime, zeroTime, "Alice", int64(0), zeroTime, zeroTime, int64(0), ""))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Profile")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(testProfile{}, results[0].Profile)
}

// TestQuery_LeftJoinHasOneMultipleRows verifies that when multiple rows are
// returned for a HasOne relation (e.g. due to a broad join), only the first
// encountered row is used and subsequent rows are silently discarded.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinHasOneMultipleRows() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_author_with_profiles`.`id`, `test_author_with_profiles`.`created_at`, `test_author_with_profiles`.`updated_at`, `test_author_with_profiles`.`name`, " +
			"`Profile`.`id` AS `Profile__id`, `Profile`.`created_at` AS `Profile__created_at`, `Profile`.`updated_at` AS `Profile__updated_at`, " +
			"`Profile`.`author_id` AS `Profile__author_id`, `Profile`.`bio` AS `Profile__bio`" +
			" FROM `test_author_with_profiles` LEFT JOIN `test_profiles` AS Profile ON `test_author_with_profiles`.`id` = `Profile`.`author_id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Profile__id", "Profile__created_at", "Profile__updated_at", "Profile__author_id", "Profile__bio"}).
			AddRow(1, now, now, "Alice", 100, now, now, int64(1), "First profile").
			AddRow(1, now, now, "Alice", 101, now, now, int64(1), "Second profile"))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Profile")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(int64(100), results[0].Profile.GetId())
	s.Equal("First profile", results[0].Profile.Bio)
}

// TestQuery_InnerJoinHasOne verifies that InnerJoin works correctly for a HasOne
// relation and that an ON condition filters the joined rows.
func (s *RepositoryQueryTestSuite) TestQuery_InnerJoinHasOne() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_author_with_profiles`.`id`, `test_author_with_profiles`.`created_at`, `test_author_with_profiles`.`updated_at`, `test_author_with_profiles`.`name`, " +
			"`Profile`.`id` AS `Profile__id`, `Profile`.`created_at` AS `Profile__created_at`, `Profile`.`updated_at` AS `Profile__updated_at`, " +
			"`Profile`.`author_id` AS `Profile__author_id`, `Profile`.`bio` AS `Profile__bio`" +
			" FROM `test_author_with_profiles` JOIN `test_profiles` AS Profile ON `test_author_with_profiles`.`id` = `Profile`.`author_id`" +
			" AND `test_profiles`.`bio` <> ?")).
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Profile__id", "Profile__created_at", "Profile__updated_at", "Profile__author_id", "Profile__bio"}).
			AddRow(1, now, now, "Alice", 100, now, now, int64(1), "Go engineer"))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.InnerJoin("Profile", sqlr.Condition("test_profiles.bio <> ?", ""))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(int64(100), results[0].Profile.GetId())
	s.Equal("Go engineer", results[0].Profile.Bio)
}

// TestQuery_LeftJoinBelongsTo verifies that a BelongsTo relation is correctly
// resolved via LEFT JOIN, using the child's foreign key to join to the parent
// table, and that all columns are mapped correctly.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_posts` LEFT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal(int64(10), results[0].GetId())
	s.Equal(int64(1), results[0].AuthorID)
	s.Equal("First Post", results[0].Title)
	s.Equal("published", results[0].Status)
	s.Equal(int64(1), results[0].Author.GetId())
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_LeftJoinBelongsToWithCondition verifies that a condition on a
// BelongsTo LEFT JOIN is correctly appended to the ON clause.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinBelongsToWithCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_posts` LEFT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`" +
			" AND `test_authors`.`name` = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Author", sqlr.Condition("test_authors.name = ?", "Alice"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_InnerJoinBelongsTo verifies that InnerJoin works correctly for a
// BelongsTo relation, generating an INNER JOIN clause.
func (s *RepositoryQueryTestSuite) TestQuery_InnerJoinBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_posts` JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.InnerJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal(int64(10), results[0].GetId())
	s.Equal(int64(1), results[0].Author.GetId())
}

// TestQuery_RightJoinBelongsTo verifies that RightJoin works correctly for a
// BelongsTo relation, generating a RIGHT JOIN clause.
func (s *RepositoryQueryTestSuite) TestQuery_RightJoinBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_posts` RIGHT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.RightJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_CrossJoinBelongsTo verifies the current CrossJoin behavior for a
// BelongsTo relation, which also uses an inner JOIN with the relation key
// predicate.
func (s *RepositoryQueryTestSuite) TestQuery_CrossJoinBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_posts` JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.CrossJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_LeftJoinBelongsToPointer verifies that join hydration populates a
// pointer belongsTo relation field.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinBelongsToPointer() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_post_with_author_pointers`.`id`, `test_post_with_author_pointers`.`created_at`, `test_post_with_author_pointers`.`updated_at`, `test_post_with_author_pointers`.`author_id`, `test_post_with_author_pointers`.`title`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_post_with_author_pointers` LEFT JOIN `test_authors` AS Author ON `test_post_with_author_pointers`.`author_id` = `Author`.`id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", 1, now, now, "Alice"))

	results, err := s.postPointerRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().NotNil(results[0].Author)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_LeftJoinBelongsToNoRelated verifies that when all joined columns for
// a BelongsTo relation are zero values (no matching parent row), the relation
// field is left as its zero value without error.
func (s *RepositoryQueryTestSuite) TestQuery_LeftJoinBelongsToNoRelated() {
	now := time.Now()
	zeroTime := time.Time{}

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`" +
			" FROM `test_posts` LEFT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(0), "Orphan Post", "draft", int64(0), zeroTime, zeroTime, ""))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal(int64(10), results[0].GetId())
	s.Equal(testAuthor{}, results[0].Author)
}

// TestQuery_JoinBelongsToWithWhere verifies that a WHERE clause on the main
// query is correctly placed after the BelongsTo JOIN clause when both are used.
func (s *RepositoryQueryTestSuite) TestQuery_JoinBelongsToWithWhere() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, "+
			"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`"+
			" FROM `test_posts` LEFT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`"+
			" AND `test_authors`.`name` = ? WHERE status = ?")).
		WithArgs("Alice", "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status", "Author__id", "Author__created_at", "Author__updated_at", "Author__name"}).
			AddRow(10, now, now, int64(1), "First Post", "published", 1, now, now, "Alice"))

	results, err := s.postRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Author", sqlr.Condition("test_authors.name = ?", "Alice")).
			Where("status = ?", "published")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("First Post", results[0].Title)
	s.Equal("Alice", results[0].Author.Name)
}

// ==========================================================================
// Many-to-Many Joins
// ==========================================================================

// TestQuery_ManyToManyJoinNotSupported verifies that attempting to use any join
// method on a ManyToMany relation returns a descriptive error, as join-based
// loading is not supported for many-to-many associations.
func (s *RepositoryQueryTestSuite) TestQuery_ManyToManyJoinNotSupported() {
	results, err := s.articleRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Tags")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "many-to-many association which is not supported")
}

// ==========================================================================
// Nested/Dotted Joins
// ==========================================================================

// TestQuery_NestedJoinNotSupported verifies that a dot-separated join path
// (e.g. "Posts.Comments") is rejected with a descriptive error that points the
// caller to use Preload instead.
func (s *RepositoryQueryTestSuite) TestQuery_NestedJoinNotSupported() {
	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts.Comments")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "nested join relation")
	s.Contains(err.Error(), "use Preload")
}

// TestQuery_NestedJoinNotSupported_InnerJoin verifies the same nested-path
// restriction as TestQuery_NestedJoinNotSupported but via InnerJoin, confirming
// the guard applies to all join types.
func (s *RepositoryQueryTestSuite) TestQuery_NestedJoinNotSupported_InnerJoin() {
	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.InnerJoin("Posts.Comments")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "nested join relation")
	s.Contains(err.Error(), "use Preload")
}
