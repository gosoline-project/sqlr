//go:build integration && fixtures

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

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
