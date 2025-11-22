package sqlr

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// dbContext is an interface that abstracts over context.Context operations
// to allow both regular repository and transactional repository to use the same code
type dbContext interface {
	context.Context
}

// createEntity is a shared helper for creating entities
func createEntity[K KeyTypes, E Entitier[K]](db *gorm.DB, ctx dbContext, entity *E) error {
	if err := gorm.G[E](db).Create(ctx, entity); err != nil {
		return fmt.Errorf("failed to create entity: %w", err)
	}

	return nil
}

// readEntity is a shared helper for reading entities by ID
func readEntity[K KeyTypes, E Entitier[K]](db *gorm.DB, ctx dbContext, id K) (*E, error) {
	var err error
	var entity E

	gdb := gorm.G[E](db).Where("id = ?", id)
	if entity, err = gdb.First(ctx); err != nil {
		return nil, fmt.Errorf("failed to read entity: %w", err)
	}

	return &entity, nil
}

func updateEntity[K KeyTypes, E Entitier[K]](db *gorm.DB, ctx dbContext, entity *E) (*E, error) {
	if err := db.WithContext(ctx).Save(entity).Error; err != nil {
		return nil, fmt.Errorf("failed to update entity: %w", err)
	}

	return entity, nil
}

// queryEntities is a shared helper for querying entities with a QueryBuilderSelect
func queryEntities[K KeyTypes, E Entitier[K]](db *gorm.DB, ctx dbContext, qb *QueryBuilderSelect) ([]E, error) {
	gdb := gorm.G[E](db).Scopes()

	var err error
	var sql string
	var args []any
	var result []E

	if !qb.where.IsEmpty() {
		if sql, args, err = qb.where.ToSql(); err != nil {
			return nil, fmt.Errorf("failed to build where clause: %w", err)
		}
		gdb = gdb.Where(sql, args...)
	}

	if !qb.groupBy.IsEmpty() {
		if sql, err = qb.groupBy.ToSql(); err != nil {
			return nil, fmt.Errorf("failed to build where clause: %w", err)
		}
		gdb = gdb.Group(sql)
	}

	if !qb.having.IsEmpty() {
		if sql, args, err = qb.having.ToSql(); err != nil {
			return nil, fmt.Errorf("failed to build where clause: %w", err)
		}
		gdb = gdb.Having(sql, args...)
	}

	if !qb.orderBy.IsEmpty() {
		if sql, err = qb.orderBy.ToSql(); err != nil {
			return nil, fmt.Errorf("failed to build where clause: %w", err)
		}
		gdb = gdb.Order(sql)
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

// deleteEntity is a shared helper for deleting entities by ID
func deleteEntity[K KeyTypes, E Entitier[K]](db *gorm.DB, ctx dbContext, id K) error {
	var err error
	var rowsAffected int

	if rowsAffected, err = gorm.G[E](db).Where("id = ?", id).Delete(ctx); err != nil {
		return fmt.Errorf("failed to read entity: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("entity id=%s does not exist", id)
	}

	return nil
}
