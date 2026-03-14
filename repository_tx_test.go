package sqlr_test

import (
	"testing"

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
