package sqlr

import (
	"fmt"
	"reflect"
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

func requireEntityValue[E any](entity *E) (reflect.Value, error) {
	entityValue := unwrapEntityValue(reflect.ValueOf(entity))
	if !entityValue.IsValid() {
		return reflect.Value{}, ErrNilEntity
	}

	return entityValue, nil
}

func setEntityPrimaryKey(schema *EntitySchema, entityValue reflect.Value, pk any, journal *mutationJournal) error {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)

	return setFieldValue(pkField, pk, schema.TableName, schema.PrimaryKey.Name, journal)
}

func setRelatedFK(entityValue reflect.Value, schema *EntitySchema, fkColName string, parentPK any, journal *mutationJournal) error {
	fkCol, ok := schema.ColumnByName(fkColName)
	if !ok {
		return fmt.Errorf("FK column %q not found in schema for %s", fkColName, schema.TableName)
	}

	fkField := entityValue.FieldByIndex(fkCol.FieldIndex)

	return setFieldValue(fkField, parentPK, schema.TableName, fkColName, journal)
}

func setFieldValue(field reflect.Value, value any, tableName string, columnName string, journal *mutationJournal) error {
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
		return setPointerFieldValue(field, valueRef, tableName, columnName, journal)
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

	if err := journal.record(field); err != nil {
		return fmt.Errorf("failed to record previous value for %s.%s: %w", tableName, columnName, err)
	}

	field.Set(valueRef.Convert(field.Type()))

	return nil
}

func setPointerFieldValue(field reflect.Value, valueRef reflect.Value, tableName string, columnName string, journal *mutationJournal) error {
	if valueRef.Kind() == reflect.Ptr {
		if valueRef.IsNil() {
			if err := journal.record(field); err != nil {
				return fmt.Errorf("failed to record previous value for %s.%s: %w", tableName, columnName, err)
			}

			field.Set(reflect.Zero(field.Type()))

			return nil
		}

		if valueRef.Type().ConvertibleTo(field.Type()) {
			if err := journal.record(field); err != nil {
				return fmt.Errorf("failed to record previous value for %s.%s: %w", tableName, columnName, err)
			}

			field.Set(valueRef.Convert(field.Type()))

			return nil
		}

		valueRef = valueRef.Elem()
	}

	if !valueRef.Type().ConvertibleTo(field.Type().Elem()) {
		return fmt.Errorf("value type %s is not convertible to field type %s for %s.%s", valueRef.Type(), field.Type(), tableName, columnName)
	}

	if err := journal.record(field); err != nil {
		return fmt.Errorf("failed to record previous value for %s.%s: %w", tableName, columnName, err)
	}

	ptr := reflect.New(field.Type().Elem())
	ptr.Elem().Set(valueRef.Convert(field.Type().Elem()))
	field.Set(ptr)

	return nil
}
