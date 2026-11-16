package sqlr_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
)

type emptyTableNamerUser struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

func (emptyTableNamerUser) TableName() string { return "" }

func TestRepositorySettings_TableNameRoutesEachRepositoryIndependently(t *testing.T) {
	client, mock := newTestClient(t)
	repoA := mustNewRepoWithSettings[int64, testUser](t, client, sqlr.Settings{TableName: "tenant_users_a"})
	repoB := mustNewRepoWithSettings[int64, testUser](t, client, sqlr.Settings{TableName: "tenant_users_b"})
	now := time.Now()

	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `tenant_users_a` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)",
	)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice", "alice@example.test").
		WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `tenant_users_b` (`created_at`, `updated_at`, `name`, `email`) VALUES (?, ?, ?, ?)",
	)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Bob", "bob@example.test").
		WillReturnResult(sqlmock.NewResult(202, 1))

	userA := testUser{Name: "Alice", Email: "alice@example.test"}
	userB := testUser{Name: "Bob", Email: "bob@example.test"}
	require.NoError(t, repoA.Create(context.Background(), &userA))
	require.NoError(t, repoB.Create(context.Background(), &userB))
	require.Equal(t, int64(101), userA.GetId())
	require.Equal(t, int64(202), userB.GetId())

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `tenant_users_a` WHERE `id` = ? LIMIT ?",
	)).
		WithArgs(int64(101), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(int64(101), now, now, "Alice", "alice@example.test"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `tenant_users_b` WHERE name = ?")).
		WithArgs("Bob").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(int64(202), now, now, "Bob", "bob@example.test"))

	readA, err := repoA.Read(context.Background(), 101)
	require.NoError(t, err)
	require.Equal(t, "Alice", readA.Name)
	queryB, err := repoB.Query(context.Background(), func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("name = ?", "Bob")
	})
	require.NoError(t, err)
	require.Len(t, queryB, 1)
	require.Equal(t, int64(202), queryB[0].GetId())

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `tenant_users_a`.`id` FROM `tenant_users_a`) AS `sqlr_count`",
	)).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `tenant_users_b`.`id` FROM `tenant_users_b`) AS `sqlr_count`",
	)).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))

	countA, err := repoA.Count(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, countA)
	countB, err := repoB.Count(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, countB)

	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `tenant_users_a` SET `created_at` = ?, `email` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?",
	)).
		WithArgs(isTimestamp{}, "alice-updated@example.test", "Alice Updated", isTimestamp{}, int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	readA.Name = "Alice Updated"
	readA.Email = "alice-updated@example.test"
	_, err = repoA.Update(context.Background(), readA)
	require.NoError(t, err)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `tenant_users_b` WHERE `id` = ?")).
		WithArgs(int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repoB.Delete(context.Background(), 202))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositorySettings_TableNameQualifiesRootJoinAndCount(t *testing.T) {
	client, mock := newTestClient(t)
	repo := mustNewRepoWithSettings[int64, testAuthor](t, client, sqlr.Settings{TableName: "tenant_authors"})
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `tenant_authors`.`id`, `tenant_authors`.`created_at`, `tenant_authors`.`updated_at`, `tenant_authors`.`name`, " +
			"`Posts`.`id` AS `Posts__id`, `Posts`.`created_at` AS `Posts__created_at`, `Posts`.`updated_at` AS `Posts__updated_at`, " +
			"`Posts`.`author_id` AS `Posts__author_id`, `Posts`.`title` AS `Posts__title`, `Posts`.`status` AS `Posts__status`" +
			" FROM `tenant_authors` LEFT JOIN `test_posts` AS Posts ON `tenant_authors`.`id` = `Posts`.`author_id`",
	)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "Posts__id", "Posts__created_at", "Posts__updated_at", "Posts__author_id", "Posts__title", "Posts__status"}).
			AddRow(int64(1), now, now, "Alice", int64(10), now, now, int64(1), "First Post", "published"))

	results, err := repo.Query(context.Background(), func(qb *sqlr.QueryBuilderSelect) {
		qb.LeftJoin("Posts")
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Posts, 1)
	require.Equal(t, "First Post", results[0].Posts[0].Title)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT COUNT(*) FROM (SELECT DISTINCT `tenant_authors`.`id` FROM `tenant_authors` " +
			"JOIN `test_posts` AS Posts ON `tenant_authors`.`id` = `Posts`.`author_id`) AS `sqlr_count`",
	)).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))

	count, err := repo.Count(context.Background(), sqlr.NewQueryBuilderSelect().InnerJoin("Posts"))
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryTxSettings_PreparedStatementsUseTableName(t *testing.T) {
	client, mock := newTestClient(t)
	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](client, sqlr.Settings{
		PreparedStatements: true,
		TableName:          "tenant_users",
	})
	require.NoError(t, err)

	readSQL := "SELECT `id`, `created_at`, `updated_at`, `name`, `email` FROM `tenant_users` WHERE `id` = ? LIMIT ?"
	mock.ExpectBegin()
	mock.ExpectPrepare(regexp.QuoteMeta(readSQL))
	mock.ExpectPrepare(regexp.QuoteMeta(readSQL))
	mock.ExpectQuery(regexp.QuoteMeta(readSQL)).
		WithArgs(int64(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "email"}).
			AddRow(int64(5), time.Now(), time.Now(), "Prepared", "prepared@example.test"))
	mock.ExpectCommit()

	var result *testUser
	err = runWithTx(context.Background(), client, func(ttx sqlr.TTx) error {
		var readErr error
		result, readErr = repo.Read(ttx, 5)

		return readErr
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(5), result.GetId())
	require.NoError(t, repo.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositorySettings_TableNameOverridesEmptyTableNamer(t *testing.T) {
	client, mock := newTestClient(t)
	repo := mustNewRepoWithSettings[int64, emptyTableNamerUser](t, client, sqlr.Settings{TableName: "explicit_users"})

	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `explicit_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)",
	)).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(77, 1))

	user := emptyTableNamerUser{Name: "Alice"}
	require.NoError(t, repo.Create(context.Background(), &user))
	require.Equal(t, int64(77), user.GetId())
	require.NoError(t, mock.ExpectationsWereMet())
}
