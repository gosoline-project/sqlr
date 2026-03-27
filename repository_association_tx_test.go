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

// --------------------------------------------------------------------------
// RepositoryTx: association save within an existing transaction
// --------------------------------------------------------------------------

// RepositoryTxAssociationTestSuite tests auto-save of associations in RepositoryTx.
type RepositoryTxAssociationTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

// TestRepositoryTxAssociationTestSuite runs the repository tx association test suite.
func TestRepositoryTxAssociationTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryTxAssociationTestSuite))
}

func (s *RepositoryTxAssociationTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryTxAssociationTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

// TestCreate_HasMany_UsesExistingTransaction verifies that Create uses existing transaction for has-many relations.
func (s *RepositoryTxAssociationTestSuite) TestCreate_HasMany_UsesExistingTransaction() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	// The caller manages the transaction; RepositoryTx uses it directly.
	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Judy").
		WillReturnResult(sqlmock.NewResult(6, 1))

	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(6), "Judy's Post").
		WillReturnResult(sqlmock.NewResult(60, 1))

	s.mock.ExpectCommit()

	err = s.client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)

		entity := assocAuthor{
			Name:  "Judy",
			Posts: []assocPost{{Title: "Judy's Post"}},
		}

		return txRepo.Create(ttx, &entity)
	})
	s.Require().NoError(err)
}

// TestCreate_HasMany_NoAssociations_NoExtraQueries verifies that RepositoryTx Create avoids extra association queries when no has-many data is populated.
func (s *RepositoryTxAssociationTestSuite) TestCreate_HasMany_NoAssociations_NoExtraQueries() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	s.mock.ExpectBegin()

	// Only the author INSERT — no association INSERTs.
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Karl").
		WillReturnResult(sqlmock.NewResult(7, 1))

	s.mock.ExpectCommit()

	err = s.client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)
		entity := assocAuthor{Name: "Karl"} // no posts, no profile

		return txRepo.Create(ttx, &entity)
	})
	s.Require().NoError(err)
}

// TestCreate_EmptyRelationSlice_NoAssociationTransactionWork verifies that RepositoryTx Create performs no association work for empty relation slices.
func (s *RepositoryTxAssociationTestSuite) TestCreate_EmptyRelationSlice_NoAssociationTransactionWork() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `assoc_authors` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "Lena").
		WillReturnResult(sqlmock.NewResult(8, 1))
	s.mock.ExpectCommit()

	err = s.client.WithTx(context.Background(), func(tx sqlc.Tx) error {
		ttx := sqlr.NewTx(tx)
		entity := assocAuthor{Name: "Lena", Posts: []assocPost{}}

		return txRepo.Create(ttx, &entity)
	})
	s.Require().NoError(err)
}

// TestCreate_NilEntity_WithAssociationOptionsReturnsError verifies that Create returns an error for nil entities even when association sync is configured.
func (s *RepositoryTxAssociationTestSuite) TestCreate_NilEntity_WithAssociationOptionsReturnsError() {
	txRepo, err := sqlr.NewRepositoryTxWithSettings[int64, assocAuthor](s.client, sqlr.DefaultSettings())
	s.Require().NoError(err)

	err = txRepo.Create(sqlr.TTx{}, nil, syncCreatePosts)

	s.Require().Error(err)
	s.Require().ErrorIs(err, sqlr.ErrNilEntity)
}
