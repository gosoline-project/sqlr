package sqlr

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gosoline-project/sqlc"
)

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
		return nil, nil, fmt.Errorf("failed to get join table columns for preload relation %q: %w", relationPath, err)
	}

	parentColIdx, relatedColIdx := findJoinColumnIndices(joinColumns, parentColName, relatedColName)
	if parentColIdx < 0 || relatedColIdx < 0 {
		return nil, nil, fmt.Errorf("join table %q missing expected columns %q or %q for preload relation %q", rel.JoinTable, parentColName, relatedColName, relationPath)
	}

	links, relatedIDs, err := scanJoinTableRows(joinRows, joinColumns, parentColIdx, relatedColIdx, parentSchema, relSchema, rel)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to scan join table rows for preload relation %q: %w", relationPath, err)
	}

	return links, relatedIDs, nil
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
