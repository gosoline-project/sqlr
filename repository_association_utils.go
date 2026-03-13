package sqlr

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gosoline-project/sqlc"
)

func unwrapEntityValue(entityValue reflect.Value) reflect.Value {
	for entityValue.IsValid() && entityValue.Kind() == reflect.Ptr {
		if entityValue.IsNil() {
			return reflect.Value{}
		}

		entityValue = entityValue.Elem()
	}

	return entityValue
}

func associationSyncKey(schema *EntitySchema, entityValue reflect.Value) (string, bool) {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		pk := pkField.Interface()
		key, ok := comparableKey(pk)
		if !ok {
			return "", false
		}

		return fmt.Sprintf("%s:%v", schema.TableName, key), true
	}

	if entityValue.CanAddr() {
		return fmt.Sprintf("%s:new:%x", schema.TableName, entityValue.Addr().Pointer()), true
	}

	return "", false
}

func setEntityPrimaryKey(schema *EntitySchema, entityValue reflect.Value, pk any) error {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)

	return setFieldValue(pkField, pk, schema.TableName, schema.PrimaryKey.Name)
}

func updateStoredEntity(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	now := time.Now()
	setUpdateTimestamps(entityValue, schema, now)
	setMap, pkValue := buildUpdateSetMap(entityValue, schema)

	sqler := sqlc.Update(schema.TableName).
		SetMap(setMap).
		Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	_, result, err := cache.Exec(ctx, sqler, q)
	if err != nil {
		return fmt.Errorf("failed to update entity %s: %w", schema.TableName, err)
	}

	if err := errNoRowsAffected(result, fmt.Errorf("entity %s id=%v: %w", schema.TableName, pkValue, ErrNotFound)); err != nil {
		return err
	}

	return nil
}

func loadChildrenByForeignKey(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, foreignKey string, parentPK any) ([]reflect.Value, error) {
	qb := sqlc.From(schema.TableName).Where(sqlc.Col(schema.TableName, foreignKey).Eq(parentPK))

	return querySchemaEntities(cache, q, ctx, qb, schema)
}

func querySchemaEntities(cache *statementCache, q sqlc.Querier, ctx context.Context, sqler sqlc.Sqler, schema *EntitySchema) ([]reflect.Value, error) {
	rows, err := cache.Query(ctx, sqler, q)
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

func loadM2MLinks(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, relSchema *EntitySchema, rel *Relationship, parentPK any) ([]m2mLink, error) {
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)
	sqler := sqlc.From(rel.JoinTable).
		Where(sqlc.Col(rel.JoinTable, parentColName).Eq(parentPK))

	joinRows, err := cache.Query(ctx, sqler, q)
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

func deleteM2MLinks(cache *statementCache, q sqlc.Querier, ctx context.Context, rel *Relationship, parentSchema *EntitySchema, relSchema *EntitySchema, parentPK any, relatedPKs []any) error {
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)

	for _, relatedPK := range relatedPKs {
		sqler := sqlc.Delete(rel.JoinTable).
			Where(sqlc.Col(parentColName).Eq(parentPK)).
			Where(sqlc.Col(relatedColName).Eq(relatedPK))

		if _, _, err := cache.Exec(ctx, sqler, q); err != nil {
			return fmt.Errorf("failed to delete join table row from %s: %w", rel.JoinTable, err)
		}
	}

	return nil
}

func deleteAllM2MLinks(cache *statementCache, q sqlc.Querier, ctx context.Context, rel *Relationship, parentSchema *EntitySchema, relSchema *EntitySchema, parentPK any) error {
	parentColName, _ := resolveM2MColumnNames(rel, parentSchema, relSchema)
	sqler := sqlc.Delete(rel.JoinTable).Where(sqlc.Col(parentColName).Eq(parentPK))

	if _, _, err := cache.Exec(ctx, sqler, q); err != nil {
		return fmt.Errorf("failed to delete join rows from %s: %w", rel.JoinTable, err)
	}

	return nil
}

func deleteStoredEntity(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, pkValue any) error {
	sqler := sqlc.Delete(schema.TableName).Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	if _, _, err := cache.Exec(ctx, sqler, q); err != nil {
		return fmt.Errorf("failed to delete row from %s: %w", schema.TableName, err)
	}

	return nil
}

func setRelatedFK(entityValue reflect.Value, schema *EntitySchema, fkColName string, parentPK any) error {
	fkCol, ok := schema.ColumnByName(fkColName)
	if !ok {
		return fmt.Errorf("FK column %q not found in schema for %s", fkColName, schema.TableName)
	}

	fkField := entityValue.FieldByIndex(fkCol.FieldIndex)

	return setFieldValue(fkField, parentPK, schema.TableName, fkColName)
}

func setFieldValue(field reflect.Value, value any, tableName string, columnName string) error {
	valueRef := reflect.ValueOf(value)
	if !valueRef.IsValid() {
		return fmt.Errorf("invalid value for %s.%s", tableName, columnName)
	}

	for valueRef.Kind() == reflect.Interface {
		if valueRef.IsNil() {
			return fmt.Errorf("invalid value for %s.%s", tableName, columnName)
		}

		valueRef = valueRef.Elem()
	}

	if field.Kind() == reflect.Ptr {
		return setPointerFieldValue(field, valueRef, tableName, columnName)
	}

	if valueRef.Kind() == reflect.Ptr {
		if valueRef.IsNil() {
			return fmt.Errorf("nil value is not assignable to non-pointer field %s.%s", tableName, columnName)
		}

		valueRef = valueRef.Elem()
	}

	if !valueRef.Type().ConvertibleTo(field.Type()) {
		return fmt.Errorf("value type %s is not convertible to field type %s for %s.%s", valueRef.Type(), field.Type(), tableName, columnName)
	}

	field.Set(valueRef.Convert(field.Type()))

	return nil
}

func setPointerFieldValue(field reflect.Value, valueRef reflect.Value, tableName string, columnName string) error {
	if valueRef.Kind() == reflect.Ptr {
		if valueRef.IsNil() {
			field.Set(reflect.Zero(field.Type()))

			return nil
		}

		if valueRef.Type().ConvertibleTo(field.Type()) {
			field.Set(valueRef.Convert(field.Type()))

			return nil
		}

		valueRef = valueRef.Elem()
	}

	if !valueRef.Type().ConvertibleTo(field.Type().Elem()) {
		return fmt.Errorf("value type %s is not convertible to field type %s for %s.%s", valueRef.Type(), field.Type(), tableName, columnName)
	}

	ptr := reflect.New(field.Type().Elem())
	ptr.Elem().Set(valueRef.Convert(field.Type().Elem()))
	field.Set(ptr)

	return nil
}
