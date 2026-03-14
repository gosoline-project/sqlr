package sqlr

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gosoline-project/sqlc"
	"golang.org/x/sync/errgroup"
)

// executePreloads executes preload queries for each preloaded relation. For each
// relation, it collects the parent entity primary key values, executes a SELECT
// on the related table with a WHERE foreignKey IN (...) clause, and assigns the
// results back to the parent entities.
//
// Preloads are normalized and then executed sequentially. This keeps execution
// deterministic at the declaration level while still allowing safe parallelism
// for independent sibling branches. Overlapping relation paths such as duplicate
// "Posts" preloads or sibling nested paths like "Posts.Comments" and
// "Posts.Tags" are collapsed into a tree so each prefix relation is loaded once
// before its child branches continue.
func (r *repositoryCommon[K, E]) executePreloads(q sqlc.Querier, ctx context.Context, preloads []preloadEntry, results []E) error {
	normalizedPreloads := normalizePreloads(preloads)

	if len(normalizedPreloads) == 0 {
		return nil
	}

	parents := make([]reflect.Value, 0, len(results))
	for i := range results {
		parents = append(parents, reflect.ValueOf(&results[i]).Elem())
	}

	return r.executePreloadNodes(q, ctx, buildPreloadTree(normalizedPreloads), parents, r.schema, "")
}

// normalizePreloads removes exact duplicate relation paths and sorts the result
// by ascending depth while preserving the caller's order within the same depth.
// Keeping the first entry preserves explicit preload conditions when duplicates
// are introduced through repeated calls or auto-preload merging.
func normalizePreloads(preloads []preloadEntry) []preloadEntry {
	if len(preloads) == 0 {
		return nil
	}

	normalized := make([]preloadEntry, 0, len(preloads))
	seen := make(map[string]struct{}, len(preloads))

	for _, preload := range preloads {
		if _, exists := seen[preload.relation]; exists {
			continue
		}

		seen[preload.relation] = struct{}{}
		normalized = append(normalized, preload)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return preloadDepth(normalized[i].relation) < preloadDepth(normalized[j].relation)
	})

	return normalized
}

type preloadNode struct {
	relation string
	where    []*sqlc.SqlerWhere
	children []*preloadNode
}

func buildPreloadTree(preloads []preloadEntry) []*preloadNode {
	if len(preloads) == 0 {
		return nil
	}

	rootNodes := make([]*preloadNode, 0)
	rootIndex := make(map[string]*preloadNode, len(preloads))

	for _, preload := range preloads {
		segments := strings.Split(preload.relation, ".")
		children := &rootNodes
		index := rootIndex

		for i, segment := range segments {
			node, ok := index[segment]
			if !ok {
				node = &preloadNode{relation: segment}
				index[segment] = node
				*children = append(*children, node)
			}

			if i == len(segments)-1 {
				node.where = preload.where

				continue
			}

			children = &node.children
			index = preloadNodeIndex(node.children)
		}
	}

	return rootNodes
}

func preloadNodeIndex(nodes []*preloadNode) map[string]*preloadNode {
	index := make(map[string]*preloadNode, len(nodes))
	for _, node := range nodes {
		index[node.relation] = node
	}

	return index
}

func preloadDepth(relation string) int {
	return strings.Count(relation, ".") + 1
}

// applyPreloadConditions applies additional WHERE conditions to a sqlc SelectQueryBuilder.
// It takes a builder and a list of SqlerWhere conditions, and returns the modified builder
// with all non-empty conditions applied via .Where() calls.
func applyPreloadConditions(qb *sqlc.SelectQueryBuilder, where []*sqlc.SqlerWhere) (*sqlc.SelectQueryBuilder, error) {
	for _, w := range where {
		if w == nil {
			continue
		}

		if w.IsEmpty() {
			continue
		}

		sql, wArgs, err := w.ToSql()
		if err != nil {
			return nil, fmt.Errorf("failed to build preload condition: %w", err)
		}

		qb = qb.Where(sql, wArgs...)
	}

	return qb, nil
}

func (r *repositoryCommon[K, E]) executePreloadNodes(
	q sqlc.Querier,
	ctx context.Context,
	nodes []*preloadNode,
	parents []reflect.Value,
	parentSchema *EntitySchema,
	pathPrefix string,
) error {
	if len(nodes) == 0 || len(parents) == 0 {
		return nil
	}

	if _, ok := q.(sqlc.Tx); ok {
		for _, node := range nodes {
			if err := r.executePreloadNode(q, ctx, node, parents, parentSchema, pathPrefix); err != nil {
				return err
			}
		}

		return nil
	}

	if len(nodes) == 1 {
		return r.executePreloadNode(q, ctx, nodes[0], parents, parentSchema, pathPrefix)
	}

	g, gCtx := errgroup.WithContext(ctx)
	for _, node := range nodes {
		node := node
		g.Go(func() error {
			return r.executePreloadNode(q, gCtx, node, parents, parentSchema, pathPrefix)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (r *repositoryCommon[K, E]) executePreloadNode(
	q sqlc.Querier,
	ctx context.Context,
	node *preloadNode,
	parents []reflect.Value,
	parentSchema *EntitySchema,
	pathPrefix string,
) error {
	if node == nil || len(parents) == 0 {
		return nil
	}

	relationPath := node.relation
	if pathPrefix != "" {
		relationPath = pathPrefix + "." + node.relation
	}

	rel, ok := parentSchema.Relationships[node.relation]
	if !ok {
		return fmt.Errorf("preload relation %q not found", relationPath)
	}

	relSchema, err := rel.resolveRelationSchema()
	if err != nil {
		return fmt.Errorf("failed to resolve schema for preload relation %q: %w", relationPath, err)
	}

	// Reset the relation field before assignment so mixed explicit/nested preloads
	// for the same prefix do not append duplicate entities.
	for _, parent := range parents {
		relField := parent.FieldByIndex(rel.FieldIndex)
		relField.Set(reflect.Zero(relField.Type()))
	}

	if rel.Type == ManyToMany {
		if err := r.executeM2MPreload(q, ctx, relationPath, rel, parentSchema, relSchema, parents, node.where); err != nil {
			return err
		}
	} else {
		if err := r.executeDirectPreload(q, ctx, relationPath, rel, parentSchema, relSchema, parents, node.where); err != nil {
			return err
		}
	}

	if len(node.children) == 0 {
		return nil
	}

	nextParents := collectAssignedRelated(parents, rel)

	return r.executePreloadNodes(q, ctx, node.children, nextParents, relSchema, relationPath)
}

func collectAssignedRelated(parents []reflect.Value, rel *Relationship) []reflect.Value {
	related := make([]reflect.Value, 0)
	for _, parent := range parents {
		relField := parent.FieldByIndex(rel.FieldIndex)
		related = append(related, collectAssignedRelatedValues(relField)...)
	}

	return related
}

func (r *repositoryCommon[K, E]) executeDirectPreload(
	q sqlc.Querier,
	ctx context.Context,
	relationPath string,
	rel *Relationship,
	parentSchema *EntitySchema,
	relSchema *EntitySchema,
	parents []reflect.Value,
	where []*sqlc.SqlerWhere,
) error {
	var err error
	var qb *sqlc.SelectQueryBuilder
	var entities []reflect.Value
	var columns []string
	var fkFieldIndex []int
	var relatedByFK map[any][]reflect.Value

	if rel.Type == BelongsTo {
		return r.executeBelongsToPreload(q, ctx, relationPath, rel, parentSchema, relSchema, parents, where)
	}

	// Collect parent primary key values.
	pkValues := collectFieldValues(parents, parentSchema.PrimaryKey.FieldIndex)

	// Build query using sqlc builder API.
	qb = sqlc.From(relSchema.TableName).Where(sqlc.Col(relSchema.TableName, rel.ForeignKey).In(pkValues...))
	if qb, err = applyPreloadConditions(qb, where); err != nil {
		return err
	}

	if entities, columns, err = r.queryAndHydratePreload(ctx, qb, relSchema, rel, relationPath, q); err != nil {
		return err
	}

	if fkFieldIndex, err = resolveFKFieldIndex(columns, rel.ForeignKey, relSchema, relationPath); err != nil {
		return err
	}

	if relatedByFK, err = groupEntitiesByField(entities, fkFieldIndex, relSchema.TableName, rel.ForeignKey); err != nil {
		return err
	}

	return assignRelatedToParentsByPK(parents, parentSchema, rel, relatedByFK)
}

func (r *repositoryCommon[K, E]) executeBelongsToPreload(
	q sqlc.Querier,
	ctx context.Context,
	relationPath string,
	rel *Relationship,
	parentSchema *EntitySchema,
	relSchema *EntitySchema,
	parents []reflect.Value,
	where []*sqlc.SqlerWhere,
) error {
	var err error
	var ok bool
	var parentFKColumn *ColumnInfo
	var parentFKValues []any
	var qb *sqlc.SelectQueryBuilder
	var entities []reflect.Value
	var relatedByPK map[any]reflect.Value

	parentFKColumn, ok = parentSchema.ColumnByName(rel.ForeignKey)
	if !ok {
		return fmt.Errorf("failed to map preload foreign key column %q for relation %q: column %q not mapped in schema for %s", rel.ForeignKey, relationPath, rel.ForeignKey, parentSchema.entityType.Name())
	}

	if parentFKValues, err = collectUniqueFKValues(parents, parentFKColumn.FieldIndex, parentSchema.TableName, rel.ForeignKey); err != nil {
		return err
	}

	if len(parentFKValues) == 0 {
		return nil
	}

	// Build query using sqlc builder API.
	qb = sqlc.From(relSchema.TableName).Where(sqlc.Col(relSchema.TableName, relSchema.PrimaryKey.Name).In(parentFKValues...))
	if qb, err = applyPreloadConditions(qb, where); err != nil {
		return err
	}

	if entities, _, err = r.queryAndHydratePreload(ctx, qb, relSchema, rel, relationPath, q); err != nil {
		return err
	}

	if relatedByPK, err = indexEntitiesByPK(entities, relSchema); err != nil {
		return err
	}

	return assignBelongsToRelations(parents, parentFKColumn.FieldIndex, rel, relatedByPK, parentSchema)
}

// executeM2MPreload handles many-to-many preloads by querying the join table first,
// then querying the related table for the matched IDs.
func (r *repositoryCommon[K, E]) executeM2MPreload(
	q sqlc.Querier,
	ctx context.Context,
	relationPath string,
	rel *Relationship,
	parentSchema *EntitySchema,
	relSchema *EntitySchema,
	parents []reflect.Value,
	where []*sqlc.SqlerWhere,
) error {
	var err error
	var sqler *sqlc.SelectQueryBuilder
	var links []m2mLink
	var relatedIDs []any
	var relQB *sqlc.SelectQueryBuilder
	var entities []reflect.Value
	var relatedByID map[any]reflect.Value

	// Derive the join table column names. Use explicit overrides from the
	// relationship tag (JoinParentKey / JoinRelatedKey) when set; otherwise fall
	// back to SchemaNameTransformer so that any custom naming convention is respected.
	parentColName, relatedColName := resolveM2MColumnNames(rel, parentSchema, relSchema)

	pkValues := collectFieldValues(parents, parentSchema.PrimaryKey.FieldIndex)

	sqler = sqlc.From(rel.JoinTable).
		Where(sqlc.Col(rel.JoinTable, parentColName).In(pkValues...))

	if links, relatedIDs, err = r.scanM2MJoinTable(ctx, sqler, parentSchema, relSchema, parentColName, relatedColName, rel, relationPath, q); err != nil {
		return err
	}

	if len(links) == 0 {
		return nil
	}

	// Query the related table for the matched IDs.
	relQB = sqlc.From(relSchema.TableName).Where(sqlc.Col(relSchema.TableName, relSchema.PrimaryKey.Name).In(relatedIDs...))
	if relQB, err = applyPreloadConditions(relQB, where); err != nil {
		return err
	}

	if entities, _, err = r.queryAndHydratePreload(ctx, relQB, relSchema, rel, relationPath, q); err != nil {
		return err
	}

	if relatedByID, err = indexEntitiesByPK(entities, relSchema); err != nil {
		return err
	}

	return assignM2MRelations(parents, parentSchema, rel, links, relatedByID)
}

// resolveM2MColumnNames returns the join table column names for both sides of a
// ManyToMany relationship. When the Relationship has explicit JoinParentKey or
// JoinRelatedKey values (set via parentKey:/relatedKey: tag options), those are
// used directly. Otherwise the names are derived from the entity type names using
// SchemaNameTransformer so that any custom naming convention is respected.
func resolveM2MColumnNames(rel *Relationship, parentSchema, relSchema *EntitySchema) (parentColName, relatedColName string) {
	if rel.JoinParentKey != "" {
		parentColName = rel.JoinParentKey
	} else {
		parentColName = SchemaNameTransformer(parentSchema.entityType.Name()) + "_id"
	}

	if rel.JoinRelatedKey != "" {
		relatedColName = rel.JoinRelatedKey
	} else {
		relatedColName = SchemaNameTransformer(relSchema.entityType.Name()) + "_id"
	}

	return parentColName, relatedColName
}

// m2mLink represents a single row from a many-to-many join table.
type m2mLink struct {
	parentID  any
	relatedID any
}

// scanM2MJoinTable queries the join table and returns the links and unique
// related IDs found.
func (r *repositoryCommon[K, E]) scanM2MJoinTable(
	ctx context.Context,
	sqler *sqlc.SelectQueryBuilder,
	parentSchema *EntitySchema,
	relSchema *EntitySchema,
	parentColName string,
	relatedColName string,
	rel *Relationship,
	relationPath string,
	q sqlc.Querier,
) ([]m2mLink, []any, error) {
	var err error
	var joinRows *sqlc.Rows
	var joinColumns []string

	if joinRows, err = r.statementCache.Query(ctx, sqler, q); err != nil {
		return nil, nil, fmt.Errorf("failed to execute preload query for %q: %w", relationPath, err)
	}
	defer joinRows.Close() //nolint:errcheck // safe to ignore in defer

	if joinColumns, err = joinRows.Columns(); err != nil {
		return nil, nil, fmt.Errorf("failed to get join table columns: %w", err)
	}

	parentColIdx, relatedColIdx := findJoinColumnIndices(joinColumns, parentColName, relatedColName)
	if parentColIdx < 0 || relatedColIdx < 0 {
		return nil, nil, fmt.Errorf("join table %q missing expected columns %q or %q", rel.JoinTable, parentColName, relatedColName)
	}

	return scanJoinTableRows(joinRows, joinColumns, parentColIdx, relatedColIdx, parentSchema, relSchema, rel)
}

// findJoinColumnIndices returns the column indices for the parent and related
// ID columns in the join table result set.
func findJoinColumnIndices(columns []string, parentColName, relatedColName string) (parentIdx int, relatedIdx int) {
	parentIdx = -1
	relatedIdx = -1

	for i, col := range columns {
		if col == parentColName {
			parentIdx = i
		}

		if col == relatedColName {
			relatedIdx = i
		}
	}

	return parentIdx, relatedIdx
}

// scanJoinTableRows scans all rows from a join table query and returns the
// parsed links and unique related IDs.
func scanJoinTableRows(
	joinRows *sqlc.Rows,
	joinColumns []string,
	parentColIdx int,
	relatedColIdx int,
	parentSchema *EntitySchema,
	relSchema *EntitySchema,
	rel *Relationship,
) ([]m2mLink, []any, error) {
	var err error
	var link m2mLink
	var links []m2mLink
	var relatedIDs []any

	relatedIDSet := make(map[any]struct{})
	scanDests := make([]any, len(joinColumns))
	discardScanDests := make([]any, len(joinColumns))

	for i := range discardScanDests {
		discardScanDests[i] = new(any)
	}

	parentScanDest := newScanDestForColumn(parentSchema, parentSchema.PrimaryKey)
	relatedScanDest := newScanDestForColumn(relSchema, relSchema.PrimaryKey)

	for joinRows.Next() {
		copy(scanDests, discardScanDests)
		scanDests[parentColIdx] = parentScanDest
		scanDests[relatedColIdx] = relatedScanDest

		if err = joinRows.Scan(scanDests...); err != nil {
			return nil, nil, fmt.Errorf("failed to scan join table row: %w", err)
		}

		if link, err = parseM2MLink(parentScanDest, relatedScanDest, rel); err != nil {
			return nil, nil, err
		}

		links = append(links, link)
		if _, found := relatedIDSet[link.relatedID]; !found {
			relatedIDSet[link.relatedID] = struct{}{}
			relatedIDs = append(relatedIDs, scannedValue(relatedScanDest))
		}
	}

	if err = joinRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to iterate join table rows: %w", err)
	}

	return links, relatedIDs, nil
}

// parseM2MLink extracts a single m2mLink from the scanned parent and related
// destinations, validating that each value is comparable.
func parseM2MLink(parentScanDest, relatedScanDest any, rel *Relationship) (m2mLink, error) {
	parentID := scannedValue(parentScanDest)
	relatedID := scannedValue(relatedScanDest)

	parentKey, ok := comparableKey(parentID)
	if !ok {
		return m2mLink{}, fmt.Errorf("join table %q parent column produced non-comparable value type %T", rel.JoinTable, parentID)
	}

	relatedKey, ok := comparableKey(relatedID)
	if !ok {
		return m2mLink{}, fmt.Errorf("join table %q related column produced non-comparable value type %T", rel.JoinTable, relatedID)
	}

	return m2mLink{parentID: parentKey, relatedID: relatedKey}, nil
}

// assignM2MRelations assigns related entities to parents based on join table
// links for many-to-many relationships.
func assignM2MRelations(parents []reflect.Value, parentSchema *EntitySchema, rel *Relationship, links []m2mLink, relatedByID map[any]reflect.Value) error {
	linksByParentID := make(map[any][]m2mLink, len(links))
	for _, link := range links {
		linksByParentID[link.parentID] = append(linksByParentID[link.parentID], link)
	}

	for _, parent := range parents {
		pkVal := parent.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
		parentID, ok := comparableKey(pkVal)
		if !ok {
			return fmt.Errorf("parent table %q primary key %q produced non-comparable value type %T", parentSchema.TableName, parentSchema.PrimaryKey.Name, pkVal)
		}

		relField := parent.FieldByIndex(rel.FieldIndex)

		for _, link := range linksByParentID[parentID] {
			if relEntity, found := relatedByID[link.relatedID]; found {
				if err := assignRelated(relField, relEntity); err != nil {
					return fmt.Errorf("failed to assign many-to-many relation %q: %w", rel.Name, err)
				}
			}
		}
	}

	return nil
}

// validatePreloadRelations checks that every preload relation referenced in the
// query exists on the entity's schema. Dotted relation paths (e.g.
// "Posts.Comments") are resolved step by step through nested schemas. Unlike join
// validation, many-to-many associations are allowed because preloads execute
// separate queries and handle them correctly.
func (r *repositoryCommon[K, E]) validatePreloadRelations(preloads []preloadEntry) error {
	var err error
	var relSchema *EntitySchema

	if len(preloads) == 0 {
		return nil
	}

	for _, p := range preloads {
		parts := strings.Split(p.relation, ".")

		currentSchema := r.schema
		for _, part := range parts {
			rel, ok := currentSchema.Relationships[part]
			if !ok {
				return fmt.Errorf("preload relation %q not found on model %s; valid relations: %v", p.relation, currentSchema.entityType.Name(), currentSchema.ValidRelationNames())
			}

			if relSchema, err = rel.resolveRelationSchema(); err != nil {
				return fmt.Errorf("failed to resolve schema for preload relation %q: %w", p.relation, err)
			}

			currentSchema = relSchema
		}
	}

	return nil
}

// collectFieldValues extracts the field value at fieldIndex from each parent and
// returns them as a slice of any.
func collectFieldValues(parents []reflect.Value, fieldIndex []int) []any {
	values := make([]any, 0, len(parents))
	for _, parent := range parents {
		values = append(values, parent.FieldByIndex(fieldIndex).Interface())
	}

	return values
}

// queryAndHydratePreload executes a preload query and hydrates the result rows.
// It returns the hydrated entities and the column names from the result set.
func (r *repositoryCommon[K, E]) queryAndHydratePreload(
	ctx context.Context,
	qb *sqlc.SelectQueryBuilder,
	relSchema *EntitySchema,
	rel *Relationship,
	relationPath string,
	q sqlc.Querier,
) ([]reflect.Value, []string, error) {
	var err error
	var rows *sqlc.Rows
	var columns []string
	var entities []reflect.Value

	if rows, err = r.statementCache.Query(ctx, qb, q); err != nil {
		return nil, nil, fmt.Errorf("failed to execute preload query for %q: %w", relationPath, err)
	}
	defer rows.Close() //nolint:errcheck // safe to ignore in defer

	if columns, err = rows.Columns(); err != nil {
		return nil, nil, fmt.Errorf("failed to get preload columns: %w", err)
	}

	if entities, err = hydrateRows(rows, columns, relSchema, rel.RelatedType); err != nil {
		return nil, nil, fmt.Errorf("failed to hydrate preload rows for %q: %w", relationPath, err)
	}

	return entities, columns, nil
}

// resolveFKFieldIndex finds the foreign key column in the result set columns and
// returns its field index from the schema.
func resolveFKFieldIndex(columns []string, foreignKey string, relSchema *EntitySchema, relationPath string) ([]int, error) {
	found := false
	for _, col := range columns {
		if col == foreignKey {
			found = true

			break
		}
	}

	if !found {
		return nil, fmt.Errorf("foreign key column %q not found in preload result for %q", foreignKey, relationPath)
	}

	fkColumn, ok := relSchema.ColumnByName(foreignKey)
	if !ok {
		return nil, fmt.Errorf("failed to map preload foreign key column %q for relation %q: column %q not mapped in schema for %s", foreignKey, relationPath, foreignKey, relSchema.entityType.Name())
	}

	return fkColumn.FieldIndex, nil
}

// groupEntitiesByField groups entities by the value of a specific field,
// returning a map from field value to the entities sharing that value.
func groupEntitiesByField(entities []reflect.Value, fieldIndex []int, tableName, columnName string) (map[any][]reflect.Value, error) {
	grouped := make(map[any][]reflect.Value)
	for _, entity := range entities {
		val := entity.FieldByIndex(fieldIndex).Interface()
		key, ok := comparableKey(val)
		if !ok {
			if isNilValue(val) {
				continue
			}

			return nil, fmt.Errorf("related table %q column %q produced non-comparable value type %T", tableName, columnName, val)
		}

		grouped[key] = append(grouped[key], entity)
	}

	return grouped, nil
}

// assignRelatedToParentsByPK assigns related entities to parent entities by
// matching each parent's primary key to the foreign key groups.
func assignRelatedToParentsByPK(parents []reflect.Value, parentSchema *EntitySchema, rel *Relationship, relatedByFK map[any][]reflect.Value) error {
	for _, parent := range parents {
		pkVal := parent.FieldByIndex(parentSchema.PrimaryKey.FieldIndex).Interface()
		pkKey, ok := comparableKey(pkVal)
		if !ok {
			return fmt.Errorf("parent table %q primary key %q produced non-comparable value type %T", parentSchema.TableName, parentSchema.PrimaryKey.Name, pkVal)
		}

		relField := parent.FieldByIndex(rel.FieldIndex)
		for _, relEntity := range relatedByFK[pkKey] {
			if err := assignRelated(relField, relEntity); err != nil {
				return fmt.Errorf("failed to assign relation %q by primary key: %w", rel.Name, err)
			}
		}
	}

	return nil
}

// collectUniqueFKValues collects unique, non-zero foreign key values from
// parent entities.
func collectUniqueFKValues(parents []reflect.Value, fkFieldIndex []int, tableName, fkColumnName string) ([]any, error) {
	values := make([]any, 0, len(parents))
	seen := make(map[any]struct{}, len(parents))

	for _, parent := range parents {
		fkField := parent.FieldByIndex(fkFieldIndex)
		fkVal := fkField.Interface()
		fkKey, present, err := optionalComparableKey(fkVal)
		if err != nil {
			return nil, fmt.Errorf("parent table %q foreign key %q produced %w", tableName, fkColumnName, err)
		}

		if !present {
			continue
		}

		if _, found := seen[fkKey]; found {
			continue
		}

		seen[fkKey] = struct{}{}
		values = append(values, fkVal)
	}

	return values, nil
}

// indexEntitiesByPK builds a map from primary key value to entity for quick lookup.
func indexEntitiesByPK(entities []reflect.Value, schema *EntitySchema) (map[any]reflect.Value, error) {
	indexed := make(map[any]reflect.Value, len(entities))
	for _, entity := range entities {
		pkVal := entity.FieldByIndex(schema.PrimaryKey.FieldIndex).Interface()
		pkKey, ok := comparableKey(pkVal)
		if !ok {
			return nil, fmt.Errorf("related table %q primary key %q produced non-comparable value type %T", schema.TableName, schema.PrimaryKey.Name, pkVal)
		}

		indexed[pkKey] = entity
	}

	return indexed, nil
}

// assignBelongsToRelations assigns related entities to parents based on each
// parent's foreign key value matching the related entity's primary key.
func assignBelongsToRelations(parents []reflect.Value, fkFieldIndex []int, rel *Relationship, relatedByPK map[any]reflect.Value, parentSchema *EntitySchema) error {
	for _, parent := range parents {
		fkField := parent.FieldByIndex(fkFieldIndex)
		fkVal := fkField.Interface()
		fkKey, present, err := optionalComparableKey(fkVal)
		if err != nil {
			return fmt.Errorf("parent table %q foreign key %q produced %w", parentSchema.TableName, rel.ForeignKey, err)
		}

		if !present {
			continue
		}

		relField := parent.FieldByIndex(rel.FieldIndex)
		if relEntity, found := relatedByPK[fkKey]; found {
			if err := assignRelated(relField, relEntity); err != nil {
				return fmt.Errorf("failed to assign belongsTo relation %q: %w", rel.Name, err)
			}
		}
	}

	return nil
}
