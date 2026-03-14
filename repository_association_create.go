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
func createRelatedEntity(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, policy *associationSyncPolicy, path string) (any, error) {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil, fmt.Errorf("invalid entity value for schema %s", schema.TableName)
	}

	if err := createRelatedBelongsTo(cache, q, ctx, schema, entityValue, policy, path); err != nil {
		return nil, err
	}

	pk, err := insertRelatedEntity(q, ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	if err := setEntityPrimaryKey(schema, entityValue, pk); err != nil {
		return nil, err
	}

	if err := createRelatedForwardAssociations(cache, q, ctx, schema, entityValue, policy, path); err != nil {
		return nil, err
	}

	return pk, nil
}

func createRelatedBelongsTo(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, policy *associationSyncPolicy, parentPath string) error {
	for _, rel := range schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		relationPath := joinAssociationPath(parentPath, rel.Name)
		if policy != nil && !policy.shouldSyncPath(relationPath) {
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
			if _, err := createRelatedEntity(cache, q, ctx, nestedSchema, field, policy, relationPath); err != nil {
				return fmt.Errorf("failed to insert BelongsTo relation %q: %w", rel.Name, err)
			}
		}

		fkCol, ok := schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("BelongsTo relation %q: FK column %q not found on schema", rel.Name, rel.ForeignKey)
		}

		fkField := entityValue.FieldByIndex(fkCol.FieldIndex)
		if err := setFieldValue(fkField, pkField.Interface(), schema.TableName, rel.ForeignKey); err != nil {
			return fmt.Errorf("BelongsTo relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func createRelatedForwardAssociations(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, policy *associationSyncPolicy, parentPath string) error {
	for _, rel := range schema.Relationships {
		relationPath := joinAssociationPath(parentPath, rel.Name)
		if policy != nil && !policy.shouldSyncPath(relationPath) {
			continue
		}

		field := entityValue.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		switch rel.Type {
		case HasOne:
			if err := createRelatedHasOne(cache, q, ctx, schema, entityValue, rel, policy, relationPath); err != nil {
				return err
			}
		case HasMany:
			if err := createRelatedHasMany(cache, q, ctx, schema, entityValue, rel, policy, relationPath); err != nil {
				return err
			}
		case ManyToMany:
			if err := createRelatedManyToMany(cache, q, ctx, schema, entityValue, rel, policy, relationPath); err != nil {
				return err
			}
		case BelongsTo:
		}
	}

	return nil
}

func createRelatedHasOne(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, policy *associationSyncPolicy, relationPath string) error {
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
		if err := updateStoredEntityForeignKey(cache, q, ctx, nestedSchema, relField, rel.ForeignKey); err != nil {
			return fmt.Errorf("failed to update HasOne relation %q: %w", rel.Name, err)
		}

		return nil
	}

	if _, err := createRelatedEntity(cache, q, ctx, nestedSchema, relField, policy, relationPath); err != nil {
		return fmt.Errorf("failed to insert HasOne relation %q: %w", rel.Name, err)
	}

	return nil
}

func createRelatedHasMany(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, policy *associationSyncPolicy, relationPath string) error {
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
			if err := updateStoredEntityForeignKey(cache, q, ctx, nestedSchema, elem, rel.ForeignKey); err != nil {
				return fmt.Errorf("failed to update HasMany relation %q[%d]: %w", rel.Name, i, err)
			}

			continue
		}

		if _, err := createRelatedEntity(cache, q, ctx, nestedSchema, elem, policy, relationPath); err != nil {
			return fmt.Errorf("failed to insert HasMany relation %q[%d]: %w", rel.Name, i, err)
		}
	}

	return nil
}

func createRelatedManyToMany(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, policy *associationSyncPolicy, relationPath string) error {
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
			if _, err := createRelatedEntity(cache, q, ctx, nestedSchema, elem, policy, relationPath); err != nil {
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
	if err := setCreateTimestamps(entityValue, schema, now); err != nil {
		return nil, fmt.Errorf("failed to set create timestamps for %s: %w", schema.TableName, err)
	}
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
