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
func createRelatedEntity(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	// Phase 1: persist BelongsTo relations and set FK columns on the current entity.
	if err := createRelatedBelongsTo(q, ctx, schema, entityValue); err != nil {
		return nil, err
	}

	// Phase 2: insert the related entity row itself.
	pk, err := insertRelatedEntity(q, ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	// Write the generated PK back onto the entity value.
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	pkField.Set(reflect.ValueOf(pk).Convert(pkField.Type()))

	// Phase 3+4: persist HasOne, HasMany, and ManyToMany relations.
	if err := createRelatedForwardAssociations(q, ctx, schema, entityValue); err != nil {
		return nil, err
	}

	return pk, nil
}

// createRelatedBelongsTo handles Phase 1 of the recursive create flow: for each
// BelongsTo relation on the entity that is non-zero, it ensures the related
// entity is persisted (recursively if its PK is zero) and then sets the FK
// column on the current entity value.
func createRelatedBelongsTo(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
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
			if _, err := createRelatedEntity(q, ctx, nestedSchema, field); err != nil {
				return fmt.Errorf("failed to insert BelongsTo relation %q: %w", rel.Name, err)
			}
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
func createRelatedForwardAssociations(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	for _, rel := range schema.Relationships {
		field := entityValue.FieldByIndex(rel.FieldIndex)
		if field.IsZero() {
			continue
		}

		switch rel.Type {
		case HasOne:
			if err := createRelatedHasOne(q, ctx, schema, entityValue, rel); err != nil {
				return err
			}
		case HasMany:
			if err := createRelatedHasMany(q, ctx, schema, entityValue, rel); err != nil {
				return err
			}
		case ManyToMany:
			if err := createRelatedManyToMany(q, ctx, schema, entityValue, rel); err != nil {
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
func createRelatedHasOne(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	relField := parentValue.FieldByIndex(rel.FieldIndex)
	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	pkField := relField.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		return nil
	}

	if _, err := createRelatedEntity(q, ctx, nestedSchema, relField); err != nil {
		return fmt.Errorf("failed to insert HasOne relation %q: %w", rel.Name, err)
	}

	return nil
}

// createRelatedHasMany handles a HasMany relation on a related entity during
// recursive association saving. It sets the FK on each element and recursively
// calls createRelatedEntity for those with a zero PK.
func createRelatedHasMany(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship) error {
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

		if _, err := createRelatedEntity(q, ctx, nestedSchema, elem); err != nil {
			return fmt.Errorf("failed to insert HasMany relation %q[%d]: %w", rel.Name, i, err)
		}
	}

	return nil
}

// createRelatedManyToMany handles a ManyToMany relation on a related entity
// during recursive association saving. It inserts related entities with zero
// PKs recursively, then inserts the join table rows.
func createRelatedManyToMany(q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship) error {
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
			if _, err := createRelatedEntity(q, ctx, nestedSchema, elem); err != nil {
				return fmt.Errorf("failed to insert ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}
		}

		relatedPKs = append(relatedPKs, pkField.Interface())
	}

	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, nestedSchema)

	if err := insertJoinTableRows(q, ctx, rel.JoinTable, parentColName, relatedColName, parentPK, relatedPKs); err != nil {
		return fmt.Errorf("failed to insert join table rows for ManyToMany relation %q: %w", rel.Name, err)
	}

	return nil
}

type associationSyncState struct {
	active map[string]struct{}
}

func newAssociationSyncState() *associationSyncState {
	return &associationSyncState{
		active: make(map[string]struct{}),
	}
}

func (s *associationSyncState) enter(key string) bool {
	if _, exists := s.active[key]; exists {
		return false
	}

	s.active[key] = struct{}{}

	return true
}

func (s *associationSyncState) leave(key string) {
	delete(s.active, key)
}

func syncExistingEntityGraph(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState) error {
	_, err := syncEntityGraph(cache, q, ctx, schema, entityValue, state)

	return err
}

func syncEntityGraph(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState) (any, error) {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil, fmt.Errorf("invalid entity value for schema %s", schema.TableName)
	}

	if state == nil {
		state = newAssociationSyncState()
	}

	key, hasKey := associationSyncKey(schema, entityValue)
	if hasKey {
		if !state.enter(key) {
			return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
		}

		defer state.leave(key)
	}

	if err := syncBelongsToAssociations(cache, q, ctx, schema, entityValue, state); err != nil {
		return nil, err
	}

	pk, err := persistEntityGraphNode(cache, q, ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	if err := syncForwardAssociations(cache, q, ctx, schema, entityValue, state); err != nil {
		return nil, err
	}

	return pk, nil
}

func persistEntityGraphNode(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		if err := updateStoredEntity(cache, q, ctx, schema, entityValue); err != nil {
			return nil, err
		}

		return pkField.Interface(), nil
	}

	pk, err := insertRelatedEntity(q, ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	if err := setEntityPrimaryKey(schema, entityValue, pk); err != nil {
		return nil, err
	}

	return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
}

func syncBelongsToAssociations(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState) error {
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

		if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, field, state); err != nil {
			return fmt.Errorf("failed to synchronize BelongsTo relation %q: %w", rel.Name, err)
		}

		pkField := unwrapEntityValue(field).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		fkCol, ok := schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("BelongsTo relation %q: FK column %q not found on schema", rel.Name, rel.ForeignKey)
		}

		fkField := entityValue.FieldByIndex(fkCol.FieldIndex)
		if err := setFieldValue(fkField, pkField.Interface(), schema.TableName, rel.ForeignKey); err != nil {
			return fmt.Errorf("BelongsTo relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func syncForwardAssociations(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState) error {
	for _, rel := range schema.Relationships {
		if err := syncForwardAssociation(cache, q, ctx, schema, entityValue, rel, state); err != nil {
			return err
		}
	}

	return nil
}

func syncForwardAssociation(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, rel *Relationship, state *associationSyncState) error {
	field := entityValue.FieldByIndex(rel.FieldIndex)

	switch rel.Type {
	case HasOne:
		if field.IsZero() {
			return nil
		}

		return syncHasOneAssociation(cache, q, ctx, schema, entityValue, rel, state)
	case HasMany:
		if field.IsNil() {
			return nil
		}

		return syncHasManyAssociation(cache, q, ctx, schema, entityValue, rel, state)
	case ManyToMany:
		if field.IsNil() {
			return nil
		}

		return syncManyToManyAssociation(cache, q, ctx, schema, entityValue, rel, state)
	case BelongsTo:
		return nil
	default:
		return nil
	}
}

func syncHasOneAssociation(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, state *associationSyncState) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	relField := parentValue.FieldByIndex(rel.FieldIndex)

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, relField, state); err != nil {
		return fmt.Errorf("failed to synchronize HasOne relation %q: %w", rel.Name, err)
	}

	currentChildren, err := loadChildrenByForeignKey(cache, q, ctx, nestedSchema, rel.ForeignKey, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load existing HasOne relation %q: %w", rel.Name, err)
	}

	desiredPK := unwrapEntityValue(relField).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex).Interface()
	desiredKey, ok := comparableKey(desiredPK)
	if !ok {
		return fmt.Errorf("HasOne relation %q produced non-comparable primary key type %T", rel.Name, desiredPK)
	}

	for _, current := range currentChildren {
		currentPK := current.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex).Interface()
		currentKey, ok := comparableKey(currentPK)
		if !ok {
			return fmt.Errorf("existing HasOne relation %q produced non-comparable primary key type %T", rel.Name, currentPK)
		}

		if currentKey == desiredKey {
			continue
		}

		if err := deleteEntityGraph(cache, q, ctx, nestedSchema, current, state); err != nil {
			return fmt.Errorf("failed to delete replaced HasOne relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func syncHasManyAssociation(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, state *associationSyncState) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasMany relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	currentChildren, err := loadChildrenByForeignKey(cache, q, ctx, nestedSchema, rel.ForeignKey, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load existing HasMany relation %q: %w", rel.Name, err)
	}

	desiredKeys := make(map[any]struct{})
	sliceField := parentValue.FieldByIndex(rel.FieldIndex)
	for i := range sliceField.Len() {
		elem := sliceField.Index(i)

		if err := setRelatedFK(elem, nestedSchema, rel.ForeignKey, parentPK); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, elem, state); err != nil {
			return fmt.Errorf("failed to synchronize HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pk := unwrapEntityValue(elem).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex).Interface()
		key, ok := comparableKey(pk)
		if !ok {
			return fmt.Errorf("HasMany relation %q[%d] produced non-comparable primary key type %T", rel.Name, i, pk)
		}

		desiredKeys[key] = struct{}{}
	}

	for _, current := range currentChildren {
		currentPK := current.FieldByIndex(nestedSchema.PrimaryKey.FieldIndex).Interface()
		currentKey, ok := comparableKey(currentPK)
		if !ok {
			return fmt.Errorf("existing HasMany relation %q produced non-comparable primary key type %T", rel.Name, currentPK)
		}

		if _, keep := desiredKeys[currentKey]; keep {
			continue
		}

		if err := deleteEntityGraph(cache, q, ctx, nestedSchema, current, state); err != nil {
			return fmt.Errorf("failed to delete orphaned HasMany relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func syncManyToManyAssociation(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, state *associationSyncState) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	sliceField := parentValue.FieldByIndex(rel.FieldIndex)
	desiredKeys := make(map[any]struct{}, sliceField.Len())
	desiredPKs := make([]any, 0, sliceField.Len())

	for i := range sliceField.Len() {
		elem := sliceField.Index(i)

		if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, elem, state); err != nil {
			return fmt.Errorf("failed to synchronize ManyToMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pk := unwrapEntityValue(elem).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex).Interface()
		key, ok := comparableKey(pk)
		if !ok {
			return fmt.Errorf("ManyToMany relation %q[%d] produced non-comparable primary key type %T", rel.Name, i, pk)
		}

		if _, exists := desiredKeys[key]; exists {
			continue
		}

		desiredKeys[key] = struct{}{}
		desiredPKs = append(desiredPKs, pk)
	}

	links, err := loadM2MLinks(cache, q, ctx, parentSchema, nestedSchema, rel, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load existing ManyToMany relation %q: %w", rel.Name, err)
	}

	existingKeys := make(map[any]struct{}, len(links))
	obsoletePKs := make([]any, 0)
	for _, link := range links {
		existingKeys[link.relatedID] = struct{}{}
		if _, keep := desiredKeys[link.relatedID]; !keep {
			obsoletePKs = append(obsoletePKs, link.relatedID)
		}
	}

	if len(obsoletePKs) > 0 {
		if err := deleteM2MLinks(cache, q, ctx, rel, parentSchema, nestedSchema, parentPK, obsoletePKs); err != nil {
			return fmt.Errorf("failed to delete obsolete ManyToMany links for relation %q: %w", rel.Name, err)
		}
	}

	missingPKs := make([]any, 0)
	for _, desiredPK := range desiredPKs {
		key, _ := comparableKey(desiredPK)
		if _, exists := existingKeys[key]; exists {
			continue
		}

		missingPKs = append(missingPKs, desiredPK)
	}

	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, nestedSchema)
	if err := insertJoinTableRows(q, ctx, rel.JoinTable, parentColName, relatedColName, parentPK, missingPKs); err != nil {
		return fmt.Errorf("failed to insert ManyToMany links for relation %q: %w", rel.Name, err)
	}

	return nil
}

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
	nestedSchema, err := rel.resolveRelationSchema()
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
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany delete relation %q: %w", rel.Name, err)
	}

	if err := deleteAllM2MLinks(cache, q, ctx, rel, schema, nestedSchema, parentPK); err != nil {
		return fmt.Errorf("failed to delete ManyToMany links for relation %q: %w", rel.Name, err)
	}

	return nil
}

func unwrapEntityValue(entityValue reflect.Value) reflect.Value {
	for entityValue.IsValid() && entityValue.Kind() == reflect.Ptr {
		if entityValue.IsNil() {
			return reflect.Value{}
		}

		entityValue = entityValue.Elem()
	}

	return entityValue
}

func associationSyncKey(schema *EntitySchema, entityValue reflect.Value) (string, bool) {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		pk := pkField.Interface()
		key, ok := comparableKey(pk)
		if !ok {
			return "", false
		}

		return fmt.Sprintf("%s:%v", schema.TableName, key), true
	}

	if entityValue.CanAddr() {
		return fmt.Sprintf("%s:new:%x", schema.TableName, entityValue.Addr().Pointer()), true
	}

	return "", false
}

func setEntityPrimaryKey(schema *EntitySchema, entityValue reflect.Value, pk any) error {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)

	return setFieldValue(pkField, pk, schema.TableName, schema.PrimaryKey.Name)
}

func updateStoredEntity(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	now := time.Now()
	setUpdateTimestamps(entityValue, schema, now)
	setMap, pkValue := buildUpdateSetMap(entityValue, schema)

	sqler := sqlc.Update(schema.TableName).
		SetMap(setMap).
		Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	_, result, err := cache.Exec(ctx, sqler, q)
	if err != nil {
		return fmt.Errorf("failed to update entity %s: %w", schema.TableName, err)
	}

	if err := errNoRowsAffected(result, fmt.Errorf("entity %s id=%v: %w", schema.TableName, pkValue, ErrNotFound)); err != nil {
		return err
	}

	return nil
}

func loadChildrenByForeignKey(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, foreignKey string, parentPK any) ([]reflect.Value, error) {
	qb := sqlc.From(schema.TableName).Where(sqlc.Col(schema.TableName, foreignKey).Eq(parentPK))

	return querySchemaEntities(cache, q, ctx, qb, schema)
}

func querySchemaEntities(cache *statementCache, q sqlc.Querier, ctx context.Context, sqler sqlc.Sqler, schema *EntitySchema) ([]reflect.Value, error) {
	rows, err := cache.Query(ctx, sqler, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // safe to ignore in defer

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get query columns for %s: %w", schema.TableName, err)
	}

	entities, err := hydrateRows(rows, columns, schema, schema.entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate rows for %s: %w", schema.TableName, err)
	}

	return entities, nil
}

func loadM2MLinks(cache *statementCache, q sqlc.Querier, ctx context.Context, parentSchema *EntitySchema, relSchema *EntitySchema, rel *Relationship, parentPK any) ([]m2mLink, error) {
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)
	sqler := sqlc.From(rel.JoinTable).
		Where(sqlc.Col(rel.JoinTable, parentColName).Eq(parentPK))

	joinRows, err := cache.Query(ctx, sqler, q)
	if err != nil {
		return nil, fmt.Errorf("failed to query join table %s: %w", rel.JoinTable, err)
	}
	defer joinRows.Close() //nolint:errcheck // safe to ignore in defer

	joinColumns, err := joinRows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get join table columns: %w", err)
	}

	parentColIdx, relatedColIdx := findJoinColumnIndices(joinColumns, parentColName, relatedColName)
	if parentColIdx < 0 || relatedColIdx < 0 {
		return nil, fmt.Errorf("join table %q missing expected columns %q or %q", rel.JoinTable, parentColName, relatedColName)
	}

	links, _, err := scanJoinTableRows(joinRows, joinColumns, parentColIdx, relatedColIdx, parentSchema, relSchema, rel)
	if err != nil {
		return nil, err
	}

	return links, nil
}

func deleteM2MLinks(cache *statementCache, q sqlc.Querier, ctx context.Context, rel *Relationship, parentSchema *EntitySchema, relSchema *EntitySchema, parentPK any, relatedPKs []any) error {
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)

	for _, relatedPK := range relatedPKs {
		sqler := sqlc.Delete(rel.JoinTable).
			Where(sqlc.Col(parentColName).Eq(parentPK)).
			Where(sqlc.Col(relatedColName).Eq(relatedPK))

		if _, _, err := cache.Exec(ctx, sqler, q); err != nil {
			return fmt.Errorf("failed to delete join table row from %s: %w", rel.JoinTable, err)
		}
	}

	return nil
}

func deleteAllM2MLinks(cache *statementCache, q sqlc.Querier, ctx context.Context, rel *Relationship, parentSchema *EntitySchema, relSchema *EntitySchema, parentPK any) error {
	parentColName, _ := resolveM2MColumnNames(rel, parentSchema, relSchema)
	sqler := sqlc.Delete(rel.JoinTable).Where(sqlc.Col(parentColName).Eq(parentPK))

	if _, _, err := cache.Exec(ctx, sqler, q); err != nil {
		return fmt.Errorf("failed to delete join rows from %s: %w", rel.JoinTable, err)
	}

	return nil
}

func deleteStoredEntity(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, pkValue any) error {
	sqler := sqlc.Delete(schema.TableName).Where(sqlc.Col(schema.PrimaryKey.Name).Eq(pkValue))

	if _, _, err := cache.Exec(ctx, sqler, q); err != nil {
		return fmt.Errorf("failed to delete row from %s: %w", schema.TableName, err)
	}

	return nil
}

// insertRelatedEntity builds and executes an INSERT for a related entity using its
// schema. It sets autoCreateTime and autoUpdateTime fields, skips auto-increment
// PKs in the INSERT columns, and returns the generated primary key value.
//
// The entityValue must be an addressable reflect.Value of the related struct.
func insertRelatedEntity(q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	now := time.Now()

	// Set auto-timestamp fields and collect INSERT values.
	insertCols := schema.InsertColumns()
	setCreateTimestamps(entityValue, schema, now)
	vals := buildInsertValues(entityValue, schema)

	sqler := sqlc.Into(schema.TableName).
		Columns(insertCols...).
		Values(vals...)

	sql, args, err := sqler.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build INSERT SQL for %s: %w", schema.TableName, err)
	}

	result, err := q.Exec(ctx, sql, args...)
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
func insertJoinTableRows(q sqlc.Querier, ctx context.Context, joinTable, parentCol, relatedCol string, parentPK any, relatedPKs []any) error {
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

	if _, err := q.Exec(ctx, sql, args...); err != nil {
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

	return setFieldValue(fkField, parentPK, schema.TableName, fkColName)
}

func setFieldValue(field reflect.Value, value any, tableName string, columnName string) error {
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
		return setPointerFieldValue(field, valueRef, tableName, columnName)
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

	field.Set(valueRef.Convert(field.Type()))

	return nil
}

func setPointerFieldValue(field reflect.Value, valueRef reflect.Value, tableName string, columnName string) error {
	if valueRef.Kind() == reflect.Ptr {
		if valueRef.IsNil() {
			field.Set(reflect.Zero(field.Type()))

			return nil
		}

		if valueRef.Type().ConvertibleTo(field.Type()) {
			field.Set(valueRef.Convert(field.Type()))

			return nil
		}

		valueRef = valueRef.Elem()
	}

	if !valueRef.Type().ConvertibleTo(field.Type().Elem()) {
		return fmt.Errorf("value type %s is not convertible to field type %s for %s.%s", valueRef.Type(), field.Type(), tableName, columnName)
	}

	ptr := reflect.New(field.Type().Elem())
	ptr.Elem().Set(valueRef.Convert(field.Type().Elem()))
	field.Set(ptr)

	return nil
}
