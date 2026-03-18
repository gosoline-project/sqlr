package sqlr

import (
	"context"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/fixtures"
	"github.com/justtrackio/gosoline/pkg/log"
	"github.com/spf13/cast"
)

func FixtureEntity[K KeyTypes](id K, createdAt string, updatedAt string) Entity[K] {
	return Entity[K]{
		Id:        id,
		CreatedAt: cast.ToTime(createdAt),
		UpdatedAt: cast.ToTime(updatedAt),
	}
}

type fixtureWriter[K KeyTypes, E Entitier[K]] struct {
	logger log.Logger
	repo   Repository[K, E]
}

func FixtureSetFactory[K KeyTypes, E Entitier[K]](data fixtures.NamedFixtures[E], options ...fixtures.FixtureSetOption) fixtures.FixtureSetFactory {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (fixtures.FixtureSet, error) {
		var err error
		var writer fixtures.FixtureWriter

		if writer, err = NewFixtureWriter[K, E](ctx, config, logger); err != nil {
			return nil, fmt.Errorf("failed to create fixture writer for %T: %w", new(E), err)
		}

		return fixtures.NewSimpleFixtureSet(data, writer, options...), nil
	}
}

func NewFixtureWriter[K KeyTypes, E Entitier[K]](ctx context.Context, config cfg.Config, logger log.Logger) (fixtures.FixtureWriter, error) {
	var err error
	var settings *sqlc.Settings
	var client sqlc.Client
	var repo Repository[K, E]

	if settings, err = sqlc.ReadSettings(config, "default"); err != nil {
		return nil, fmt.Errorf("can not create repo: %w", err)
	}
	settings.Parameters["FOREIGN_KEY_CHECKS"] = "0"

	if client, err = sqlc.NewClientWithSettings(ctx, config, logger, "default", settings); err != nil {
		return nil, fmt.Errorf("can not create client: %w", err)
	}

	if repo, err = NewRepositoryWithInterfaces[K, E](client, DefaultSettings()); err != nil {
		return nil, fmt.Errorf("can not create repository: %w", err)
	}

	return NewFixtureWriterWithInterfaces(logger, repo), nil
}

func NewFixtureWriterWithInterfaces[K KeyTypes, E Entitier[K]](logger log.Logger, repo Repository[K, E]) fixtures.FixtureWriter {
	return &fixtureWriter[K, E]{
		logger: logger,
		repo:   repo,
	}
}

func (m *fixtureWriter[K, E]) Write(ctx context.Context, fixtures []any) error {
	var ok bool
	var entity E

	for _, item := range fixtures {
		if entity, ok = item.(E); !ok {
			return fmt.Errorf("assertion failed: %T is not db_repo.ModelBased", item)
		}

		if err := m.repo.Create(ctx, &entity, func(qb *QueryBuilderCreate) {
			qb.DisableAutoUpdates()
		}); err != nil {
			return err
		}
	}

	m.logger.Info(ctx, "loaded %d mysql fixtures", len(fixtures))

	return nil
}
