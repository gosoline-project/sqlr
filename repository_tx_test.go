package sqlr_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
)

// TestNewRepositoryTxWithSettings_PreparedStatementsWithoutClient verifies that
// transactional repositories reject prepared statement mode when no client is
// available for preparing statements.
func TestNewRepositoryTxWithSettings_PreparedStatementsWithoutClient(t *testing.T) {
	t.Parallel()

	_, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.Settings{PreparedStatements: true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "client")
}

func TestRepositoryTxCreate_NilEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Create(sqlr.TTx{}, nil)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
}

func TestRepositoryTxUpdate_NilEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testUser](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	result, err := repo.Update(sqlr.TTx{}, nil)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
	require.Nil(t, result)
}

func TestRepositoryTxCreate_NilAssociationEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	err = repo.Create(sqlr.TTx{}, nil, syncCreatePosts)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
}

func TestRepositoryTxUpdate_NilAssociationEntityReturnsError(t *testing.T) {
	t.Parallel()

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](nil, sqlr.DefaultSettings())
	require.NoError(t, err)

	result, err := repo.Update(sqlr.TTx{}, nil, syncAllAssociations)
	require.ErrorIs(t, err, sqlr.ErrNilEntity)
	require.Nil(t, result)
}

func TestRepositoryTxCreate_AutoIncrementPrimaryKey_UsesReflectionWithoutSetId(t *testing.T) {
	client, mock := newTestClient(t)

	repo, err := sqlr.NewRepositoryTxWithSettings[int64, testNoSetterUser](client, sqlr.DefaultSettings())
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `test_no_setter_users` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Alice").
		WillReturnResult(sqlmock.NewResult(51, 1))
	mock.ExpectCommit()

	err = client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)
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
