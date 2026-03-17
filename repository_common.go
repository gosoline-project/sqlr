package sqlr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gosoline-project/sqlc"
	"github.com/spf13/cast"
)

// ErrNotFound indicates that the requested entity does not exist.
// Use errors.Is(err, ErrNotFound) to check for this condition.
var ErrNotFound = errors.New("entity not found")

// ErrNilEntity indicates that a repository method received a nil entity pointer.
// Use errors.Is(err, ErrNilEntity) to check for this condition.
var ErrNilEntity = errors.New("entity must not be nil")

// newRepositoryCommon creates a new repositoryCommon by parsing the schema for entity
// type E using reflection. The schema is used at query time to generate SQL and
// validate join/preload relations. Returns an error if the entity's schema cannot be parsed.
func newRepositoryCommon[K KeyTypes, E Entitier[K]](client sqlc.Client, settings Settings) (repositoryCommon[K, E], error) {
	schema, err := ParseSchema[E]()
	if err != nil {
		return repositoryCommon[K, E]{}, fmt.Errorf("can not parse model schema of %T: %w", *new(E), err)
	}

	return repositoryCommon[K, E]{
		schema:         schema,
		settings:       settings,
		statementCache: newStatementCache(client, settings.PreparedStatements),
	}, nil
}

// repositoryCommon provides the shared CRUD and query logic used by both
// Repository (non-transactional) and RepositoryTx (transactional). It is
// parameterized by a key type K and an entity type E that implements Entitier[K].
// The parsed EntitySchema is stored to validate join/preload relations and
// generate SQL at query time.
type repositoryCommon[K KeyTypes, E Entitier[K]] struct {
	schema         *EntitySchema
	settings       Settings
	statementCache *statementCache
}

func applyOptions[T any](builder T, opts []func(T)) T {
	for _, opt := range opts {
		opt(builder)
	}

	return builder
}

// createEntityWithAssociations persists an entity together with all populated
// association fields. It handles the four relationship types in the correct order:
//
//  1. BelongsTo associations are inserted first (if their PK is zero) so that the
//     parent's FK column can be set before the parent row is written.
//  2. The parent entity row is inserted.
//  3. HasOne and HasMany associations are inserted with their FK set to the parent PK.
//  4. ManyToMany associations are inserted and join-table rows are created.
//
// All operations must be executed within a transaction so that a failure at any step
// rolls back the entire tree. The caller is responsible for providing a transaction
// querier (q) and a non-nil ttx when association saves are required.
func (r *repositoryCommon[K, E]) createEntityWithAssociations(q sqlc.Querier, ctx context.Context, entity *E, policy *associationSyncPolicy) error {
	// Phase 1: persist BelongsTo relations and set their FKs on the parent.
	if err := r.saveBelongsToAssociations(q, ctx, entity, policy); err != nil {
		return err
	}

	// Phase 2: persist the parent entity row.
	if err := r.createEntity(q, ctx, entity); err != nil {
		return err
	}

	// Phase 3 + 4: persist HasOne, HasMany, and ManyToMany relations.
	return r.saveAssociations(q, ctx, entity, policy)
}

// createEntity persists a new entity to the database. It extracts insert column
// values from the entity using reflection, executes an INSERT via sqlc, and sets
// an auto-increment primary key back on the entity when applicable.
// Relationship fields are not persisted; use createEntityWithAssociations to also
// persist populated association fields.
func (r *repositoryCommon[K, E]) createEntity(q sqlc.Querier, ctx context.Context, entity *E) error {
	rv, err := requireEntityValue(entity)
	if err != nil {
		return err
	}

	now := time.Now()

	insertCols := r.schema.InsertColumns()
	if err := setCreateTimestamps(rv, r.schema, now); err != nil {
		return fmt.Errorf("failed to set create timestamps: %w", err)
	}
	vals := buildInsertValues(rv, r.schema)

	sqler := sqlc.IntoG[E](r.schema.TableName).
		Columns(insertCols...).
		Values(vals...)

	_, result, err := r.statementCache.Exec(ctx, sqler, q)
	if err != nil {
		return fmt.Errorf("failed to create entity: %w", err)
	}

	if r.schema.PrimaryKey == nil || !r.schema.PrimaryKey.AutoIncrement {
		return nil
	}

	// Set the auto-generated primary key back on the entity.
	lastID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	if setter, ok := any(entity).(setIdAware[K]); ok {
		key, err := castIntKey[K](lastID)
		if err != nil {
			return fmt.Errorf("failed to set primary key: %w", err)
		}

		setter.SetId(key)

		return nil
	}

	if err := setEntityPrimaryKey(r.schema, rv, lastID); err != nil {
		return fmt.Errorf("failed to set primary key: %w", err)
	}

	return nil
}

// readEntity retrieves a single entity by its primary key id. It uses sqlc's
// query builder to construct the SQL and then executes the raw query via
// q.Get() for proper sqlx scanning (including embedded struct fields that
// ForType/refl.GetTags does not discover). Relations tagged with the "preload"
// db option are automatically loaded after the main query. Auto-preload supports
// HasOne, HasMany, BelongsTo, and ManyToMany, including nested preload paths.
func (r *repositoryCommon[K, E]) readEntity(q sqlc.Querier, ctx context.Context, id K) (*E, error) {
	if r.schema.PrimaryKey == nil {
		return nil, fmt.Errorf("primary key not defined for %s", r.schema.TableName)
	}

	// Build the SELECT SQL and args via sqlc builder.
	sqler := sqlc.FromG[E](r.schema.TableName).
		Columns(r.schema.allColumnsAny...).
		Where(sqlc.Col(r.schema.PrimaryKey.Name).Eq(id)).
		Limit(1)

	var entity E
	if err := r.statementCache.Get(ctx, sqler, q, &entity); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("entity id=%v: %w", id, ErrNotFound)
		}

		return nil, fmt.Errorf("failed to read entity: %w", err)
	}

	// Execute auto-preloads for relations tagged with "preload".
	autoPreloads := r.schema.AutoPreloads()
	if len(autoPreloads) > 0 {
		results := []E{entity}
		if err := r.executePreloads(q, ctx, autoPreloads, results); err != nil {
			return nil, err
		}

		entity = results[0]
	}

	return &entity, nil
}

// readEntityWithOpts retrieves a single entity by primary key with optional join
// and preload configuration. When the QueryBuilderRead has no explicit joins or
// preloads, this delegates to readEntity for the simple fast path. When joins or
// preloads are requested, it converts the read builder into a QueryBuilderSelect
// with a WHERE pk = ? constraint and delegates to queryEntities, reusing the
// full join/preload infrastructure. Preload-only reads still apply LIMIT 1,
// while joined reads enforce single-entity semantics after hydration so has-many
// joins do not truncate related rows at the SQL level.
func (r *repositoryCommon[K, E]) readEntityWithOpts(q sqlc.Querier, ctx context.Context, id K, qbr *QueryBuilderRead) (*E, error) {
	if !qbr.hasRelations() {
		return r.readEntity(q, ctx, id)
	}

	if r.schema.PrimaryKey == nil {
		return nil, fmt.Errorf("primary key not defined for %s", r.schema.TableName)
	}

	qb := qbr.toQueryBuilderSelect(r.schema.TableName, r.schema.PrimaryKey.Name, id)

	results, err := r.queryEntities(q, ctx, qb)
	if err != nil {
		return nil, fmt.Errorf("failed to read entity: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("entity id=%v: %w", id, ErrNotFound)
	}

	return &results[0], nil
}

// updateEntity saves all fields of the given entity back to the database. It builds
// a column-value map from the entity using reflection and executes an UPDATE via sqlc.
// Relationship fields are not synchronized; Update is intentionally not cascade-aware.
func (r *repositoryCommon[K, E]) updateEntity(q sqlc.Querier, ctx context.Context, entity *E) (*E, error) {
	rv, err := requireEntityValue(entity)
	if err != nil {
		return nil, err
	}

	if r.schema.PrimaryKey == nil {
		return nil, fmt.Errorf("primary key not defined for %s", r.schema.TableName)
	}

	now := time.Now()

	if err := setUpdateTimestamps(rv, r.schema, now); err != nil {
		return nil, fmt.Errorf("failed to set update timestamps: %w", err)
	}
	setMap, pkValue := buildUpdateSetMap(rv, r.schema)

	// Build the UPDATE SQL and args via sqlc builder.
	sqler := sqlc.UpdateG[E](r.schema.TableName).
		SetMap(setMap).
		Where(sqlc.Col(r.schema.PrimaryKey.Name).Eq(pkValue))

	_, result, err := r.statementCache.Exec(ctx, sqler, q)
	if err != nil {
		return nil, fmt.Errorf("failed to update entity: %w", err)
	}

	if err := errNoRowsAffected(result, fmt.Errorf("entity id=%v: %w", (*entity).GetId(), ErrNotFound)); err != nil {
		return nil, err
	}

	return entity, nil
}

// updateEntityWithAssociations synchronizes the entity graph rooted at entity.
// It updates the parent row and all explicitly-present associations in one
// transaction-aware operation after the caller opted into association sync via
// QueryBuilderUpdate. BelongsTo relations are synchronized first so the parent's
// foreign keys are current before the root row is updated. HasOne, HasMany, and
// ManyToMany relations are synchronized afterwards.
func (r *repositoryCommon[K, E]) updateEntityWithAssociations(q sqlc.Querier, ctx context.Context, entity *E, policy *associationSyncPolicy) (*E, error) {
	rv, err := requireEntityValue(entity)
	if err != nil {
		return nil, err
	}

	state := newAssociationSyncState()

	if err := syncExistingEntityGraph(r.statementCache, q, ctx, r.schema, rv, state, policy); err != nil {
		return nil, err
	}

	return entity, nil
}

// deleteEntity removes the entity with the given id from the database. Returns an
// error if no entity with that id exists. Related rows are not cascade-deleted by
// this method.
func (r *repositoryCommon[K, E]) deleteEntity(q sqlc.Querier, ctx context.Context, id K) error {
	if r.schema.PrimaryKey == nil {
		return fmt.Errorf("primary key not defined for %s", r.schema.TableName)
	}

	sqler := sqlc.DeleteG[E](r.schema.TableName).
		Where(sqlc.Col(r.schema.PrimaryKey.Name).Eq(id))

	_, result, err := r.statementCache.Exec(ctx, sqler, q)
	if err != nil {
		return fmt.Errorf("failed to delete entity: %w", err)
	}

	if err := errNoRowsAffected(result, fmt.Errorf("entity id=%v: %w", id, ErrNotFound)); err != nil {
		return err
	}

	return nil
}

// mergeAutoPreloads merges schema-defined auto-preloads (from "preload" tag) with
// explicit preloads, deduplicating by relation name. Explicit preloads take
// precedence so their conditions are preserved when both are present.
func (r *repositoryCommon[K, E]) mergeAutoPreloads(explicit []preloadEntry) []preloadEntry {
	autoPreloads := r.schema.AutoPreloads()
	if len(autoPreloads) == 0 {
		return explicit
	}

	// Build a set of explicitly preloaded relation names.
	seen := make(map[string]struct{}, len(explicit))
	for _, p := range explicit {
		seen[p.relation] = struct{}{}
	}

	// Add auto-preloads that are not already explicitly requested.
	merged := make([]preloadEntry, len(explicit))
	copy(merged, explicit)

	for _, ap := range autoPreloads {
		if _, exists := seen[ap.relation]; !exists {
			merged = append(merged, ap)
		}
	}

	return merged
}

// queryEntities executes a SELECT query built from the given QueryBuilderSelect. It
// validates join/preload relations against the entity's schema, applies joins, WHERE,
// GROUP BY, HAVING, ORDER BY, LIMIT, and OFFSET clauses, and returns the matching entities.
// Relations tagged with the "preload" db option are automatically merged into the
// preload list (deduplicated against explicit preloads). Join loading supports
// direct HasOne/HasMany/BelongsTo relations; preload loading supports HasOne,
// HasMany, BelongsTo, and ManyToMany, including nested preload paths.
func (r *repositoryCommon[K, E]) queryEntities(q sqlc.Querier, ctx context.Context, qb *QueryBuilderSelect) ([]E, error) {
	// Merge auto-preloads (from "preload" tag) with explicit preloads, deduplicating.
	preloads := r.mergeAutoPreloads(qb.preloads)

	hasJoins := len(qb.joins) > 0
	hasPreloads := len(preloads) > 0

	// Validate relations before executing any queries.
	if hasJoins {
		if err := r.validateJoinRelations(qb.joins); err != nil {
			return nil, err
		}
	}

	if hasPreloads {
		if err := r.validatePreloadRelations(preloads); err != nil {
			return nil, err
		}
	}

	if hasJoins {
		return r.queryWithJoins(q, ctx, qb, preloads)
	}

	return r.querySimple(q, ctx, qb, preloads)
}

// querySimple executes a query without joins. It uses sqlc's generic select builder
// for the main query and optionally executes preload queries afterwards.
func (r *repositoryCommon[K, E]) querySimple(q sqlc.Querier, ctx context.Context, qb *QueryBuilderSelect, preloads []preloadEntry) ([]E, error) {
	// Build the SELECT query using sqlc builder.
	sqlcQB := sqlc.From(r.schema.TableName)

	// Apply WHERE, GROUP BY, HAVING, ORDER BY, LIMIT, and OFFSET clauses.
	sqlcQB, err := qb.applyToSqlcBuilder(sqlcQB)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var results []E
	if err := r.statementCache.Select(ctx, sqlcQB, q, &results); err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Execute preloads if any.
	if len(preloads) > 0 && len(results) > 0 {
		if err := r.executePreloads(q, ctx, preloads, results); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// castIntKey converts a LastInsertId int64 value to an integer key type K.
// It is intended for auto-increment primary keys only.
func castIntKey[K KeyTypes](id int64) (K, error) {
	var zero K

	switch any(zero).(type) {
	case int:
		v, err := cast.ToE[int](id)

		return any(v).(K), err
	case int64:
		v, err := cast.ToE[int64](id)

		return any(v).(K), err
	case uint:
		v, err := cast.ToE[uint](id)

		return any(v).(K), err
	case uint64:
		v, err := cast.ToE[uint64](id)

		return any(v).(K), err
	case *int:
		v, err := cast.ToE[int](id)
		ptr := &v

		return any(ptr).(K), err
	case *int64:
		v, err := cast.ToE[int64](id)
		ptr := &v

		return any(ptr).(K), err
	case *uint:
		v, err := cast.ToE[uint](id)
		ptr := &v

		return any(ptr).(K), err
	case *uint64:
		v, err := cast.ToE[uint64](id)
		ptr := &v

		return any(ptr).(K), err
	default:

		return zero, fmt.Errorf("unsupported non-integer key type for auto-increment: %T", zero)
	}
}
