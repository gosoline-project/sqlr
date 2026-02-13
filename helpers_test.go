package sqlr_test

import (
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/jmoiron/sqlx"
	"github.com/justtrackio/gosoline/pkg/exec"
	logMocks "github.com/justtrackio/gosoline/pkg/log/mocks"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ==========================================================================
// Custom Matchers
// ==========================================================================

// isTimestamp is a custom sqlmock.Argument matcher that asserts the value
// is a time.Time. It implements the sqlmock.Argument interface.
type isTimestamp struct{}

func (isTimestamp) Match(v driver.Value) bool {
	_, ok := v.(time.Time)

	return ok
}

// ==========================================================================
// Test Entity Types
// ==========================================================================

// testUser is a custom entity type used throughout the repository tests.
// The table name "test_users" is derived from the type name via snake_case + pluralize.
type testUser struct {
	sqlr.Entity[int64]
	Name  string `db:"name"`
	Email string `db:"email"`
}

type testStringKeyUser struct {
	sqlr.Entity[string]
	Name string `db:"name"`
}

type testBoolKeyUser struct {
	sqlr.Entity[bool]
	Name string `db:"name"`
}

type testFloatKeyUser struct {
	sqlr.Entity[float64]
	Name string `db:"name"`
}

// testCustomPkUser uses a non-standard primary key column name.
// Table name "test_custom_pk_users" is derived from the type name.
type testCustomPkUser struct {
	Id        int64     `db:"user_id,primaryKey"`
	CreatedAt time.Time `db:"created_at,autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at,autoUpdateTime"`
	Name      string    `db:"name"`
}

func (u *testCustomPkUser) SetId(id int64) {
	u.Id = id
}

func (u testCustomPkUser) GetId() int64 {
	return u.Id
}

func (u testCustomPkUser) GetUpdatedAt() time.Time {
	return u.UpdatedAt
}

func (u testCustomPkUser) GetCreatedAt() time.Time {
	return u.CreatedAt
}

// testPost is a related model used in join tests. Table: "test_posts".
type testPost struct {
	sqlr.Entity[int64]
	AuthorID int64         `db:"author_id"`
	Title    string        `db:"title"`
	Status   string        `db:"status"`
	Author   testAuthor    `db:"-,belongsTo:author_id"`
	Comments []testComment `db:"-,foreignKey:post_id"`
	Tags     []testTag     `db:"-,many2many:post_tags"`
}

type testPostWithAuthor struct {
	sqlr.Entity[int64]
	AuthorID int64      `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `db:"-,belongsTo:author_id"`
}

type testPostWithNullableAuthor struct {
	sqlr.Entity[int64]
	AuthorID *int64     `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `db:"-,belongsTo:author_id"`
}

type testPostWithAuthorAutoPreload struct {
	sqlr.Entity[int64]
	AuthorID int64      `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `db:"-,belongsTo:author_id,preload"`
}

type testPostWithoutPrimaryKey struct {
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
}

// testComment is a second related model used in multiple-join tests. Table: "test_comments".
type testComment struct {
	sqlr.Entity[int64]
	AuthorID  int64          `db:"author_id"`
	PostID    int64          `db:"post_id"`
	Body      string         `db:"body"`
	Reactions []testReaction `db:"-,foreignKey:comment_id"`
}

type testReaction struct {
	sqlr.Entity[int64]
	CommentID int64  `db:"comment_id"`
	Kind      string `db:"kind"`
}

// testAuthor is an entity with relationships, used for join and preload tests.
// Table: "test_authors".
type testAuthor struct {
	sqlr.Entity[int64]
	Name     string        `db:"name"`
	Posts    []testPost    `db:"-,foreignKey:author_id"`
	Comments []testComment `db:"-,foreignKey:author_id"`
}

type testProfile struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Bio      string `db:"bio"`
}

type testAuthorWithProfile struct {
	sqlr.Entity[int64]
	Name    string      `db:"name"`
	Profile testProfile `db:"-,foreignKey:author_id"`
}

type testAuthorWithProfileAutoPreload struct {
	sqlr.Entity[int64]
	Name    string      `db:"name"`
	Profile testProfile `db:"-,foreignKey:author_id,preload"`
}

type testAuthorWithPostWithoutPrimaryKey struct {
	sqlr.Entity[int64]
	Name  string                      `db:"name"`
	Posts []testPostWithoutPrimaryKey `db:"-,foreignKey:author_id"`
}

// testTag is used for many-to-many relationship tests. Table: "test_tags".
type testTag struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

// testArticle is an entity with a many-to-many relationship to testTag.
// Table: "test_articles", join table: "article_tags".
type testArticle struct {
	sqlr.Entity[int64]
	Title string    `db:"title"`
	Tags  []testTag `db:"-,many2many:article_tags"`
}

// testAuthorAutoPreload is an entity where Posts are automatically preloaded via
// the "preload" tag option. Used to test auto-preloading on Read() and Query().
// Table: "test_author_auto_preloads".
type testAuthorAutoPreload struct {
	sqlr.Entity[int64]
	Name     string        `db:"name"`
	Posts    []testPost    `db:"-,foreignKey:author_id,preload"`
	Comments []testComment `db:"-,foreignKey:author_id"`
}

type testPostWithCommentsAutoPreload struct {
	sqlr.Entity[int64]
	AuthorID int64         `db:"author_id"`
	Title    string        `db:"title"`
	Comments []testComment `db:"-,foreignKey:post_id,preload"`
}

type testAuthorDeepAutoPreload struct {
	sqlr.Entity[int64]
	Name  string                            `db:"name"`
	Posts []testPostWithCommentsAutoPreload `db:"-,foreignKey:author_id,preload"`
}

// testArticleAutoPreload is an entity with a many-to-many preload tag.
// Table: "test_article_auto_preloads", join table: "article_tags".
type testArticleAutoPreload struct {
	sqlr.Entity[int64]
	Title string    `db:"title"`
	Tags  []testTag `db:"-,many2many:article_tags,preload"`
}

type testBrokenPost struct {
	sqlr.Entity[int64]
	Title string `db:"title"`
}

type testBrokenAuthor struct {
	sqlr.Entity[int64]
	Name  string           `db:"name"`
	Posts []testBrokenPost `db:"-,foreignKey:author_id"`
}

type testUint64Tag struct {
	sqlr.Entity[uint64]
	Name string `db:"name"`
}

type testUint64Article struct {
	sqlr.Entity[uint64]
	Title string          `db:"title"`
	Tags  []testUint64Tag `db:"-,many2many:uint64_article_tags"`
}

type testStringTag struct {
	sqlr.Entity[string]
	Name string `db:"name"`
}

type testStringArticle struct {
	sqlr.Entity[string]
	Title string          `db:"title"`
	Tags  []testStringTag `db:"-,many2many:string_article_tags"`
}

// ==========================================================================
// Column Definitions
// ==========================================================================

// testUserColumns are the columns for testUser, in schema declaration order.
var testUserColumns = []string{"id", "created_at", "updated_at", "name", "email"}

var testKeyUserColumns = []string{"id", "created_at", "updated_at", "name"}

// testCustomPkUserColumns are the columns for testCustomPkUser, in schema declaration order.
var testCustomPkUserColumns = []string{"user_id", "created_at", "updated_at", "name"}

// testAuthorColumns are the unqualified column names for testAuthor used in
// sqlmock result rows.
var testAuthorColumns = []string{
	"id", "created_at", "updated_at", "name",
}

var testAuthorWithProfileColumns = []string{
	"id", "created_at", "updated_at", "name",
}

// testAuthorAutoPreloadColumns are the unqualified column names for testAuthorAutoPreload.
var testAuthorAutoPreloadColumns = []string{
	"id", "created_at", "updated_at", "name",
}

var testAuthorWithProfileAutoPreloadColumns = []string{
	"id", "created_at", "updated_at", "name",
}

// testAuthorPostsColumns are column names returned when joining testAuthor with Posts.
// The joined columns use the double-underscore alias format: Posts__<column>.
var testAuthorPostsColumns = []string{
	"id", "created_at", "updated_at", "name",
	"Posts__id", "Posts__created_at", "Posts__updated_at",
	"Posts__author_id", "Posts__title", "Posts__status",
}

var testAuthorProfileColumns = []string{
	"id", "created_at", "updated_at", "name",
	"Profile__id", "Profile__created_at", "Profile__updated_at",
	"Profile__author_id", "Profile__bio",
}

var testPostAuthorColumns = []string{
	"id", "created_at", "updated_at", "author_id", "title", "status",
	"Author__id", "Author__created_at", "Author__updated_at", "Author__name",
}

// testAuthorCommentsPostsColumns are column names returned when joining testAuthor
// with both Comments and Posts. Joins are sorted alphabetically, so Comments
// columns appear before Posts columns.
var testAuthorCommentsPostsColumns = []string{
	"id", "created_at", "updated_at", "name",
	"Comments__id", "Comments__created_at", "Comments__updated_at",
	"Comments__author_id", "Comments__post_id", "Comments__body",
	"Posts__id", "Posts__created_at", "Posts__updated_at",
	"Posts__author_id", "Posts__title", "Posts__status",
}

// ==========================================================================
// SQL Constants
// ==========================================================================

// authorPostsSelectSQL is the SELECT clause our code generates for testAuthor with a Posts join.
const authorPostsSelectSQL = "SELECT `test_authors`.`id`, `test_authors`.`created_at`, `test_authors`.`updated_at`, `test_authors`.`name`, " +
	"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
	"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`"

const authorProfileSelectSQL = "SELECT `test_author_with_profiles`.`id`, `test_author_with_profiles`.`created_at`, `test_author_with_profiles`.`updated_at`, `test_author_with_profiles`.`name`, " +
	"`Profile`.`id` AS `Profile__id`, `Profile`.`created_at` AS `Profile__created_at`, `Profile`.`updated_at` AS `Profile__updated_at`, " +
	"`Profile`.`author_id` AS `Profile__author_id`, `Profile`.`bio` AS `Profile__bio`"

const postAuthorSelectSQL = "SELECT `test_posts`.`id`, `test_posts`.`created_at`, `test_posts`.`updated_at`, `test_posts`.`author_id`, `test_posts`.`title`, `test_posts`.`status`, " +
	"`Author`.`id` AS `Author__id`, `Author`.`created_at` AS `Author__created_at`, `Author`.`updated_at` AS `Author__updated_at`, `Author`.`name` AS `Author__name`"

// authorPostsLeftJoinSQL is the FROM + LEFT JOIN clause for Posts.
const authorPostsLeftJoinSQL = "FROM `test_authors` LEFT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`"

const authorProfileLeftJoinSQL = "FROM `test_author_with_profiles` LEFT JOIN `test_profiles` AS Profile ON `test_author_with_profiles`.`id` = `Profile`.`author_id`"

const postAuthorLeftJoinSQL = "FROM `test_posts` LEFT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`"

// authorPostsInnerJoinSQL is the FROM + JOIN clause for Posts.
// Note: sqlc uses "JOIN" not "INNER JOIN" for JoinInner.
const authorPostsInnerJoinSQL = "FROM `test_authors` JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`"

const authorProfileInnerJoinSQL = "FROM `test_author_with_profiles` JOIN `test_profiles` AS Profile ON `test_author_with_profiles`.`id` = `Profile`.`author_id`"

const postAuthorInnerJoinSQL = "FROM `test_posts` JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`"

// authorPostsRightJoinSQL is the FROM + RIGHT JOIN clause for Posts.
const authorPostsRightJoinSQL = "FROM `test_authors` RIGHT JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`"

const postAuthorRightJoinSQL = "FROM `test_posts` RIGHT JOIN `test_authors` AS Author ON `test_posts`.`author_id` = `Author`.`id`"

// ==========================================================================
// Expected Value Structs for Assertions
// ==========================================================================

// expectedPost represents expected values for a testPost entity in assertions.
// Only non-nil fields are checked; nil fields are ignored.
type expectedPost struct {
	Id       *int64
	AuthorID *int64
	Title    *string
	Status   *string
}

// expectedComment represents expected values for a testComment entity in assertions.
// Only non-nil fields are checked; nil fields are ignored.
type expectedComment struct {
	Id       *int64
	AuthorID *int64
	Body     *string
}

// expectedTag represents expected values for a testTag entity in assertions.
// Only non-nil fields are checked; nil fields are ignored.
type expectedTag struct {
	Id   *int64
	Name *string
}

// expectedAuthor represents expected values for a testAuthor entity in assertions.
// Only non-nil fields are checked; nil fields are ignored.
// For slices: nil = don't check, empty = assert empty, non-empty = assert contents.
type expectedAuthor struct {
	Name     *string
	Posts    []expectedPost
	Comments []expectedComment
}

// expectedArticle represents expected values for a testArticle entity in assertions.
// Only non-nil fields are checked; nil fields are ignored.
// For slices: nil = don't check, empty = assert empty, non-empty = assert contents.
type expectedArticle struct {
	Title *string
	Tags  []expectedTag
}

// ptr returns a pointer to the given value. Useful for constructing expected value structs.
func ptr[T any](v T) *T {
	return &v
}

// ==========================================================================
// Test Helpers
// ==========================================================================

// newTestClient creates a new sqlc.Client backed by sqlmock for testing.
func newTestClient(t *testing.T) (sqlc.Client, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	logger := logMocks.NewLoggerMock(logMocks.WithMockAll)
	sqlxDB := sqlx.NewDb(sqlDB, "mysql")
	client := sqlc.NewClientWithInterfaces(logger, sqlxDB, exec.NewDefaultExecutor(), sqlc.DefaultConfig())

	return client, mock
}

// mustNewRepo creates a repository from an existing client, failing the test on error.
// This helper reduces boilerplate in test suite SetupTest methods.
func mustNewRepo[K sqlr.KeyTypes, E sqlr.Entitier[K]](t *testing.T, client sqlc.Client) sqlr.Repository[K, E] {
	t.Helper()

	repo, err := sqlr.NewRepositoryWithInterfaces[K, E](client, sqlr.DefaultSettings())
	require.NoError(t, err)

	return repo
}

// mustNewRepoWithSettings creates a repository from an existing client with custom settings,
// failing the test on error. This helper is used for testing prepared statements and other
// configurable features.
func mustNewRepoWithSettings[K sqlr.KeyTypes, E sqlr.Entitier[K]](t *testing.T, client sqlc.Client, settings sqlr.Settings) sqlr.Repository[K, E] {
	t.Helper()

	repo, err := sqlr.NewRepositoryWithInterfaces[K, E](client, settings)
	require.NoError(t, err)

	return repo
}

// ==========================================================================
// Assertion Helpers
// ==========================================================================

// assertPost checks a testPost entity against expected values.
// Only non-nil fields in the expected struct are asserted.
func assertPost(s *suite.Suite, actual testPost, expected expectedPost) {
	if expected.Id != nil {
		s.Equal(*expected.Id, actual.Id)
	}
	if expected.AuthorID != nil {
		s.Equal(*expected.AuthorID, actual.AuthorID)
	}
	if expected.Title != nil {
		s.Equal(*expected.Title, actual.Title)
	}
	if expected.Status != nil {
		s.Equal(*expected.Status, actual.Status)
	}
}

// assertComment checks a testComment entity against expected values.
// Only non-nil fields in the expected struct are asserted.
func assertComment(s *suite.Suite, actual testComment, expected expectedComment) {
	if expected.Id != nil {
		s.Equal(*expected.Id, actual.Id)
	}
	if expected.AuthorID != nil {
		s.Equal(*expected.AuthorID, actual.AuthorID)
	}
	if expected.Body != nil {
		s.Equal(*expected.Body, actual.Body)
	}
}

// assertTag checks a testTag entity against expected values.
// Only non-nil fields in the expected struct are asserted.
func assertTag(s *suite.Suite, actual testTag, expected expectedTag) {
	if expected.Id != nil {
		s.Equal(*expected.Id, actual.Id)
	}
	if expected.Name != nil {
		s.Equal(*expected.Name, actual.Name)
	}
}

// assertAuthor checks a testAuthor entity against expected values.
// Only non-nil fields in the expected struct are asserted.
// For slices: nil = don't check, empty = assert empty, non-empty = assert contents.
func assertAuthor(s *suite.Suite, actual testAuthor, expected expectedAuthor) {
	if expected.Name != nil {
		s.Equal(*expected.Name, actual.Name)
	}

	if expected.Posts != nil {
		s.Require().Len(actual.Posts, len(expected.Posts))
		for i, expectedPost := range expected.Posts {
			assertPost(s, actual.Posts[i], expectedPost)
		}
	}

	if expected.Comments != nil {
		s.Require().Len(actual.Comments, len(expected.Comments))
		for i, expectedComment := range expected.Comments {
			assertComment(s, actual.Comments[i], expectedComment)
		}
	}
}

// assertAuthorAutoPreload checks a testAuthorAutoPreload entity against expected values.
// Only non-nil fields in the expected struct are asserted.
// For slices: nil = don't check, empty = assert empty, non-empty = assert contents.
func assertAuthorAutoPreload(s *suite.Suite, actual testAuthorAutoPreload, expected expectedAuthor) {
	if expected.Name != nil {
		s.Equal(*expected.Name, actual.Name)
	}

	if expected.Posts != nil {
		s.Require().Len(actual.Posts, len(expected.Posts))
		for i, expectedPost := range expected.Posts {
			assertPost(s, actual.Posts[i], expectedPost)
		}
	}

	if expected.Comments != nil {
		s.Require().Len(actual.Comments, len(expected.Comments))
		for i, expectedComment := range expected.Comments {
			assertComment(s, actual.Comments[i], expectedComment)
		}
	}
}

// assertArticle checks a testArticle entity against expected values.
// Only non-nil fields in the expected struct are asserted.
// For slices: nil = don't check, empty = assert empty, non-empty = assert contents.
func assertArticle(s *suite.Suite, actual testArticle, expected expectedArticle) {
	if expected.Title != nil {
		s.Equal(*expected.Title, actual.Title)
	}

	if expected.Tags != nil {
		s.Require().Len(actual.Tags, len(expected.Tags))
		for i, expectedTag := range expected.Tags {
			assertTag(s, actual.Tags[i], expectedTag)
		}
	}
}

// assertArticleAutoPreload checks a testArticleAutoPreload entity against expected values.
// Only non-nil fields in the expected struct are asserted.
// For slices: nil = don't check, empty = assert empty, non-empty = assert contents.
func assertArticleAutoPreload(s *suite.Suite, actual testArticleAutoPreload, expected expectedArticle) {
	if expected.Title != nil {
		s.Equal(*expected.Title, actual.Title)
	}

	if expected.Tags != nil {
		s.Require().Len(actual.Tags, len(expected.Tags))
		for i, expectedTag := range expected.Tags {
			assertTag(s, actual.Tags[i], expectedTag)
		}
	}
}
