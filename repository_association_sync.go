package sqlr

import (
	"context"
	"fmt"
	"reflect"
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

func (c *associationSyncContext) syncExistingEntityGraph(ctx context.Context, schema *EntitySchema, entityValue reflect.Value) error {
	_, err := c.syncEntityGraph(ctx, schema, entityValue, "")

	return err
}

func (c *associationSyncContext) syncEntityGraph(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, path string) (any, error) {
	entityValue = unwrapEntityValue(entityValue)
	if !entityValue.IsValid() {
		return nil, fmt.Errorf("invalid entity value for schema %s", schema.TableName)
	}

	if c.state == nil {
		c.state = newAssociationSyncState()
	}

	key, hasKey := associationSyncKey(schema, entityValue)
	if hasKey {
		if !c.state.enter(key) {
			return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
		}

		defer c.state.leave(key)
	}

	if err := c.syncBelongsToAssociations(ctx, schema, entityValue, path); err != nil {
		return nil, err
	}

	pk, err := c.persistEntityGraphNode(ctx, schema, entityValue)
	if err != nil {
		return nil, err
	}

	if err := c.syncForwardAssociations(ctx, schema, entityValue, path); err != nil {
		return nil, err
	}

	return pk, nil
}

func (c *associationSyncContext) persistEntityGraphNode(ctx context.Context, schema *EntitySchema, entityValue reflect.Value) (any, error) {
	pkField := entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex)
	if !pkField.IsZero() {
		if err := c.updateStoredEntity(ctx, schema, entityValue); err != nil {
			return nil, err
		}

		return pkField.Interface(), nil
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

	return entityValue.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface(), nil
}

func (c *associationSyncContext) syncBelongsToAssociations(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, parentPath string) error {
	for _, rel := range schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		relationPath := joinAssociationPath(parentPath, rel.Name)
		if c.policy != nil && !c.policy.shouldSyncPath(relationPath) {
			continue
		}

		field := entityValue.FieldByIndex(rel.FieldIndex)
		field = unwrapEntityValue(field)
		if !field.IsValid() || field.IsZero() {
			continue
		}

		nestedSchema, err := rel.ResolveRelatedSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for BelongsTo relation %q: %w", rel.Name, err)
		}

		if _, err := c.syncEntityGraph(ctx, nestedSchema, field, relationPath); err != nil {
			return fmt.Errorf("failed to synchronize BelongsTo relation %q: %w", rel.Name, err)
		}

		pkField := unwrapEntityValue(field).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)

		fkCol, ok := schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("BelongsTo relation %q: FK column %q not found on schema", rel.Name, rel.ForeignKey)
		}

		fkField := entityValue.FieldByIndex(fkCol.FieldIndex)
		var setErr error
		if c.mutationOptions.autoUpdatesDisabled() {
			setErr = reconcileExistingFieldValue(fkField, pkField.Interface(), schema.TableName, rel.ForeignKey, c.journal)
		} else {
			setErr = setFieldValue(fkField, pkField.Interface(), schema.TableName, rel.ForeignKey, c.journal)
		}

		if setErr != nil {
			return fmt.Errorf("BelongsTo relation %q: %w", rel.Name, setErr)
		}
	}

	return nil
}

func (c *associationSyncContext) syncForwardAssociations(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, parentPath string) error {
	for _, rel := range schema.Relationships {
		if err := c.syncForwardAssociation(ctx, schema, entityValue, rel, parentPath); err != nil {
			return err
		}
	}

	return nil
}

func (c *associationSyncContext) syncForwardAssociation(ctx context.Context, schema *EntitySchema, entityValue reflect.Value, rel *Relationship, parentPath string) error {
	relationPath := joinAssociationPath(parentPath, rel.Name)
	if c.policy != nil && !c.policy.shouldSyncPath(relationPath) {
		return nil
	}

	field := entityValue.FieldByIndex(rel.FieldIndex)

	switch rel.Type {
	case HasOne:
		if field.IsZero() {
			return nil
		}

		return c.syncHasOneAssociation(ctx, schema, entityValue, rel, relationPath)
	case HasMany:
		if field.IsNil() {
			return nil
		}

		return c.syncHasManyAssociation(ctx, schema, entityValue, rel, relationPath)
	case ManyToMany:
		if field.IsNil() {
			return nil
		}

		return c.syncManyToManyAssociation(ctx, schema, entityValue, rel, relationPath)
	case BelongsTo:
		return nil
	default:
		return nil
	}
}

func (c *associationSyncContext) syncHasOneAssociation(ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasOne relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	relField := parentValue.FieldByIndex(rel.FieldIndex)
	relField = unwrapEntityValue(relField)
	if !relField.IsValid() {
		return fmt.Errorf("HasOne relation %q is nil", rel.Name)
	}

	if err := setRelatedFK(relField, nestedSchema, rel.ForeignKey, parentPK, c.journal, c.mutationOptions); err != nil {
		return fmt.Errorf("HasOne relation %q: %w", rel.Name, err)
	}

	if _, err := c.syncEntityGraph(ctx, nestedSchema, relField, relationPath); err != nil {
		return fmt.Errorf("failed to synchronize HasOne relation %q: %w", rel.Name, err)
	}

	currentChildren, err := c.loadChildrenByForeignKey(ctx, nestedSchema, rel.ForeignKey, parentPK)
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

		if err := c.deleteEntityGraph(ctx, nestedSchema, current); err != nil {
			return fmt.Errorf("failed to delete replaced HasOne relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func (c *associationSyncContext) syncHasManyAssociation(ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for HasMany relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	currentChildren, err := c.loadChildrenByForeignKey(ctx, nestedSchema, rel.ForeignKey, parentPK)
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

		if err := setRelatedFK(elem, nestedSchema, rel.ForeignKey, parentPK, c.journal, c.mutationOptions); err != nil {
			return fmt.Errorf("HasMany relation %q[%d]: %w", rel.Name, i, err)
		}

		if _, err := c.syncEntityGraph(ctx, nestedSchema, elem, relationPath); err != nil {
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

		if err := c.deleteEntityGraph(ctx, nestedSchema, current); err != nil {
			return fmt.Errorf("failed to delete orphaned HasMany relation %q: %w", rel.Name, err)
		}
	}

	return nil
}

func (c *associationSyncContext) syncManyToManyAssociation(ctx context.Context, parentSchema *EntitySchema, parentValue reflect.Value, rel *Relationship, relationPath string) error {
	nestedSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for ManyToMany relation %q: %w", rel.Name, err)
	}

	parentPK := parentValue.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
	fullEntitySync := c.policy != nil && c.policy.shouldFullSyncMany2manyPath(relationPath)
	desiredKeys, desiredPKs, err := c.collectDesiredManyToManyTargets(ctx, nestedSchema, parentValue.FieldByIndex(rel.FieldIndex), rel, relationPath, fullEntitySync)
	if err != nil {
		return err
	}

	links, err := c.loadM2MLinks(ctx, parentSchema, nestedSchema, rel, parentPK)
	if err != nil {
		return fmt.Errorf("failed to load existing ManyToMany relation %q: %w", rel.Name, err)
	}

	existingKeys, obsoletePKs := partitionManyToManyLinks(links, desiredKeys)

	if len(obsoletePKs) > 0 {
		if err := c.deleteM2MLinks(ctx, rel, parentSchema, nestedSchema, parentPK, obsoletePKs); err != nil {
			return fmt.Errorf("failed to delete obsolete ManyToMany links for relation %q: %w", rel.Name, err)
		}
	}

	missingPKs := collectMissingManyToManyPKs(desiredPKs, existingKeys)

	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, nestedSchema)
	if err := c.insertJoinTableRows(ctx, rel.JoinTable, parentColName, relatedColName, parentPK, missingPKs); err != nil {
		return fmt.Errorf("failed to insert ManyToMany links for relation %q: %w", rel.Name, err)
	}

	return nil
}

func (c *associationSyncContext) collectDesiredManyToManyTargets(
	ctx context.Context,
	nestedSchema *EntitySchema,
	sliceField reflect.Value,
	rel *Relationship,
	relationPath string,
	fullEntitySync bool,
) (desiredKeys map[any]struct{}, desiredPKs []any, err error) {
	desiredKeys = make(map[any]struct{}, sliceField.Len())
	desiredPKs = make([]any, 0, sliceField.Len())

	for i := range sliceField.Len() {
		elem := unwrapEntityValue(sliceField.Index(i))
		if !elem.IsValid() {
			return nil, nil, fmt.Errorf("ManyToMany relation %q[%d] is nil", rel.Name, i)
		}

		pkField := unwrapEntityValue(elem).FieldByIndex(nestedSchema.PrimaryKey.FieldIndex)
		if pkField.IsZero() {
			if _, err := c.syncEntityGraph(ctx, nestedSchema, elem, relationPath); err != nil {
				return nil, nil, fmt.Errorf("failed to synchronize ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}
		} else if fullEntitySync {
			if _, err := c.syncEntityGraph(ctx, nestedSchema, elem, relationPath); err != nil {
				return nil, nil, fmt.Errorf("failed to synchronize ManyToMany relation %q[%d]: %w", rel.Name, i, err)
			}
		} else if err := c.ensureEntityExists(ctx, nestedSchema, pkField.Interface()); err != nil {
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
