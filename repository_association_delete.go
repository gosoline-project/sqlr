package sqlr

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gosoline-project/sqlc"
)

func deleteEntityGraph(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState) error {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil
	}

	if state == nil {
		state = newAssociationSyncState()
	}

	key, hasKey := associationSyncKey(schema, entityValue)
	if hasKey {
		if !state.enter(key) {
			return nil
		}

		defer state.leave(key)
	}

	parentPK := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface()
	if err := deleteEntityAssociations(cache, q, ctx, schema, parentPK, state); err != nil {
		return err
	}

	if err := deleteStoredEntity(cache, q, ctx, schema, parentPK); err != nil {
		return fmt.Errorf("failed to delete entity %s: %w", schema.TableName, err)
	}

	return nil
}

func deleteEntityAssociations(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, parentPK any, state *associationSyncState) error {
	for _, rel := range schema.Relationships {
		if err := deleteAssociationForEntity(cache, q, ctx, schema, rel, parentPK, state); err != nil {
			return err
		}
	}

	return nil
}

func deleteAssociationForEntity(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, rel *Relationship, parentPK any, state *associationSyncState) error {
	switch rel.Type {
	case HasOne, HasMany:
		return deleteChildAssociation(cache, q, ctx, schema, rel, parentPK, state)
	case ManyToMany:
		return deleteManyToManyAssociation(cache, q, ctx, schema, rel, parentPK)
	case BelongsTo:
		return nil
	default:
		return nil
	}
}

func deleteChildAssociation(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, rel *Relationship, parentPK any, state *associationSyncState) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for delete relation %q: %w", rel.Name, err)
	}

	children, err := loadChildrenByForeignKey(cache, q, ctx, nestedSchema, rel.ForeignKey, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load related rows for delete relation %q: %w", rel.Name, err)
	}

	for _, child := range children {
		if err := deleteEntityGraph(cache, q, ctx, nestedSchema, child, state); err != nil {
			return fmt.Errorf("failed to cascade delete relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func deleteManyToManyAssociation(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, rel *Relationship, parentPK any) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany delete relation %q: %w", rel.Name, err)
	}

	if err := deleteAllM2MLinks(cache, q, ctx, rel, schema, nestedSchema, parentPK); err != nil {
		return fmt.Errorf("failed to delete ManyToMany links for relation %q: %w", rel.Name, err)
	}

	return nil
}
