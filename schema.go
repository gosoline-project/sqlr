package sqlr

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/jinzhu/inflection"
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
	// IsPrimaryKey is true if the field is tagged with the "primaryKey" db tag option.
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
	JoinTable string
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
}

// EntitySchema holds the parsed metadata for an entity type, including its table
// name, column mappings, primary key, and relationships. It is created once at
// repository construction time via parseSchema and reused for all operations.
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
	insertColumns    []string
	allColumns       []string
	allColumnsAny    []any
	qualifiedColumns []string
	autoPreloads     []preloadEntry
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

// parseSchema parses the entity type E using reflection to build an EntitySchema.
// It reads `db` tags for column mappings and field behavior metadata. The db tag
// format is "column_name[,option1][,option2]..." where options include primaryKey,
// autoCreateTime, autoUpdateTime, foreignKey:<column>, belongsTo:<column>,
// many2many:<table>, and preload.
//
// Relationship examples:
//
//	type Author struct {
//		Entity[int64]
//		Posts []Post `db:"-,foreignKey:author_id"`
//	}
//
//	type AuthorWithProfile struct {
//		Entity[int64]
//		AuthorProfile Profile `db:"-,foreignKey:author_id"`
//	}
//
//	type PostWithAuthor struct {
//		Entity[int64]
//		AuthorID int64  `db:"author_id"`
//		AuthorRef Author `db:"-,belongsTo:author_id"`
//	}
//
//	type Article struct {
//		Entity[int64]
//		Tags []Tag `db:"-,many2many:article_tags"`
//	}
//
// The schema is used at query time for SQL generation, relationship validation,
// and struct hydration.
func parseSchema[E any]() (*EntitySchema, error) {
	var zero E
	t := reflect.TypeOf(zero)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("entity type %s is not a struct", t.Name())
	}

	schema := &EntitySchema{
		TableName:     tableNameFor[E](),
		Relationships: make(map[string]*Relationship),
		entityType:    t,
	}

	if err := parseFields(t, nil, schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema for %s: %w", t.Name(), err)
	}
	setPrimaryKeyFromColumns(schema)
	if err := validateRelationships(schema); err != nil {
		return nil, fmt.Errorf("failed to validate relationships for %s: %w", t.Name(), err)
	}

	if schema.PrimaryKey == nil {
		return nil, fmt.Errorf("entity type %s has no primary key (tag with db:\"...,primaryKey\")", t.Name())
	}

	return schema, nil
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

	err := parseFields(ft, fieldIndex, schema)
	if err != nil {
		return true, err
	}

	return true, nil
}

// parseFieldTag parses a single struct field's db tag and adds it to the schema
// as either a relationship or a column.
func parseFieldTag(field reflect.StructField, fieldIndex []int, schema *EntitySchema) error {
	var dbTag string
	var parts []string
	var colName string
	var options []string

	dbTag = field.Tag.Get("db")
	if dbTag == "" {
		return nil
	}

	parts = strings.Split(dbTag, ",")
	colName = parts[0]
	options = parts[1:]

	if isRelationshipField(options) {
		rel, err := parseRelationship(field, options, fieldIndex)
		if err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}

		schema.Relationships[rel.Name] = rel

		return nil
	}

	if colName == "-" {
		return nil
	}

	col := parseColumnInfo(field, colName, fieldIndex, options)
	schema.Columns = append(schema.Columns, col)

	return nil
}

// parseColumnInfo builds a ColumnInfo from a struct field, column name, and db
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

func setPrimaryKeyFromColumns(schema *EntitySchema) {
	schema.PrimaryKey = nil
	schema.columnByName = make(map[string]*ColumnInfo, len(schema.Columns))
	for i := range schema.Columns {
		schema.columnByName[schema.Columns[i].Name] = &schema.Columns[i]
		if schema.Columns[i].IsPrimaryKey {
			schema.PrimaryKey = &schema.Columns[i]
		}
	}

	cacheSchemaDerivedValues(schema)
}

func validateRelationships(schema *EntitySchema) error {
	for _, rel := range schema.Relationships {
		if rel.Type != BelongsTo {
			continue
		}

		if _, ok := schema.ColumnByName(rel.ForeignKey); !ok {
			return fmt.Errorf("relationship %s belongsTo foreign key column %q not found on entity", rel.Name, rel.ForeignKey)
		}
	}

	return nil
}

func cacheSchemaDerivedValues(schema *EntitySchema) {
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

	autoPreloads := collectAutoPreloads(schema)

	schema.insertColumns = insertCols
	schema.allColumns = allCols
	schema.allColumnsAny = allColsAny
	schema.qualifiedColumns = qualifiedCols
	schema.autoPreloads = autoPreloads
}

func collectAutoPreloads(schema *EntitySchema) []preloadEntry {
	autoPreloads := make([]preloadEntry, 0)
	seenPaths := make(map[string]struct{})
	visitedTypes := make(map[reflect.Type]struct{}, 1)
	if schema.entityType != nil {
		visitedTypes[schema.entityType] = struct{}{}
	}

	collectAutoPreloadsRecursive(schema, "", visitedTypes, seenPaths, &autoPreloads)

	sort.Slice(autoPreloads, func(i, j int) bool {
		return autoPreloads[i].relation < autoPreloads[j].relation
	})

	return autoPreloads
}

func collectAutoPreloadsRecursive(schema *EntitySchema, prefix string, visitedTypes map[reflect.Type]struct{}, seenPaths map[string]struct{}, autoPreloads *[]preloadEntry) {
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
			continue
		}

		visitedTypes[rel.RelatedType] = struct{}{}
		collectAutoPreloadsRecursive(relSchema, relationPath, visitedTypes, seenPaths, autoPreloads)
		delete(visitedTypes, rel.RelatedType)
	}
}

func parseRelatedSchemaForAutoPreload(relatedType reflect.Type) (*EntitySchema, error) {
	var err error

	relatedSchema := &EntitySchema{
		TableName:     inflection.Plural(toSnakeCase(relatedType.Name())),
		Relationships: make(map[string]*Relationship),
		entityType:    relatedType,
	}
	if err = parseFields(relatedType, nil, relatedSchema); err != nil {
		return nil, fmt.Errorf("failed to parse related schema for %s: %w", relatedType.Name(), err)
	}

	return relatedSchema, nil
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

// isRelationshipField returns true if the db tag options indicate a relationship
// (containing foreignKey:, belongsTo:, or many2many: options).
func isRelationshipField(options []string) bool {
	for _, opt := range options {
		if strings.HasPrefix(opt, "foreignKey:") || strings.HasPrefix(opt, "belongsTo:") || strings.HasPrefix(opt, "many2many:") {
			return true
		}
	}

	return false
}

// parseRelationship parses a struct field's db tag options to create a Relationship.
func parseRelationship(field reflect.StructField, options []string, fieldIndex []int) (*Relationship, error) {
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

	applyRelationshipOptions(rel, options)

	return rel, validateRelationshipType(rel, isSlice)
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

// applyRelationshipOptions processes db tag options and sets the appropriate
// fields on the Relationship.
func applyRelationshipOptions(rel *Relationship, options []string) {
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
		case strings.HasPrefix(opt, "belongsTo:"):
			rel.ForeignKey = strings.TrimPrefix(opt, "belongsTo:")
			rel.Type = BelongsTo
		case opt == "preload":
			rel.Preload = true
		}
	}
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
		return fmt.Errorf("relationship %s requires a foreignKey option in the db tag", rel.Name)
	}

	return nil
}

// resolveRelationSchema lazily parses and caches the related entity's schema.
func (r *Relationship) resolveRelationSchema() (*EntitySchema, error) {
	r.resolveOnce.Do(func() {
		t := r.RelatedType

		schema := &EntitySchema{
			TableName:     inflection.Plural(toSnakeCase(t.Name())),
			Relationships: make(map[string]*Relationship),
			entityType:    t,
		}

		if err := parseFields(t, nil, schema); err != nil {
			r.resolveErr = fmt.Errorf("failed to parse related schema for %s: %w", t.Name(), err)

			return
		}
		setPrimaryKeyFromColumns(schema)

		if schema.PrimaryKey == nil {
			r.resolveErr = fmt.Errorf("related entity type %s has no primary key", t.Name())

			return
		}

		r.RelatedSchema = schema
	})
	if r.resolveErr != nil {
		return nil, r.resolveErr
	}

	return r.RelatedSchema, nil
}
