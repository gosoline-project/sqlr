//go:build integration && fixtures

package test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/clock"
	"github.com/justtrackio/gosoline/pkg/test/suite"
)

// TestQueryTestSuite runs the query test suite.
func TestQueryTestSuite(t *testing.T) {
	suite.Run(t, new(QueryTestSuite))
}

type QueryTestSuite struct {
	suite.Suite

	ctx  context.Context
	repo sqlr.Repository[int64, Post]
}

func (s *QueryTestSuite) SetupSuite() []suite.Option {
	return []suite.Option{
		suite.WithLogLevel("debug"),
		suite.WithConfigFile("config.yml"),
		suite.WithFixtureSetFactory(Fixtures()),
		suite.WithClockProvider(clock.NewRealClock()),
	}
}

func (s *QueryTestSuite) SetupTest() error {
	s.ctx = s.Env().Context()
	config := s.Env().Config()
	logger := s.Env().Logger()

	var err error
	if s.repo, err = sqlr.NewRepository[int64, Post](s.ctx, config, logger, "default"); err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	return nil
}

// TestQuery verifies that the fixture-backed repository reads the seeded post correctly.
func (s *QueryTestSuite) TestQuery() {
	post, err := s.repo.Read(s.ctx, 1)
	s.Require().NoError(err, "could not read post")

	expected := &Post{
		Entity: sqlr.Entity[int64]{
			Id:        1,
			CreatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		},
		AuthorID: 1,
		Title:    "Getting Started with Go",
		Status:   "published",
		Author:   Author{},
		Comments: nil,
		Tags: []Tag{
			{
				Entity: sqlr.Entity[int64]{
					Id:        1,
					CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				},
				Name: "golang",
			},
			{
				Entity: sqlr.Entity[int64]{
					Id:        4,
					CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				},
				Name: "tutorial",
			},
		},
	}

	s.Equal(expected, post)
}

// TestUpdate_ZeroRowsUsesCurrentReadWithinRepeatableRead covers both sides of
// MySQL's stale consistent-read snapshot after a zero-row UPDATE.
func (s *QueryTestSuite) TestUpdate_ZeroRowsUsesCurrentReadWithinRepeatableRead() {
	client, err := sqlc.ProvideClient(s.ctx, s.Env().Config(), s.Env().Logger(), "default")
	s.Require().NoError(err)

	var firstID int64
	err = client.Get(s.ctx, &firstID, "SELECT COALESCE(MAX(id), 0) + 100 FROM posts")
	s.Require().NoError(err)

	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	updatedAt := createdAt
	makePost := func(id int64, title string) Post {
		return Post{
			Entity: sqlr.Entity[int64]{
				Id:        id,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			AuthorID: 1,
			Title:    title,
			Status:   "published",
		}
	}
	insertPost := func(post Post) error {
		_, err := client.Exec(
			s.ctx,
			"INSERT INTO posts (id, created_at, updated_at, author_id, title, status) VALUES (?, ?, ?, ?, ?, ?)",
			post.GetId(), post.CreatedAt, post.UpdatedAt, post.AuthorID, post.Title, post.Status,
		)

		return err
	}

	deletedPost := makePost(firstID, "snapshot delete")
	insertedPost := makePost(firstID+1, "snapshot insert")
	defer func() {
		_, cleanupErr := client.Exec(context.Background(), "DELETE FROM posts WHERE id IN (?, ?)", firstID, firstID+1)
		s.Require().NoError(cleanupErr)
	}()
	s.Require().NoError(insertPost(deletedPost))

	txRepo, err := sqlr.NewRepositoryTx[int64, Post]()
	s.Require().NoError(err)
	disableAutoUpdates := func(qb *sqlr.QueryBuilderUpdate) {
		qb.DisableAutoUpdates()
	}

	err = client.WithTx(s.ctx, func(tx sqlc.Tx) error {
		var snapshotID int64
		if err := tx.Get(s.ctx, &snapshotID, "SELECT id FROM posts WHERE id = ?", deletedPost.GetId()); err != nil {
			return fmt.Errorf("failed to establish existing-row snapshot: %w", err)
		}

		result, err := client.Exec(s.ctx, "DELETE FROM posts WHERE id = ?", deletedPost.GetId())
		if err != nil {
			return fmt.Errorf("failed to delete row from concurrent connection: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to read concurrent delete rows affected: %w", err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("expected concurrent delete to affect one row, got %d", rowsAffected)
		}

		_, err = txRepo.Update(sqlr.NewTx(tx), &deletedPost, disableAutoUpdates)

		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	s.ErrorIs(err, sqlr.ErrNotFound)

	var updatedPost *Post
	err = client.WithTx(s.ctx, func(tx sqlc.Tx) error {
		var snapshotID int64
		err := tx.Get(s.ctx, &snapshotID, "SELECT id FROM posts WHERE id = ?", insertedPost.GetId())
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("expected empty-row snapshot to return sql.ErrNoRows, got %v", err)
		}

		if err := insertPost(insertedPost); err != nil {
			return fmt.Errorf("failed to insert row from concurrent connection: %w", err)
		}

		updatedPost, err = txRepo.Update(sqlr.NewTx(tx), &insertedPost, disableAutoUpdates)

		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	s.Require().NoError(err)
	s.Require().NotNil(updatedPost)
	s.Equal(insertedPost.GetId(), updatedPost.GetId())
}
