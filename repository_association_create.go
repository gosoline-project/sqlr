package sqlr

import (
	"context"
	"fmt"
	"reflect"
)

// createRelatedEntity persists a related entity and all of its own populated
// associations recursively. It mirrors the four-phase flow of
// createEntityWithAssociations but operates on a bare reflect.Value and
// *EntitySchema so it can be used for any related entity type without needing
// a typed repositoryCommon.
func (c *associationCreateContext) createRelatedEntity(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, path string) (any, error) {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil, fmt.Errorf("invalid entity value for schema %s", schema.TableName)
	}

	if err := c.createRelatedBelongsTo(ctx, schema, entityValue, path); err != nil {
		return nil, err
	}

	pk, err := c.insertRelatedEntity(ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	if !c.mutationOptions.autoUpdatesDisabled() {
		if err := setEntityPrimaryKey(schema, entityValue, pk, c.journal); err != nil {
			return nil, err
		}
	}

	if err := c.createRelatedForwardAssociations(ctx, schema, entityValue, path); err != nil {
		return nil, err
	}

	return pk, nil
}

func (c *associationCreateContext) createRelatedBelongsTo(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, parentPath string) error {
	for _, rel := range schema.Relationships {
		if !c.shouldCreateBelongsTo(rel, parentPath) {
			continue
		}

		relationPath := joinAssociationPath(parentPath, rel.Name)
		field := entityValue.FieldByIndex(rel.FieldIndex)
		field = unwrapEntityValue(field)
		if !field.IsValid() || field.IsZero() {
			continue
		}

		nestedSchema, err := rel.ResolveRelatedSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for BelongsTo relation %q: %w", rel.Name, err)
		}

		pkField := field.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		if pkField.IsZero() {
			if _, err := c.createRelatedEntity(ctx, nestedSchema, field, relationPath); err != nil {
				return fmt.Errorf("failed to insert BelongsTo relation %q: %w", rel.Name, err)
			}
		}

		if err := c.setBelongsToForeignKey(entityValue, schema, rel, pkField.Interface()); err != nil {
			return fmt.Errorf("BelongsTo relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func (c *associationCreateContext) shouldCreateBelongsTo(rel *Relationship, parentPath string) bool {
	if rel.Type != BelongsTo {
		return false
	}

	relationPath := joinAssociationPath(parentPath, rel.Name)

	return c.policy == nil || c.policy.shouldSyncPath(relationPath)
}

func (c *associationCreateContext) setBelongsToForeignKey(entityValue reflect.Value, schema *EntitySchema, rel *Relationship, value any) error {
	fkCol, ok := schema.ColumnByName(rel.ForeignKey)
	if !ok {
		return fmt.Errorf("FK column %q not found on schema", rel.ForeignKey)
	}

	fkField := entityValue.FieldByIndex(fkCol.FieldIndex)
	if c.mutationOptions.autoUpdatesDisabled() {
		return reconcileExistingFieldValue(fkField, value, schema.TableName, rel.ForeignKey, c.journal)
	}

	return setFieldValue(fkField, value, schema.TableName, rel.ForeignKey, c.journal)
}

func (c *associationCreateContext) createRelatedForwardAssociations(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, parentPath string) error {
	for _, rel := range schema.Relationships {
		relationPath := joinAssociationPath(parentPath, rel.Name)
		if c.policy != nil && !c.policy.shouldSyncPath(relationPath) {
			continue
		}

		field := entityValue.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		switch rel.Type {
		case HasOne:
			if err := c.createRelatedHasOne(ctx, schema, entityValue, rel, relationPath); err != nil {
				return err
			}
		case HasMany:
			if err := c.createRelatedHasMany(ctx, schema, entityValue, rel, relationPath); err != nil {
				return err
			}
		case ManyToMany:
			if err := c.createRelatedManyToMany(ctx, schema, entityValue, rel, relationPath); err != nil {
				return err
			}
		case BelongsTo:
		}
	}

	return nil
}

func (c *associationCreateContext) createRelatedHasOne(ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	relField := parentValue.FieldByIndex(rel.FieldIndex)
	relField = unwrapEntityValue(relField)
	if !relField.IsValid() {
		return fmt.Errorf("HasOne relation %q is nil", rel.Name)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK, c.journal, c.mutationOptions); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	pkField := relField.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		if err := c.updateStoredEntityForeignKey(ctx, nestedSchema, relField, rel.ForeignKey); err != nil {
			return fmt.Errorf("failed to update HasOne relation %q: %w", rel.Name, err)
		}

		return nil
	}

	if _, err := c.createRelatedEntity(ctx, nestedSchema, relField, relationPath); err != nil {
		return fmt.Errorf("failed to insert HasOne relation %q: %w", rel.Name, err)
	}

	return nil
}

func (c *associationCreateContext) createRelatedHasMany(ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
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

		if err := setRelatedFK(elem, nestedSchema, rel.ForeignKey, parentPK, c.journal, c.mutationOptions); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pkField := elem.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
		if !pkField.IsZero() {
			if err := c.updateStoredEntityForeignKey(ctx, nestedSchema, elem, rel.ForeignKey); err != nil {
				return fmt.Errorf("failed to update HasMany relation %q[%d]: %w", rel.Name, i, err)
			}

			continue
		}

		if _, err := c.createRelatedEntity(ctx, nestedSchema, elem, relationPath); err != nil {
			return fmt.Errorf("failed to insert HasMany relation %q[%d]: %w", rel.Name, i, err)
		}
	}

	return nil
}

func (c *associationCreateContext) createRelatedManyToMany(ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
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
			if _, err := c.createRelatedEntity(ctx, nestedSchema, elem, relationPath); err != nil {
				return fmt.Errorf("failed to insert ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}
		}

		relatedPKs = append(relatedPKs, pkField.Interface())
	}

	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, nestedSchema)

	if err := c.insertJoinTableRows(ctx, rel.JoinTable, parentColName, relatedColName, parentPK, relatedPKs); err != nil {
		return fmt.Errorf("failed to insert join table rows for ManyToMany relation %q: %w", rel.Name, err)
	}

	return nil
}
