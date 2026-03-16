package sqlr

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gosoline-project/sqlc"
)

type associationSyncState struct {
	active map[string]struct{}
}

func newAssociationSyncState() *associationSyncState {
	return &associationSyncState{active: make(map[string]struct{})}
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

func syncExistingEntityGraph(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState, policy *associationSyncPolicy) error {
	_, err := syncEntityGraph(cache, q, ctx, schema, entityValue, state, policy, "")

	return err
}

func syncEntityGraph(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState, policy *associationSyncPolicy, path string) (any, error) {
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

	if err := syncBelongsToAssociations(cache, q, ctx, schema, entityValue, state, policy, path); err != nil {
		return nil, err
	}

	pk, err := persistEntityGraphNode(cache, q, ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	if err := syncForwardAssociations(cache, q, ctx, schema, entityValue, state, policy, path); err != nil {
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

func syncBelongsToAssociations(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState, policy *associationSyncPolicy, parentPath string) error {
	for _, rel := range schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		relationPath := joinAssociationPath(parentPath, rel.Name)
		if policy != nil && !policy.shouldSyncPath(relationPath) {
			continue
		}

		field := entityValue.FieldByIndex(rel.FieldIndex)
		field = unwrapEntityValue(field)
		if !field.IsValid() || field.IsZero() {
			continue
		}

		nestedSchema, err := rel.resolveRelationSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for BelongsTo relation %q: %w", rel.Name, err)
		}

		if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, field, state, policy, relationPath); err != nil {
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

func syncForwardAssociations(cache *statementCache, q sqlc.Querier, ctx context.Context, schema *EntitySchema, entityValue reflect.Value, state *associationSyncState, policy *associationSyncPolicy, parentPath string) error {
	for _, rel := range schema.Relationships {
		if err := syncForwardAssociation(cache, q, ctx, schema, entityValue, rel, state, policy, parentPath); err != nil {
			return err
		}
	}

	return nil
}

func syncForwardAssociation(
	cache *statementCache,
	q sqlc.Querier,
	ctx context.Context,
	schema *EntitySchema,
	entityValue reflect.Value,
	rel *Relationship,
	state *associationSyncState,
	policy *associationSyncPolicy,
	parentPath string,
) error {
	relationPath := joinAssociationPath(parentPath, rel.Name)
	if policy != nil && !policy.shouldSyncPath(relationPath) {
		return nil
	}

	field := entityValue.FieldByIndex(rel.FieldIndex)

	switch rel.Type {
	case HasOne:
		if field.IsZero() {
			return nil
		}

		return syncHasOneAssociation(cache, q, ctx, schema, entityValue, rel, state, policy, relationPath)
	case HasMany:
		if field.IsNil() {
			return nil
		}

		return syncHasManyAssociation(cache, q, ctx, schema, entityValue, rel, state, policy, relationPath)
	case ManyToMany:
		if field.IsNil() {
			return nil
		}

		return syncManyToManyAssociation(cache, q, ctx, schema, entityValue, rel, state, policy, relationPath)
	case BelongsTo:
		return nil
	default:
		return nil
	}
}

func syncHasOneAssociation(
	cache *statementCache,
	q sqlc.Querier,
	ctx context.Context,
	parentSchema *EntitySchema,
	parentValue reflect.Value,
	rel *Relationship,
	state *associationSyncState,
	policy *associationSyncPolicy,
	relationPath string,
) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	relField := parentValue.FieldByIndex(rel.FieldIndex)
	relField = unwrapEntityValue(relField)
	if !relField.IsValid() {
		return fmt.Errorf("HasOne relation %q is nil", rel.Name)
	}

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, relField, state, policy, relationPath); err != nil {
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

func syncHasManyAssociation(
	cache *statementCache,
	q sqlc.Querier,
	ctx context.Context,
	parentSchema *EntitySchema,
	parentValue reflect.Value,
	rel *Relationship,
	state *associationSyncState,
	policy *associationSyncPolicy,
	relationPath string,
) error {
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
		elem := unwrapEntityValue(sliceField.Index(i))
		if !elem.IsValid() {
			return fmt.Errorf("HasMany relation %q[%d] is nil", rel.Name, i)
		}

		if err := setRelatedFK(elem, nestedSchema, rel.ForeignKey, parentPK); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, elem, state, policy, relationPath); err != nil {
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

func syncManyToManyAssociation(
	cache *statementCache,
	q sqlc.Querier,
	ctx context.Context,
	parentSchema *EntitySchema,
	parentValue reflect.Value,
	rel *Relationship,
	state *associationSyncState,
	policy *associationSyncPolicy,
	relationPath string,
) error {
	nestedSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	desiredKeys, desiredPKs, err := collectDesiredManyToManyTargets(cache, q, ctx, nestedSchema, parentValue.FieldByIndex(rel.FieldIndex), rel, state, policy, relationPath)
	if err != nil {
		return err
	}

	links, err := loadM2MLinks(cache, q, ctx, parentSchema, nestedSchema, rel, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load existing ManyToMany relation %q: %w", rel.Name, err)
	}

	existingKeys, obsoletePKs := partitionManyToManyLinks(links, desiredKeys)

	if len(obsoletePKs) > 0 {
		if err := deleteM2MLinks(cache, q, ctx, rel, parentSchema, nestedSchema, parentPK, obsoletePKs); err != nil {
			return fmt.Errorf("failed to delete obsolete ManyToMany links for relation %q: %w", rel.Name, err)
		}
	}

	missingPKs := collectMissingManyToManyPKs(desiredPKs, existingKeys)

	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, nestedSchema)
	if err := insertJoinTableRows(q, ctx, rel.JoinTable, parentColName, relatedColName, parentPK, missingPKs); err != nil {
		return fmt.Errorf("failed to insert ManyToMany links for relation %q: %w", rel.Name, err)
	}

	return nil
}

func collectDesiredManyToManyTargets(
	cache *statementCache,
	q sqlc.Querier,
	ctx context.Context,
	nestedSchema *EntitySchema,
	sliceField reflect.Value,
	rel *Relationship,
	state *associationSyncState,
	policy *associationSyncPolicy,
	relationPath string,
) (desiredKeys map[any]struct{}, desiredPKs []any, err error) {
	desiredKeys = make(map[any]struct{}, sliceField.Len())
	desiredPKs = make([]any, 0, sliceField.Len())

	for i := range sliceField.Len() {
		elem := unwrapEntityValue(sliceField.Index(i))
		if !elem.IsValid() {
			return nil, nil, fmt.Errorf("ManyToMany relation %q[%d] is nil", rel.Name, i)
		}

		if _, err := syncEntityGraph(cache, q, ctx, nestedSchema, elem, state, policy, relationPath); err != nil {
			return nil, nil, fmt.Errorf("failed to synchronize ManyToMany relation %q[%d]: %w", rel.Name, i, err)
		}

		pk := unwrapEntityValue(elem).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex).Interface()
		key, ok := comparableKey(pk)
		if !ok {
			return nil, nil, fmt.Errorf("ManyToMany relation %q[%d] produced non-comparable primary key type %T", rel.Name, i, pk)
		}

		if _, exists := desiredKeys[key]; exists {
			continue
		}

		desiredKeys[key] = struct{}{}
		desiredPKs = append(desiredPKs, pk)
	}

	return desiredKeys, desiredPKs, nil
}

func partitionManyToManyLinks(links []m2mLink, desiredKeys map[any]struct{}) (existingKeys map[any]struct{}, obsoletePKs []any) {
	existingKeys = make(map[any]struct{}, len(links))
	obsoletePKs = make([]any, 0)

	for _, link := range links {
		existingKeys[link.relatedID] = struct{}{}
		if _, keep := desiredKeys[link.relatedID]; !keep {
			obsoletePKs = append(obsoletePKs, link.relatedID)
		}
	}

	return existingKeys, obsoletePKs
}

func collectMissingManyToManyPKs(desiredPKs []any, existingKeys map[any]struct{}) []any {
	missingPKs := make([]any, 0)

	for _, desiredPK := range desiredPKs {
		key, _ := comparableKey(desiredPK)
		if _, exists := existingKeys[key]; exists {
			continue
		}

		missingPKs = append(missingPKs, desiredPK)
	}

	return missingPKs
}
