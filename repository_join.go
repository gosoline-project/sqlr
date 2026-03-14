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

		if relSchema, err = rel.resolveRelationSchema(); err != nil {
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
	var idx int

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
		for i := range relationStates {
			relationStates[i].value.SetZero()
		}

		entity := *new(E)
		rv := reflect.ValueOf(&entity).Elem()

		for i := range columns {
			scanDests[i] = scanDestForJoinColumn(rv, relationStates, columnScanTargets[i])
		}

		if err = rows.Scan(scanDests...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		pk := entity.GetId()
		pkKey, ok := comparableKey(pk)
		if !ok {
			return nil, fmt.Errorf("base entity produced non-comparable primary key type %T", pk)
		}
		if idx, ok = entityIndex[pkKey]; !ok {
			results = append(results, entity)
			idx = len(results) - 1
			entityIndex[pkKey] = idx
			relatedSeen[pkKey] = make(map[string]map[any]struct{})
		}

		target := reflect.ValueOf(&results[idx]).Elem()
		if err = assignJoinedRelations(target, relationStates, relatedSeen[pkKey]); err != nil {
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

// assignJoinedRelations assigns scanned join relation values to the target entity,
// tracking already-seen related entities to avoid duplicates.
func assignJoinedRelations(target reflect.Value, relationStates []joinRelationState, seen map[string]map[any]struct{}) error {
	for i := range relationStates {
		state := relationStates[i]
		if state.joinInfo.schema.PrimaryKey == nil {
			continue
		}

		relPkVal := state.value.FieldByIndex(state.joinInfo.schema.PrimaryKey.FieldIndex)
		if relPkVal.IsZero() {
			continue
		}

		relPk, ok := comparableKey(relPkVal.Interface())
		if !ok {
			return fmt.Errorf("joined relation %q produced non-comparable primary key type %T", state.name, relPkVal.Interface())
		}
		relationSeen := seen[state.name]
		if relationSeen == nil {
			relationSeen = make(map[any]struct{})
			seen[state.name] = relationSeen
		}

		if _, exists := relationSeen[relPk]; exists {
			continue
		}

		relationSeen[relPk] = struct{}{}

		relField := target.FieldByIndex(state.joinInfo.rel.FieldIndex)
		if relField.Kind() == reflect.Slice {
			if err := assignRelated(relField, state.value); err != nil {
				return fmt.Errorf("failed to assign joined relation %q: %w", state.name, err)
			}

			continue
		}

		if relField.IsZero() {
			if err := assignRelated(relField, state.value); err != nil {
				return fmt.Errorf("failed to assign joined relation %q: %w", state.name, err)
			}
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
	case sqlc.JoinRight:

		return sqlcQB.RightJoin(tableName).As(alias).On(onCondition, params...), nil
	case sqlc.JoinCross:

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
		return relationStates[target.relationIndex].value.FieldByIndex(target.relationFieldIndex).Addr().Interface()
	}

	return target.discard
}

// validateJoinRelations checks that every join relation referenced in the query
// exists in the schema and is not ManyToMany. Nested/dotted join paths are
// validated segment-by-segment, but join execution is limited to direct relations;
// use Preload for nested relation loading.
func (r *repositoryCommon[K, E]) validateJoinRelations(joins []joinEntry) error {
	var err error
	var relSchema *EntitySchema
	seenRelations := make(map[string]struct{}, len(joins))

	if len(joins) == 0 {
		return nil
	}

	for _, j := range joins {
		if _, seen := seenRelations[j.relation]; seen {
			return fmt.Errorf("join relation %q specified multiple times", j.relation)
		}

		seenRelations[j.relation] = struct{}{}

		parts := strings.Split(j.relation, ".")

		// Nested/dotted join paths (e.g. "Posts.Comments") are not supported.
		// Use Preload("Posts.Comments") instead for nested relation loading.
		if len(parts) > 1 {
			return fmt.Errorf("nested join relation %q is not supported; use Preload(%q) instead for nested relation loading", j.relation, j.relation)
		}

		currentSchema := r.schema
		for _, part := range parts {
			rel, ok := currentSchema.Relationships[part]
			if !ok {
				return fmt.Errorf("join relation %q not found on model %s; valid relations: %v", j.relation, currentSchema.entityType.Name(), currentSchema.ValidRelationNames())
			}

			// Many-to-many associations require two joins (through a join table) but
			// our join generation joins directly to the related table. This produces
			// SQL referencing columns that don't exist on the target table.
			if rel.Type == ManyToMany {
				return fmt.Errorf("join relation %q is a many-to-many association which is not supported; many-to-many joins require a join table and are not supported (use Preload instead)", j.relation)
			}

			if relSchema, err = rel.resolveRelationSchema(); err != nil {
				return fmt.Errorf("failed to resolve schema for join relation %q: %w", j.relation, err)
			}

			currentSchema = relSchema
		}
	}

	return nil
}
