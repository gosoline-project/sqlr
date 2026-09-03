package sqlr

import (
	"fmt"
	"reflect"
)

func preserveTransientFields(dst reflect.Value, src reflect.Value, schema *EntitySchema) error {
	dst = unwrapEntityValue(dst)
	src = unwrapEntityValue(src)
	if !dst.IsValid() || !src.IsValid() || schema == nil {
		return nil
	}

	if dst.Kind() != reflect.Struct || src.Kind() != reflect.Struct {
		return nil
	}

	preserveTransientStructFields(dst, src, schema, nil)

	for _, rel := range schema.Relationships {
		relSchema, err := rel.ResolveRelatedSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for relation %s: %w", rel.Name, err)
		}

		if err := preserveTransientRelationField(dst.FieldByIndex(rel.FieldIndex), src.FieldByIndex(rel.FieldIndex), relSchema); err != nil {
			return fmt.Errorf("failed to preserve transient fields for relation %s: %w", rel.Name, err)
		}
	}

	return nil
}

func preserveTransientStructFields(dst reflect.Value, src reflect.Value, schema *EntitySchema, parentPath []int) {
	for i := range dst.NumField() {
		path := appendFieldPath(parentPath, i)
		if hasExactManagedFieldPath(schema, path) {
			continue
		}

		dstField := dst.Field(i)
		srcField := src.Field(i)
		if !dstField.CanSet() {
			continue
		}

		if hasManagedDescendantPath(schema, path) {
			nestedDst := unwrapEntityValue(dstField)
			nestedSrc := unwrapEntityValue(srcField)
			if nestedDst.IsValid() && nestedSrc.IsValid() && nestedDst.Kind() == reflect.Struct && nestedSrc.Kind() == reflect.Struct {
				preserveTransientStructFields(nestedDst, nestedSrc, schema, path)
			}

			continue
		}

		dstField.Set(srcField)
	}
}

func preserveTransientRelationField(dst reflect.Value, src reflect.Value, schema *EntitySchema) error {
	if !dst.IsValid() || !src.IsValid() || schema == nil {
		return nil
	}

	if dst.Kind() == reflect.Slice && src.Kind() == reflect.Slice {
		return preserveTransientSliceRelationFields(dst, src, schema)
	}

	if !relationFieldHasValue(dst) || !relationFieldHasValue(src) {
		return nil
	}

	dstEntity := unwrapEntityValue(dst)
	srcEntity := unwrapEntityValue(src)
	if !dstEntity.IsValid() || !srcEntity.IsValid() || dstEntity.Kind() != reflect.Struct || srcEntity.Kind() != reflect.Struct {
		return nil
	}

	match, err := relationEntitiesMatch(dstEntity, srcEntity, schema)
	if err != nil {
		return err
	}

	if !match {
		return nil
	}

	return preserveTransientFields(dstEntity, srcEntity, schema)
}

func preserveTransientSliceRelationFields(dst reflect.Value, src reflect.Value, schema *EntitySchema) error {
	if dst.Len() == 0 || src.Len() == 0 {
		return nil
	}

	srcByKey, err := indexRelatedEntitiesByKey(src, schema)
	if err != nil {
		return err
	}

	for i := range dst.Len() {
		dstEntity := unwrapEntityValue(dst.Index(i))
		if !dstEntity.IsValid() || dstEntity.Kind() != reflect.Struct {
			continue
		}

		srcEntity, matchedByKey, err := matchRelatedEntity(dstEntity, src, srcByKey, i, schema)
		if err != nil {
			return err
		}

		if !srcEntity.IsValid() {
			continue
		}

		match, err := relationEntitiesMatchWhenNeeded(dstEntity, srcEntity, matchedByKey, schema)
		if err != nil {
			return err
		}

		if !match {
			continue
		}

		if err := preserveTransientFields(dstEntity, srcEntity, schema); err != nil {
			return err
		}
	}

	return nil
}

func relationEntitiesMatchWhenNeeded(dst reflect.Value, src reflect.Value, matchedByKey bool, schema *EntitySchema) (bool, error) {
	if matchedByKey {
		return true, nil
	}

	return relationEntitiesMatch(dst, src, schema)
}

func indexRelatedEntitiesByKey(slice reflect.Value, schema *EntitySchema) (map[any][]reflect.Value, error) {
	byKey := make(map[any][]reflect.Value, slice.Len())
	for i := range slice.Len() {
		entity := unwrapEntityValue(slice.Index(i))
		if !entity.IsValid() || entity.Kind() != reflect.Struct {
			continue
		}

		key, present, err := relationEntityKey(entity, schema)
		if err != nil {
			return nil, err
		}

		if !present {
			continue
		}

		byKey[key] = append(byKey[key], entity)
	}

	return byKey, nil
}

func matchRelatedEntity(dstEntity reflect.Value, srcSlice reflect.Value, srcByKey map[any][]reflect.Value, index int, schema *EntitySchema) (reflect.Value, bool, error) {
	key, present, err := relationEntityKey(dstEntity, schema)
	if err != nil {
		return reflect.Value{}, false, err
	}

	if present {
		matches := srcByKey[key]
		if len(matches) > 0 {
			match := matches[0]
			srcByKey[key] = matches[1:]

			return match, true, nil
		}
	}

	if index >= srcSlice.Len() {
		return reflect.Value{}, false, nil
	}

	candidate := unwrapEntityValue(srcSlice.Index(index))
	if !candidate.IsValid() || candidate.Kind() != reflect.Struct {
		return reflect.Value{}, false, nil
	}

	return candidate, false, nil
}

func relationFieldHasValue(field reflect.Value) bool {
	if !field.IsValid() {
		return false
	}

	if field.Kind() == reflect.Ptr {
		return !field.IsNil()
	}

	entity := unwrapEntityValue(field)
	if !entity.IsValid() {
		return false
	}

	return !entity.IsZero()
}

func relationEntitiesMatch(left reflect.Value, right reflect.Value, schema *EntitySchema) (bool, error) {
	leftKey, leftPresent, err := relationEntityKey(left, schema)
	if err != nil {
		return false, err
	}

	rightKey, rightPresent, err := relationEntityKey(right, schema)
	if err != nil {
		return false, err
	}

	if !leftPresent || !rightPresent {
		return !leftPresent && !rightPresent, nil
	}

	return leftKey == rightKey, nil
}

func relationEntityKey(entity reflect.Value, schema *EntitySchema) (key any, present bool, err error) {
	if schema == nil || schema.PrimaryKey == nil {
		return nil, false, nil
	}

	return optionalComparableKey(entity.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface())
}

func hasExactManagedFieldPath(schema *EntitySchema, path []int) bool {
	for _, col := range schema.Columns {
		if fieldPathsEqual(col.FieldIndex, path) {
			return true
		}
	}

	for _, rel := range schema.Relationships {
		if fieldPathsEqual(rel.FieldIndex, path) {
			return true
		}
	}

	return false
}

func hasManagedDescendantPath(schema *EntitySchema, path []int) bool {
	for _, col := range schema.Columns {
		if fieldPathHasPrefix(col.FieldIndex, path) && !fieldPathsEqual(col.FieldIndex, path) {
			return true
		}
	}

	for _, rel := range schema.Relationships {
		if fieldPathHasPrefix(rel.FieldIndex, path) && !fieldPathsEqual(rel.FieldIndex, path) {
			return true
		}
	}

	return false
}

func appendFieldPath(parentPath []int, index int) []int {
	path := make([]int, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = index

	return path
}

func fieldPathsEqual(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}

func fieldPathHasPrefix(path []int, prefix []int) bool {
	if len(prefix) > len(path) {
		return false
	}

	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}

	return true
}
