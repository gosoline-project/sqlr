package sqlr_test

import (
	"context"
	"database/sql/driver"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTestSuite))
}

// isTimestamp is a custom sqlmock.Argument matcher that asserts the value
// is a time.Time. It implements the sqlmock.Argument interface.
type isTimestamp struct{}

func (isTimestamp) Match(v driver.Value) bool {
	_, ok := v.(time.Time)

	return ok
}

// testUser is a custom entity type used throughout the repository tests.
// GORM will derive the relation name "test_users" from this type.
type testUser struct {
	sqlr.Entity[int64]
	Name  string `gorm:"column:name"`
	Email string `gorm:"column:email"`
}

// testUserColumns are the columns gorm expects for testUser, in schema order.
var testUserColumns = []string{"id", "created_at", "updated_at", "name", "email"}

// testPost is a related model used in join tests. GORM relation: "test_posts".
type testPost struct {
	sqlr.Entity[int64]
	AuthorID int64  `gorm:"column:author_id"`
	Title    string `gorm:"column:title"`
	Status   string `gorm:"column:status"`
}

// testComment is a second related model used in multiple-join tests. GORM relation: "test_comments".
type testComment struct {
	sqlr.Entity[int64]
	AuthorID int64  `gorm:"column:author_id"`
	Body     string `gorm:"column:body"`
}

// testAuthor is an entity with GORM relationships, used for join tests.
// GORM relation: "test_authors".
type testAuthor struct {
	sqlr.Entity[int64]
	Name     string        `gorm:"column:name"`
	Posts    []testPost    `gorm:"foreignKey:AuthorID"`
	Comments []testComment `gorm:"foreignKey:AuthorID"`
}

// testTag is used for many-to-many relationship tests. GORM relation: "test_tags".
type testTag struct {
	sqlr.Entity[int64]
	Name string `gorm:"column:name"`
}

// testArticle is an entity with a many-to-many relationship to testTag.
// GORM relation: "test_articles", join table: "article_tags".
type testArticle struct {
	sqlr.Entity[int64]
	Title string    `gorm:"column:title"`
	Tags  []testTag `gorm:"many2many:article_tags;"`
}

// RepositoryTestSuite tests the Repository CRUD operations using sqlmock.
type RepositoryTestSuite struct {
	suite.Suite
	db          *gorm.DB
	mock        sqlmock.Sqlmock
	repo        sqlr.Repository[int64, testUser]
	authorRepo  sqlr.Repository[int64, testAuthor]
	articleRepo sqlr.Repository[int64, testArticle]
}

func (s *RepositoryTestSuite) SetupTest() {
	sqlDB, mock, err := sqlmock.New()
	s.Require().NoError(err)

	s.mock = mock

	// Expect the SELECT VERSION() query that the mysql dialector issues during initialization.
	s.mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("8.0.33"))

	dialector := mysql.New(mysql.Config{
		Conn: sqlDB,
	})

	db, err := gorm.Open(dialector, &gorm.Config{
		// SkipDefaultTransaction disables the implicit transaction wrapping
		// for single Create/Update/Delete operations, making the SQL
		// expectations simpler and more predictable.
		SkipDefaultTransaction: true,
	})
	s.Require().NoError(err)

	s.db = db
	s.repo, err = sqlr.NewRepositoryWithInterfaces[int64, testUser](db)
	s.Require().NoError(err)

	s.authorRepo, err = sqlr.NewRepositoryWithInterfaces[int64, testAuthor](db)
	s.Require().NoError(err)

	s.articleRepo, err = sqlr.NewRepositoryWithInterfaces[int64, testArticle](db)
	s.Require().NoError(err)
}

func (s *RepositoryTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// ==========================================================================
// Create
// ==========================================================================

func (s *RepositoryTestSuite) TestCreate_Success() {
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`,`updated_at`,`name`,`email`) VALUES (?,?,?,?)")).
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

func (s *RepositoryTestSuite) TestCreate_Error() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `test_users` (`created_at`,`updated_at`,`name`,`email`) VALUES (?,?,?,?)")).
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
// Read
// ==========================================================================

func (s *RepositoryTestSuite) TestRead_Success() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE id = ? ORDER BY `test_users`.`id` LIMIT ?")).
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

func (s *RepositoryTestSuite) TestRead_NotFound() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE id = ? ORDER BY `test_users`.`id` LIMIT ?")).
		WithArgs(int64(999), 1).
		WillReturnRows(sqlmock.NewRows(testUserColumns))

	result, err := s.repo.Read(context.Background(), 999)

	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to read entity")
}

// ==========================================================================
// Query
// ==========================================================================

func (s *RepositoryTestSuite) TestQuery_Success() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(1, now, now, "Alice", "alice@test.com").
			AddRow(2, now, now, "Alice", "alice2@test.com"))

	qb := sqlr.NewQueryBuilderSelect().Where("name = ?", "Alice")
	results, err := s.repo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.Equal("Alice", results[0].Name)
	s.Equal("alice@test.com", results[0].Email)
	s.Equal("Alice", results[1].Name)
	s.Equal("alice2@test.com", results[1].Email)
}

func (s *RepositoryTestSuite) TestQuery_EmptyResult() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Nobody").
		WillReturnRows(sqlmock.NewRows(testUserColumns))

	qb := sqlr.NewQueryBuilderSelect().Where("name = ?", "Nobody")
	results, err := s.repo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Empty(results)
}

func (s *RepositoryTestSuite) TestQuery_WithLimitAndOffset() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ? LIMIT ? OFFSET ?")).
		WithArgs("Alice", 10, 5).
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(1, now, now, "Alice", "alice@test.com"))

	qb := sqlr.NewQueryBuilderSelect().
		Where("name = ?", "Alice").
		Limit(10).
		Offset(5)
	results, err := s.repo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
}

func (s *RepositoryTestSuite) TestQuery_WithOrderBy() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ? ORDER BY `created_at` DESC")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows(testUserColumns).
			AddRow(2, now, now, "Alice", "alice2@test.com").
			AddRow(1, now, now, "Alice", "alice@test.com"))

	qb := sqlr.NewQueryBuilderSelect().
		Where("name = ?", "Alice").
		OrderBy("created_at DESC")
	results, err := s.repo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 2)
}

func (s *RepositoryTestSuite) TestQuery_Error() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_users` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnError(fmt.Errorf("connection lost"))

	qb := sqlr.NewQueryBuilderSelect().Where("name = ?", "Alice")
	results, err := s.repo.Query(context.Background(), qb)

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "failed to execute query")
}

// ==========================================================================
// Update
// ==========================================================================

func (s *RepositoryTestSuite) TestUpdate_Success() {
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

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_users` SET `created_at`=?,`updated_at`=?,`name`=?,`email`=? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, isTimestamp{}, entity.Name, entity.Email, entity.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal("Alice Updated", result.Name)
	s.Equal("alice-updated@test.com", result.Email)
}

func (s *RepositoryTestSuite) TestUpdate_Error() {
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
		"UPDATE `test_users` SET `created_at`=?,`updated_at`=?,`name`=?,`email`=? WHERE `id` = ?")).
		WithArgs(isTimestamp{}, isTimestamp{}, entity.Name, entity.Email, entity.Id).
		WillReturnError(fmt.Errorf("deadlock"))

	result, err := s.repo.Update(context.Background(), &entity)

	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "failed to update entity")
}

// ==========================================================================
// Delete
// ==========================================================================

func (s *RepositoryTestSuite) TestDelete_Success() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE id = ?")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.Delete(context.Background(), 1)

	s.Require().NoError(err)
}

func (s *RepositoryTestSuite) TestDelete_NotFound() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE id = ?")).
		WithArgs(int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.Delete(context.Background(), 999)

	s.Require().Error(err)
	s.Contains(err.Error(), "entity id=999 does not exist")
}

func (s *RepositoryTestSuite) TestDelete_Error() {
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `test_users` WHERE id = ?")).
		WithArgs(int64(1)).
		WillReturnError(fmt.Errorf("foreign key constraint"))

	err := s.repo.Delete(context.Background(), 1)

	s.Require().Error(err)
	s.Contains(err.Error(), "failed to delete entity")
}

// ==========================================================================
// Query with Joins
// ==========================================================================

// testAuthorColumns are the unqualified column names for testAuthor used in
// sqlmock result rows. Even though GORM generates qualified SELECT columns
// (e.g. `test_authors`.`id`), the database driver returns unqualified names.
var testAuthorColumns = []string{
	"id", "created_at", "updated_at", "name",
}

// authorPostsSelectSQL is the SELECT clause GORM generates for testAuthor with a Posts join.
const authorPostsSelectSQL = "SELECT `test_authors`.`id`,`test_authors`.`created_at`,`test_authors`.`updated_at`,`test_authors`.`name`," +
	"`Posts`.`id` AS `Posts__id`,`Posts`.`created_at` AS `Posts__created_at`,`Posts`.`updated_at` AS `Posts__updated_at`," +
	"`Posts`.`author_id` AS `Posts__author_id`,`Posts`.`title` AS `Posts__title`,`Posts`.`status` AS `Posts__status`"

// authorPostsLeftJoinSQL is the FROM + LEFT JOIN clause for Posts.
const authorPostsLeftJoinSQL = "FROM `test_authors` LEFT JOIN `test_posts` `Posts` ON `test_authors`.`id` = `Posts`.`author_id`"

// authorPostsInnerJoinSQL is the FROM + INNER JOIN clause for Posts.
const authorPostsInnerJoinSQL = "FROM `test_authors` INNER JOIN `test_posts` `Posts` ON `test_authors`.`id` = `Posts`.`author_id`"

// authorPostsRightJoinSQL is the FROM + RIGHT JOIN clause for Posts.
const authorPostsRightJoinSQL = "FROM `test_authors` RIGHT JOIN `test_posts` `Posts` ON `test_authors`.`id` = `Posts`.`author_id`"

func (s *RepositoryTestSuite) TestQuery_LeftJoinWithoutCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(authorPostsSelectSQL + " " + authorPostsLeftJoinSQL)).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_LeftJoinWithUnknownRelation() {
	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Unknown")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `join relation "Unknown" not found`)
}

func (s *RepositoryTestSuite) TestQuery_LeftJoinWithCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " " + authorPostsLeftJoinSQL + " AND test_posts.status = ?")).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published"))
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_LeftJoinWithMultipleConditions() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " " + authorPostsLeftJoinSQL +
			" AND (test_posts.status = ? AND test_posts.title IS NOT NULL)")).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts",
			sqlr.Condition("test_posts.status = ?", "published"),
			sqlr.Condition("test_posts.title IS NOT NULL"),
		)
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_LeftJoinWithParameterizedCondition() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " " + authorPostsLeftJoinSQL + " AND test_posts.author_id = ?")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts", sqlr.Condition("test_posts.author_id = ?", int64(42)))
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_InnerJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " " + authorPostsInnerJoinSQL + " AND test_posts.status = ?")).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		InnerJoin("Posts", sqlr.Condition("test_posts.status = ?", "published"))
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_RightJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " " + authorPostsRightJoinSQL + " AND test_posts.status = ?")).
		WithArgs("draft").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Bob"))

	qb := sqlr.NewQueryBuilderSelect().
		RightJoin("Posts", sqlr.Condition("test_posts.status = ?", "draft"))
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Bob", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_CrossJoin() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " FROM `test_authors` CROSS JOIN `test_posts` `Posts` ON `test_authors`.`id` = `Posts`.`author_id`")).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice").
			AddRow(2, now, now, "Bob"))

	qb := sqlr.NewQueryBuilderSelect().
		CrossJoin("Posts")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 2)
}

func (s *RepositoryTestSuite) TestQuery_JoinWithWhere() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL+" "+authorPostsLeftJoinSQL+" AND test_posts.status = ? WHERE name = ?")).
		WithArgs("published", "Alice").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published")).
		Where("name = ?", "Alice")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

func (s *RepositoryTestSuite) TestQuery_JoinWithOrderBy() {
	now := time.Now()

	s.mock.ExpectQuery(regexp.QuoteMeta(
		authorPostsSelectSQL + " " + authorPostsLeftJoinSQL +
			" ORDER BY `created_at` DESC")).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(2, now, now, "Bob").
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts").
		OrderBy("created_at DESC")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 2)
	s.Equal("Bob", results[0].Name)
	s.Equal("Alice", results[1].Name)
}

func (s *RepositoryTestSuite) TestQuery_MultipleJoins() {
	now := time.Now()

	// With multiple joins, GORM adds columns from both joined tables.
	// Joins are sorted alphabetically by name, so Comments comes before Posts.
	commentsSelectSQL := "`Comments`.`id` AS `Comments__id`,`Comments`.`created_at` AS `Comments__created_at`," +
		"`Comments`.`updated_at` AS `Comments__updated_at`,`Comments`.`author_id` AS `Comments__author_id`," +
		"`Comments`.`body` AS `Comments__body`"

	postsSelectSQL := "`Posts`.`id` AS `Posts__id`,`Posts`.`created_at` AS `Posts__created_at`," +
		"`Posts`.`updated_at` AS `Posts__updated_at`,`Posts`.`author_id` AS `Posts__author_id`," +
		"`Posts`.`title` AS `Posts__title`,`Posts`.`status` AS `Posts__status`"

	fullSelectSQL := "SELECT `test_authors`.`id`,`test_authors`.`created_at`,`test_authors`.`updated_at`,`test_authors`.`name`," +
		commentsSelectSQL + "," + postsSelectSQL

	fullFromSQL := "FROM `test_authors` LEFT JOIN `test_comments` `Comments` ON `test_authors`.`id` = `Comments`.`author_id`" +
		" LEFT JOIN `test_posts` `Posts` ON `test_authors`.`id` = `Posts`.`author_id` AND test_posts.status = ?"

	s.mock.ExpectQuery(regexp.QuoteMeta(fullSelectSQL + " " + fullFromSQL)).
		WithArgs("published").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Posts", sqlr.Condition("test_posts.status = ?", "published")).
		LeftJoin("Comments")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
}

// ==========================================================================
// Query with Many-to-Many Joins
// ==========================================================================

func (s *RepositoryTestSuite) TestQuery_ManyToManyJoinNotSupported() {
	// Many-to-many associations are not supported because GORM's Joins() method
	// generates invalid SQL for them (it joins directly to the related table
	// instead of going through the join table).
	qb := sqlr.NewQueryBuilderSelect().
		LeftJoin("Tags")
	results, err := s.articleRepo.Query(context.Background(), qb)

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "many-to-many association which is not supported")
}

// ==========================================================================
// Query with Preloads
// ==========================================================================

func (s *RepositoryTestSuite) TestQuery_PreloadWithoutCondition() {
	now := time.Now()

	// GORM Preload executes a separate query for the preloaded relation.
	// First: main query for authors.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Second: preload query for posts belonging to found authors.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	qb := sqlr.NewQueryBuilderSelect().
		Preload("Posts")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Require().Len(results[0].Posts, 1)
	s.Equal("First Post", results[0].Posts[0].Title)
}

func (s *RepositoryTestSuite) TestQuery_PreloadWithUnknownRelation() {
	qb := sqlr.NewQueryBuilderSelect().
		Preload("Unknown")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().Error(err)
	s.Nil(results)
	s.Contains(err.Error(), `preload relation "Unknown" not found`)
}

func (s *RepositoryTestSuite) TestQuery_PreloadWithCondition() {
	now := time.Now()

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload query with additional WHERE condition.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` = ? AND status = ?")).
		WithArgs(int64(1), "published").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "Published Post", "published"))

	qb := sqlr.NewQueryBuilderSelect().
		Preload("Posts", sqlr.Condition("status = ?", "published"))
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Require().Len(results[0].Posts, 1)
	s.Equal("Published Post", results[0].Posts[0].Title)
}

func (s *RepositoryTestSuite) TestQuery_PreloadMultipleRelations() {
	now := time.Now()

	// Main query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors`")).
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload Comments (alphabetically first).
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_comments` WHERE `test_comments`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "body"}).
			AddRow(20, now, now, 1, "A comment"))

	// Preload Posts.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	qb := sqlr.NewQueryBuilderSelect().
		Preload("Posts").
		Preload("Comments")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Require().Len(results[0].Posts, 1)
	s.Equal("First Post", results[0].Posts[0].Title)
	s.Require().Len(results[0].Comments, 1)
	s.Equal("A comment", results[0].Comments[0].Body)
}

func (s *RepositoryTestSuite) TestQuery_PreloadWithWhere() {
	now := time.Now()

	// Main query with WHERE.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_authors` WHERE name = ?")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows(testAuthorColumns).
			AddRow(1, now, now, "Alice"))

	// Preload query.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_posts` WHERE `test_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title", "status"}).
			AddRow(10, now, now, 1, "First Post", "published"))

	qb := sqlr.NewQueryBuilderSelect().
		Preload("Posts").
		Where("name = ?", "Alice")
	results, err := s.authorRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("Alice", results[0].Name)
	s.Require().Len(results[0].Posts, 1)
}

func (s *RepositoryTestSuite) TestQuery_PreloadManyToManyAllowed() {
	now := time.Now()

	// Main query for articles.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_articles`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title"}).
			AddRow(1, now, now, "My Article"))

	// Preload many-to-many: GORM first queries the join table to resolve the association.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `article_tags` WHERE `article_tags`.`test_article_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"test_article_id", "test_tag_id"}).
			AddRow(1, 100))

	// Then GORM queries the related table for the matched IDs.
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `test_tags` WHERE `test_tags`.`id` = ?")).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name"}).
			AddRow(100, now, now, "Go"))

	qb := sqlr.NewQueryBuilderSelect().
		Preload("Tags")
	results, err := s.articleRepo.Query(context.Background(), qb)

	s.Require().NoError(err)
	s.Require().Len(results, 1)
	s.Equal("My Article", results[0].Title)
	s.Require().Len(results[0].Tags, 1)
	s.Equal("Go", results[0].Tags[0].Name)
}
