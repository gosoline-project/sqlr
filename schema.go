package sqlr

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// RelationType describes the cardinality of a relationship between entities.
type RelationType int

const (
	// HasOne indicates a one-to-one relationship where the current entity owns one
	// related row and the foreign key is stored on the related table.
	HasOne RelationType = iota
	// HasMany indicates a one-to-many relationship where the current entity owns
	// multiple related rows and the foreign key is stored on the related table.
	HasMany
	// ManyToMany indicates a many-to-many relationship through a join table that
	// stores foreign keys for both the current and related entity.
	ManyToMany
	// BelongsTo indicates a relationship where the current entity stores the
	// foreign key pointing to the related entity's primary key.
	BelongsTo
)

// ColumnInfo holds metadata about a single database column mapped to a struct field.
type ColumnInfo struct {
	// Name is the database column name (from the db struct tag).
	Name string
	// FieldIndex is the chain of struct field indices needed to reach this field
	// via reflect.Value.FieldByIndex. For embedded struct fields this may be
	// multiple levels deep (e.g., [0, 1] for the second field of the first embedded struct).
	FieldIndex []int
	// IsPrimaryKey is true if the field is tagged with the "primaryKey" sqlr tag option.
	IsPrimaryKey bool
	// AutoIncrement is true when a primary key field's runtime type is an integer kind.
	// This is inferred from the field type during schema parsing, not from a struct tag.
	AutoIncrement bool
	// AutoCreateTime is true if the field should be auto-set on insert.
	AutoCreateTime bool
	// AutoUpdateTime is true if the field should be auto-set on insert and update.
	AutoUpdateTime bool
}

// Relationship holds metadata about a relationship between two entity types.
// Relation fields may be declared as values or pointers, and slice relations may
// use either value or pointer elements. RelatedType always stores the unwrapped
// non-pointer entity type used for schema resolution.
type Relationship struct {
	// Name is the Go struct field name (e.g., "Posts", "Tags").
	Name string
	// Type describes the relationship cardinality.
	Type RelationType
	// ForeignKey is the relationship foreign key column name. For HasOne/HasMany
	// it names the related-table column that references the current entity primary
	// key. For BelongsTo it names the current-entity column that references the
	// related entity primary key. For ManyToMany this field is not used directly.
	ForeignKey string
	// JoinTable is the name of the intermediate table for ManyToMany relationships.
	// When empty it is auto-derived at parse time by sorting the two entity table
	// names alphabetically and joining them with an underscore.
	JoinTable string
	// JoinParentKey is the join table column name that references the parent
	// entity's primary key. When empty the name is derived at query time as
	// SchemaNameTransformer(parentType.Name()) + "_id".
	JoinParentKey string
	// JoinRelatedKey is the join table column name that references the related
	// entity's primary key. When empty the name is derived at query time as
	// SchemaNameTransformer(relatedType.Name()) + "_id".
	JoinRelatedKey string
	// RelatedType is the reflect.Type of the related entity (element type, not slice).
	RelatedType reflect.Type
	// RelatedSchema is the parsed schema of the related entity. It is lazily
	// populated when needed (e.g., for nested preloads or validating dotted paths).
	RelatedSchema *EntitySchema
	resolveOnce   sync.Once
	resolveErr    error
	// FieldIndex is the struct field index for setting the relationship slice/struct.
	FieldIndex []int
	// Preload indicates that this relationship should be automatically loaded
	// when reading entities, without requiring an explicit Preload() call on the
	// query builder.
	Preload bool
	// SyncCreate indicates that this relationship should be synchronized by
	// default during Create. When at least one relationship on the schema tree is
	// tagged with sync:create, Create limits default synchronization to the tagged
	// relation paths unless the query builder overrides that behavior.
	SyncCreate bool
	// SyncUpdate indicates that this relationship should be synchronized by
	// default during Update. Tagged relation paths are merged with per-call
	// QueryBuilderUpdate options.
	SyncUpdate bool
	// SyncDelete indicates that this relationship was tagged with sync:delete.
	// The flag is retained in schema metadata, but Delete currently cascades owned
	// associations by default and does not consult this flag at runtime.
	SyncDelete bool
	// SyncMany2many indicates that this many-to-many relationship should
	// perform full related-entity synchronization during Update by default when
	// tagged with syncMode:many2many, instead of the link-only reconciliation
	// strategy.
	SyncMany2many bool
}

// EntitySchema holds the parsed metadata for an entity type, including its table
// name, column mappings, primary key, and relationships. It is created once at
// repository construction time via ParseSchema and reused for all operations.
type EntitySchema struct {
	// TableName is the database table name.
	TableName string
	// Columns is the ordered list of all mapped columns (including embedded struct fields).
	Columns []ColumnInfo
	// PrimaryKey points to the primary key column info (nil if none found).
	PrimaryKey *ColumnInfo
	// Relationships maps Go struct field names to their relationship metadata.
	Relationships map[string]*Relationship
	// columnByName maps db column names to their parsed column metadata for O(1) lookups.
	columnByName map[string]*ColumnInfo
	// entityType is the reflect.Type of the entity struct.
	entityType reflect.Type
	// Cached schema-derived values, computed once after parsing.
	insertColumns     []string
	allColumns        []string
	allColumnsAny     []any
	qualifiedColumns  []string
	autoPreloads      []preloadEntry
	autoSyncCreates   []string
	autoSyncUpdates   []string
	autoSyncDeletes   []string
	autoSyncMany2many []string
}

type schemaParseOptions struct {
	parseFieldsErrorFormat       string
	cacheDerivedValues           bool
	missingPrimaryKeyErrorFormat string
}

// InsertColumns returns the column names for INSERT statements, excluding the
// primary key only when it is marked as auto-increment.
func (s *EntitySchema) InsertColumns() []string {
	return s.insertColumns
}

// AllColumns returns all column names in declaration order.
func (s *EntitySchema) AllColumns() []string {
	return s.allColumns
}

// QualifiedColumns returns all column names qualified with the table name
// (e.g., "`test_users`.`id`") for use in SELECT statements with joins.
func (s *EntitySchema) QualifiedColumns() []string {
	return s.qualifiedColumns
}

// ColumnByName returns column metadata for a database column name.
func (s *EntitySchema) ColumnByName(name string) (*ColumnInfo, bool) {
	column, ok := s.columnByName[name]

	return column, ok
}

// ValidRelationNames returns a sorted list of relationship names for error messages.
func (s *EntitySchema) ValidRelationNames() []string {
	names := make([]string, 0, len(s.Relationships))
	for name := range s.Relationships {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// AutoPreloads returns preloadEntry values for all relationships that have the
// "preload" tag option set. These are merged with explicit preloads at query
// time so that tagged relations are automatically loaded without requiring an
// explicit Preload() call. The result is sorted by relation name for
// deterministic query ordering.
func (s *EntitySchema) AutoPreloads() []preloadEntry {
	return s.autoPreloads
}

// AutoSyncCreatePaths returns relation paths that have the "sync:create" tag
// option set somewhere in the schema tree. These defaults are merged with
// per-call Create association options.
func (s *EntitySchema) AutoSyncCreatePaths() []string {
	return s.autoSyncCreates
}

// AutoSyncUpdatePaths returns relation paths that have the "sync:update" tag
// option set somewhere in the schema tree. These defaults are merged with
// per-call Update association options.
func (s *EntitySchema) AutoSyncUpdatePaths() []string {
	return s.autoSyncUpdates
}

// AutoSyncDeletePaths returns relation paths that have the "sync:delete" tag
// option set somewhere in the schema tree. These paths are retained as schema
// metadata and are not currently consulted by Delete runtime behavior.
func (s *EntitySchema) AutoSyncDeletePaths() []string {
	return s.autoSyncDeletes
}

// AutoSyncMany2manyPaths returns many-to-many relation paths that have the
// "syncMode:many2many" tag option set somewhere in the schema tree.
// These defaults opt tagged paths into full entity synchronization during Update.
func (s *EntitySchema) AutoSyncMany2manyPaths() []string {
	return s.autoSyncMany2many
}

// ParseSchema parses the entity type E using reflection to build an EntitySchema.
// It reads the `db` tag for an optional column name override and the `sqlr` tag
// for field behaviour metadata. The db tag accepts only a column name or "-".
// The sqlr tag accepts semicolon-separated options including primaryKey,
// autoCreateTime, autoUpdateTime, foreignKey:<column>, belongsTo:<column>,
// many2many:<table>, preload, sync:create, sync:update, sync:delete, and
// syncMode:many2many.
//
// When a public field has no db tag, its name is transformed by
// SchemaNameTransformer (default: toSnakeCase) to derive the column name.
// Untagged struct and slice-of-struct fields are only auto-detected as
// relationships when sqlr finds stronger evidence that they represent entities:
// the related type must declare a primary key, and the conventional inferred
// foreign key must exist on the parent or related type. Otherwise the field is
// treated like any other untagged field. Unexported fields with no db or sqlr
// tag are silently ignored. To explicitly exclude a public field from mapping, use
// db:"-".
//
// Relationship examples:
//
//	// Explicit relation metadata (db:"-" is optional for relationships):
//	type Author struct {
//		Entity[int64]
//		Posts []Post `sqlr:"foreignKey:author_id"`
//	}
//
//	// Auto-detected relationship (no db or sqlr tag):
//	type Post struct {
//		Entity[int64]
//		AuthorID int64  // column "author_id" via SchemaNameTransformer
//		Author   Person // BelongsTo, FK = "author_id" (derived from field name)
//	}
//
//	type Person struct {
//		Entity[int64]
//		Posts []Post // HasMany, FK = "person_id" (derived from parent type name "Person")
//	}
//
//	// Explicit relationship with belongsTo option:
//	type PostWithAuthor struct {
//		Entity[int64]
//		AuthorID int64  `db:"author_id"`
//		AuthorRef Author `sqlr:"belongsTo:author_id"`
//	}
//
//	// ManyToMany (always requires explicit sqlr metadata):
//	type Article struct {
//		Entity[int64]
//		Tags []Tag `sqlr:"many2many:article_tags"`
//	}
//
// The schema is used at query time for SQL generation, relationship validation,
// and struct hydration.
//
// Pointer relation semantics:
//   - Scalar HasOne and BelongsTo relations may be declared as either T or *T.
//   - Collection HasMany and ManyToMany relations may be declared as either []T
//     or []*T.
//   - Relationship kind is determined by sqlr tag options or field shape, not by
//     pointer usage.
//   - Missing related rows keep the field at its natural zero value: nil for
//     pointer scalars, zero struct for value scalars, and empty or nil slices
//     for collection relations when no related rows are assigned.
func ParseSchema[E any]() (*EntitySchema, error) {
	var t reflect.Type
	var zero E

	t = reflect.TypeOf(zero)
	if t == nil {
		t = reflect.TypeOf((*E)(nil)).Elem()
	}

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return ParseSchemaType(t)
}

// ParseSchemaType parses the provided entity type using reflection to build an
// EntitySchema. Nil types are rejected, pointer chains are unwrapped to their
// element struct type, and unnamed struct types are rejected because sqlr cannot
// derive a stable table name for them.
func ParseSchemaType(t reflect.Type) (*EntitySchema, error) {
	return parseSchemaType(t, defaultSchemaParseOptions())
}

func defaultSchemaParseOptions() schemaParseOptions {
	return schemaParseOptions{
		parseFieldsErrorFormat:       "failed to parse schema for %s: %w",
		cacheDerivedValues:           true,
		missingPrimaryKeyErrorFormat: "entity type %s has no primary key (tag with sqlr:\"primaryKey\")",
	}
}

func relatedSchemaParseOptions() schemaParseOptions {
	return schemaParseOptions{
		parseFieldsErrorFormat:       "failed to parse related schema for %s: %w",
		cacheDerivedValues:           true,
		missingPrimaryKeyErrorFormat: "related entity type %s has no primary key",
	}
}

func autoPreloadSchemaParseOptions() schemaParseOptions {
	return schemaParseOptions{
		parseFieldsErrorFormat:       "failed to parse related schema for %s: %w",
		cacheDerivedValues:           false,
		missingPrimaryKeyErrorFormat: "related entity type %s has no primary key",
	}
}

func parseSchemaType(t reflect.Type, options schemaParseOptions) (*EntitySchema, error) {
	var err error

	if t, err = normalizeSchemaType(t); err != nil {
		return nil, err
	}

	typeName := t.Name()
	schema := &EntitySchema{
		TableName:     tableNameForType(t),
		Relationships: make(map[string]*Relationship),
		entityType:    t,
	}

	if err = parseFields(t, nil, schema); err != nil {
		return nil, fmt.Errorf(options.parseFieldsErrorFormat, typeName, err)
	}
	if err = validateSchemaColumns(schema); err != nil {
		return nil, fmt.Errorf("failed to validate columns for %s: %w", typeName, err)
	}

	setPrimaryKeyFromColumns(schema)

	if options.cacheDerivedValues {
		if err = cacheSchemaDerivedValues(schema); err != nil {
			return nil, fmt.Errorf("failed to collect schema defaults for %s: %w", typeName, err)
		}
	}

	if err = validateRelationships(schema); err != nil {
		return nil, fmt.Errorf("failed to validate relationships for %s: %w", typeName, err)
	}

	if schema.PrimaryKey == nil {
		return nil, fmt.Errorf(options.missingPrimaryKeyErrorFormat, typeName)
	}

	return schema, nil
}

func normalizeSchemaType(t reflect.Type) (reflect.Type, error) {
	if t == nil {
		return nil, fmt.Errorf("entity type is nil")
	}

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("entity type %s is not a struct", schemaTypeName(t))
	}

	if t.Name() == "" {
		return nil, fmt.Errorf("entity type %s is unnamed", schemaTypeName(t))
	}

	return t, nil
}

func schemaTypeName(t reflect.Type) string {
	if t == nil {
		return "<nil>"
	}

	if t.Name() != "" {
		return t.Name()
	}

	return t.String()
}

// parseFields recursively walks struct fields (including embedded structs) to
// populate the EntitySchema with column and relationship metadata.
func parseFields(t reflect.Type, parentIndex []int, schema *EntitySchema) error {
	var handled bool

	for i := range t.NumField() {
		field := t.Field(i)
		fieldIndex := append(append([]int(nil), parentIndex...), i)

		var err error
		if handled, err = tryParseEmbeddedField(field, fieldIndex, schema); err != nil {
			return err
		}

		if handled {
			continue
		}

		if err := parseFieldTag(field, fieldIndex, schema); err != nil {
			return err
		}
	}

	return nil
}

// tryParseEmbeddedField checks if the field is an embedded struct and recursively
// parses it. Returns true if the field was handled (either parsed or skipped).
func tryParseEmbeddedField(field reflect.StructField, fieldIndex []int, schema *EntitySchema) (bool, error) {
	var ft reflect.Type

	ft = field.Type
	for ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}

	if !field.Anonymous || ft.Kind() != reflect.Struct {
		return false, nil
	}

	if !isSchemaAccessibleField(field) {
		return true, fmt.Errorf("field %s: anonymous embedded fields must be exported", field.Name)
	}

	err := parseFields(ft, fieldIndex, schema)
	if err != nil {
		return true, err
	}

	return true, nil
}

type schemaFieldTags struct {
	hasDBTag    bool
	columnName  string
	sqlrOptions []string
}

func parseSchemaFieldTags(field reflect.StructField) (schemaFieldTags, error) {
	var err error
	var tags schemaFieldTags

	if dbTag, ok := field.Tag.Lookup("db"); ok {
		dbTag = strings.TrimSpace(dbTag)
		if dbTag != "" {
			if strings.Contains(dbTag, ",") {
				return schemaFieldTags{}, fmt.Errorf("db tag must contain only a column name or \"-\"")
			}

			tags.hasDBTag = true
			tags.columnName = dbTag
		}
	}

	if tags.sqlrOptions, err = splitTagOptions(field.Tag.Get("sqlr")); err != nil {
		return schemaFieldTags{}, err
	}

	return tags, nil
}

func splitTagOptions(tagValue string) ([]string, error) {
	tagValue = strings.TrimSpace(tagValue)
	if tagValue == "" {
		return nil, nil
	}

	parts := strings.Split(tagValue, ";")
	options := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if err := validateTagOption(part); err != nil {
			return nil, err
		}

		options = append(options, part)
	}

	return options, nil
}

func validateTagOption(opt string) error {
	if strings.Contains(opt, ",") && !strings.HasPrefix(opt, "sync:") {
		return fmt.Errorf("sqlr tag option %q must not contain \",\"; use \";\" to separate options", opt)
	}

	if isColumnMetadataOption(opt) || isRelationshipDefiningOption(opt) {
		return nil
	}

	if strings.HasPrefix(opt, "parentKey:") || strings.HasPrefix(opt, "relatedKey:") || opt == "preload" {
		return nil
	}

	if strings.HasPrefix(opt, "sync:") {
		_, _, _, err := parseSyncOption(opt)

		return err
	}

	if strings.HasPrefix(opt, "syncMode:") {
		_, err := parseSyncModeOption(opt)

		return err
	}

	return fmt.Errorf("unknown sqlr option %q", opt)
}

// parseFieldTag parses a single struct field's db/sqlr tags and adds it to the
// schema as either a relationship or a column. When no db tag is present on a
// public field, the field name is passed through SchemaNameTransformer to
// derive the column name. Untagged struct fields are only auto-detected as
// relationships when the related type looks like an entity and the inferred
// conventional foreign key can be found on the parent or related type.
func parseFieldTag(field reflect.StructField, fieldIndex []int, schema *EntitySchema) error {
	tags, err := parseSchemaFieldTags(field)
	if err != nil {
		return fmt.Errorf("field %s: %w", field.Name, err)
	}

	if !isSchemaAccessibleField(field) {
		if tags.hasDBTag && tags.columnName != "-" {
			return fmt.Errorf("field %s: unexported fields cannot define db column mappings", field.Name)
		}

		if len(tags.sqlrOptions) > 0 {
			return fmt.Errorf("field %s: unexported fields cannot define sqlr metadata", field.Name)
		}

		return nil
	}

	hasRelationshipDefinition := isRelationshipField(tags.sqlrOptions)
	hasRelationshipMetadata := hasRelationshipMetadataOption(tags.sqlrOptions)
	hasColumnMetadata := hasColumnMetadataOption(tags.sqlrOptions)

	if hasColumnMetadata && hasRelationshipMetadata {
		return fmt.Errorf("field %s: sqlr column metadata cannot be combined with relationship metadata", field.Name)
	}

	if hasRelationshipMetadata {
		if tags.hasDBTag && tags.columnName != "-" {
			return fmt.Errorf("field %s: relationship metadata cannot be combined with a db column name", field.Name)
		}

		autoDetected := !hasRelationshipDefinition
		if autoDetected && !shouldAutoDetectRelationship(field, schema.entityType) {
			return fmt.Errorf("field %s: relationship-only sqlr options require an auto-detected relationship or explicit relation metadata", field.Name)
		}

		rel, err := parseRelationship(field, tags.sqlrOptions, fieldIndex, schema.entityType, autoDetected)
		if err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}

		schema.Relationships[rel.Name] = rel

		return nil
	}

	if tags.hasDBTag && tags.columnName == "-" {
		if hasColumnMetadata {
			return fmt.Errorf("field %s: db:\"-\" cannot be combined with column metadata", field.Name)
		}

		return nil
	}

	if !tags.hasDBTag && len(tags.sqlrOptions) == 0 {
		return parseUntaggedField(field, fieldIndex, schema)
	}

	colName := tags.columnName
	if colName == "" {
		if !isPublicField(field) {
			return nil
		}

		colName = SchemaNameTransformer(field.Name)
	}

	col := parseColumnInfo(field, colName, fieldIndex, tags.sqlrOptions)
	schema.Columns = append(schema.Columns, col)

	return nil
}

// parseUntaggedField handles a struct field that has no db or sqlr tag.
// Unexported fields are silently skipped. Public struct and slice-of-struct fields are only
// auto-detected as relationships when sqlr can validate conventional entity
// evidence for them; all other public fields are mapped to a column whose name is
// derived by applying SchemaNameTransformer to the Go field name.
func parseUntaggedField(field reflect.StructField, fieldIndex []int, schema *EntitySchema) error {
	if !isPublicField(field) {
		return nil
	}

	if shouldAutoDetectRelationship(field, schema.entityType) {
		rel, err := parseRelationship(field, nil, fieldIndex, schema.entityType, true)
		if err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}

		schema.Relationships[rel.Name] = rel

		return nil
	}

	col := parseColumnInfo(field, SchemaNameTransformer(field.Name), fieldIndex, nil)
	schema.Columns = append(schema.Columns, col)

	return nil
}

// parseColumnInfo builds a ColumnInfo from a struct field, column name, and sqlr
// tag options.
func parseColumnInfo(field reflect.StructField, colName string, fieldIndex []int, options []string) ColumnInfo {
	col := ColumnInfo{
		Name:       colName,
		FieldIndex: fieldIndex,
	}

	for _, opt := range options {
		opt = strings.TrimSpace(opt)
		switch opt {
		case "primaryKey":
			col.IsPrimaryKey = true
		case "autoCreateTime":
			col.AutoCreateTime = true
		case "autoUpdateTime":
			col.AutoUpdateTime = true
		}
	}

	if col.IsPrimaryKey {
		fieldType := field.Type
		for fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		col.AutoIncrement = isIntegerKind(fieldType.Kind())
	}

	return col
}

func validateSchemaColumns(schema *EntitySchema) error {
	seenColumns := make(map[string]struct{}, len(schema.Columns))
	primaryKeyCount := 0
	timeType := reflect.TypeOf(time.Time{})

	for _, col := range schema.Columns {
		if _, exists := seenColumns[col.Name]; exists {
			return fmt.Errorf("duplicate column name %q on entity", col.Name)
		}

		seenColumns[col.Name] = struct{}{}

		if col.IsPrimaryKey {
			primaryKeyCount++
		}

		if col.AutoCreateTime || col.AutoUpdateTime {
			field := schema.entityType.FieldByIndex(col.FieldIndex)
			if !isAutoTimestampFieldTypeSupported(field.Type, timeType) {
				return fmt.Errorf("column %q on entity %s uses auto timestamp tags but is not assignable from time.Time", col.Name, schema.entityType.Name())
			}
		}
	}

	if primaryKeyCount > 1 {
		return fmt.Errorf("entity %s defines multiple primary keys", schema.entityType.Name())
	}

	return nil
}

func isAutoTimestampFieldTypeSupported(fieldType reflect.Type, timeType reflect.Type) bool {
	if timeType.ConvertibleTo(fieldType) {
		return true
	}

	if fieldType.Kind() != reflect.Ptr {
		return false
	}

	return timeType.ConvertibleTo(fieldType.Elem())
}

func setPrimaryKeyFromColumns(schema *EntitySchema) {
	schema.PrimaryKey = nil
	schema.columnByName = make(map[string]*ColumnInfo, len(schema.Columns))
	for i := range schema.Columns {
		schema.columnByName[schema.Columns[i].Name] = &schema.Columns[i]
		if schema.Columns[i].IsPrimaryKey {
			schema.PrimaryKey = &schema.Columns[i]
		}
	}
}

func validateRelationships(schema *EntitySchema) error {
	for _, rel := range schema.Relationships {
		if rel.Type == ManyToMany && rel.JoinTable == "" {
			return fmt.Errorf("relationship %s is ManyToMany but has no join table; provide many2many:<table> or leave the table name empty for auto-detection", rel.Name)
		}

		if rel.SyncMany2many && rel.Type != ManyToMany {
			return fmt.Errorf("relationship %s uses syncMode:many2many but is not many-to-many", rel.Name)
		}

		if rel.SyncMany2many && !rel.SyncUpdate {
			return fmt.Errorf("relationship %s uses syncMode:many2many but is missing sync:update", rel.Name)
		}

		if rel.Type != BelongsTo {
			continue
		}

		if _, ok := schema.ColumnByName(rel.ForeignKey); !ok {
			return fmt.Errorf("relationship %s belongsTo foreign key column %q not found on entity", rel.Name, rel.ForeignKey)
		}
	}

	return nil
}

func cacheSchemaDerivedValues(schema *EntitySchema) error {
	insertCols := make([]string, 0, len(schema.Columns))
	allCols := make([]string, 0, len(schema.Columns))
	allColsAny := make([]any, 0, len(schema.Columns))
	qualifiedCols := make([]string, 0, len(schema.Columns))
	for _, c := range schema.Columns {
		allCols = append(allCols, c.Name)
		allColsAny = append(allColsAny, c.Name)
		qualifiedCols = append(qualifiedCols, fmt.Sprintf("`%s`.`%s`", schema.TableName, c.Name))
		if c.IsPrimaryKey && c.AutoIncrement {
			continue
		}

		insertCols = append(insertCols, c.Name)
	}

	autoPreloads, err := collectAutoPreloads(schema)
	if err != nil {
		return err
	}

	autoSyncCreates, err := collectTaggedRelationPaths(schema, func(rel *Relationship) bool {
		return rel.SyncCreate
	})
	if err != nil {
		return fmt.Errorf("failed to collect syncCreate defaults: %w", err)
	}

	autoSyncUpdates, err := collectTaggedRelationPaths(schema, func(rel *Relationship) bool {
		return rel.SyncUpdate
	})
	if err != nil {
		return fmt.Errorf("failed to collect syncUpdate defaults: %w", err)
	}

	autoSyncDeletes, err := collectTaggedRelationPaths(schema, func(rel *Relationship) bool {
		return rel.SyncDelete
	})
	if err != nil {
		return fmt.Errorf("failed to collect syncDelete defaults: %w", err)
	}

	autoSyncMany2many, err := collectTaggedRelationPaths(schema, func(rel *Relationship) bool {
		return rel.SyncMany2many
	})
	if err != nil {
		return fmt.Errorf("failed to collect syncMany2many defaults: %w", err)
	}

	schema.insertColumns = insertCols
	schema.allColumns = allCols
	schema.allColumnsAny = allColsAny
	schema.qualifiedColumns = qualifiedCols
	schema.autoPreloads = autoPreloads
	schema.autoSyncCreates = autoSyncCreates
	schema.autoSyncUpdates = autoSyncUpdates
	schema.autoSyncDeletes = autoSyncDeletes
	schema.autoSyncMany2many = autoSyncMany2many

	return nil
}

func collectAutoPreloads(schema *EntitySchema) ([]preloadEntry, error) {
	autoPreloads := make([]preloadEntry, 0)
	seenPaths := make(map[string]struct{})
	visitedTypes := make(map[reflect.Type]struct{}, 1)
	if schema.entityType != nil {
		visitedTypes[schema.entityType] = struct{}{}
	}

	if err := collectAutoPreloadsRecursive(schema, "", visitedTypes, seenPaths, &autoPreloads); err != nil {
		return nil, err
	}

	sort.Slice(autoPreloads, func(i, j int) bool {
		return autoPreloads[i].relation < autoPreloads[j].relation
	})

	return autoPreloads, nil
}

func collectAutoPreloadsRecursive(schema *EntitySchema, prefix string, visitedTypes map[reflect.Type]struct{}, seenPaths map[string]struct{}, autoPreloads *[]preloadEntry) error {
	var err error
	var relSchema *EntitySchema
	var relationPath string
	var seen bool

	for _, rel := range schema.Relationships {
		if !rel.Preload {
			continue
		}

		relationPath = rel.Name
		if prefix != "" {
			relationPath = prefix + "." + rel.Name
		}

		_, seen = seenPaths[relationPath]
		if !seen {
			*autoPreloads = append(*autoPreloads, preloadEntry{relation: relationPath})
			seenPaths[relationPath] = struct{}{}
		}

		_, seen = visitedTypes[rel.RelatedType]
		if seen {
			continue
		}

		if relSchema, err = parseRelatedSchemaForAutoPreload(rel.RelatedType); err != nil {
			return fmt.Errorf("failed to parse auto-preload schema for relation %q: %w", relationPath, err)
		}

		visitedTypes[rel.RelatedType] = struct{}{}
		if err := collectAutoPreloadsRecursive(relSchema, relationPath, visitedTypes, seenPaths, autoPreloads); err != nil {
			delete(visitedTypes, rel.RelatedType)

			return err
		}
		delete(visitedTypes, rel.RelatedType)
	}

	return nil
}

func parseRelatedSchemaForAutoPreload(relatedType reflect.Type) (*EntitySchema, error) {
	return parseSchemaType(relatedType, autoPreloadSchemaParseOptions())
}

func collectTaggedRelationPaths(schema *EntitySchema, predicate func(rel *Relationship) bool) ([]string, error) {
	taggedPaths := make([]string, 0)
	seenPaths := make(map[string]struct{})
	visitedTypes := make(map[reflect.Type]struct{}, 1)
	if schema.entityType != nil {
		visitedTypes[schema.entityType] = struct{}{}
	}

	if err := collectTaggedRelationPathsRecursive(schema, "", visitedTypes, seenPaths, &taggedPaths, predicate); err != nil {
		return nil, err
	}

	sort.Strings(taggedPaths)

	return taggedPaths, nil
}

func collectTaggedRelationPathsRecursive(
	schema *EntitySchema,
	prefix string,
	visitedTypes map[reflect.Type]struct{},
	seenPaths map[string]struct{},
	taggedPaths *[]string,
	predicate func(rel *Relationship) bool,
) error {
	for _, rel := range schema.Relationships {
		relationPath := rel.Name
		if prefix != "" {
			relationPath = prefix + "." + rel.Name
		}

		if !predicate(rel) {
			continue
		}

		if _, seen := seenPaths[relationPath]; !seen {
			*taggedPaths = append(*taggedPaths, relationPath)
			seenPaths[relationPath] = struct{}{}
		}

		if _, seen := visitedTypes[rel.RelatedType]; seen {
			continue
		}

		relSchema, err := parseRelatedSchemaForAssociationDefaults(rel.RelatedType)
		if err != nil {
			return fmt.Errorf("failed to parse default-sync schema for relation %q: %w", relationPath, err)
		}

		visitedTypes[rel.RelatedType] = struct{}{}
		if err := collectTaggedRelationPathsRecursive(relSchema, relationPath, visitedTypes, seenPaths, taggedPaths, predicate); err != nil {
			delete(visitedTypes, rel.RelatedType)

			return err
		}

		delete(visitedTypes, rel.RelatedType)
	}

	return nil
}

func parseRelatedSchemaForAssociationDefaults(relatedType reflect.Type) (*EntitySchema, error) {
	return parseSchemaType(relatedType, autoPreloadSchemaParseOptions())
}

// isPublicField returns true if the struct field is exported (accessible from
// outside the package). Unexported fields (PkgPath != "") are never mapped.
func isPublicField(field reflect.StructField) bool {
	return field.PkgPath == ""
}

func isSchemaAccessibleField(field reflect.StructField) bool {
	return isPublicField(field) || field.PkgPath == reflect.TypeOf(Entity[int64]{}).PkgPath()
}

// valueTypePackages lists standard-library packages whose struct types are
// scalar value types and must never be auto-detected as entity relationships.
// Fields with types from these packages (e.g. time.Time, sql.NullString) are
// treated as plain database columns even when they carry no explicit tags.
//
// This set can be extended at program startup if additional packages need to be
// excluded from auto-relationship detection.
var valueTypePackages = map[string]struct{}{ //nolint:gochecknoglobals // intentional package-level default, extendable at startup
	"time":         {},
	"database/sql": {},
	"net":          {},
	"net/url":      {},
}

// isAutoRelationshipType returns true when the field's underlying type is a
// struct or slice of structs that looks like an entity candidate for untagged
// relationship auto-detection. Besides excluding well-known scalar value-type
// packages, the related type must also declare a primary key somewhere in its
// field tree. This keeps custom value objects from being misclassified as
// relationships based on shape alone.
func isAutoRelationshipType(ft reflect.Type) bool {
	unwrapped, _ := unwrapRelatedType(ft)
	if unwrapped.Kind() != reflect.Struct {
		return false
	}

	if _, excluded := valueTypePackages[unwrapped.PkgPath()]; excluded {
		return false
	}

	return typeHasPrimaryKey(unwrapped)
}

func shouldAutoDetectRelationship(field reflect.StructField, parentEntityType reflect.Type) bool {
	relatedType, isSlice := unwrapRelatedType(field.Type)
	if !isAutoRelationshipType(field.Type) {
		return false
	}

	if isSlice {
		return typeDefinesColumn(relatedType, SchemaNameTransformer(parentEntityType.Name())+"_id")
	}

	return typeDefinesColumn(parentEntityType, SchemaNameTransformer(field.Name)+"_id")
}

func typeHasPrimaryKey(t reflect.Type) bool {
	t = unwrapSchemaFieldType(t)
	if t.Kind() != reflect.Struct {
		return false
	}

	for i := range t.NumField() {
		field := t.Field(i)
		fieldType := unwrapSchemaFieldType(field.Type)

		if field.Anonymous && isSchemaAccessibleField(field) && fieldType.Kind() == reflect.Struct && typeHasPrimaryKey(fieldType) {
			return true
		}

		if !isSchemaAccessibleField(field) {
			continue
		}

		options, err := splitTagOptions(field.Tag.Get("sqlr"))
		if err != nil {
			continue
		}

		for _, opt := range options {
			if strings.TrimSpace(opt) == "primaryKey" {
				return true
			}
		}
	}

	return false
}

func typeDefinesColumn(t reflect.Type, columnName string) bool {
	t = unwrapSchemaFieldType(t)
	if t.Kind() != reflect.Struct {
		return false
	}

	for i := range t.NumField() {
		field := t.Field(i)
		fieldType := unwrapSchemaFieldType(field.Type)

		if field.Anonymous && isSchemaAccessibleField(field) && fieldType.Kind() == reflect.Struct && typeDefinesColumn(fieldType, columnName) {
			return true
		}

		if !isSchemaAccessibleField(field) {
			continue
		}

		dbTag := strings.TrimSpace(field.Tag.Get("db"))
		sqlrOptions, err := splitTagOptions(field.Tag.Get("sqlr"))
		if err != nil {
			continue
		}
		if dbTag != "" {
			if !strings.Contains(dbTag, ",") && dbTag == columnName && !hasRelationshipMetadataOption(sqlrOptions) && dbTag != "-" {
				return true
			}

			continue
		}

		if hasRelationshipMetadataOption(sqlrOptions) {
			continue
		}

		if isAutoRelationshipType(field.Type) {
			continue
		}

		if SchemaNameTransformer(field.Name) == columnName {
			return true
		}
	}

	return false
}

func unwrapSchemaFieldType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t
}

func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:

		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:

		return true
	default:

		return false
	}
}

// isRelationshipField returns true if the sqlr tag options explicitly define a
// relationship (containing foreignKey:, belongsTo:, or many2many: options).
func isRelationshipField(options []string) bool {
	for _, opt := range options {
		if isRelationshipDefiningOption(opt) {
			return true
		}
	}

	return false
}

func hasRelationshipMetadataOption(options []string) bool {
	for _, opt := range options {
		if isRelationshipMetadataOption(opt) {
			return true
		}
	}

	return false
}

func hasColumnMetadataOption(options []string) bool {
	for _, opt := range options {
		if isColumnMetadataOption(opt) {
			return true
		}
	}

	return false
}

func isRelationshipDefiningOption(opt string) bool {
	return strings.HasPrefix(opt, "foreignKey:") || strings.HasPrefix(opt, "belongsTo:") || strings.HasPrefix(opt, "many2many:")
}

func isRelationshipMetadataOption(opt string) bool {
	return isRelationshipDefiningOption(opt) || strings.HasPrefix(opt, "parentKey:") || strings.HasPrefix(opt, "relatedKey:") || opt == "preload" || strings.HasPrefix(opt, "sync:") || strings.HasPrefix(opt, "syncMode:")
}

func isColumnMetadataOption(opt string) bool {
	switch opt {
	case "primaryKey", "autoCreateTime", "autoUpdateTime":
		return true
	default:
		return false
	}
}

// hasExplicitFKOption returns true when options contain an explicit foreignKey:
// or belongsTo: key, regardless of whether its value is empty. This is used to
// distinguish "user wrote foreignKey: with an empty value" from "no FK option
// was given at all", so that the empty-value case still triggers a validation
// error rather than being silently replaced by a derived default.
func hasExplicitFKOption(options []string) bool {
	for _, opt := range options {
		if strings.HasPrefix(opt, "foreignKey:") || strings.HasPrefix(opt, "belongsTo:") {
			return true
		}
	}

	return false
}

// parseRelationship parses a struct field's sqlr tag options to create a Relationship.
// parentEntityType is the reflect.Type of the entity being parsed; its Name() is used
// to derive a default foreign key for HasOne/HasMany relationships, and its derived
// table name is used when auto-detecting the ManyToMany join table name.
func parseRelationship(field reflect.StructField, options []string, fieldIndex []int, parentEntityType reflect.Type, autoDetected bool) (*Relationship, error) {
	var ft reflect.Type
	var isSlice bool

	ft, isSlice = unwrapRelatedType(field.Type)
	if ft.Kind() != reflect.Struct {
		return nil, fmt.Errorf("relationship field must be a struct or slice of structs, got %s", field.Type)
	}

	rel := &Relationship{
		Name:        field.Name,
		FieldIndex:  fieldIndex,
		RelatedType: ft,
	}

	if err := applyRelationshipOptions(rel, options); err != nil {
		return nil, err
	}

	// Only derive a default FK when the caller did not supply any explicit
	// foreignKey:/belongsTo: option (even an empty one). An explicit but empty
	// option (e.g. `sqlr:"foreignKey:"`) must still be caught by the validator.
	hasFKOption := hasExplicitFKOption(options)
	applyDefaultForeignKey(rel, field.Name, parentEntityType.Name(), isSlice, autoDetected && !hasFKOption)

	// Auto-detect the join table name when the relationship is ManyToMany but no
	// explicit table name was provided via many2many:<table>.
	if rel.Type == ManyToMany && rel.JoinTable == "" {
		applyDefaultJoinTable(rel, parentEntityType, ft)
	}

	return rel, validateRelationshipType(rel, isSlice)
}

// applyDefaultForeignKey derives a foreign key name when none was set by the
// sqlr tag. The convention mirrors ActiveRecord / GORM defaults:
//
//   - BelongsTo (non-slice): FK = SchemaNameTransformer(fieldName) + "_id"
//     e.g. field "Author Author" → FK "author_id" on the current entity table.
//   - HasMany (slice): FK = SchemaNameTransformer(parentEntityTypeName) + "_id"
//     e.g. field "Posts []Post" on "Author" → FK "author_id" on the Posts table.
//
// autoDetected must be true for defaults to be applied. It is false when the
// caller supplied explicit relation-defining sqlr tag options (even with an
// empty foreignKey: value), so that validation errors for missing FK values are
// preserved.
func applyDefaultForeignKey(rel *Relationship, fieldName, parentEntityTypeName string, isSlice, autoDetected bool) {
	if !autoDetected || rel.Type == ManyToMany {
		return
	}

	if rel.ForeignKey == "" {
		if isSlice {
			// HasMany: FK on the related table references this entity's PK.
			rel.ForeignKey = SchemaNameTransformer(parentEntityTypeName) + "_id"
		} else {
			// BelongsTo: FK on the current entity table references the related entity's PK.
			rel.ForeignKey = SchemaNameTransformer(fieldName) + "_id"
		}
	}

	// For auto-detected relationships (no explicit relation-defining sqlr option)
	// set the relationship type from the field shape: slice → HasMany, non-slice
	// → BelongsTo. This must not override an explicitly-set type from a sqlr tag
	// option.
	if isSlice {
		rel.Type = HasMany
	} else {
		rel.Type = BelongsTo
	}
}

// applyDefaultJoinTable derives a join table name for a ManyToMany relationship
// when none was supplied via the many2many: sqlr tag option. The convention is:
//
//  1. Derive the table name for each side using tableNameForType (which honours
//     the TableNamer interface and applies inflection.Plural).
//  2. Sort the two names alphabetically.
//  3. Join them with an underscore.
//
// Example: Article + Tag → "articles" and "tags" → sorted: ["articles", "tags"]
// → join table: "articles_tags".
//
// This mirrors the Rails/GORM auto-join-table convention and produces a
// deterministic, symmetric name regardless of which side the field is declared on.
func applyDefaultJoinTable(rel *Relationship, parentType, relatedType reflect.Type) {
	parentTable := tableNameForType(parentType)
	relatedTable := tableNameForType(relatedType)

	names := []string{parentTable, relatedTable}
	sort.Strings(names)

	rel.JoinTable = names[0] + "_" + names[1]
}

// unwrapRelatedType extracts the element type from a field type, unwrapping
// slices and pointers. It returns the underlying type and whether it was a slice.
func unwrapRelatedType(ft reflect.Type) (reflect.Type, bool) {
	isSlice := false
	if ft.Kind() == reflect.Slice {
		isSlice = true
		ft = ft.Elem()
	}

	for ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}

	return ft, isSlice
}

// applyRelationshipOptions processes sqlr tag options and sets the appropriate
// fields on the Relationship.
func applyRelationshipOptions(rel *Relationship, options []string) error {
	for _, opt := range options {
		opt = strings.TrimSpace(opt)
		switch {
		case opt == "":
			continue
		case strings.HasPrefix(opt, "foreignKey:"):
			rel.ForeignKey = strings.TrimPrefix(opt, "foreignKey:")
		case strings.HasPrefix(opt, "many2many:"):
			rel.JoinTable = strings.TrimPrefix(opt, "many2many:")
			rel.Type = ManyToMany
		case strings.HasPrefix(opt, "parentKey:"):
			rel.JoinParentKey = strings.TrimPrefix(opt, "parentKey:")
		case strings.HasPrefix(opt, "relatedKey:"):
			rel.JoinRelatedKey = strings.TrimPrefix(opt, "relatedKey:")
		case strings.HasPrefix(opt, "belongsTo:"):
			rel.ForeignKey = strings.TrimPrefix(opt, "belongsTo:")
			rel.Type = BelongsTo
		case opt == "preload":
			rel.Preload = true
		case strings.HasPrefix(opt, "sync:"):
			syncCreate, syncUpdate, syncDelete, err := parseSyncOption(opt)
			if err != nil {
				return err
			}

			rel.SyncCreate = rel.SyncCreate || syncCreate
			rel.SyncUpdate = rel.SyncUpdate || syncUpdate
			rel.SyncDelete = rel.SyncDelete || syncDelete
		case strings.HasPrefix(opt, "syncMode:"):
			syncMany2many, err := parseSyncModeOption(opt)
			if err != nil {
				return err
			}

			rel.SyncMany2many = rel.SyncMany2many || syncMany2many
		}
	}

	return nil
}

func parseSyncOption(opt string) (syncCreate bool, syncUpdate bool, syncDelete bool, err error) {
	value := strings.TrimSpace(strings.TrimPrefix(opt, "sync:"))
	if value == "" {
		return false, false, false, fmt.Errorf("sqlr sync option %q requires at least one mode", opt)
	}

	parts := strings.Split(value, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return false, false, false, fmt.Errorf("sqlr sync option %q contains an empty mode", opt)
		}

		switch part {
		case "create":
			syncCreate = true
		case "update":
			syncUpdate = true
		case "delete":
			syncDelete = true
		default:
			return false, false, false, fmt.Errorf("sqlr sync option %q contains unsupported mode %q", opt, part)
		}
	}

	return syncCreate, syncUpdate, syncDelete, nil
}

func parseSyncModeOption(opt string) (bool, error) {
	value := strings.TrimSpace(strings.TrimPrefix(opt, "syncMode:"))
	if value == "" {
		return false, fmt.Errorf("sqlr syncMode option %q requires a mode", opt)
	}

	if value != "many2many" {
		return false, fmt.Errorf("sqlr syncMode option %q contains unsupported mode %q", opt, value)
	}

	return true, nil
}

// validateRelationshipType determines the final relationship type if not already
// set and validates the type-slice combination.
func validateRelationshipType(rel *Relationship, isSlice bool) error {
	if rel.Type != ManyToMany && rel.Type != BelongsTo {
		if isSlice {
			rel.Type = HasMany
		} else {
			rel.Type = HasOne
		}
	}

	if rel.Type == BelongsTo && isSlice {
		return fmt.Errorf("belongsTo relationship %s must not be a slice", rel.Name)
	}

	if rel.Type != ManyToMany && rel.ForeignKey == "" {
		return fmt.Errorf("relationship %s requires a foreignKey option in the sqlr tag", rel.Name)
	}

	return nil
}

// ResolveRelatedSchema lazily parses the related entity schema on first use,
// caches the result on the relationship, and returns the cached schema on
// subsequent calls. It is safe for concurrent use and returns validation errors
// when the related type cannot be parsed into a valid entity schema.
func (r *Relationship) ResolveRelatedSchema() (*EntitySchema, error) {
	r.resolveOnce.Do(func() {
		r.RelatedSchema, r.resolveErr = parseSchemaType(r.RelatedType, relatedSchemaParseOptions())
	})
	if r.resolveErr != nil {
		return nil, r.resolveErr
	}

	return r.RelatedSchema, nil
}
