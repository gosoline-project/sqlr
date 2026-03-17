package sqlr

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gosoline-project/sqlc"
)

type joinHydrationInfo struct {
	rel    *Relationship
	schema *EntitySchema
}

type joinRelationState struct {
	name     string
	joinInfo joinHydrationInfo
	value    reflect.Value
}

type joinColumnScanTarget struct {
	baseFieldIndex     []int
	relationIndex      int
	relationFieldIndex []int
	relationColumnName string
	discard            *any
}

// queryWithJoins executes a query with JOIN clauses. It builds raw SQL with aliased
// columns for joined tables, executes the query, and hydrates the results using
// reflection-based scanning. Joins operate on direct relation names and support
// HasOne, HasMany, and BelongsTo relations.
func (r *repositoryCommon[K, E]) queryWithJoins(q sqlc.Querier, ctx context.Context, qb *QueryBuilderSelect, preloads []preloadEntry) ([]E, error) {
	var err error
	var sqlcQB *sqlc.SelectQueryBuilder
	var joinInfoByName map[string]joinHydrationInfo
	var rows *sqlc.Rows

	sortedJoins := sortJoinEntries(qb.joins)

	if sqlcQB, joinInfoByName, err = r.buildJoinQuery(sortedJoins); err != nil {
		return nil, err
	}

	// Apply WHERE/GROUP BY/HAVING/ORDER BY/LIMIT/OFFSET from QueryBuilderSelect.
	if sqlcQB, err = qb.applyToSqlcBuilder(sqlcQB); err != nil {
		return nil, fmt.Errorf("failed to apply query clauses: %w", err)
	}

	// Execute the raw query.
	if rows, err = r.statementCache.Query(ctx, sqlcQB, q); err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close() //nolint:errcheck // safe to ignore in defer

	results, err := r.hydrateJoinResults(rows, sortedJoins, joinInfoByName)
	if err != nil {
		return nil, err
	}

	if len(preloads) > 0 && len(results) > 0 {
		if err := r.executePreloads(q, ctx, preloads, results); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// sortJoinEntries returns a copy of the join entries sorted alphabetically by
// relation name (matching GORM behavior).
func sortJoinEntries(joins []joinEntry) []joinEntry {
	sorted := make([]joinEntry, len(joins))
	copy(sorted, joins)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].relation < sorted[j].relation
	})

	return sorted
}

// buildJoinQuery constructs the sqlc SelectQueryBuilder with all base columns,
// aliased join columns, and JOIN clauses. It returns the builder and a map of
// join hydration metadata keyed by relation name.
func (r *repositoryCommon[K, E]) buildJoinQuery(sortedJoins []joinEntry) (*sqlc.SelectQueryBuilder, map[string]joinHydrationInfo, error) {
	var err error
	var relSchema *EntitySchema

	sqlcQB := sqlc.From(r.schema.TableName)
	for _, col := range r.schema.QualifiedColumns() {
		sqlcQB = sqlcQB.Column(col)
	}

	joinInfoByName := make(map[string]joinHydrationInfo, len(sortedJoins))

	for _, j := range sortedJoins {
		rel, ok := r.schema.Relationships[j.relation]
		if !ok {
			return nil, nil, fmt.Errorf("join relation %q not found", j.relation)
		}

		if relSchema, err = rel.ResolveRelatedSchema(); err != nil {
			return nil, nil, fmt.Errorf("failed to resolve schema for relation %q: %w", j.relation, err)
		}

		joinInfoByName[j.relation] = joinHydrationInfo{rel: rel, schema: relSchema}

		sqlcQB = addJoinAliasedColumns(sqlcQB, j.relation, relSchema)

		if sqlcQB, err = r.applyJoinClause(sqlcQB, j, rel, relSchema); err != nil {
			return nil, nil, err
		}
	}

	return sqlcQB, joinInfoByName, nil
}

// addJoinAliasedColumns adds aliased columns for a joined relation to the query
// builder: `Posts`.`id` AS `Posts__id`, etc.
func addJoinAliasedColumns(sqlcQB *sqlc.SelectQueryBuilder, relation string, relSchema *EntitySchema) *sqlc.SelectQueryBuilder {
	for _, col := range relSchema.Columns {
		sqlcQB = sqlcQB.Column(sqlc.Col(relation, col.Name).As(fmt.Sprintf("`%s__%s`", relation, col.Name)))
	}

	return sqlcQB
}

// applyJoinClause builds the ON condition for a single join entry and appends
// the JOIN clause to the query builder.
func (r *repositoryCommon[K, E]) applyJoinClause(sqlcQB *sqlc.SelectQueryBuilder, j joinEntry, rel *Relationship, relSchema *EntitySchema) (*sqlc.SelectQueryBuilder, error) {
	var err error
	var sql string
	var args []any
	var onCondition string
	var onParams []any

	baseOnExpr := buildBaseOnExpression(r.schema, rel, relSchema, j.relation)

	onConditions := sqlc.NewSqlerWhere().Where(baseOnExpr)
	for _, w := range j.where {
		if w == nil {
			continue
		}

		if w.IsEmpty() {
			continue
		}

		if sql, args, err = w.ToSql(); err != nil {
			return nil, fmt.Errorf("failed to build join condition: %w", err)
		}

		onConditions = onConditions.Where(sql, args...)
	}

	if onCondition, onParams, err = onConditions.ToSql(); err != nil {
		return nil, fmt.Errorf("failed to build ON condition for relation %q: %w", j.relation, err)
	}

	if sqlcQB, err = addJoinToBuilder(sqlcQB, j.joinType, relSchema.TableName, j.relation, onCondition, onParams...); err != nil {
		return nil, fmt.Errorf("failed to build join clause for relation %q: %w", j.relation, err)
	}

	return sqlcQB, nil
}

// buildBaseOnExpression constructs the base ON expression for a join based on
// the relationship type (BelongsTo vs HasOne/HasMany).
func buildBaseOnExpression(schema *EntitySchema, rel *Relationship, relSchema *EntitySchema, relation string) *sqlc.Expression {
	if rel.Type == BelongsTo {
		return sqlc.Col(schema.TableName, rel.ForeignKey).
			Eq(sqlc.Col(relation, relSchema.PrimaryKey.Name))
	}

	return sqlc.Col(schema.TableName, schema.PrimaryKey.Name).
		Eq(sqlc.Col(relation, rel.ForeignKey))
}

// hydrateJoinResults scans rows from a join query and deduplicates/assigns
// related entities to the correct parent entities.
func (r *repositoryCommon[K, E]) hydrateJoinResults(rows *sqlc.Rows, sortedJoins []joinEntry, joinInfoByName map[string]joinHydrationInfo) ([]E, error) {
	var err error
	var columns []string

	if columns, err = rows.Columns(); err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	relationStates := precomputeJoinRelationStates(sortedJoins, joinInfoByName)
	columnScanTargets, scanErrors := precomputeJoinColumnScanTargets(columns, r.schema, relationStates)

	results := make([]E, 0)
	entityIndex := make(map[any]int)
	relatedSeen := make(map[any]map[string]map[any]struct{})
	scanDests := make([]any, len(columns))

	for rows.Next() {
		entity, relationPresent, err := scanJoinRow[K, E](rows, columns, relationStates, columnScanTargets, scanDests)
		if err != nil {
			return nil, err
		}

		idx, pkKey, err := upsertHydratedEntity[K, E](&results, entityIndex, relatedSeen, entity)
		if err != nil {
			return nil, err
		}

		target := reflect.ValueOf(&results[idx]).Elem()
		if err := assignJoinedRelations(target, relationStates, relatedSeen[pkKey], relationPresent); err != nil {
			return nil, err
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	if len(scanErrors) > 0 {
		return nil, fmt.Errorf("failed to map joined columns: %w", errors.Join(scanErrors...))
	}

	return results, nil
}

func scanJoinRow[K KeyTypes, E Entitier[K]](rows *sqlc.Rows, columns []string, relationStates []joinRelationState, columnScanTargets []joinColumnScanTarget, scanDests []any) (entity E, relationPresent []bool, err error) {
	resetJoinRelationStates(relationStates)

	entity = *new(E)
	rv := reflect.ValueOf(&entity).Elem()
	populateJoinScanDests(scanDests, columns, rv, relationStates, columnScanTargets)

	if err = rows.Scan(scanDests...); err != nil {
		return entity, nil, fmt.Errorf("failed to scan row: %w", err)
	}

	relationPresent, err = hydrateScannedJoinRelations(columns, relationStates, columnScanTargets, scanDests)
	if err != nil {
		return entity, nil, err
	}

	return entity, relationPresent, nil
}

func resetJoinRelationStates(relationStates []joinRelationState) {
	for i := range relationStates {
		relationStates[i].value.SetZero()
	}
}

func populateJoinScanDests(scanDests []any, columns []string, rv reflect.Value, relationStates []joinRelationState, columnScanTargets []joinColumnScanTarget) {
	for i := range columns {
		scanDests[i] = scanDestForJoinColumn(rv, relationStates, columnScanTargets[i])
	}
}

func hydrateScannedJoinRelations(columns []string, relationStates []joinRelationState, columnScanTargets []joinColumnScanTarget, scanDests []any) ([]bool, error) {
	relationPresent := make([]bool, len(relationStates))

	for i := range columns {
		targetInfo := columnScanTargets[i]
		if targetInfo.relationIndex < 0 || targetInfo.relationFieldIndex == nil {
			continue
		}

		scanned := scannedValue(scanDests[i])
		if scanned == nil {
			continue
		}

		relationPresent[targetInfo.relationIndex] = true
		relState := relationStates[targetInfo.relationIndex]
		relField := relState.value.FieldByIndex(targetInfo.relationFieldIndex)
		if err := setFieldValue(relField, scanned, relState.joinInfo.schema.TableName, targetInfo.relationColumnName); err != nil {
			return nil, fmt.Errorf("failed to hydrate joined relation %q column %q: %w", relState.name, targetInfo.relationColumnName, err)
		}
	}

	return relationPresent, nil
}

func upsertHydratedEntity[K KeyTypes, E Entitier[K]](results *[]E, entityIndex map[any]int, relatedSeen map[any]map[string]map[any]struct{}, entity E) (idx int, pkKey any, err error) {
	pk := entity.GetId()
	var ok bool
	pkKey, ok = comparableKey(pk)
	if !ok {
		return 0, nil, fmt.Errorf("base entity produced non-comparable primary key type %T", pk)
	}

	if existingIdx, exists := entityIndex[pkKey]; exists {
		return existingIdx, pkKey, nil
	}

	*results = append(*results, entity)
	idx = len(*results) - 1
	entityIndex[pkKey] = idx
	relatedSeen[pkKey] = make(map[string]map[any]struct{})

	return idx, pkKey, nil
}

// assignJoinedRelations assigns scanned join relation values to the target entity,
// tracking already-seen related entities to avoid duplicates.
func assignJoinedRelations(target reflect.Value, relationStates []joinRelationState, seen map[string]map[any]struct{}, relationPresent []bool) error {
	for i := range relationStates {
		state := relationStates[i]
		if !joinRelationShouldBeAssigned(state, relationPresent[i]) {
			continue
		}

		relPkVal := state.value.FieldByIndex(state.joinInfo.schema.PrimaryKey.FieldIndex)
		relPk, present, err := optionalComparableKey(relPkVal.Interface())
		if err != nil {
			return fmt.Errorf("joined relation %q produced %w", state.name, err)
		}

		if !present {
			continue
		}

		relationSeen := seenJoinRelation(state.name, seen)
		if isSeenJoinRelation(relationSeen, relPk) {
			continue
		}

		markSeenJoinRelation(relationSeen, relPk)
		if err := assignJoinRelation(target, state); err != nil {
			return err
		}
	}

	return nil
}

func joinRelationShouldBeAssigned(state joinRelationState, relationPresent bool) bool {
	return relationPresent && state.joinInfo.schema.PrimaryKey != nil
}

func seenJoinRelation(name string, seen map[string]map[any]struct{}) map[any]struct{} {
	relationSeen := seen[name]
	if relationSeen == nil {
		relationSeen = make(map[any]struct{})
		seen[name] = relationSeen
	}

	return relationSeen
}

func isSeenJoinRelation(relationSeen map[any]struct{}, relPk any) bool {
	_, exists := relationSeen[relPk]

	return exists
}

func markSeenJoinRelation(relationSeen map[any]struct{}, relPk any) {
	relationSeen[relPk] = struct{}{}
}

func assignJoinRelation(target reflect.Value, state joinRelationState) error {
	relField := target.FieldByIndex(state.joinInfo.rel.FieldIndex)
	if relField.Kind() == reflect.Slice {
		if err := assignRelated(relField, state.value); err != nil {
			return fmt.Errorf("failed to assign joined relation %q: %w", state.name, err)
		}

		return nil
	}

	if relField.IsZero() {
		if err := assignRelated(relField, state.value); err != nil {
			return fmt.Errorf("failed to assign joined relation %q: %w", state.name, err)
		}
	}

	return nil
}

func addJoinToBuilder(
	sqlcQB *sqlc.SelectQueryBuilder,
	joinType sqlc.JoinType,
	tableName string,
	alias string,
	onCondition string,
	params ...any,
) (*sqlc.SelectQueryBuilder, error) {
	switch joinType {
	case sqlc.JoinLeft:

		return sqlcQB.LeftJoin(tableName).As(alias).On(onCondition, params...), nil
	case sqlc.JoinInner:

		return sqlcQB.Join(tableName).As(alias).On(onCondition, params...), nil
	default:

		return nil, fmt.Errorf("unsupported join type: %s", joinType)
	}
}

func precomputeJoinRelationStates(sortedJoins []joinEntry, joinInfoByName map[string]joinHydrationInfo) []joinRelationState {
	relationStates := make([]joinRelationState, 0, len(joinInfoByName))
	seen := make(map[string]struct{}, len(joinInfoByName))

	for _, j := range sortedJoins {
		if _, ok := seen[j.relation]; ok {
			continue
		}

		joinInfo, ok := joinInfoByName[j.relation]
		if !ok {
			continue
		}

		seen[j.relation] = struct{}{}
		relationStates = append(relationStates, joinRelationState{
			name:     j.relation,
			joinInfo: joinInfo,
			value:    reflect.New(joinInfo.rel.RelatedType).Elem(),
		})
	}

	return relationStates
}

func precomputeJoinColumnScanTargets(columns []string, schema *EntitySchema, relationStates []joinRelationState) ([]joinColumnScanTarget, []error) {
	var err error
	var target joinColumnScanTarget

	relationIndexByName := make(map[string]int, len(relationStates))
	for i := range relationStates {
		relationIndexByName[relationStates[i].name] = i
	}

	targets := make([]joinColumnScanTarget, len(columns))
	scanErrors := make([]error, 0)
	scanErrorSeen := make(map[string]struct{})

	for i, colName := range columns {
		if target, err = precomputeJoinColumnScanTarget(colName, schema, relationStates, relationIndexByName); err == nil {
			targets[i] = target

			continue
		}

		targets[i] = target
		errMsg := err.Error()
		if _, seen := scanErrorSeen[errMsg]; seen {
			continue
		}

		scanErrorSeen[errMsg] = struct{}{}
		scanErrors = append(scanErrors, err)
	}

	return targets, scanErrors
}

func precomputeJoinColumnScanTarget(colName string, schema *EntitySchema, relationStates []joinRelationState, relationIndexByName map[string]int) (joinColumnScanTarget, error) {
	target := joinColumnScanTarget{
		relationIndex: -1,
		discard:       new(any),
	}

	relationName, fieldColName, isJoinedColumn := strings.Cut(colName, "__")
	if isJoinedColumn {
		relationIndex, ok := relationIndexByName[relationName]
		if !ok {
			return target, fmt.Errorf("joined column %q references unknown relation %q", colName, relationName)
		}

		if col, ok := relationStates[relationIndex].joinInfo.schema.ColumnByName(fieldColName); ok {
			target.relationIndex = relationIndex
			target.relationFieldIndex = col.FieldIndex
			target.relationColumnName = col.Name

			return target, nil
		}

		return target, fmt.Errorf("joined column %q references unknown column %q on relation %q", colName, fieldColName, relationName)
	}

	if col, ok := schema.ColumnByName(colName); ok {
		target.baseFieldIndex = col.FieldIndex

		return target, nil
	}

	return target, fmt.Errorf("column %q does not map to base entity columns or joined relations", colName)
}

func scanDestForJoinColumn(rv reflect.Value, relationStates []joinRelationState, target joinColumnScanTarget) any {
	if target.baseFieldIndex != nil {
		return rv.FieldByIndex(target.baseFieldIndex).Addr().Interface()
	}

	if target.relationIndex >= 0 && target.relationFieldIndex != nil {
		var scanned any

		return &scanned
	}

	return target.discard
}

// validateJoinRelations checks that every join relation referenced in the query
// exists in the schema and is not ManyToMany. Join execution is limited to
// direct relations; use Preload for nested relation loading.
func (r *repositoryCommon[K, E]) validateJoinRelations(joins []joinEntry) error {
	seenRelations := make(map[string]struct{}, len(joins))

	if len(joins) == 0 {
		return nil
	}

	for _, j := range joins {
		if _, seen := seenRelations[j.relation]; seen {
			return fmt.Errorf("join relation %q specified multiple times", j.relation)
		}

		seenRelations[j.relation] = struct{}{}

		segments, err := splitRelationPath(j.relation)
		if err != nil {
			return wrapRelationPathError("join relation", j.relation, err)
		}

		// Nested/dotted join paths (e.g. "Posts.Comments") are not supported.
		// Use Preload("Posts.Comments") instead for nested relation loading.
		if len(segments) > 1 {
			return fmt.Errorf("nested join relation %q is not supported; use Preload(%q) instead for nested relation loading", j.relation, j.relation)
		}

		rel, _, err := r.schema.ResolveRelationPath(j.relation)
		if err != nil {
			return wrapRelationPathError("join relation", j.relation, err)
		}

		// Many-to-many associations require two joins (through a join table) but
		// our join generation joins directly to the related table. This produces
		// SQL referencing columns that don't exist on the target table.
		if rel.Type == ManyToMany {
			return fmt.Errorf("join relation %q is a many-to-many association which is not supported; many-to-many joins require a join table and are not supported (use Preload instead)", j.relation)
		}
	}

	return nil
}
