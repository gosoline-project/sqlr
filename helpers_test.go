package sqlr_test

import (
	"database/sql/driver"
	"reflect"
	"testing"
	"time"
	"unsafe"

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
	Id        int64     `db:"user_id" sqlr:"primaryKey"`
	CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string    `db:"name"`
}

type testNoSetterUser struct {
	Id        int64     `db:"id" sqlr:"primaryKey"`
	CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string    `db:"name"`
}

func (u testNoSetterUser) GetId() int64 {
	return u.Id
}

func (u testNoSetterUser) GetUpdatedAt() time.Time {
	return u.UpdatedAt
}

func (u testNoSetterUser) GetCreatedAt() time.Time {
	return u.CreatedAt
}

type testNoSetterCustomPkUser struct {
	Id        int64     `db:"user_id" sqlr:"primaryKey"`
	CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string    `db:"name"`
}

func (u testNoSetterCustomPkUser) GetId() int64 {
	return u.Id
}

func (u testNoSetterCustomPkUser) GetUpdatedAt() time.Time {
	return u.UpdatedAt
}

func (u testNoSetterCustomPkUser) GetCreatedAt() time.Time {
	return u.CreatedAt
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
	Author   testAuthor    `sqlr:"belongsTo:author_id"`
	Comments []testComment `sqlr:"foreignKey:post_id"`
	Tags     []testTag     `sqlr:"many2many:post_tags"`
}

type testPostWithAuthor struct {
	sqlr.Entity[int64]
	AuthorID int64      `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `sqlr:"belongsTo:author_id"`
}

type testPostWithNullableAuthor struct {
	sqlr.Entity[int64]
	AuthorID *int64     `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `sqlr:"belongsTo:author_id"`
}

type testBoolAuthor struct {
	sqlr.Entity[bool]
	Name string `db:"name"`
}

type testPostWithBoolAuthor struct {
	sqlr.Entity[int64]
	AuthorID bool           `db:"author_id"`
	Title    string         `db:"title"`
	Author   testBoolAuthor `sqlr:"belongsTo:author_id"`
}

type testStringAuthor struct {
	sqlr.Entity[string]
	Name string `db:"name"`
}

type testPostWithStringAuthor struct {
	sqlr.Entity[int64]
	AuthorID string           `db:"author_id"`
	Title    string           `db:"title"`
	Author   testStringAuthor `sqlr:"belongsTo:author_id"`
}

type testFloatAuthor struct {
	sqlr.Entity[float64]
	Name string `db:"name"`
}

type testPostWithFloatAuthor struct {
	sqlr.Entity[int64]
	AuthorID float64         `db:"author_id"`
	Title    string          `db:"title"`
	Author   testFloatAuthor `sqlr:"belongsTo:author_id"`
}

type testPointerKeyUser struct {
	Id        *int64    `db:"id" sqlr:"primaryKey"`
	CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string    `db:"name"`
}

type testNoSetterPointerKeyUser struct {
	Id        *int64    `db:"id" sqlr:"primaryKey"`
	CreatedAt time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string    `db:"name"`
}

func (u testNoSetterPointerKeyUser) GetId() *int64 {
	return u.Id
}

func (u testNoSetterPointerKeyUser) GetUpdatedAt() time.Time {
	return u.UpdatedAt
}

func (u testNoSetterPointerKeyUser) GetCreatedAt() time.Time {
	return u.CreatedAt
}

func (u *testPointerKeyUser) SetId(id *int64) {
	u.Id = id
}

func (u testPointerKeyUser) GetId() *int64 {
	return u.Id
}

func (u testPointerKeyUser) GetUpdatedAt() time.Time {
	return u.UpdatedAt
}

func (u testPointerKeyUser) GetCreatedAt() time.Time {
	return u.CreatedAt
}

type testPointerTimestampUser struct {
	Id        int64      `db:"id" sqlr:"primaryKey"`
	CreatedAt *time.Time `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt *time.Time `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string     `db:"name"`
}

func (u *testPointerTimestampUser) SetId(id int64) {
	u.Id = id
}

func (u testPointerTimestampUser) GetId() int64 {
	return u.Id
}

func (u testPointerTimestampUser) GetUpdatedAt() time.Time {
	if u.UpdatedAt == nil {
		return time.Time{}
	}

	return *u.UpdatedAt
}

func (u testPointerTimestampUser) GetCreatedAt() time.Time {
	if u.CreatedAt == nil {
		return time.Time{}
	}

	return *u.CreatedAt
}

type testPostWithPointerAuthorID struct {
	sqlr.Entity[int64]
	AuthorID *int64     `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `sqlr:"belongsTo:author_id"`
}

type testAuthorWithPointerKeyProfile struct {
	Id        *int64             `db:"id" sqlr:"primaryKey"`
	CreatedAt time.Time          `db:"created_at" sqlr:"autoCreateTime"`
	UpdatedAt time.Time          `db:"updated_at" sqlr:"autoUpdateTime"`
	Name      string             `db:"name"`
	Profiles  []testPointerChild `sqlr:"foreignKey:author_id"`
}

func (u *testAuthorWithPointerKeyProfile) SetId(id *int64) {
	u.Id = id
}

func (u testAuthorWithPointerKeyProfile) GetId() *int64 {
	return u.Id
}

func (u testAuthorWithPointerKeyProfile) GetUpdatedAt() time.Time {
	return u.UpdatedAt
}

func (u testAuthorWithPointerKeyProfile) GetCreatedAt() time.Time {
	return u.CreatedAt
}

type testPointerChild struct {
	sqlr.Entity[int64]
	AuthorID *int64 `db:"author_id"`
	Title    string `db:"title"`
}

type testPostWithAuthorAutoPreload struct {
	sqlr.Entity[int64]
	AuthorID int64      `db:"author_id"`
	Title    string     `db:"title"`
	Author   testAuthor `sqlr:"belongsTo:author_id;preload"`
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
	Reactions []testReaction `sqlr:"foreignKey:comment_id"`
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
	Posts    []testPost    `sqlr:"foreignKey:author_id"`
	Comments []testComment `sqlr:"foreignKey:author_id"`
}

type testProfile struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Bio      string `db:"bio"`
}

type testAuthorWithProfile struct {
	sqlr.Entity[int64]
	Name    string      `db:"name"`
	Profile testProfile `sqlr:"foreignKey:author_id"`
}

type testAuthorWithProfilePointer struct {
	sqlr.Entity[int64]
	Name    string       `db:"name"`
	Profile *testProfile `sqlr:"foreignKey:author_id"`
}

type testAuthorWithProfileAutoPreload struct {
	sqlr.Entity[int64]
	Name    string      `db:"name"`
	Profile testProfile `sqlr:"foreignKey:author_id;preload"`
}

type testAuthorWithPostWithoutPrimaryKey struct {
	sqlr.Entity[int64]
	Name  string                      `db:"name"`
	Posts []testPostWithoutPrimaryKey `sqlr:"foreignKey:author_id"`
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
	Tags  []testTag `sqlr:"many2many:article_tags"`
}

type testArticleWithPointerTags struct {
	sqlr.Entity[int64]
	Title string     `db:"title"`
	Tags  []*testTag `sqlr:"many2many:article_tags"`
}

type testPostWithAuthorPointer struct {
	sqlr.Entity[int64]
	AuthorID int64       `db:"author_id"`
	Title    string      `db:"title"`
	Author   *testAuthor `sqlr:"belongsTo:author_id"`
}

// testAuthorAutoPreload is an entity where Posts are automatically preloaded via
// the "preload" tag option. Used to test auto-preloading on Read() and Query().
// Table: "test_author_auto_preloads".
type testAuthorAutoPreload struct {
	sqlr.Entity[int64]
	Name     string        `db:"name"`
	Posts    []testPost    `sqlr:"foreignKey:author_id;preload"`
	Comments []testComment `sqlr:"foreignKey:author_id"`
}

type testPostWithCommentsAutoPreload struct {
	sqlr.Entity[int64]
	AuthorID int64         `db:"author_id"`
	Title    string        `db:"title"`
	Comments []testComment `sqlr:"foreignKey:post_id;preload"`
}

type testAuthorDeepAutoPreload struct {
	sqlr.Entity[int64]
	Name  string                            `db:"name"`
	Posts []testPostWithCommentsAutoPreload `sqlr:"foreignKey:author_id;preload"`
}

// testArticleAutoPreload is an entity with a many-to-many preload tag.
// Table: "test_article_auto_preloads", join table: "article_tags".
type testArticleAutoPreload struct {
	sqlr.Entity[int64]
	Title string    `db:"title"`
	Tags  []testTag `sqlr:"many2many:article_tags;preload"`
}

type testBrokenPost struct {
	sqlr.Entity[int64]
	Title string `db:"title"`
}

type testBrokenAuthor struct {
	sqlr.Entity[int64]
	Name  string           `db:"name"`
	Posts []testBrokenPost `sqlr:"foreignKey:author_id"`
}

type testUint64Tag struct {
	sqlr.Entity[uint64]
	Name string `db:"name"`
}

type testUint64Article struct {
	sqlr.Entity[uint64]
	Title string          `db:"title"`
	Tags  []testUint64Tag `sqlr:"many2many:uint64_article_tags"`
}

type testStringTag struct {
	sqlr.Entity[string]
	Name string `db:"name"`
}

type testStringArticle struct {
	sqlr.Entity[string]
	Title string          `db:"title"`
	Tags  []testStringTag `sqlr:"many2many:string_article_tags"`
}

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

func forceFirstCachedStmtCloseError(t testing.TB, repo any, closeErr error) {
	t.Helper()

	repoValue := reflect.ValueOf(repo)
	if repoValue.Kind() == reflect.Pointer {
		repoValue = repoValue.Elem()
	}

	commonValue := unsafeReflectValue(repoValue.FieldByName("repositoryCommon"))
	statementCacheValue := unsafeReflectValue(commonValue.FieldByName("statementCache"))
	cacheValue := unsafeReflectValue(statementCacheValue.Elem().FieldByName("cache"))
	cache := cacheValue.Interface().(map[string]*sqlx.Stmt)

	for _, stmt := range cache {
		stmtValue := reflect.ValueOf(stmt).Elem()
		sqlStmtValue := stmtValue.FieldByName("Stmt").Elem()
		stickyErrValue := unsafeReflectValue(sqlStmtValue.FieldByName("stickyErr"))
		stickyErrValue.Set(reflect.ValueOf(closeErr))

		return
	}

	t.Fatal("no cached prepared statement found")
}

func unsafeReflectValue(value reflect.Value) reflect.Value {
	return reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
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

func assertExpectedPosts(s *suite.Suite, actual []testPost, expected []expectedPost) {
	s.Require().Len(actual, len(expected))
	for i, expectedPost := range expected {
		assertPost(s, actual[i], expectedPost)
	}
}

func assertExpectedComments(s *suite.Suite, actual []testComment, expected []expectedComment) {
	s.Require().Len(actual, len(expected))
	for i, expectedComment := range expected {
		assertComment(s, actual[i], expectedComment)
	}
}

func assertExpectedTags(s *suite.Suite, actual []testTag, expected []expectedTag) {
	s.Require().Len(actual, len(expected))
	for i, expectedTag := range expected {
		assertTag(s, actual[i], expectedTag)
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
		assertExpectedPosts(s, actual.Posts, expected.Posts)
	}

	if expected.Comments != nil {
		assertExpectedComments(s, actual.Comments, expected.Comments)
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
		assertExpectedPosts(s, actual.Posts, expected.Posts)
	}

	if expected.Comments != nil {
		assertExpectedComments(s, actual.Comments, expected.Comments)
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
		assertExpectedTags(s, actual.Tags, expected.Tags)
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
		assertExpectedTags(s, actual.Tags, expected.Tags)
	}
}
