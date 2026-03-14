package sqlr

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/gosoline-project/sqlc"
)

// hydrateRows scans all rows from the result set into reflect.Values of the given
// entityType using the schema's column mappings. It precomputes column-to-field
// index mappings once and reuses a single scanDests slice across rows for efficiency.
// Returns the scanned entities as reflect.Values.
func hydrateRows(rows *sqlc.Rows, columns []string, schema *EntitySchema, entityType reflect.Type) ([]reflect.Value, error) {
	columnFieldIndices := precomputeColumnFieldIndices(schema, columns)
	scanDests := make([]any, len(columns))

	var entities []reflect.Value

	for rows.Next() {
		entity := reflect.New(entityType).Elem()

		for i := range columns {
			scanDests[i] = scanDestForPrecomputedField(entity, columnFieldIndices[i])
		}

		if err := rows.Scan(scanDests...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		entities = append(entities, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return entities, nil
}

func precomputeColumnFieldIndices(schema *EntitySchema, columns []string) [][]int {
	columnFieldIndices := make([][]int, len(columns))

	for i, colName := range columns {
		if col, ok := schema.ColumnByName(colName); ok {
			columnFieldIndices[i] = col.FieldIndex
		}
	}

	return columnFieldIndices
}

func scanDestForPrecomputedField(rv reflect.Value, fieldIndex []int) any {
	if fieldIndex != nil {
		return rv.FieldByIndex(fieldIndex).Addr().Interface()
	}

	var discard any

	return &discard
}

func assignRelated(relField, relEntity reflect.Value) error {
	assignedValue, err := relationAssignmentValue(relField.Type(), relEntity)
	if err != nil {
		return err
	}

	if relField.Kind() == reflect.Slice {
		relField.Set(reflect.Append(relField, assignedValue))

		return nil
	}

	relField.Set(assignedValue)

	return nil
}

func relationAssignmentValue(targetType reflect.Type, relEntity reflect.Value) (reflect.Value, error) {
	if targetType.Kind() == reflect.Slice {
		return relationAssignmentValue(targetType.Elem(), relEntity)
	}

	if relEntity.Type().AssignableTo(targetType) {
		return relEntity, nil
	}

	if relEntity.Type().ConvertibleTo(targetType) {
		return relEntity.Convert(targetType), nil
	}

	if targetType.Kind() == reflect.Ptr {
		if relEntity.Type().AssignableTo(targetType.Elem()) {
			ptr := reflect.New(targetType.Elem())
			ptr.Elem().Set(relEntity)

			return ptr, nil
		}

		if relEntity.Type().ConvertibleTo(targetType.Elem()) {
			ptr := reflect.New(targetType.Elem())
			ptr.Elem().Set(relEntity.Convert(targetType.Elem()))

			return ptr, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("related entity type %s is not assignable to relation field type %s", relEntity.Type(), targetType)
}

func collectAssignedRelatedValues(relField reflect.Value) []reflect.Value {
	if relField.Kind() == reflect.Slice {
		related := make([]reflect.Value, 0, relField.Len())
		for i := range relField.Len() {
			elem := unwrapEntityValue(relField.Index(i))
			if !elem.IsValid() {
				continue
			}

			related = append(related, elem)
		}

		return related
	}

	relValue := unwrapEntityValue(relField)
	if !relValue.IsValid() || relValue.IsZero() {
		return nil
	}

	return []reflect.Value{relValue}
}

// newScanDestForColumn allocates a scan destination matching the column field type.
// If the destination implements sql.Scanner, it is passed as such to rows.Scan.
func newScanDestForColumn(schema *EntitySchema, column *ColumnInfo) any {
	fieldType := schema.entityType.FieldByIndex(column.FieldIndex).Type
	dest := reflect.New(fieldType).Interface()

	if scanner, ok := dest.(sql.Scanner); ok {
		return scanner
	}

	return dest
}

// scannedValue extracts the scanned value from a pointer destination.
func scannedValue(dest any) any {
	rv := reflect.ValueOf(dest)
	if !rv.IsValid() || rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}

	return rv.Elem().Interface()
}

func isNilValue(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}

	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:

		return rv.IsNil()
	}

	return false
}

// comparableKey returns v if it can be used as a non-nil map key.
func comparableKey(v any) (any, bool) {
	if v == nil {
		return nil, false
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, false
	}

	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return nil, false
		}
	}

	for rv.IsValid() {
		switch rv.Kind() {
		case reflect.Interface, reflect.Ptr:
			if rv.IsNil() {
				return nil, false
			}

			rv = rv.Elem()
		default:
			if !rv.Type().Comparable() {
				return nil, false
			}

			return rv.Interface(), true
		}
	}

	return nil, false
}

// optionalComparableKey returns a comparable key for present values while
// treating only nil pointer/interface-like values as absent. Zero-valued scalar
// keys such as false, "", and 0 are considered present.
func optionalComparableKey(v any) (any, bool, error) {
	if isNilValue(v) {
		return nil, false, nil
	}

	key, ok := comparableKey(v)
	if !ok {
		return nil, false, fmt.Errorf("non-comparable value type %T", v)
	}

	return key, true, nil
}
