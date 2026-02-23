package sqlr_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

// RepositoryPreloadTestSuite tests the Repository preload operations using sqlmock,
// including simple preloads, preloads with conditions, many-to-many preloads,
// and auto-preloads via the "preload" struct tag.
type RepositoryPreloadTestSuite struct {
	suite.Suite
	ctx                    context.Context
	client                 sqlc.Client
	mock                   sqlmock.Sqlmock
	authorRepo             sqlr.Repository[int64, testAuthor]
	authorWithProfileRepo  sqlr.Repository[int64, testAuthorWithProfile]
	postWithAuthorRepo     sqlr.Repository[int64, testPostWithAuthor]
	postWithNullableAuthor sqlr.Repository[int64, testPostWithNullableAuthor]
	postWithAuthorAutoRepo sqlr.Repository[int64, testPostWithAuthorAutoPreload]
	brokenAuthorRepo       sqlr.Repository[int64, testBrokenAuthor]
	articleRepo            sqlr.Repository[int64, testArticle]
	articleUint64Repo      sqlr.Repository[uint64, testUint64Article]
	articleStringRepo      sqlr.Repository[string, testStringArticle]
	authorAutoPreloadRepo  sqlr.Repository[int64, testAuthorAutoPreload]
	deepAuthorAutoRepo     sqlr.Repository[int64, testAuthorDeepAutoPreload]
	profileAutoPreloadRepo sqlr.Repository[int64, testAuthorWithProfileAutoPreload]
	articleAutoPreloadRepo sqlr.Repository[int64, testArticleAutoPreload]
}

func TestRepositoryPreloadTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryPreloadTestSuite))
}

func (s *RepositoryPreloadTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.ctx = s.T().Context()
	s.client = client
	s.mock = mock

	s.authorRepo = mustNewRepo[int64, testAuthor](s.T(), s.client)
	s.authorWithProfileRepo = mustNewRepo[int64, testAuthorWithProfile](s.T(), s.client)
	s.postWithAuthorRepo = mustNewRepo[int64, testPostWithAuthor](s.T(), s.client)
	s.postWithNullableAuthor = mustNewRepo[int64, testPostWithNullableAuthor](s.T(), s.client)
	s.postWithAuthorAutoRepo = mustNewRepo[int64, testPostWithAuthorAutoPreload](s.T(), s.client)
	s.brokenAuthorRepo = mustNewRepo[int64, testBrokenAuthor](s.T(), s.client)
	s.articleRepo = mustNewRepo[int64, testArticle](s.T(), s.client)
	s.articleUint64Repo = mustNewRepo[uint64, testUint64Article](s.T(), s.client)
	s.articleStringRepo = mustNewRepo[string, testStringArticle](s.T(), s.client)
	s.authorAutoPreloadRepo = mustNewRepo[int64, testAuthorAutoPreload](s.T(), s.client)
	s.deepAuthorAutoRepo = mustNewRepo[int64, testAuthorDeepAutoPreload](s.T(), s.client)
	s.profileAutoPreloadRepo = mustNewRepo[int64, testAuthorWithProfileAutoPreload](s.T(), s.client)
	s.articleAutoPreloadRepo = mustNewRepo[int64, testArticleAutoPreload](s.T(), s.client)
}

func (s *RepositoryPreloadTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Preloads
// ==========================================================================

// TestQuery_PreloadWithoutCondition verifies that a HasMany relation (Posts) is
// loaded via a secondary IN-query when a single parent record is returned.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithoutCondition() {
	now := time.Now()

	// First: main query for authors.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Second: preload query for posts belonging to found authors.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
	})
}

// TestQuery_PreloadWithoutCondition_MultipleParents verifies that HasMany preload
// correctly distributes related records across multiple parent entities using a
// single batched IN-query.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithoutCondition_MultipleParents() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice").
			AddRow(2, now, now, "Bob"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?, ?)")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(20, now, now, 2, "Bob Post 1", "published").
			AddRow(10, now, now, 1, "Alice Post", "published").
			AddRow(21, now, now, 2, "Bob Post 2", "draft"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.Equal("Alice", results[0].Name)
	s.Require().Len(results[0].Posts, 1)
	s.Equal("Alice Post", results[0].Posts[0].Title)

	s.Equal("Bob", results[1].Name)
	s.Require().Len(results[1].Posts, 2)
	s.Equal("Bob Post 1", results[1].Posts[0].Title)
	s.Equal("Bob Post 2", results[1].Posts[1].Title)
}

// TestQuery_PreloadHasOne verifies that a HasOne relation (Profile) is loaded
// for a single parent record and mapped to the struct field correctly.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadHasOne() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_with_profiles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_profiles` WHERE `test_profiles`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(10, now, now, 1, "Alice profile"))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Profile")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(int64(10), results[0].Profile.GetId())
	s.Equal(int64(1), results[0].Profile.AuthorID)
	s.Equal("Alice profile", results[0].Profile.Bio)
}

// TestQuery_PreloadHasOneMultipleParents verifies that a HasOne relation is
// correctly distributed across multiple parent records in a single batched query.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadHasOneMultipleParents() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_with_profiles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice").
			AddRow(2, now, now, "Bob"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_profiles` WHERE `test_profiles`.`author_id` IN (?, ?)")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(20, now, now, 2, "Bob profile").
			AddRow(10, now, now, 1, "Alice profile"))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Profile")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.Equal("Alice", results[0].Name)
	s.Equal(int64(10), results[0].Profile.GetId())
	s.Equal("Alice profile", results[0].Profile.Bio)
	s.Equal("Bob", results[1].Name)
	s.Equal(int64(20), results[1].Profile.GetId())
	s.Equal("Bob profile", results[1].Profile.Bio)
}

// TestQuery_PreloadHasOneNoRelated verifies that when no related record exists for
// a HasOne relation, the field is left as its zero value without error.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadHasOneNoRelated() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_with_profiles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_profiles` WHERE `test_profiles`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}))

	results, err := s.authorWithProfileRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Profile")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(testProfile{}, results[0].Profile)
}

// TestQuery_PreloadBelongsTo verifies that a BelongsTo relation (Author) is
// loaded for a single child record by querying the parent table using the
// foreign key value stored on the child.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(1), "First Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	results, err := s.postWithAuthorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("First Post", results[0].Title)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_PreloadBelongsToMultipleParents verifies that BelongsTo preload
// deduplicates foreign keys across multiple child records, issues a single
// batched query, and handles a zero-value foreign key (no author assigned).
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadBelongsToMultipleParents() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(1), "Post 1").
			AddRow(11, now, now, int64(1), "Post 2").
			AddRow(12, now, now, int64(2), "Post 3").
			AddRow(13, now, now, int64(0), "Post 4"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?, ?)")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(2, now, now, "Bob").
			AddRow(1, now, now, "Alice"))

	results, err := s.postWithAuthorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 4)
	s.Equal("Alice", results[0].Author.Name)
	s.Equal("Alice", results[1].Author.Name)
	s.Equal("Bob", results[2].Author.Name)
	s.Equal(testAuthor{}, results[3].Author)
}

// TestQuery_PreloadBelongsToWithMultipleNilForeignKeys verifies that BelongsTo
// preload handles nullable foreign keys correctly: nil values are excluded from
// the IN-query and the corresponding relation field is left as its zero value.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadBelongsToWithMultipleNilForeignKeys() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_nullable_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(1), "Post 1").
			AddRow(11, now, now, nil, "Post 2").
			AddRow(12, now, now, nil, "Post 3"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	results, err := s.postWithNullableAuthor.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 3)
	s.Equal("Alice", results[0].Author.Name)
	s.Equal(testAuthor{}, results[1].Author)
	s.Equal(testAuthor{}, results[2].Author)
}

// TestQuery_PreloadBelongsToNoRelated verifies that when the parent record
// referenced by a BelongsTo foreign key does not exist, the relation field
// is left as its zero value without returning an error.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadBelongsToNoRelated() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(999), "Orphan Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}))

	results, err := s.postWithAuthorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Author")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal(testAuthor{}, results[0].Author)
}

// TestQuery_PreloadBelongsToWithCondition verifies that an extra WHERE condition
// passed to Preload is appended to the BelongsTo preload query.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadBelongsToWithCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(1), "First Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?) AND name = ?")).
		WithArgs(int64(1), "Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	results, err := s.postWithAuthorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Author", sqlr.Condition("name = ?", "Alice"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_PreloadWithUnknownRelation verifies that referencing a relation name
// that does not exist on the entity returns a descriptive error.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithUnknownRelation() {
	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Unknown")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `preload relation "Unknown" not found`)
}

// TestQuery_PreloadWithRelatedEntityWithoutPrimaryKey verifies that attempting to
// preload a relation whose related entity type has no primary key field returns
// a descriptive error during schema resolution.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithRelatedEntityWithoutPrimaryKey() {
	repo, err := sqlr.NewRepositoryWithInterfaces[int64, testAuthorWithPostWithoutPrimaryKey](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	results, err := repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `failed to resolve schema for preload relation "Posts"`)
	s.Contains(err.Error(), "related entity type testPostWithoutPrimaryKey has no primary key")
}

// TestQuery_PreloadWithForeignKeyNotMappedInRelatedStruct verifies that when the
// foreign key column declared in the relation tag is absent from the related
// struct's db tags, a descriptive mapping error is returned.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithForeignKeyNotMappedInRelatedStruct() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_broken_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_broken_posts` WHERE `test_broken_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, 1, "Broken Post"))

	results, err := s.brokenAuthorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts")
	})

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `failed to map preload foreign key column "author_id" for relation "Posts"`)
}

// TestQuery_PreloadWithCondition verifies that an extra WHERE condition passed to
// Preload is appended to the HasMany preload query.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithCondition() {
	now := time.Now()

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Preload query with additional WHERE condition.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?) AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "Published Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts", sqlr.Condition("status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("Published Post")},
		},
	})
}

// TestQuery_PreloadMultipleRelations verifies that two sibling preloads (Posts
// and Comments) are executed concurrently and both results are mapped correctly.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadMultipleRelations() {
	now := time.Now()

	// Concurrent preloads at the same depth may execute in any order.
	s.mock.MatchExpectationsInOrder(false)

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Preloads at the same depth execute concurrently; order is non-deterministic.
	// Posts.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	// Preload Comments.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "body"}).
			AddRow(20, now, now, 1, "A comment"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts").
			Preload("Comments")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
		Comments: []expectedComment{
			{Body: ptr("A comment")},
		},
	})
}

// TestQuery_PreloadWithWhere verifies that a WHERE clause on the main query
// correctly filters the parent records while the preload query still uses the
// IDs of only those filtered records.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadWithWhere() {
	now := time.Now()

	// Main query with WHERE.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Preload query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts").
			Where("name = ?", "Alice")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthor(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
	})
}

// TestQuery_PreloadManyToManyAllowed verifies that a ManyToMany relation (Tags)
// is loaded via two sequential queries: first the join table, then the related
// entity table.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadManyToManyAllowed() {
	now := time.Now()

	// Main query for articles.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_articles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(1, now, now, "My Article"))

	// Many-to-many preload step 1: Query the join table to find related tag IDs.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `article_tags` WHERE `article_tags`.`test_article_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"test_article_id", "test_tag_id"}).
			AddRow(1, 100))

	// Many-to-many preload step 2: Query the related table for the matched IDs.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_tags` WHERE `test_tags`.`id` IN (?)")).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(100, now, now, "Go"))

	results, err := s.articleRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Tags")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertArticle(&s.Suite, results[0], expectedArticle{
		Title: ptr("My Article"),
		Tags: []expectedTag{
			{Name: ptr("Go")},
		},
	})
}

// TestQuery_PreloadManyToManyWithUint64Key verifies that ManyToMany preload works
// correctly when the primary key type is uint64 rather than int64.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadManyToManyWithUint64Key() {
	now := time.Now()
	uint64ID := uint64(9001)

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_uint64articles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(uint64ID, now, now, "Large Article"))

	// Many-to-many preload step 1: Query join table.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `uint64_article_tags` WHERE `uint64_article_tags`.`test_uint64article_id` IN (?)")).
		WithArgs(uint64ID).
		WillReturnRows(sqlmock.NewRows([]string{"test_uint64article_id", "test_uint64tag_id"}).
			AddRow(uint64ID, uint64ID))

	// Many-to-many preload step 2: Query related tags.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_uint64tags` WHERE `test_uint64tags`.`id` IN (?)")).
		WithArgs(uint64ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(uint64ID, now, now, "BigTag"))

	results, err := s.articleUint64Repo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Tags")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Large Article", results[0].Title)
	s.Require().Len(results[0].Tags, 1)
	s.Equal("BigTag", results[0].Tags[0].Name)
	s.Equal(uint64ID, results[0].Tags[0].GetId())
}

// TestQuery_PreloadManyToManyWithEmptyStringKey verifies that ManyToMany preload
// handles a string primary key whose value is an empty string without skipping
// the record (empty string is a valid key, unlike nil).
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadManyToManyWithEmptyStringKey() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_string_articles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow("", now, now, "Untitled"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `string_article_tags` WHERE `string_article_tags`.`test_string_article_id` IN (?)")).
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"test_string_article_id", "test_string_tag_id"}).
			AddRow("", ""))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_string_tags` WHERE `test_string_tags`.`id` IN (?)")).
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow("", now, now, "EmptyTag"))

	results, err := s.articleStringRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Tags")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Untitled", results[0].Title)
	s.Require().Len(results[0].Tags, 1)
	s.Equal("EmptyTag", results[0].Tags[0].Name)
	s.Equal("", results[0].Tags[0].GetId())
}

// TestQuery_PreloadNested verifies that a dot-separated path ("Posts.Comments")
// loads an intermediate relation first and then applies preload conditions to
// the leaf relation only.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNested() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`post_id` IN (?) AND body = ?")).
		WithArgs(int64(10), "keep").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "keep"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts.Comments", sqlr.Condition("body = ?", "keep"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Comments, 1)
	s.Equal("keep", results[0].Posts[0].Comments[0].Body)
}

// TestQuery_PreloadNestedNoIntermediateResults verifies that when the
// intermediate relation returns no records, the leaf preload query is skipped
// entirely and the result is an empty slice without error.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNestedNoIntermediateResults() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts.Comments")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Empty(results[0].Posts)
}

// TestQuery_PreloadNestedThreeLevels verifies that a three-level dot-separated
// path ("Posts.Comments.Reactions") chains three sequential preload queries and
// maps all results into the correct nested struct fields.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNestedThreeLevels() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`post_id` IN (?)")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "Comment 1"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_reactions` WHERE `test_reactions`.`comment_id` IN (?)")).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "comment_id", "kind"}).
			AddRow(1000, now, now, 100, "like"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts.Comments.Reactions")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Comments, 1)
	s.Require().Len(results[0].Posts[0].Comments[0].Reactions, 1)
	s.Equal("like", results[0].Posts[0].Comments[0].Reactions[0].Kind)
}

// TestQuery_PreloadNestedMixed verifies that specifying both "Posts" and
// "Posts.Comments" as separate Preload calls merges correctly: Posts is loaded
// once and Comments is loaded as a nested preload beneath it.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNestedMixed() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`post_id` IN (?)")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "Nested Comment"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts").
			Preload("Posts.Comments")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Comments, 1)
	s.Equal("Nested Comment", results[0].Posts[0].Comments[0].Body)
}

// TestQuery_PreloadNestedMixedReverseOrder verifies that the order in which
// "Posts.Comments" and "Posts" are declared does not affect the outcome: the
// tree is merged the same way regardless of declaration order.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNestedMixedReverseOrder() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`post_id` IN (?)")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "post_id", "body"}).
			AddRow(100, now, now, 1, 10, "Nested Comment"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts.Comments").
			Preload("Posts")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Comments, 1)
	s.Equal("Nested Comment", results[0].Posts[0].Comments[0].Body)
}

// TestQuery_PreloadNestedBelongsToSegment verifies that a nested path starting
// with a BelongsTo segment ("Author.Comments") loads the parent via BelongsTo
// and then loads Comments via HasMany on the resolved parent records.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNestedBelongsToSegment() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, 1, "First Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "body"}).
			AddRow(100, now, now, 1, "Author Comment"))

	results, err := s.postWithAuthorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Author.Comments")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Author.Name)
	s.Require().Len(results[0].Author.Comments, 1)
	s.Equal("Author Comment", results[0].Author.Comments[0].Body)
}

// TestQuery_PreloadNestedManyToManySegment verifies that a ManyToMany relation
// can appear as a nested segment ("Posts.Tags"), executing the join-table and
// related-table queries beneath the intermediate HasMany preload.
func (s *RepositoryPreloadTestSuite) TestQuery_PreloadNestedManyToManySegment() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `post_tags` WHERE `post_tags`.`test_post_id` IN (?)")).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"test_post_id", "test_tag_id"}).
			AddRow(10, 100))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_tags` WHERE `test_tags`.`id` IN (?)")).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(100, now, now, "Go"))

	results, err := s.authorRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts.Tags")
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Tags, 1)
	s.Equal("Go", results[0].Posts[0].Tags[0].Name)
}

// ==========================================================================
// Auto-Preloads (preload tag)
// ==========================================================================

// TestQuery_AutoPreloadWithoutExplicitPreload verifies that a relation tagged
// with "preload" in the struct tag is automatically loaded without any explicit
// Preload() call on the query builder.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadWithoutExplicitPreload() {
	now := time.Now()

	// Main query for authors.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Auto-preload query for posts (triggered by "preload" tag, no explicit Preload() call).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	results, err := s.authorAutoPreloadRepo.Query(s.ctx)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthorAutoPreload(&s.Suite, results[0], expectedAuthor{
		Name: ptr("Alice"),
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
		Comments: []expectedComment{},
	})
}

// TestQuery_AutoPreloadHasOne verifies that a HasOne relation tagged with
// "preload" is automatically loaded without an explicit Preload() call.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadHasOne() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_with_profile_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_profiles` WHERE `test_profiles`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "bio"}).
			AddRow(10, now, now, 1, "Auto profile"))

	results, err := s.profileAutoPreloadRepo.Query(s.ctx)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Equal(int64(10), results[0].Profile.GetId())
	s.Equal("Auto profile", results[0].Profile.Bio)
}

// TestQuery_AutoPreloadBelongsTo verifies that a BelongsTo relation tagged with
// "preload" is automatically loaded without an explicit Preload() call.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadBelongsTo() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_post_with_author_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(10, now, now, int64(1), "First Post"))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE `test_authors`.`id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	results, err := s.postWithAuthorAutoRepo.Query(s.ctx)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Author.Name)
}

// TestQuery_AutoPreloadNested verifies that the "preload" tag is followed
// transitively: when a relation's own related type also has "preload"-tagged
// fields, those nested relations are loaded automatically as well.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadNested() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_deep_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
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

	results, err := s.deepAuthorAutoRepo.Query(s.ctx)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Comments, 1)
	s.Equal("Nested Comment", results[0].Posts[0].Comments[0].Body)
}

// TestQuery_AutoPreloadNestedMixed verifies that deep auto-preload does not
// exceed the tagged relation boundaries: relations without the "preload" tag on
// a nested entity (e.g. Reactions on Comment) are not loaded automatically.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadNestedMixed() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_deep_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
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

	results, err := s.deepAuthorAutoRepo.Query(s.ctx)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Require().Len(results[0].Posts[0].Comments, 1)
	s.Empty(results[0].Posts[0].Comments[0].Reactions)
}

// TestQuery_AutoPreloadDeduplication verifies that when an explicit Preload()
// call targets a relation that is also auto-preloaded via the struct tag, only
// one preload query is issued (no duplicate).
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadDeduplication() {
	now := time.Now()

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Only ONE preload query should execute (explicit takes precedence, no duplicate).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	results, err := s.authorAutoPreloadRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts") // explicit preload for a relation that also has the "preload" tag
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthorAutoPreload(&s.Suite, results[0], expectedAuthor{
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
	})
}

// TestQuery_AutoPreloadExplicitWithConditionTakesPrecedence verifies that when
// an explicit Preload() with a condition is provided for a relation that is also
// auto-preloaded, the explicit (conditioned) version takes precedence and only
// one query with the condition is executed.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadExplicitWithConditionTakesPrecedence() {
	now := time.Now()

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// The explicit Preload with condition takes precedence over the auto-preload.
	// Only ONE query with the condition should execute.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?) AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "Published Post", "published"))

	results, err := s.authorAutoPreloadRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Posts", sqlr.Condition("status = ?", "published"))
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Require().Len(results[0].Posts, 1)
	s.Equal("Published Post", results[0].Posts[0].Title)
}

// TestQuery_AutoPreloadMixedWithExplicit verifies that auto-preload and an
// explicit Preload() for a different relation coexist: both are executed and
// their results are merged into the result set.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadMixedWithExplicit() {
	now := time.Now()

	// Concurrent preloads at the same depth may execute in any order.
	s.mock.MatchExpectationsInOrder(false)

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(1, now, now, "Alice"))

	// Preloads at the same depth execute concurrently; order is non-deterministic.
	// Explicit preload for Comments (not auto-preloaded).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "body"}).
			AddRow(20, now, now, 1, "A comment"))

	// Auto-preload for Posts (from "preload" tag).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	results, err := s.authorAutoPreloadRepo.Query(s.ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Preload("Comments") // explicit preload for non-auto relation
	})

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertAuthorAutoPreload(&s.Suite, results[0], expectedAuthor{
		Posts: []expectedPost{
			{Title: ptr("First Post")},
		},
		Comments: []expectedComment{
			{Body: ptr("A comment")},
		},
	})
}

// TestQuery_AutoPreloadManyToMany verifies that a ManyToMany relation tagged
// with "preload" is automatically loaded via the two-step join-table query
// without any explicit Preload() call.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadManyToMany() {
	now := time.Now()

	// Main query for articles.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_article_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(1, now, now, "My Article"))

	// Auto-preload M2M: first queries the join table.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `article_tags` WHERE `article_tags`.`test_article_auto_preload_id` IN (?)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"test_article_auto_preload_id", "test_tag_id"}).
			AddRow(1, 100))

	// Then queries the related table for the matched IDs.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_tags` WHERE `test_tags`.`id` IN (?)")).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(100, now, now, "Go"))

	results, err := s.articleAutoPreloadRepo.Query(s.ctx) // no explicit Preload call

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	assertArticleAutoPreload(&s.Suite, results[0], expectedArticle{
		Title: ptr("My Article"),
		Tags: []expectedTag{
			{Name: ptr("Go")},
		},
	})
}

// TestQuery_AutoPreloadEmptyResultNoPreloadQuery verifies that when the main
// query returns no rows, no preload queries are issued at all, even for
// relations marked with the "preload" struct tag.
func (s *RepositoryPreloadTestSuite) TestQuery_AutoPreloadEmptyResultNoPreloadQuery() {
	// When the main query returns no results, preload queries should NOT execute.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_author_auto_preloads`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}))

	results, err := s.authorAutoPreloadRepo.Query(s.ctx)

	s.Require().NoError(err)
	s.Empty(results)
	// No preload queries should have been executed — sqlmock will fail in
	// TearDownTest if any unexpected queries were issued.
}
