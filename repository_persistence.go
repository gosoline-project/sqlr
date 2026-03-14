package sqlr

import (
	"fmt"
	"reflect"
	"time"

	"github.com/gosoline-project/sqlc"
)

func setCreateTimestamps(entityValue reflect.Value, schema *EntitySchema, now time.Time) error {
	for _, col := range schema.Columns {
		if col.AutoCreateTime || col.AutoUpdateTime {
			field := entityValue.FieldByIndex(col.FieldIndex)
			if err := setFieldValue(field, now, schema.TableName, col.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func setUpdateTimestamps(entityValue reflect.Value, schema *EntitySchema, now time.Time) error {
	for _, col := range schema.Columns {
		if col.AutoUpdateTime {
			field := entityValue.FieldByIndex(col.FieldIndex)
			if err := setFieldValue(field, now, schema.TableName, col.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildInsertValues(entityValue reflect.Value, schema *EntitySchema) []any {
	insertCols := schema.InsertColumns()
	vals := make([]any, 0, len(insertCols))

	for _, col := range schema.Columns {
		if col.IsPrimaryKey && col.AutoIncrement {
			continue
		}

		vals = append(vals, entityValue.FieldByIndex(col.FieldIndex).Interface())
	}

	return vals
}

func buildUpdateSetMap(entityValue reflect.Value, schema *EntitySchema) (map[string]any, any) {
	setMap := make(map[string]any, len(schema.Columns))
	pkValue := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface()

	for _, col := range schema.Columns {
		if col.IsPrimaryKey {
			continue
		}

		setMap[col.Name] = entityValue.FieldByIndex(col.FieldIndex).Interface()
	}

	return setMap, pkValue
}

func errNoRowsAffected(result sqlc.Result, notFoundErr error) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return notFoundErr
	}

	return nil
}
