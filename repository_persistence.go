package sqlr

import (
	"fmt"
	"reflect"
	"time"

	"github.com/gosoline-project/sqlc"
)

var ErrAutoUpdatesRequirePresetPrimaryKey = fmt.Errorf("disable auto updates requires primary key values to be preset")

func setCreateTimestamps(entityValue reflect.Value, schema *EntitySchema, now time.Time, journal *mutationJournal) error {
	for _, col := range schema.Columns {
		if col.AutoCreateTime || col.AutoUpdateTime {
			field := entityValue.FieldByIndex(col.FieldIndex)
			if err := setFieldValue(field, now, schema.TableName, col.Name, journal); err != nil {
				return err
			}
		}
	}

	return nil
}

func setUpdateTimestamps(entityValue reflect.Value, schema *EntitySchema, now time.Time, journal *mutationJournal) error {
	for _, col := range schema.Columns {
		if col.AutoUpdateTime {
			field := entityValue.FieldByIndex(col.FieldIndex)
			if err := setFieldValue(field, now, schema.TableName, col.Name, journal); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildInsertValues(entityValue reflect.Value, schema *EntitySchema, options mutationOptions) (insertCols []string, vals []any, err error) {
	insertCols = make([]string, 0, len(schema.Columns))
	vals = make([]any, 0, len(schema.Columns))

	for _, col := range schema.Columns {
		if col.IsPrimaryKey && col.AutoIncrement && !options.autoUpdatesDisabled() {
			continue
		}

		field := entityValue.FieldByIndex(col.FieldIndex)
		if col.IsPrimaryKey && options.autoUpdatesDisabled() && field.IsZero() {
			return nil, nil, fmt.Errorf("%w for %s.%s", ErrAutoUpdatesRequirePresetPrimaryKey, schema.TableName, col.Name)
		}

		insertCols = append(insertCols, col.Name)
		vals = append(vals, field.Interface())
	}

	return insertCols, vals, nil
}

func buildUpdateSetMap(entityValue reflect.Value, schema *EntitySchema, _ mutationOptions) (setMap map[string]any, pkValue any) {
	setMap = make(map[string]any, len(schema.Columns))
	pkValue = entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface()

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
