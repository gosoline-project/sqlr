package sqlr_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

// RepositoryCountTestSuite tests the Repository Count operation using sqlmock.
// Count wraps the scoped select in a subquery, so these tests pin the generated
// SQL as well as the clauses that must be stripped from a count.
type RepositoryCountTestSuite struct {
	suite.Suite
	ctx        context.Context
	client     sqlc.Client
	mock       sqlmock.Sqlmock
	repo       sqlr.Repository[int64, testUser]
	authorRepo sqlr.Repository[int64, testAuthor]
}

// TestRepositoryCountTestSuite runs the repository count test suite.
func TestRepositoryCountTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryCountTestSuite))
}

func (s *RepositoryCountTestSuite) SetupTest() {
	client, mock := newTestClient(s.T())
	s.ctx = s.T().Context()
	s.client = client
	s.mock = mock

	s.repo = mustNewRepo[int64, testUser](s.T(), s.client)
	s.authorRepo = mustNewRepo[int64, testAuthor](s.T(), s.client)
}

func (s *RepositoryCountTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// TestCount_CountsDistinctPrimaryKeys verifies that a count without grouping
// counts distinct primary keys, so joined rows cannot inflate the total.
func (s *RepositoryCountTestSuite) TestCount_CountsDistinctPrimaryKeys() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `test_users`.`id` FROM `test_users` WHERE name = ?) AS `sqlr_count`")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(3))

	qb := sqlr.NewQueryBuilderSelect()
	qb.Where("name = ?", "Alice")

	total, err := s.repo.Count(s.ctx, qb)

	s.Require().NoError(err)
	s.Equal(3, total)
}

// TestCount_WithNilBuilderCountsEveryRow verifies that Count tolerates a nil
// query builder and then counts the whole table.
func (s *RepositoryCountTestSuite) TestCount_WithNilBuilderCountsEveryRow() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `test_users`.`id` FROM `test_users`) AS `sqlr_count`")).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(7))

	total, err := s.repo.Count(s.ctx, nil)

	s.Require().NoError(err)
	s.Equal(7, total)
}

// TestCount_StripsPaginationOrderAndLocking verifies that a count reuses the
// filter scope but drops ORDER BY, LIMIT, OFFSET, and FOR UPDATE. Counting a
// page would otherwise return the page size instead of the total.
func (s *RepositoryCountTestSuite) TestCount_StripsPaginationOrderAndLocking() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `test_users`.`id` FROM `test_users` WHERE name = ?) AS `sqlr_count`")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(42))

	qb := sqlr.NewQueryBuilderSelect()
	qb.Where("name = ?", "Alice").
		OrderBy("created_at DESC").
		Limit(10).
		Offset(20).
		ForUpdate()

	total, err := s.repo.Count(s.ctx, qb)

	s.Require().NoError(err)
	s.Equal(42, total)
}

// TestCount_DoesNotMutateBuilder verifies that stripping the row-only clauses
// happens on a copy, so the caller can reuse one builder for query and count.
func (s *RepositoryCountTestSuite) TestCount_DoesNotMutateBuilder() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `test_users`.`id` FROM `test_users` WHERE name = ?) AS `sqlr_count`")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))

	qb := sqlr.NewQueryBuilderSelect()
	qb.Where("name = ?", "Alice").
		OrderBy("created_at DESC").
		Limit(10).
		Offset(20).
		ForUpdate()

	before, beforeParams, err := qb.ToSql()
	s.Require().NoError(err)

	_, err = s.repo.Count(s.ctx, qb)
	s.Require().NoError(err)

	after, afterParams, err := qb.ToSql()
	s.Require().NoError(err)

	s.Equal(before, after)
	s.Equal(beforeParams, afterParams)
	s.Contains(after, "ORDER BY")
	s.Contains(after, "LIMIT")
	s.Contains(after, "OFFSET")
	s.Contains(after, "FOR UPDATE")
}

// TestCount_WithGroupByCountsGroups verifies that grouping switches the inner
// projection away from the distinct primary key so the outer COUNT(*) returns
// the number of groups.
func (s *RepositoryCountTestSuite) TestCount_WithGroupByCountsGroups() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT 1 FROM `test_users` WHERE name = ? GROUP BY `email`) AS `sqlr_count`")).
		WithArgs("Alice").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(2))

	qb := sqlr.NewQueryBuilderSelect()
	qb.Where("name = ?", "Alice").
		GroupBy("email")

	total, err := s.repo.Count(s.ctx, qb)

	s.Require().NoError(err)
	s.Equal(2, total)
}

// TestCount_WithJoinAppliesJoinClause verifies that joins are replayed into the
// count subquery, because a join can restrict which rows are countable.
func (s *RepositoryCountTestSuite) TestCount_WithJoinAppliesJoinClause() {
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `test_authors`.`id` FROM `test_authors` " +
			"JOIN `test_posts` AS Posts ON `test_authors`.`id` = `Posts`.`author_id`) AS `sqlr_count`")).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(4))

	qb := sqlr.NewQueryBuilderSelect()
	qb.InnerJoin("Posts")

	total, err := s.authorRepo.Count(s.ctx, qb)

	s.Require().NoError(err)
	s.Equal(4, total)
}

// TestCount_WithUnknownJoinRelation verifies that an unknown relation name is
// reported instead of producing an invalid count query.
func (s *RepositoryCountTestSuite) TestCount_WithUnknownJoinRelation() {
	qb := sqlr.NewQueryBuilderSelect()
	qb.InnerJoin("DoesNotExist")

	_, err := s.authorRepo.Count(s.ctx, qb)

	s.Require().Error(err)
	s.Contains(err.Error(), "DoesNotExist")
}
