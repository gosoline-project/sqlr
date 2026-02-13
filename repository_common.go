package sqlr

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type applyFunc[K KeyTypes, E Entitier[K]] func(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error)

// newRepositoryCommon creates a new repositoryCommon by parsing the GORM schema for
// entity type E. The schema is used at query time to validate join relations. Returns
// an error if the entity's model cannot be parsed.
func newRepositoryCommon[K KeyTypes, E Entitier[K]](db *gorm.DB) (repositoryCommon[K, E], error) {
	var model E
	stmt := &gorm.Statement{DB: db}

	if err := stmt.Parse(&model); err != nil {
		return repositoryCommon[K, E]{}, fmt.Errorf("can not parse model schema of %T: %w", model, err)
	}

	return repositoryCommon[K, E]{
		schema: stmt.Schema,
	}, nil
}

// repositoryCommon provides the shared CRUD and query logic used by both
// Repository (non-transactional) and RepositoryTx (transactional). It is
// parameterized by a key type K and an entity type E that implements Entitier[K].
// The parsed GORM schema is stored to validate join relations at query time.
type repositoryCommon[K KeyTypes, E Entitier[K]] struct {
	schema *schema.Schema
}

// createEntity persists a new entity to the database.
func (r *repositoryCommon[K, E]) createEntity(db *gorm.DB, ctx context.Context, entity *E) error {
	if err := gorm.G[E](db).Create(ctx, entity); err != nil {
		return fmt.Errorf("failed to create entity: %w", err)
	}

	return nil
}

// readEntity retrieves a single entity by its primary key id.
func (r *repositoryCommon[K, E]) readEntity(db *gorm.DB, ctx context.Context, id K) (*E, error) {
	var err error
	var entity E

	gdb := gorm.G[E](db).Where("id = ?", id)
	if entity, err = gdb.First(ctx); err != nil {
		return nil, fmt.Errorf("failed to read entity: %w", err)
	}

	return &entity, nil
}

// updateEntity saves all fields of the given entity back to the database. The
// entity must already exist; GORM's Save performs an upsert based on the primary key.
func (r *repositoryCommon[K, E]) updateEntity(db *gorm.DB, ctx context.Context, entity *E) (*E, error) {
	if err := db.WithContext(ctx).Save(entity).Error; err != nil {
		return nil, fmt.Errorf("failed to update entity: %w", err)
	}

	return entity, nil
}

// deleteEntity removes the entity with the given id from the database. Returns an
// error if no entity with that id exists.
func (r *repositoryCommon[K, E]) deleteEntity(db *gorm.DB, ctx context.Context, id K) error {
	var err error
	var rowsAffected int

	if rowsAffected, err = gorm.G[E](db).Where("id = ?", id).Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete entity: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("entity id=%v does not exist", id)
	}

	return nil
}

// queryEntities executes a SELECT query built from the given QueryBuilderSelect. It
// validates join relations against the entity's schema, applies joins, WHERE, GROUP BY,
// HAVING, ORDER BY, LIMIT, and OFFSET clauses, and returns the matching entities.
func (r *repositoryCommon[K, E]) queryEntities(db *gorm.DB, ctx context.Context, qb *QueryBuilderSelect) ([]E, error) {
	gdb := gorm.G[E](db).Scopes()

	var err error
	var result []E

	for _, applier := range []applyFunc[K, E]{r.applyJoins, r.applyPreloads, r.applyWhere, r.applyGroupBy, r.applyHaving, r.applyOrderBy} {
		if gdb, err = applier(gdb, qb); err != nil {
			return nil, err
		}
	}

	if qb.limit != nil {
		gdb = gdb.Limit(*qb.limit)
	}

	if qb.offset != nil {
		gdb = gdb.Offset(*qb.offset)
	}

	if result, err = gdb.Find(ctx); err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return result, nil
}

func (r *repositoryCommon[K, E]) applyWhere(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error) {
	if qb.where.IsEmpty() {
		return gdb, nil
	}

	sql, args, err := qb.where.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build where clause: %w", err)
	}

	return gdb.Where(sql, args...), nil
}

func (r *repositoryCommon[K, E]) applyGroupBy(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error) {
	if qb.groupBy.IsEmpty() {
		return gdb, nil
	}

	sql, err := qb.groupBy.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build group by clause: %w", err)
	}

	return gdb.Group(sql), nil
}

func (r *repositoryCommon[K, E]) applyHaving(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error) {
	if qb.having.IsEmpty() {
		return gdb, nil
	}

	sql, args, err := qb.having.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build having clause: %w", err)
	}

	return gdb.Having(sql, args...), nil
}

func (r *repositoryCommon[K, E]) applyOrderBy(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error) {
	if qb.orderBy.IsEmpty() {
		return gdb, nil
	}

	sql, err := qb.orderBy.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build order by clause: %w", err)
	}

	return gdb.Order(sql), nil
}

func (r *repositoryCommon[K, E]) applyJoins(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error) {
	if len(qb.joins) == 0 {
		return gdb, nil
	}

	if err := r.validateJoinRelations(qb.joins); err != nil {
		return nil, err
	}

	for _, j := range qb.joins {
		join := j // capture for closure
		gdb = gdb.Joins(join.joinType.Association(join.relation), func(db gorm.JoinBuilder, joinTable clause.Table, curTable clause.Table) error {
			for _, w := range join.where {
				if w.IsEmpty() {
					continue
				}
				sql, args, err := w.ToSql()
				if err != nil {
					return err
				}
				db.Where(sql, args...)
			}

			return nil
		})
	}

	return gdb, nil
}

func (r *repositoryCommon[K, E]) applyPreloads(gdb gorm.ChainInterface[E], qb *QueryBuilderSelect) (gorm.ChainInterface[E], error) {
	if len(qb.preloads) == 0 {
		return gdb, nil
	}

	if err := r.validatePreloadRelations(qb.preloads); err != nil {
		return nil, err
	}

	for _, p := range qb.preloads {
		preload := p // capture for closure

		if len(preload.where) == 0 {
			gdb = gdb.Preload(preload.relation, nil)
			continue
		}

		gdb = gdb.Preload(preload.relation, func(db gorm.PreloadBuilder) error {
			for _, w := range preload.where {
				if w.IsEmpty() {
					continue
				}
				sql, args, err := w.ToSql()
				if err != nil {
					return err
				}
				db.Where(sql, args...)
			}

			return nil
		})
	}

	return gdb, nil
}

// validatePreloadRelations checks that every preload relation referenced in the
// query exists on the entity's GORM schema. Dotted relation paths (e.g.
// "Posts.Comments") are resolved step by step through nested schemas. Unlike join
// validation, many-to-many associations are allowed because GORM's Preload
// executes separate queries and handles them correctly.
func (r *repositoryCommon[K, E]) validatePreloadRelations(preloads []preloadEntry) error {
	if len(preloads) == 0 {
		return nil
	}

	for _, p := range preloads {
		parts := strings.Split(p.relation, ".")

		currentSchema := r.schema
		for _, part := range parts {
			relation, ok := currentSchema.Relationships.Relations[part]
			if !ok {
				return fmt.Errorf("preload relation %q not found on model %s; valid relations: %v", p.relation, currentSchema.Name, r.validRelationNames(currentSchema))
			}

			currentSchema = relation.FieldSchema
		}
	}

	return nil
}

// validateJoinRelations checks that every join relation referenced in the query
// exists on the entity's GORM schema. Dotted relation paths (e.g. "Company.Address")
// are resolved step by step through nested schemas. Returns an error listing valid
// relation names if a relation is not found, or if the relation is a many-to-many
// association (which is not supported due to GORM limitations).
func (r *repositoryCommon[K, E]) validateJoinRelations(joins []joinEntry) error {
	if len(joins) == 0 {
		return nil
	}

	var ok bool
	var relation *schema.Relationship

	for _, j := range joins {
		parts := strings.Split(j.relation, ".")

		currentSchema := r.schema
		for _, part := range parts {
			if relation, ok = currentSchema.Relationships.Relations[part]; !ok {
				return fmt.Errorf("join relation %q not found on model %s; valid relations: %v", j.relation, currentSchema.Name, r.validRelationNames(currentSchema))
			}

			// Many-to-many associations require two joins (through a join table) but GORM's
			// Joins() method generates incorrect SQL that joins directly to the related table.
			// This produces SQL referencing columns that don't exist on the target table.
			if relation.JoinTable != nil {
				return fmt.Errorf("join relation %q is a many-to-many association which is not supported; GORM does not properly handle many-to-many joins (it generates invalid SQL)", j.relation)
			}

			currentSchema = relation.FieldSchema
		}
	}

	return nil
}

// validRelationNames returns a sorted list of relation names defined on the given schema.
func (r *repositoryCommon[K, E]) validRelationNames(s *schema.Schema) []string {
	names := make([]string, 0, len(s.Relationships.Relations))
	for name := range s.Relationships.Relations {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
