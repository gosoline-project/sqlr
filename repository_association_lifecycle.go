package sqlr

import (
	"context"
	"reflect"
)

// createEntityWithAssociations persists an entity together with all populated
// association fields. It handles the four relationship types in the correct order:
//
//  1. BelongsTo associations are inserted first (if their PK is zero) so that the
//     parent's FK column can be set before the parent row is written.
//  2. The parent entity row is inserted.
//  3. HasOne and HasMany associations are inserted with their FK set to the parent PK.
//  4. ManyToMany associations are inserted and join-table rows are created.
//
// All operations must be executed within a transaction so that a failure at any step
// rolls back the entire tree. The caller is responsible for providing a transaction
// querier (q) and a non-nil ttx when association saves are required.
func (r *repositoryCommon[K, E]) createEntityWithAssociations(ctx context.Context, associationCtx *associationCreateContext, entity *E) error {
	if err := r.saveBelongsToAssociations(ctx, associationCtx, entity); err != nil {
		return err
	}

	if err := r.createEntity(associationCtx.q, ctx, entity, associationCtx.journal, associationCtx.mutationOptions); err != nil {
		return err
	}

	return r.saveAssociations(ctx, associationCtx, entity)
}

// updateEntityWithAssociations synchronizes the entity graph rooted at entity.
// It updates the parent row and all selected associations in one
// transaction-aware operation after the caller opted into association sync via
// QueryBuilderUpdate. BelongsTo relations are synchronized first so the parent's
// foreign keys are current before the root row is updated. HasOne and HasMany
// relations are fully synchronized afterwards, including deleting existing
// HasOne rows when the caller clears the relation to its zero value or nil,
// while ManyToMany relations
// reconcile join-table membership by default unless explicitly opted into full
// related-entity synchronization.
func (r *repositoryCommon[K, E]) updateEntityWithAssociations(ctx context.Context, associationCtx *associationSyncContext, entity *E) (*E, error) {
	rv, err := requireEntityValue(entity)
	if err != nil {
		return nil, err
	}

	if err := associationCtx.syncExistingEntityGraph(ctx, r.schema, rv); err != nil {
		return nil, err
	}

	return entity, nil
}

// deleteEntityWithAssociations synchronizes association cleanup for the entity
// identified by id before deleting the root row. Delete selects this path when
// owned associations are included by the active delete policy. It applies the
// ownership model used by Update reconciliation: HasOne and HasMany relations
// are recursively deleted, ManyToMany relations remove join-table rows, and
// BelongsTo relations are left untouched.
func (r *repositoryCommon[K, E]) deleteEntityWithAssociations(ctx context.Context, associationCtx *associationSyncContext, id K) error {
	rootValue, err := associationCtx.loadEntityByPrimaryKey(ctx, r.schema, id)
	if err != nil {
		return err
	}

	return associationCtx.deleteEntityGraph(ctx, r.schema, rootValue, "", true)
}

func (r *repositoryCommon[K, E]) hasAssociationsToDelete(policy *associationSyncPolicy) bool {
	if len(r.schema.Relationships) == 0 {
		return false
	}

	if policy == nil || !policy.shouldSyncRootAssociations() {
		return false
	}

	for _, rel := range r.schema.Relationships {
		if rel.Type == BelongsTo {
			continue
		}

		if policy.shouldSyncPath(rel.Name) {
			return true
		}
	}

	return false
}

// hasAssociationsToSave returns true if any association field on the entity is non-zero.
// This is used to decide whether to start a transaction for the create operation.
func (r *repositoryCommon[K, E]) hasAssociationsToSave(entity *E, policy *associationSyncPolicy) bool {
	if len(r.schema.Relationships) == 0 {
		return false
	}

	if policy == nil || !policy.shouldSyncRootAssociations() {
		return false
	}

	rv, err := requireEntityValue(entity)
	if err != nil {
		return false
	}

	for _, rel := range r.schema.Relationships {
		relationPath := rel.Name
		if !policy.shouldSyncPath(relationPath) {
			continue
		}

		field := rv.FieldByIndex(rel.FieldIndex)
		if relationHasValues(field) {
			return true
		}
	}

	return false
}

func relationHasValues(field reflect.Value) bool {
	if !field.IsValid() {
		return false
	}

	switch field.Kind() {
	case reflect.Slice, reflect.Array:
		return field.Len() > 0
	default:
		return !field.IsZero()
	}
}

// saveBelongsToAssociations inserts any BelongsTo related entities that have a zero
// primary key, then sets the corresponding FK column on the parent entity. This must be
// called BEFORE the parent entity is inserted so that the FK value is populated in time.
func (r *repositoryCommon[K, E]) saveBelongsToAssociations(ctx context.Context, associationCtx *associationCreateContext, entity *E) error {
	rv, err := requireEntityValue(entity)
	if err != nil {
		return err
	}

	return associationCtx.createRelatedBelongsTo(ctx, r.schema, rv, "")
}

// saveAssociations persists all populated association fields on the entity after the
// parent entity has been inserted. BelongsTo associations are expected to be already
// persisted via saveBelongsToAssociations called before the parent insert. This
// function handles HasOne, HasMany, and ManyToMany phases by delegating to the
// schema-based createRelatedForwardAssociations helper.
func (r *repositoryCommon[K, E]) saveAssociations(ctx context.Context, associationCtx *associationCreateContext, entity *E) error {
	rv, err := requireEntityValue(entity)
	if err != nil {
		return err
	}

	return associationCtx.createRelatedForwardAssociations(ctx, r.schema, rv, "")
}
