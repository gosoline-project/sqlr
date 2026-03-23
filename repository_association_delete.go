package sqlr

import (
	"context"
	"fmt"
	"reflect"
)

func (c *associationSyncContext) deleteEntityGraph(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, parentPath string, applyPolicy bool) error {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil
	}

	if c.state == nil {
		c.state = newAssociationSyncState()
	}

	key, hasKey := associationSyncKey(schema, entityValue)
	if hasKey {
		if !c.state.enter(key) {
			return nil
		}

		defer c.state.leave(key)
	}

	parentPK := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface()
	if err := c.deleteEntityAssociations(ctx, schema, parentPK, parentPath, applyPolicy); err != nil {
		return err
	}

	if err := c.deleteStoredEntity(ctx, schema, parentPK); err != nil {
		return fmt.Errorf("failed to delete entity %s: %w", schema.TableName, err)
	}

	return nil
}

func (c *associationSyncContext) deleteEntityAssociations(ctx context.Context, schema *EntitySchema, parentPK any, parentPath string, applyPolicy bool) error {
	for _, name := range schema.ValidRelationNames() {
		rel := schema.Relationships[name]
		if err := c.deleteAssociationForEntity(ctx, schema, rel, parentPK, parentPath, applyPolicy); err != nil {
			return err
		}
	}

	return nil
}

func (c *associationSyncContext) deleteAssociationForEntity(ctx context.Context, schema *EntitySchema, rel *Relationship, parentPK any, parentPath string, applyPolicy bool) error {
	relationPath := joinAssociationPath(parentPath, rel.Name)
	if applyPolicy && c.policy != nil && !c.policy.shouldSyncPath(relationPath) {
		return nil
	}

	switch rel.Type {
	case HasOne, HasMany:
		return c.deleteChildAssociation(ctx, schema, rel, parentPK, relationPath)
	case ManyToMany:
		return c.deleteManyToManyAssociation(ctx, schema, rel, parentPK)
	case BelongsTo:
		return nil
	default:
		return nil
	}
}

func (c *associationSyncContext) deleteChildAssociation(ctx context.Context, schema *EntitySchema, rel *Relationship, parentPK any, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for delete relation %q: %w", rel.Name, err)
	}

	children, err := c.loadChildrenByForeignKey(ctx, nestedSchema, rel.ForeignKey, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load related rows for delete relation %q: %w", rel.Name, err)
	}

	for _, child := range children {
		if err := c.deleteEntityGraph(ctx, nestedSchema, child, relationPath, false); err != nil {
			return fmt.Errorf("failed to cascade delete relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func (c *associationSyncContext) deleteManyToManyAssociation(ctx context.Context, schema *EntitySchema, rel *Relationship, parentPK any) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany delete relation %q: %w", rel.Name, err)
	}

	if err := c.deleteAllM2MLinks(ctx, rel, schema, nestedSchema, parentPK); err != nil {
		return fmt.Errorf("failed to delete ManyToMany links for relation %q: %w", rel.Name, err)
	}

	return nil
}
