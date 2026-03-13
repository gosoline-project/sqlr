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
