package sqlr_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/suite"
)

// RepositoryAssociationUpdateTestSuite tests association-aware graph synchronization on Update.
type RepositoryAssociationUpdateTestSuite struct {
	suite.Suite
	client sqlc.Client
	mock   sqlmock.Sqlmock
}

func TestRepositoryAssociationUpdateTestSuite(t *testing.T) {
	suite.Run(t, new(RepositoryAssociationUpdateTestSuite))
}

func (s *RepositoryAssociationUpdateTestSuite) SetupTest() {
	s.client, s.mock = newTestClient(s.T())
}

func (s *RepositoryAssociationUpdateTestSuite) TearDownTest() {
	s.Require().NoError(s.mock.ExpectationsWereMet())
}

func syncAssociations(qb *sqlr.QueryBuilderUpdate) {
	qb.SyncAssociations()
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_Default_DoesNotSynchronizePopulatedAssociations() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{
			{Title: "In Memory Only"},
		},
	}

	result, err := repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 1)
	s.Equal(int64(0), result.Posts[0].AuthorID)
	s.Equal(int64(0), result.Posts[0].GetId())
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasMany_NilSlice_UntouchedNoTransaction() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
	}

	result, err := repo.Update(context.Background(), &entity)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Nil(result.Posts)
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_BelongsTo_UpdatesRelatedAndParentFK() {
	repo := mustNewRepo[int64, assocPostWithAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorNow, "Alice Updated", isTimestamp{}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_post_with_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(7), now, "Updated Post", isTimestamp{}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocPostWithAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Updated Post",
		Author: assocAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorNow,
				UpdatedAt: authorNow,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(int64(7), result.AuthorID)
	s.Equal("Alice Updated", result.Author.Name)
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_BelongsToPointer_UpdatesRelatedAndParentFK() {
	repo := mustNewRepo[int64, assocPostWithPointerAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorNow, "Alice Updated", isTimestamp{}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_post_with_pointer_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(7), now, "Updated Post", isTimestamp{}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	entity := assocPostWithPointerAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Updated Post",
		Author: &assocAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorNow,
				UpdatedAt: authorNow,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().NotNil(result.Author)
	s.Equal(int64(7), result.AuthorID)
	s.Equal("Alice Updated", result.Author.Name)
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasMany_SynchronizesAndDeletesMissingChildren() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post").
			AddRow(int64(11), postNow, postNow, int64(1), "Delete Me"))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_posts` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(int64(1), postNow, "Updated Post", isTimestamp{}, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_posts` (`created_at`, `updated_at`, `author_id`, `title`) VALUES (?, ?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, int64(1), "Brand New").
		WillReturnResult(sqlmock.NewResult(12, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_posts` WHERE `id` = ?")).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Alice Updated",
		Posts: []assocPost{
			{
				Entity: sqlr.Entity[int64]{
					Id:        10,
					CreatedAt: postNow,
					UpdatedAt: postNow,
				},
				Title: "Updated Post",
			},
			{Title: "Brand New"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Posts, 2)
	s.Equal(int64(1), result.Posts[0].AuthorID)
	s.Equal(int64(10), result.Posts[0].GetId())
	s.Equal(int64(1), result.Posts[1].AuthorID)
	s.Equal(int64(12), result.Posts[1].GetId())
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToMany_SynchronizesLinksAndRelatedRows() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()
	tagNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_articles` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_tags` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(tagNow, "golang-updated", isTimestamp{}, int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `assoc_tags` (`created_at`, `updated_at`, `name`) VALUES (?, ?, ?)")).
		WithArgs(isTimestamp{}, isTimestamp{}, "new-tag").
		WillReturnResult(sqlmock.NewResult(102, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_tags` WHERE `assoc_article_tags`.`assoc_article_id` = ?")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_id", "assoc_tag_id"}).
			AddRow(int64(2), int64(100)).
			AddRow(int64(2), int64(101)))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `assoc_article_tags` (`assoc_article_id`, `assoc_tag_id`) VALUES (?, ?)")).
		WithArgs(int64(2), int64(102)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags: []assocTag{
			{
				Entity: sqlr.Entity[int64]{
					Id:        100,
					CreatedAt: tagNow,
					UpdatedAt: tagNow,
				},
				Name: "golang-updated",
			},
			{Name: "new-tag"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Tags, 2)
	s.Equal(int64(100), result.Tags[0].GetId())
	s.Equal(int64(102), result.Tags[1].GetId())
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToMany_EmptySlice_RemovesAllLinks() {
	repo := mustNewRepo[int64, assocArticle](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_articles` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Clear Tags", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_article_tags` WHERE `assoc_article_tags`.`assoc_article_id` = ?")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"assoc_article_id", "assoc_tag_id"}).
			AddRow(int64(2), int64(100)).
			AddRow(int64(2), int64(101)))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `assoc_article_tags` WHERE `assoc_article_id` = ? AND `assoc_tag_id` = ?")).
		WithArgs(int64(2), int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := assocArticle{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Clear Tags",
		Tags:  []assocTag{},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Empty(result.Tags)
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_HasManyPointerElements_RejectsNilElement() {
	repo := mustNewRepo[int64, assocAuthorWithPointerPosts](s.T(), s.client)
	now := time.Now()
	postNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_author_with_pointer_posts` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Alice Updated", isTimestamp{}, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT * FROM `assoc_posts` WHERE `assoc_posts`.`author_id` = ?")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "author_id", "title"}).
			AddRow(int64(10), postNow, postNow, int64(1), "Old Post"))
	s.mock.ExpectRollback()

	entity := assocAuthorWithPointerPosts{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:  "Alice Updated",
		Posts: []*assocPost{nil},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)
	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "HasMany relation \"Posts\"[0] is nil")
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_ManyToManyPointerElements_RejectsNilElement() {
	repo := mustNewRepo[int64, assocArticleWithPointerTags](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_article_with_pointer_tags` SET `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Go Tips Updated", isTimestamp{}, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectRollback()

	entity := assocArticleWithPointerTags{
		Entity: sqlr.Entity[int64]{
			Id:        2,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Go Tips Updated",
		Tags:  []*assocTag{nil},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)
	s.Require().Error(err)
	s.Nil(result)
	s.ErrorContains(err, "ManyToMany relation \"Tags\"[0] is nil")
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAssociations_BelongsToWithNullableFK() {
	repo := mustNewRepo[int64, testPostWithNullableAuthor](s.T(), s.client)
	now := time.Now()
	authorNow := now.Add(-time.Hour)

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(authorNow, "Alice Updated", isTimestamp{}, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `test_post_with_nullable_authors` SET `author_id` = ?, `created_at` = ?, `title` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(sqlmock.AnyArg(), now, "Updated Post", isTimestamp{}, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectCommit()

	entity := testPostWithNullableAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        5,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title: "Updated Post",
		Author: testAuthor{
			Entity: sqlr.Entity[int64]{
				Id:        7,
				CreatedAt: authorNow,
				UpdatedAt: authorNow,
			},
			Name: "Alice Updated",
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().NotNil(result.AuthorID)
	s.Equal(int64(7), *result.AuthorID)
	s.Equal("Alice Updated", result.Author.Name)
}

func (s *RepositoryAssociationUpdateTestSuite) TestUpdate_SyncAssociations_MissingParentReturnsNotFound() {
	repo := mustNewRepo[int64, assocAuthor](s.T(), s.client)
	now := time.Now()

	s.mock.ExpectBegin()

	s.mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `assoc_authors` SET `created_at` = ?, `name` = ?, `updated_at` = ? WHERE `id` = ?")).
		WithArgs(now, "Missing", isTimestamp{}, int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	s.mock.ExpectRollback()

	entity := assocAuthor{
		Entity: sqlr.Entity[int64]{
			Id:        99,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Missing",
		Posts: []assocPost{
			{Title: "Should Not Be Saved"},
		},
	}

	result, err := repo.Update(context.Background(), &entity, syncAssociations)

	s.Require().Error(err)
	s.Nil(result)
	s.True(errors.Is(err, sqlr.ErrNotFound))
}
