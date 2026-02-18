package sqlr

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gosoline-project/sqlc"
)

// hasAssociationsToSave returns true if any association field on the entity is non-zero.
// This is used to decide whether to start a transaction for the create operation.
func (r *repositoryCommon[K, E]) hasAssociationsToSave(entity *E) bool {
	if len(r.schema.Relationships) == 0 {
		return false
	}

	rv := reflect.ValueOf(entity).Elem()

	for _, rel := range r.schema.Relationships {
		field := rv.FieldByIndex(rel.FieldIndex)
		if !field.IsZero() {
			return true
		}
	}

	return false
}

// saveAssociations persists all populated association fields on the entity after the
// parent entity has been inserted. It handles the four relationship types:
//
//   - BelongsTo: the related entity is inserted first (before the parent in the calling
//     flow) and the parent's FK column is updated to the related entity's PK. This function
//     is called after the parent insert, so BelongsTo entities are expected to be already
//     persisted (either pre-existing with non-zero PK, or newly inserted via
//     saveBelongsToAssociations called before the parent insert). This function only
//     handles HasOne, HasMany, and ManyToMany phases.
//
//   - HasOne / HasMany: the FK column on the related entity is set to the parent PK,
//     then the related entity is inserted. Only related entities with a zero primary key
//     are inserted; those with non-zero PKs are already persisted and skipped.
//
//   - ManyToMany: related entities with zero PKs are inserted first, then join table
//     rows are created for all related entities (both newly inserted and pre-existing).
func (r *repositoryCommon[K, E]) saveAssociations(q sqlc.Querier, ctx context.Context, entity *E, ttx *TTx) error {
	rv := reflect.ValueOf(entity).Elem()

	for _, rel := range r.schema.Relationships {
		field := rv.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		switch rel.Type {
		case HasOne:
			if err := r.saveHasOneAssociation(q, ctx, rv, rel, ttx); err != nil {
				return err
			}
		case HasMany:
			if err := r.saveHasManyAssociation(q, ctx, rv, rel, ttx); err != nil {
				return err
			}
		case ManyToMany:
			if err := r.saveManyToManyAssociation(q, ctx, rv, rel, ttx); err != nil {
				return err
			}
		case BelongsTo:
			// BelongsTo was already handled before the parent insert; nothing to do here.
		}
	}

	return nil
}

// saveBelongsToAssociations inserts any BelongsTo related entities that have a zero
// primary key, then sets the corresponding FK column on the parent entity. This must be
// called BEFORE the parent entity is inserted so that the FK value is populated in time.
func (r *repositoryCommon[K, E]) saveBelongsToAssociations(q sqlc.Querier, ctx context.Context, entity *E, ttx *TTx) error {
	rv := reflect.ValueOf(entity).Elem()

	for _, rel := range r.schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		field := rv.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		relSchema, err := rel.resolveRelationSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for BelongsTo relation %q: %w", rel.Name, err)
		}

		// Only insert the related entity if it does not already have a primary key.
		pkField := field.FieldByIndex(relSchema.PrimaryKey.FieldIndex)
		if !pkField.IsZero() {
			// Already persisted — just make sure the FK column on the parent is set.
			parentFKCol, ok := r.schema.ColumnByName(rel.ForeignKey)
			if !ok {
				return fmt.Errorf("BelongsTo relation %q: FK column %q not found on parent schema", rel.Name, rel.ForeignKey)
			}

			rv.FieldByIndex(parentFKCol.FieldIndex).Set(pkField)

			continue
		}

		// Insert the related entity (and its own associations) recursively.
		relPK, err := createRelatedEntity(q, ctx, relSchema, field, ttx)
		if err != nil {
			return fmt.Errorf("failed to insert BelongsTo relation %q: %w", rel.Name, err)
		}

		// Set the generated PK back on the related struct field.
		pkField.Set(reflect.ValueOf(relPK).Convert(pkField.Type()))

		// Set the FK column on the parent entity to point to the related entity.
		parentFKCol, ok := r.schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("BelongsTo relation %q: FK column %q not found on parent schema", rel.Name, rel.ForeignKey)
		}

		rv.FieldByIndex(parentFKCol.FieldIndex).Set(pkField)
	}

	return nil
}

// saveHasOneAssociation sets the FK on the related HasOne entity to the parent PK
// and inserts it when its primary key is zero.
func (r *repositoryCommon[K, E]) saveHasOneAssociation(q sqlc.Querier, ctx context.Context, parentRV reflect.Value, rel *Relationship, ttx *TTx) error {
	relSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	relField := parentRV.FieldByIndex(rel.FieldIndex)
	if relField.IsZero() {
		return nil
	}

	parentPK := parentRV.FieldByIndex(r.schema.PrimaryKey.FieldIndex).Interface()

	if err := setRelatedFK(relField, relSchema, rel.ForeignKey, parentPK); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	pkField := relField.FieldByIndex(relSchema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		// Already persisted — skip insert but FK is already set above.
		return nil
	}

	relPK, err := createRelatedEntity(q, ctx, relSchema, relField, ttx)
	if err != nil {
		return fmt.Errorf("failed to insert HasOne relation %q: %w", rel.Name, err)
	}

	pkField.Set(reflect.ValueOf(relPK).Convert(pkField.Type()))

	return nil
}

// saveHasManyAssociation iterates the slice of related HasMany entities, sets the FK
// on each, and inserts those with a zero primary key.
func (r *repositoryCommon[K, E]) saveHasManyAssociation(q sqlc.Querier, ctx context.Context, parentRV reflect.Value, rel *Relationship, ttx *TTx) error {
	relSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasMany relation %q: %w", rel.Name, err)
	}

	sliceField := parentRV.FieldByIndex(rel.FieldIndex)
	if sliceField.IsZero() || sliceField.Len() == 0 {
		return nil
	}

	parentPK := parentRV.FieldByIndex(r.schema.PrimaryKey.FieldIndex).Interface()

	for i := range sliceField.Len() {
		elem := sliceField.Index(i)

		if err := setRelatedFK(elem, relSchema, rel.ForeignKey, parentPK); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pkField := elem.FieldByIndex(relSchema.PrimaryKey.FieldIndex)
		if !pkField.IsZero() {
			continue
		}

		relPK, err := createRelatedEntity(q, ctx, relSchema, elem, ttx)
		if err != nil {
			return fmt.Errorf("failed to insert HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pkField.Set(reflect.ValueOf(relPK).Convert(pkField.Type()))
	}

	return nil
}

// saveManyToManyAssociation inserts related entities with zero PKs, then inserts the
// join table rows linking the parent to all related entities.
func (r *repositoryCommon[K, E]) saveManyToManyAssociation(q sqlc.Querier, ctx context.Context, parentRV reflect.Value, rel *Relationship, ttx *TTx) error {
	relSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany relation %q: %w", rel.Name, err)
	}

	sliceField := parentRV.FieldByIndex(rel.FieldIndex)
	if sliceField.IsZero() || sliceField.Len() == 0 {
		return nil
	}

	parentPK := parentRV.FieldByIndex(r.schema.PrimaryKey.FieldIndex).Interface()

	// Insert new related entities and collect all related PKs.
	relatedPKs := make([]any, 0, sliceField.Len())

	for i := range sliceField.Len() {
		elem := sliceField.Index(i)
		pkField := elem.FieldByIndex(relSchema.PrimaryKey.FieldIndex)

		if pkField.IsZero() {
			relPK, err := createRelatedEntity(q, ctx, relSchema, elem, ttx)
			if err != nil {
				return fmt.Errorf("failed to insert ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}

			pkField.Set(reflect.ValueOf(relPK).Convert(pkField.Type()))
		}

		relatedPKs = append(relatedPKs, pkField.Interface())
	}

	// Insert join table rows. Convention: snake_case(ParentType)_id and snake_case(RelatedType)_id.
	parentColName := toSnakeCase(r.schema.entityType.Name()) + "_id"
	relatedColName := toSnakeCase(rel.RelatedType.Name()) + "_id"

	if err := insertJoinTableRows(q, ctx, rel.JoinTable, parentColName, relatedColName, parentPK, relatedPKs, ttx); err != nil {
		return fmt.Errorf("failed to insert join table rows for ManyToMany relation %q: %w", rel.Name, err)
	}

	return nil
}

// createRelatedEntity persists a related entity and all of its own populated
// associations recursively. It mirrors the four-phase flow of
// createEntityWithAssociations but operates on a bare reflect.Value and
// *EntitySchema so it can be used for any related entity type without needing
// a typed repositoryCommon.
//
// Phase 1: BelongsTo associations on the related entity are inserted first so
// that the related entity's FK columns are populated before its own row is
// written.
// Phase 2: The related entity row itself is inserted via insertRelatedEntity.
// Phase 3/4: HasOne, HasMany, and ManyToMany associations on the related entity
// are inserted with their FK columns pointing to the newly inserted related PK.
//
// Returns the primary key value of the inserted (or already-persisted) entity.
func createRelatedEntity(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, ttx *TTx) (any, error) {
	// Phase 1: persist BelongsTo relations and set FK columns on the current entity.
	if err := createRelatedBelongsTo(q, ctx, schema, entityValue, ttx); err != nil {
		return nil, err
	}

	// Phase 2: insert the related entity row itself.
	pk, err := insertRelatedEntity(q, ctx, schema, entityValue, ttx)
	if err != nil {
		return nil, err
	}

	// Write the generated PK back onto the entity value.
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	pkField.Set(reflect.ValueOf(pk).Convert(pkField.Type()))

	// Phase 3+4: persist HasOne, HasMany, and ManyToMany relations.
	if err := createRelatedForwardAssociations(q, ctx, schema, entityValue, ttx); err != nil {
		return nil, err
	}

	return pk, nil
}

// createRelatedBelongsTo handles Phase 1 of the recursive create flow: for each
// BelongsTo relation on the entity that is non-zero, it ensures the related
// entity is persisted (recursively if its PK is zero) and then sets the FK
// column on the current entity value.
func createRelatedBelongsTo(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, ttx *TTx) error {
	for _, rel := range schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		field := entityValue.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		nestedSchema, err := rel.resolveRelationSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for BelongsTo relation %q: %w", rel.Name, err)
		}

		pkField := field.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		if pkField.IsZero() {
			nestedPK, err := createRelatedEntity(q, ctx, nestedSchema, field, ttx)
			if err != nil {
				return fmt.Errorf("failed to insert BelongsTo relation %q: %w", rel.Name, err)
			}

			pkField.Set(reflect.ValueOf(nestedPK).Convert(pkField.Type()))
		}

		// Set the FK column on the current entity to point to the related entity.
		fkCol, ok := schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("BelongsTo relation %q: FK column %q not found on schema", rel.Name, rel.ForeignKey)
		}

		entityValue.FieldByIndex(fkCol.FieldIndex).Set(pkField)
	}

	return nil
}

// createRelatedForwardAssociations handles Phase 3+4 of the recursive create
// flow: for each HasOne, HasMany, or ManyToMany relation on the entity that is
// non-zero, it persists the related entities with the parent PK already set.
func createRelatedForwardAssociations(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, ttx *TTx) error {
	for _, rel := range schema.Relationships {
		field := entityValue.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		switch rel.Type {
		case HasOne:
			if err := createRelatedHasOne(q, ctx, schema, entityValue, rel, ttx); err != nil {
				return err
			}
		case HasMany:
			if err := createRelatedHasMany(q, ctx, schema, entityValue, rel, ttx); err != nil {
				return err
			}
		case ManyToMany:
			if err := createRelatedManyToMany(q, ctx, schema, entityValue, rel, ttx); err != nil {
				return err
			}
		case BelongsTo:
			// Already handled in Phase 1.
		}
	}

	return nil
}

// createRelatedHasOne handles a HasOne relation on a related entity during
// recursive association saving. It sets the FK on the nested entity and
// recursively calls createRelatedEntity for those with a zero PK.
func createRelatedHasOne(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, ttx *TTx) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	relField := parentValue.FieldByIndex(rel.FieldIndex)
	if relField.IsZero() {
		return nil
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	pkField := relField.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		return nil
	}

	nestedPK, err := createRelatedEntity(q, ctx, nestedSchema, relField, ttx)
	if err != nil {
		return fmt.Errorf("failed to insert HasOne relation %q: %w", rel.Name, err)
	}

	pkField.Set(reflect.ValueOf(nestedPK).Convert(pkField.Type()))

	return nil
}

// createRelatedHasMany handles a HasMany relation on a related entity during
// recursive association saving. It sets the FK on each element and recursively
// calls createRelatedEntity for those with a zero PK.
func createRelatedHasMany(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, ttx *TTx) error {
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
		elem := sliceField.Index(i)

		if err := setRelatedFK(elem, nestedSchema, rel.ForeignKey, parentPK); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pkField := elem.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
		if !pkField.IsZero() {
			continue
		}

		nestedPK, err := createRelatedEntity(q, ctx, nestedSchema, elem, ttx)
		if err != nil {
			return fmt.Errorf("failed to insert HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pkField.Set(reflect.ValueOf(nestedPK).Convert(pkField.Type()))
	}

	return nil
}

// createRelatedManyToMany handles a ManyToMany relation on a related entity
// during recursive association saving. It inserts related entities with zero
// PKs recursively, then inserts the join table rows.
func createRelatedManyToMany(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, ttx *TTx) error {
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
		elem := sliceField.Index(i)
		pkField := elem.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		if pkField.IsZero() {
			nestedPK, err := createRelatedEntity(q, ctx, nestedSchema, elem, ttx)
			if err != nil {
				return fmt.Errorf("failed to insert ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}

			pkField.Set(reflect.ValueOf(nestedPK).Convert(pkField.Type()))
		}

		relatedPKs = append(relatedPKs, pkField.Interface())
	}

	parentColName := toSnakeCase(parentSchema.entityType.Name()) + "_id"
	relatedColName := toSnakeCase(rel.RelatedType.Name()) + "_id"

	if err := insertJoinTableRows(q, ctx, rel.JoinTable, parentColName, relatedColName, parentPK, relatedPKs, ttx); err != nil {
		return fmt.Errorf("failed to insert join table rows for ManyToMany relation %q: %w", rel.Name, err)
	}

	return nil
}

// insertRelatedEntity builds and executes an INSERT for a related entity using its
// schema. It sets autoCreateTime and autoUpdateTime fields, skips auto-increment
// PKs in the INSERT columns, and returns the generated primary key value.
//
// The entityValue must be an addressable reflect.Value of the related struct.
func insertRelatedEntity(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, ttx *TTx) (any, error) {
	now := time.Now()

	// Set auto-timestamp fields and collect INSERT values.
	insertCols := schema.InsertColumns()
	vals := make([]any, 0, len(insertCols))

	for _, col := range schema.Columns {
		if col.AutoCreateTime || col.AutoUpdateTime {
			entityValue.FieldByIndex(col.FieldIndex).Set(reflect.ValueOf(now))
		}

		if col.IsPrimaryKey && col.AutoIncrement {
			continue
		}

		vals = append(vals, entityValue.FieldByIndex(col.FieldIndex).Interface())
	}

	sqler := sqlc.Into(schema.TableName).
		Columns(insertCols...).
		Values(vals...)

	sql, args, err := sqler.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build INSERT SQL for %s: %w", schema.TableName, err)
	}

	result, err := execWithQuerier(q, ctx, sql, args, ttx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute INSERT for %s: %w", schema.TableName, err)
	}

	if schema.PrimaryKey == nil || !schema.PrimaryKey.AutoIncrement {
		// Return the existing (non-auto-increment) PK value.
		return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID for %s: %w", schema.TableName, err)
	}

	return lastID, nil
}

// insertJoinTableRows inserts rows into a many-to-many join table. It uses INSERT IGNORE
// to avoid errors if the link already exists.
func insertJoinTableRows(q sqlc.Querier, ctx context.Context, joinTable, parentCol, relatedCol string, parentPK any, relatedPKs []any, ttx *TTx) error {
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

	if _, err := execWithQuerier(q, ctx, sql, args, ttx); err != nil {
		return fmt.Errorf("failed to insert join table rows: %w", err)
	}

	return nil
}

// setRelatedFK sets the foreign key field on a related entity to the given parent PK value.
// The FK column name is resolved through the schema to get the struct field index.
func setRelatedFK(entityValue reflect.Value, schema *EntitySchema, fkColName string, parentPK any) error {
	fkCol, ok := schema.ColumnByName(fkColName)
	if !ok {
		return fmt.Errorf("FK column %q not found in schema for %s", fkColName, schema.TableName)
	}

	fkField := entityValue.FieldByIndex(fkCol.FieldIndex)
	pkVal := reflect.ValueOf(parentPK)

	if !pkVal.Type().ConvertibleTo(fkField.Type()) {
		return fmt.Errorf("parent PK type %s is not convertible to FK field type %s for column %q", pkVal.Type(), fkField.Type(), fkColName)
	}

	fkField.Set(pkVal.Convert(fkField.Type()))

	return nil
}

// execWithQuerier executes a raw SQL statement. When a transaction is active it uses
// the transaction's Exec method; otherwise it uses the plain querier.
func execWithQuerier(q sqlc.Querier, ctx context.Context, sql string, args []any, ttx *TTx) (sqlc.Result, error) {
	if ttx != nil {
		return ttx.Exec(ctx, sql, args...)
	}

	return q.Exec(ctx, sql, args...)
}
