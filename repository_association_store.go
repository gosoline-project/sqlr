package sqlr

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gosoline-project/sqlc"
)

func (c *associationMutationContext) insertRelatedEntity(ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	if !c.mutationOptions.autoUpdatesDisabled() {
		now := time.Now()
		if err := setCreateTimestamps(entityValue, schema, now, c.journal); err != nil {
			return nil, fmt.Errorf("failed to set create timestamps for %s: %w", schema.TableName, err)
		}
	}

	insertCols, vals, err := buildInsertValues(entityValue, schema, c.mutationOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to build insert values for %s: %w", schema.TableName, err)
	}

	sqler := sqlc.Into(schema.TableName).
		Columns(insertCols...).
		Values(vals...)

	sql, args, err := sqler.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build INSERT SQL for %s: %w", schema.TableName, err)
	}

	result, err := c.q.Exec(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute INSERT for %s: %w", schema.TableName, err)
	}

	if c.mutationOptions.autoUpdatesDisabled() || schema.PrimaryKey == nil || !schema.PrimaryKey.AutoIncrement {
		return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID for %s: %w", schema.TableName, err)
	}

	return lastID, nil
}

func (c *associationMutationContext) updateStoredEntity(ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	if !c.mutationOptions.autoUpdatesDisabled() {
		now := time.Now()
		if err := setUpdateTimestamps(entityValue, schema, now, c.journal); err != nil {
			return fmt.Errorf("failed to set update timestamps for %s: %w", schema.TableName, err)
		}
	}
	setMap, pkValue := buildUpdateSetMap(entityValue, schema, c.mutationOptions)

	sqler := sqlc.Update(schema.TableName).
		SetMap(setMap).
		Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	_, result, err := c.cache.Exec(ctx, sqler, c.q)
	if err != nil {
		return fmt.Errorf("failed to update entity %s: %w", schema.TableName, err)
	}

	if err := errNoRowsAffected(result, fmt.Errorf("entity %s id=%v: %w", schema.TableName, pkValue, ErrNotFound)); err != nil {
		return err
	}

	return nil
}

func (c *associationMutationContext) updateStoredEntityForeignKey(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, fkColName string) error {
	fkCol, ok := schema.ColumnByName(fkColName)
	if !ok {
		return fmt.Errorf("FK column %q not found in schema for %s", fkColName, schema.TableName)
	}

	setMap := map[string]any{
		fkColName: entityValue.FieldByIndex(fkCol.FieldIndex).Interface(),
	}

	if !c.mutationOptions.autoUpdatesDisabled() {
		now := time.Now()
		for _, col := range schema.Columns {
			if !col.AutoUpdateTime {
				continue
			}

			field := entityValue.FieldByIndex(col.FieldIndex)
			if err := setFieldValue(field, now, schema.TableName, col.Name, c.journal); err != nil {
				return fmt.Errorf("failed to set update timestamps for %s: %w", schema.TableName, err)
			}

			setMap[col.Name] = field.Interface()
		}
	}

	pkValue := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface()
	sqler := sqlc.Update(schema.TableName).
		SetMap(setMap).
		Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	_, result, err := c.cache.Exec(ctx, sqler, c.q)
	if err != nil {
		return fmt.Errorf("failed to update entity %s foreign key %s: %w", schema.TableName, fkColName, err)
	}

	if err := errNoRowsAffected(result, fmt.Errorf("entity %s id=%v: %w", schema.TableName, pkValue, ErrNotFound)); err != nil {
		return err
	}

	return nil
}

func (c *associationCallContext) loadChildrenByForeignKey(ctx context.Context, schema *EntitySchema, foreignKey string, parentPK any) ([]reflect.Value, error) {
	qb := sqlc.From(schema.TableName).Where(sqlc.Col(schema.TableName, foreignKey).Eq(parentPK))

	return c.querySchemaEntities(ctx, qb, schema)
}

func (c *associationCallContext) querySchemaEntities(ctx context.Context, sqler sqlc.Sqler, schema *EntitySchema) ([]reflect.Value, error) {
	rows, err := c.cache.Query(ctx, sqler, c.q)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // safe to ignore in defer

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get query columns for %s: %w", schema.TableName, err)
	}

	entities, err := hydrateRows(rows, columns, schema, schema.entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate rows for %s: %w", schema.TableName, err)
	}

	return entities, nil
}

func (c *associationCallContext) loadM2MLinks(ctx context.Context, parentSchema *EntitySchema, relSchema *EntitySchema, rel *Relationship, parentPK any) ([]m2mLink, error) {
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)
	sqler := sqlc.From(rel.JoinTable).
		Where(sqlc.Col(rel.JoinTable, parentColName).Eq(parentPK))

	joinRows, err := c.cache.Query(ctx, sqler, c.q)
	if err != nil {
		return nil, fmt.Errorf("failed to query join table %s: %w", rel.JoinTable, err)
	}
	defer joinRows.Close() //nolint:errcheck // safe to ignore in defer

	joinColumns, err := joinRows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get join table columns: %w", err)
	}

	parentColIdx, relatedColIdx := findJoinColumnIndices(joinColumns, parentColName, relatedColName)
	if parentColIdx < 0 || relatedColIdx < 0 {
		return nil, fmt.Errorf("join table %q missing expected columns %q or %q", rel.JoinTable, parentColName, relatedColName)
	}

	links, _, err := scanJoinTableRows(joinRows, joinColumns, parentColIdx, relatedColIdx, parentSchema, relSchema, rel)
	if err != nil {
		return nil, err
	}

	return links, nil
}

func (c *associationCallContext) deleteM2MLinks(ctx context.Context, rel *Relationship, parentSchema *EntitySchema, relSchema *EntitySchema, parentPK any, relatedPKs []any) error {
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)

	for _, relatedPK := range relatedPKs {
		sqler := sqlc.Delete(rel.JoinTable).
			Where(sqlc.Col(parentColName).Eq(parentPK)).
			Where(sqlc.Col(relatedColName).Eq(relatedPK))

		if _, _, err := c.cache.Exec(ctx, sqler, c.q); err != nil {
			return fmt.Errorf("failed to delete join table row from %s: %w", rel.JoinTable, err)
		}
	}

	return nil
}

func (c *associationCallContext) deleteAllM2MLinks(ctx context.Context, rel *Relationship, parentSchema *EntitySchema, relSchema *EntitySchema, parentPK any) error {
	parentColName, _ := resolveM2MColumnNames(rel, parentSchema, relSchema)
	sqler := sqlc.Delete(rel.JoinTable).Where(sqlc.Col(parentColName).Eq(parentPK))

	if _, _, err := c.cache.Exec(ctx, sqler, c.q); err != nil {
		return fmt.Errorf("failed to delete join rows from %s: %w", rel.JoinTable, err)
	}

	return nil
}

func (c *associationCallContext) deleteStoredEntity(ctx context.Context, schema *EntitySchema, pkValue any) error {
	sqler := sqlc.Delete(schema.TableName).Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	if _, _, err := c.cache.Exec(ctx, sqler, c.q); err != nil {
		return fmt.Errorf("failed to delete row from %s: %w", schema.TableName, err)
	}

	return nil
}

func (c *associationCallContext) insertJoinTableRows(ctx context.Context, joinTable string, parentCol string, relatedCol string, parentPK any, relatedPKs []any) error {
	if len(relatedPKs) == 0 {
		return nil
	}

	rows := make([][]any, 0, len(relatedPKs))
	for _, relPK := range relatedPKs {
		rows = append(rows, []any{parentPK, relPK})
	}

	sqler := sqlc.Into(joinTable).
		Columns(parentCol, relatedCol).
		Ignore().
		ValuesRows(rows...)

	sql, args, err := sqler.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build join table INSERT SQL: %w", err)
	}

	if _, err := c.q.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("failed to insert join table rows: %w", err)
	}

	return nil
}
