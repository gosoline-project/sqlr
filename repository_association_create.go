package sqlr

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gosoline-project/sqlc"
)

// createRelatedEntity persists a related entity and all of its own populated
// associations recursively. It mirrors the four-phase flow of
// createEntityWithAssociations but operates on a bare reflect.Value and
// *EntitySchema so it can be used for any related entity type without needing
// a typed repositoryCommon.
func createRelatedEntity(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil, fmt.Errorf("invalid entity value for schema %s", schema.TableName)
	}

	if err := createRelatedBelongsTo(q, ctx, schema, entityValue); err != nil {
		return nil, err
	}

	pk, err := insertRelatedEntity(q, ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	pkField.Set(reflect.ValueOf(pk).Convert(pkField.Type()))

	if err := createRelatedForwardAssociations(q, ctx, schema, entityValue); err != nil {
		return nil, err
	}

	return pk, nil
}

func createRelatedBelongsTo(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	for _, rel := range schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		field := entityValue.FieldByIndex(rel.FieldIndex)
		field = unwrapEntityValue(field)
		if !field.IsValid() || field.IsZero() {
			continue
		}

		nestedSchema, err := rel.resolveRelationSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for BelongsTo relation %q: %w", rel.Name, err)
		}

		pkField := field.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		if pkField.IsZero() {
			if _, err := createRelatedEntity(q, ctx, nestedSchema, field); err != nil {
				return fmt.Errorf("failed to insert BelongsTo relation %q: %w", rel.Name, err)
			}
		}

		fkCol, ok := schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("BelongsTo relation %q: FK column %q not found on schema", rel.Name, rel.ForeignKey)
		}

		entityValue.FieldByIndex(fkCol.FieldIndex).Set(pkField)
	}

	return nil
}

func createRelatedForwardAssociations(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	for _, rel := range schema.Relationships {
		field := entityValue.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		switch rel.Type {
		case HasOne:
			if err := createRelatedHasOne(q, ctx, schema, entityValue, rel); err != nil {
				return err
			}
		case HasMany:
			if err := createRelatedHasMany(q, ctx, schema, entityValue, rel); err != nil {
				return err
			}
		case ManyToMany:
			if err := createRelatedManyToMany(q, ctx, schema, entityValue, rel); err != nil {
				return err
			}
		case BelongsTo:
		}
	}

	return nil
}

func createRelatedHasOne(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	relField := parentValue.FieldByIndex(rel.FieldIndex)
	relField = unwrapEntityValue(relField)
	if !relField.IsValid() {
		return fmt.Errorf("HasOne relation %q is nil", rel.Name)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	pkField := relField.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		return nil
	}

	if _, err := createRelatedEntity(q, ctx, nestedSchema, relField); err != nil {
		return fmt.Errorf("failed to insert HasOne relation %q: %w", rel.Name, err)
	}

	return nil
}

func createRelatedHasMany(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasMany relation %q: %w", rel.Name, err)
	}

	sliceField := parentValue.FieldByIndex(rel.FieldIndex)
	if sliceField.IsZero() || sliceField.Len() == 0 {
		return nil
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()

	for i := range sliceField.Len() {
		elem := unwrapEntityValue(sliceField.Index(i))
		if !elem.IsValid() {
			return fmt.Errorf("HasMany relation %q[%d] is nil", rel.Name, i)
		}

		if err := setRelatedFK(elem, nestedSchema, rel.ForeignKey, parentPK); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pkField := elem.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
		if !pkField.IsZero() {
			continue
		}

		if _, err := createRelatedEntity(q, ctx, nestedSchema, elem); err != nil {
			return fmt.Errorf("failed to insert HasMany relation %q[%d]: %w", rel.Name, i, err)
		}
	}

	return nil
}

func createRelatedManyToMany(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany relation %q: %w", rel.Name, err)
	}

	sliceField := parentValue.FieldByIndex(rel.FieldIndex)
	if sliceField.IsZero() || sliceField.Len() == 0 {
		return nil
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	relatedPKs := make([]any, 0, sliceField.Len())

	for i := range sliceField.Len() {
		elem := unwrapEntityValue(sliceField.Index(i))
		if !elem.IsValid() {
			return fmt.Errorf("ManyToMany relation %q[%d] is nil", rel.Name, i)
		}

		pkField := elem.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		if pkField.IsZero() {
			if _, err := createRelatedEntity(q, ctx, nestedSchema, elem); err != nil {
				return fmt.Errorf("failed to insert ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}
		}

		relatedPKs = append(relatedPKs, pkField.Interface())
	}

	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, nestedSchema)

	if err := insertJoinTableRows(q, ctx, rel.JoinTable, parentColName, relatedColName, parentPK, relatedPKs); err != nil {
		return fmt.Errorf("failed to insert join table rows for ManyToMany relation %q: %w", rel.Name, err)
	}

	return nil
}

func insertRelatedEntity(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	now := time.Now()
	insertCols := schema.InsertColumns()
	setCreateTimestamps(entityValue, schema, now)
	vals := buildInsertValues(entityValue, schema)

	sqler := sqlc.Into(schema.TableName).
		Columns(insertCols...).
		Values(vals...)

	sql, args, err := sqler.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build INSERT SQL for %s: %w", schema.TableName, err)
	}

	result, err := q.Exec(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute INSERT for %s: %w", schema.TableName, err)
	}

	if schema.PrimaryKey == nil || !schema.PrimaryKey.AutoIncrement {
		return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID for %s: %w", schema.TableName, err)
	}

	return lastID, nil
}

func insertJoinTableRows(q sqlc.Querier, ctx context.Context, joinTable, parentCol, relatedCol string, parentPK any, relatedPKs []any) error {
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

	if _, err := q.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("failed to insert join table rows: %w", err)
	}

	return nil
}
