package sqlr

import (
	"context"
	"reflect"

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

// saveAssociations persists all populated association fields on the entity after the
// parent entity has been inserted. BelongsTo associations are expected to be already
// persisted via saveBelongsToAssociations called before the parent insert. This
// function handles HasOne, HasMany, and ManyToMany phases by delegating to the
// schema-based createRelatedForwardAssociations helper.
func (r *repositoryCommon[K, E]) saveAssociations(q sqlc.Querier, ctx context.Context, entity *E) error {
	rv := reflect.ValueOf(entity).Elem()

	return createRelatedForwardAssociations(q, ctx, r.schema, rv)
}

// saveBelongsToAssociations inserts any BelongsTo related entities that have a zero
// primary key, then sets the corresponding FK column on the parent entity. This must be
// called BEFORE the parent entity is inserted so that the FK value is populated in time.
func (r *repositoryCommon[K, E]) saveBelongsToAssociations(q sqlc.Querier, ctx context.Context, entity *E) error {
	rv := reflect.ValueOf(entity).Elem()

	return createRelatedBelongsTo(q, ctx, r.schema, rv)
}
