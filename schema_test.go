package sqlr

import (
	"database/sql"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Shared test entity types (referenced by more than one section)
// ============================================================

// schemaNoPrimaryKey is used by both the primary key error test and the
// ResolveRelatedSchema error test.
type schemaNoPrimaryKey struct {
	Name string `db:"name"`
}

// schemaPrimaryKeyEmbedded / schemaPrimaryKeyEntityWithEmbedded are used by
// the primary key tests and the column-indexing test.
type schemaPrimaryKeyEmbedded struct {
	ID int64 `db:"id,primaryKey"`
}

type schemaPrimaryKeyEntityWithEmbedded struct {
	schemaPrimaryKeyEmbedded
	Name string `db:"name"`
}

// schemaPrimaryKeyEntityWithDirectField is used by the primary key tests
// (pointer-to-struct unwrap, string PK, and PrimaryKeyPointsToColumnsEntry).
type schemaPrimaryKeyEntityWithDirectField struct {
	Key   string `db:"key,primaryKey"`
	Value string `db:"value"`
}

// schemaCacheComment / schemaCachePost / schemaCacheAuthor are used by the
// schema caching tests and the ResolveRelatedSchema concurrency test.
type schemaCacheComment struct {
	ID       int64  `db:"id,primaryKey"`
	AuthorID int64  `db:"author_id"`
	Body     string `db:"body"`
}

type schemaCachePost struct {
	ID       int64  `db:"id,primaryKey"`
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
}

type schemaCacheAuthor struct {
	ID       int64                `db:"id,primaryKey"`
	Name     string               `db:"name"`
	Posts    []schemaCachePost    `db:"-,foreignKey:author_id,preload"`
	Comments []schemaCacheComment `db:"-,foreignKey:author_id,preload"`
}

// schemaTableNamerRelated is used by the ResolveRelatedSchema TableNamer test
// and as a field type in schemaInvalidBelongsToRelated.
type schemaTableNamerRelated struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

func (schemaTableNamerRelated) TableName() string { return "custom_related_table" }

// schemaM2MAutoTag / schemaM2MAutoArticle are used by the M2M relationship test
// and the resolveM2MColumnNames derivation tests.
type schemaM2MAutoTag struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

type schemaM2MAutoArticle struct {
	ID   int64              `db:"id,primaryKey"`
	Name string             `db:"name"`
	Tags []schemaM2MAutoTag `db:"-,many2many:"`
}

// schemaM2MColOverrideTag / schemaM2MColOverrideArticle are used by the M2M
// column-override test and the resolveM2MColumnNames override tests.
type schemaM2MColOverrideTag struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

type schemaM2MColOverrideArticle struct {
	ID   int64                     `db:"id,primaryKey"`
	Name string                    `db:"name"`
	Tags []schemaM2MColOverrideTag `db:"-,many2many:override_table,parentKey:art_id,relatedKey:tag_id"`
}

// ============================================================
// Primary key parsing
// ============================================================

type schemaPointerIntPK struct {
	ID   *int64 `db:"id,primaryKey"`
	Name string `db:"name"`
}

// TestParseSchema_PrimaryKeyPointsToColumnsEntry verifies that schema.PrimaryKey
// is the same pointer as the matching entry in schema.Columns, both for an
// embedded primary key and for a directly declared primary key field.
func TestParseSchema_PrimaryKeyPointsToColumnsEntry(t *testing.T) {
	assertPrimaryKeyPointsToColumnsEntry[schemaPrimaryKeyEntityWithEmbedded](t)
	assertPrimaryKeyPointsToColumnsEntry[schemaPrimaryKeyEntityWithDirectField](t)
}

func assertPrimaryKeyPointsToColumnsEntry[E any](t *testing.T) {
	t.Helper()

	schema, err := ParseSchema[E]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)

	primaryKeyIndex := -1
	for i := range schema.Columns {
		if schema.Columns[i].IsPrimaryKey {
			primaryKeyIndex = i
		}
	}

	require.NotEqual(t, -1, primaryKeyIndex)
	assert.Same(t, &schema.Columns[primaryKeyIndex], schema.PrimaryKey)
	assert.Equal(t, schema.Columns[primaryKeyIndex], *schema.PrimaryKey)
}

// TestParseSchema_PointerToStruct_UnwrapsSuccessfully verifies that ParseSchema
// accepts a pointer-to-struct type parameter and produces a valid schema identical
// to parsing the struct directly.
func TestParseSchema_PointerToStruct_UnwrapsSuccessfully(t *testing.T) {
	schema, err := ParseSchema[*schemaPrimaryKeyEntityWithEmbedded]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)
	assert.Equal(t, "id", schema.PrimaryKey.Name)
	assert.Equal(t, []string{"id", "name"}, schema.AllColumns())
}

// TestParseSchema_StringPK_IncludedInInsertColumns verifies that a non-integer
// primary key is not treated as auto-increment and is therefore included in the
// insert column list.
func TestParseSchema_StringPK_IncludedInInsertColumns(t *testing.T) {
	schema, err := ParseSchema[schemaPrimaryKeyEntityWithDirectField]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)
	assert.False(t, schema.PrimaryKey.AutoIncrement)
	assert.Contains(t, schema.InsertColumns(), "key")
	assert.Equal(t, []string{"key", "value"}, schema.InsertColumns())
}

// TestParseSchema_PointerIntegerPK_AutoIncrementTrue verifies that a *int64
// primary key field is inferred as auto-increment and therefore excluded from
// the insert column list.
func TestParseSchema_PointerIntegerPK_AutoIncrementTrue(t *testing.T) {
	schema, err := ParseSchema[schemaPointerIntPK]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)
	assert.True(t, schema.PrimaryKey.AutoIncrement)
	assert.Equal(t, []string{"name"}, schema.InsertColumns())
}

// TestParseSchema_NoPrimaryKey_ReturnsError verifies that parsing a struct with
// no primaryKey-tagged field fails with an appropriate error message.
func TestParseSchema_NoPrimaryKey_ReturnsError(t *testing.T) {
	_, err := ParseSchema[schemaNoPrimaryKey]()
	require.Error(t, err)
	require.ErrorContains(t, err, "has no primary key")
}

// ============================================================
// Column indexing and annotations
// ============================================================

type schemaAutoTimestamps struct {
	ID        int64     `db:"id,primaryKey"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at,autoCreateTime"`
	UpdatedAt time.Time `db:"updated_at,autoUpdateTime"`
}

type schemaPointerAutoTimestamps struct {
	ID        int64      `db:"id,primaryKey"`
	Name      string     `db:"name"`
	CreatedAt *time.Time `db:"created_at,autoCreateTime"`
	UpdatedAt *time.Time `db:"updated_at,autoUpdateTime"`
}

type schemaInvalidAutoTimestamps struct {
	ID        int64  `db:"id,primaryKey"`
	Name      string `db:"name"`
	CreatedAt string `db:"created_at,autoCreateTime"`
	UpdatedAt string `db:"updated_at,autoUpdateTime"`
}

type schemaDashField struct {
	ID      int64  `db:"id,primaryKey"`
	Name    string `db:"name"`
	Ignored string `db:"-"`
}

// TestParseSchema_ColumnByNamePointsToColumnsEntries verifies that every entry
// returned by ColumnByName is the same pointer as the corresponding entry in
// schema.Columns, ensuring the map and the slice stay in sync.
func TestParseSchema_ColumnByNamePointsToColumnsEntries(t *testing.T) {
	schema, err := ParseSchema[schemaPrimaryKeyEntityWithEmbedded]()
	require.NoError(t, err)
	require.Len(t, schema.columnByName, len(schema.Columns))

	for i := range schema.Columns {
		col := &schema.Columns[i]
		mapped, ok := schema.ColumnByName(col.Name)
		require.True(t, ok)
		assert.Same(t, col, mapped)
	}
}

// TestParseSchema_AutoCreateTimeAndUpdateTime verifies that columns annotated
// with autoCreateTime and autoUpdateTime have exactly the correct flags set and
// the opposite flag cleared.
func TestParseSchema_AutoCreateTimeAndUpdateTime(t *testing.T) {
	schema, err := ParseSchema[schemaAutoTimestamps]()
	require.NoError(t, err)

	createdAt, ok := schema.ColumnByName("created_at")
	require.True(t, ok)
	assert.True(t, createdAt.AutoCreateTime)
	assert.False(t, createdAt.AutoUpdateTime)

	updatedAt, ok := schema.ColumnByName("updated_at")
	require.True(t, ok)
	assert.True(t, updatedAt.AutoUpdateTime)
	assert.False(t, updatedAt.AutoCreateTime)
}

func TestParseSchema_AutoTimestampPointerFields_AreAccepted(t *testing.T) {
	schema, err := ParseSchema[schemaPointerAutoTimestamps]()
	require.NoError(t, err)

	createdAt, ok := schema.ColumnByName("created_at")
	require.True(t, ok)
	assert.True(t, createdAt.AutoCreateTime)
	assert.False(t, createdAt.AutoUpdateTime)

	updatedAt, ok := schema.ColumnByName("updated_at")
	require.True(t, ok)
	assert.True(t, updatedAt.AutoUpdateTime)
	assert.False(t, updatedAt.AutoCreateTime)
}

func TestParseSchema_AutoTimestampWrongType_ReturnsError(t *testing.T) {
	_, err := ParseSchema[schemaInvalidAutoTimestamps]()
	require.Error(t, err)
	require.ErrorContains(t, err, "auto timestamp tags")
}

func TestParseSchema_DuplicateColumnNames_ReturnsError(t *testing.T) {
	type duplicateColumns struct {
		ID    int64  `db:"id,primaryKey"`
		Name1 string `db:"name"`
		Name2 string `db:"name"`
	}

	_, err := ParseSchema[duplicateColumns]()
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate column name \"name\"")
}

func TestParseSchema_MultiplePrimaryKeys_ReturnsError(t *testing.T) {
	type multiplePrimaryKeys struct {
		ID    int64  `db:"id,primaryKey"`
		AltID int64  `db:"alt_id,primaryKey"`
		Name  string `db:"name"`
	}

	_, err := ParseSchema[multiplePrimaryKeys]()
	require.Error(t, err)
	require.ErrorContains(t, err, "defines multiple primary keys")
}

// TestParseSchema_DbDashWithoutRelationship_FieldSkipped verifies that a field
// tagged db:"-" (without any relationship option) is skipped entirely — it is
// not added to the column list and no relationship is registered for it.
func TestParseSchema_DbDashWithoutRelationship_FieldSkipped(t *testing.T) {
	schema, err := ParseSchema[schemaDashField]()
	require.NoError(t, err)

	_, ok := schema.ColumnByName("-")
	assert.False(t, ok)
	assert.Equal(t, []string{"id", "name"}, schema.AllColumns())
	assert.Empty(t, schema.Relationships)
}

// ============================================================
// Schema caching (derived values computed and cached on first call)
// ============================================================

// TestParseSchema_CachesDerivedValues verifies that InsertColumns, AllColumns,
// QualifiedColumns, and AutoPreloads all return consistent values and that each
// method returns the same underlying slice that is stored in the cached field,
// confirming no redundant recomputation.
func TestParseSchema_CachesDerivedValues(t *testing.T) {
	schema, err := ParseSchema[schemaCacheAuthor]()
	require.NoError(t, err)

	assert.Equal(t, []string{"name"}, schema.InsertColumns())
	assert.Equal(t, []string{"id", "name"}, schema.AllColumns())
	assert.Equal(t, []string{
		"`schema_cache_authors`.`id`",
		"`schema_cache_authors`.`name`",
	}, schema.QualifiedColumns())
	assert.Equal(t, []preloadEntry{
		{relation: "Comments"},
		{relation: "Posts"},
	}, schema.AutoPreloads())

	assert.Equal(t, schema.insertColumns, schema.InsertColumns())
	assert.Equal(t, schema.allColumns, schema.AllColumns())
	assert.Equal(t, []any{"id", "name"}, schema.allColumnsAny)
	assert.Equal(t, schema.qualifiedColumns, schema.QualifiedColumns())
	assert.Equal(t, schema.autoPreloads, schema.AutoPreloads())
}

// TestValidRelationNames_SortedOutput verifies that ValidRelationNames returns
// all relationship names in sorted order.
func TestValidRelationNames_SortedOutput(t *testing.T) {
	schema, err := ParseSchema[schemaCacheAuthor]()
	require.NoError(t, err)

	names := schema.ValidRelationNames()
	assert.Equal(t, []string{"Comments", "Posts"}, names)
}

// ============================================================
// Relationship parsing: HasOne
// ============================================================

type schemaHasOneProfile struct {
	ID       int64  `db:"id,primaryKey"`
	AuthorID int64  `db:"author_id"`
	Bio      string `db:"bio"`
}

type schemaHasOneAuthor struct {
	ID      int64               `db:"id,primaryKey"`
	Name    string              `db:"name"`
	Profile schemaHasOneProfile `db:"-,foreignKey:author_id"`
}

// TestParseSchema_HasOne_Relationship verifies that a non-slice struct field
// tagged with foreignKey is parsed as a HasOne relationship with the correct
// foreign key column name.
func TestParseSchema_HasOne_Relationship(t *testing.T) {
	schema, err := ParseSchema[schemaHasOneAuthor]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Profile"]
	require.True(t, ok)
	assert.Equal(t, HasOne, rel.Type)
	assert.Equal(t, "author_id", rel.ForeignKey)
}

// ============================================================
// Relationship parsing: BelongsTo
// ============================================================

type schemaBelongsToAuthor struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

type schemaBelongsToPost struct {
	ID       int64                 `db:"id,primaryKey"`
	AuthorID int64                 `db:"author_id"`
	Title    string                `db:"title"`
	Author   schemaBelongsToAuthor `db:"-,belongsTo:author_id"`
}

type schemaBelongsToPointerPost struct {
	ID       int64                  `db:"id,primaryKey"`
	AuthorID int64                  `db:"author_id"`
	Title    string                 `db:"title"`
	Author   *schemaBelongsToAuthor `db:"-,belongsTo:author_id"`
}

type schemaBelongsToPostInvalidSlice struct {
	ID       int64                   `db:"id,primaryKey"`
	AuthorID int64                   `db:"author_id"`
	Title    string                  `db:"title"`
	Author   []schemaBelongsToAuthor `db:"-,belongsTo:author_id"`
}

type schemaBelongsToPostMissingFK struct {
	ID     int64                 `db:"id,primaryKey"`
	Title  string                `db:"title"`
	Author schemaBelongsToAuthor `db:"-,belongsTo:author_id"`
}

// TestParseSchema_BelongsTo verifies that a non-slice struct field tagged with
// belongsTo is parsed as a BelongsTo relationship with the correct foreign key.
func TestParseSchema_BelongsTo(t *testing.T) {
	schema, err := ParseSchema[schemaBelongsToPost]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Author"]
	require.True(t, ok)
	assert.Equal(t, BelongsTo, rel.Type)
	assert.Equal(t, "author_id", rel.ForeignKey)
}

// TestParseSchema_BelongsToPointer verifies that a pointer scalar field tagged
// with belongsTo is parsed the same way as the value form.
func TestParseSchema_BelongsToPointer(t *testing.T) {
	schema, err := ParseSchema[schemaBelongsToPointerPost]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Author"]
	require.True(t, ok)
	assert.Equal(t, BelongsTo, rel.Type)
	assert.Equal(t, "author_id", rel.ForeignKey)
	assert.Equal(t, reflect.TypeOf(schemaBelongsToAuthor{}), rel.RelatedType)
}

// TestParseSchema_BelongsToSliceRejected verifies that a slice field tagged as
// belongsTo is rejected with a descriptive error, because BelongsTo must be a
// single struct, not a collection.
func TestParseSchema_BelongsToSliceRejected(t *testing.T) {
	_, err := ParseSchema[schemaBelongsToPostInvalidSlice]()
	require.Error(t, err)
	require.ErrorContains(t, err, "belongsTo relationship Author must not be a slice")
}

// TestParseSchema_BelongsToMissingForeignKeyColumnRejected verifies that a
// BelongsTo tag referencing a foreign key column that does not exist on the
// entity struct is rejected with a descriptive error.
func TestParseSchema_BelongsToMissingForeignKeyColumnRejected(t *testing.T) {
	_, err := ParseSchema[schemaBelongsToPostMissingFK]()
	require.Error(t, err)
	require.ErrorContains(t, err, "belongsTo foreign key column \"author_id\" not found on entity")
}

// ============================================================
// Relationship parsing: ManyToMany
// ============================================================

type schemaManyToManyTag struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

type schemaManyToManyArticle struct {
	ID   int64                 `db:"id,primaryKey"`
	Name string                `db:"name"`
	Tags []schemaManyToManyTag `db:"-,many2many:article_tags"`
}

type schemaHasManyPointerAuthor struct {
	ID    int64                  `db:"id,primaryKey"`
	Name  string                 `db:"name"`
	Posts []*schemaBelongsToPost `db:"-,foreignKey:author_id"`
}

type schemaManyToManyPointerArticle struct {
	ID   int64                  `db:"id,primaryKey"`
	Name string                 `db:"name"`
	Tags []*schemaManyToManyTag `db:"-,many2many:article_tags"`
}

// TestParseSchema_ManyToMany_Relationship verifies that a slice field tagged
// with many2many is parsed as a ManyToMany relationship with the correct join
// table name and no foreign key.
func TestParseSchema_ManyToMany_Relationship(t *testing.T) {
	schema, err := ParseSchema[schemaManyToManyArticle]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Tags"]
	require.True(t, ok)
	assert.Equal(t, ManyToMany, rel.Type)
	assert.Equal(t, "article_tags", rel.JoinTable)
	assert.Equal(t, "", rel.ForeignKey)
}

// TestParseSchema_PointerCollectionRelationships verifies that pointer-element
// slices are accepted for HasMany and ManyToMany relations and resolve the same
// related entity type as value-element slices.
func TestParseSchema_PointerCollectionRelationships(t *testing.T) {
	authorSchema, err := ParseSchema[schemaHasManyPointerAuthor]()
	require.NoError(t, err)

	postsRel, ok := authorSchema.Relationships["Posts"]
	require.True(t, ok)
	assert.Equal(t, HasMany, postsRel.Type)
	assert.Equal(t, "author_id", postsRel.ForeignKey)
	assert.Equal(t, reflect.TypeOf(schemaBelongsToPost{}), postsRel.RelatedType)

	articleSchema, err := ParseSchema[schemaManyToManyPointerArticle]()
	require.NoError(t, err)

	tagsRel, ok := articleSchema.Relationships["Tags"]
	require.True(t, ok)
	assert.Equal(t, ManyToMany, tagsRel.Type)
	assert.Equal(t, "article_tags", tagsRel.JoinTable)
	assert.Equal(t, reflect.TypeOf(schemaManyToManyTag{}), tagsRel.RelatedType)
}

// ============================================================
// Auto-preloads
// ============================================================

type schemaMixedPreloadPost struct {
	ID   int64  `db:"id,primaryKey"`
	Body string `db:"body"`
}

type schemaMixedPreloadComment struct {
	ID   int64  `db:"id,primaryKey"`
	Body string `db:"body"`
}

type schemaMixedPreloadAuthor struct {
	ID       int64                       `db:"id,primaryKey"`
	Name     string                      `db:"name"`
	Posts    []schemaMixedPreloadPost    `db:"-,foreignKey:author_id,preload"`
	Comments []schemaMixedPreloadComment `db:"-,foreignKey:author_id"`
}

type schemaNestedAutoComment struct {
	ID     int64  `db:"id,primaryKey"`
	PostID int64  `db:"post_id"`
	Body   string `db:"body"`
}

type schemaNestedAutoPost struct {
	ID       int64                     `db:"id,primaryKey"`
	AuthorID int64                     `db:"author_id"`
	Title    string                    `db:"title"`
	Comments []schemaNestedAutoComment `db:"-,foreignKey:post_id,preload"`
}

type schemaNestedAutoAuthor struct {
	ID    int64                  `db:"id,primaryKey"`
	Name  string                 `db:"name"`
	Posts []schemaNestedAutoPost `db:"-,foreignKey:author_id,preload"`
}

type schemaCircularAutoParent struct {
	ID       int64                     `db:"id,primaryKey"`
	Name     string                    `db:"name"`
	Children []schemaCircularAutoChild `db:"-,foreignKey:parent_id,preload"`
}

type schemaCircularAutoChild struct {
	ID       int64                     `db:"id,primaryKey"`
	ParentID int64                     `db:"parent_id"`
	Body     string                    `db:"body"`
	Parent   *schemaCircularAutoParent `db:"-,belongsTo:parent_id,preload"`
}

// TestParseSchema_MixedPreloadAndNonPreload_AutoPreloads verifies that only
// relations explicitly tagged with the "preload" option appear in AutoPreloads,
// and relations without that tag are excluded.
func TestParseSchema_MixedPreloadAndNonPreload_AutoPreloads(t *testing.T) {
	schema, err := ParseSchema[schemaMixedPreloadAuthor]()
	require.NoError(t, err)

	autoPreloads := schema.AutoPreloads()
	require.Len(t, autoPreloads, 1)
	assert.Equal(t, "Posts", autoPreloads[0].relation)
}

// TestParseSchema_AutoPreloadsNested verifies that AutoPreloads follows tagged
// preload chains transitively, producing dot-separated paths for nested relations
// (e.g. "Posts.Comments" when both levels carry the preload tag).
func TestParseSchema_AutoPreloadsNested(t *testing.T) {
	schema, err := ParseSchema[schemaNestedAutoAuthor]()
	require.NoError(t, err)
	assert.Equal(t, []preloadEntry{
		{relation: "Posts"},
		{relation: "Posts.Comments"},
	}, schema.AutoPreloads())
}

// TestParseSchema_AutoPreloadsCircular verifies that AutoPreloads terminates
// without panicking when two entity types reference each other via preload tags,
// and that the resulting paths are correct up to the cycle boundary.
func TestParseSchema_AutoPreloadsCircular(t *testing.T) {
	var schema *EntitySchema
	require.NotPanics(t, func() {
		var err error
		schema, err = ParseSchema[schemaCircularAutoParent]()
		require.NoError(t, err)
	})
	assert.Equal(t, []preloadEntry{
		{relation: "Children"},
		{relation: "Children.Parent"},
	}, schema.AutoPreloads())
}

// ============================================================
// isRelationshipField helper
// ============================================================

// TestIsRelationshipField_ForeignKey verifies that a foreignKey option is
// recognised as a relationship field indicator.
func TestIsRelationshipField_ForeignKey(t *testing.T) {
	assert.True(t, isRelationshipField([]string{"foreignKey:post_id"}))
}

// TestIsRelationshipField_ManyToMany verifies that a many2many option is
// recognised as a relationship field indicator.
func TestIsRelationshipField_ManyToMany(t *testing.T) {
	assert.True(t, isRelationshipField([]string{"many2many:post_tags"}))
}

// TestIsRelationshipField_BelongsTo verifies that a belongsTo option is
// recognised as a relationship field indicator.
func TestIsRelationshipField_BelongsTo(t *testing.T) {
	assert.True(t, isRelationshipField([]string{"belongsTo:author_id"}))
}

// TestIsRelationshipField_NoRelationshipOption_ReturnsFalse verifies that a
// non-relationship option (e.g. primaryKey) is not mistaken for a relationship.
func TestIsRelationshipField_NoRelationshipOption_ReturnsFalse(t *testing.T) {
	assert.False(t, isRelationshipField([]string{"primaryKey"}))
}

// TestIsRelationshipField_EmptySlice_ReturnsFalse verifies that an empty option
// slice does not trigger relationship detection.
func TestIsRelationshipField_EmptySlice_ReturnsFalse(t *testing.T) {
	assert.False(t, isRelationshipField([]string{}))
}

// ============================================================
// ResolveRelatedSchema
// ============================================================

// TestRelationshipResolveRelatedSchema_Concurrent verifies that concurrent
// calls to ResolveRelatedSchema on the same Relationship always return the
// identical *EntitySchema pointer, confirming that lazy initialisation is
// thread-safe and the result is cached after the first resolution.
func TestRelationshipResolveRelatedSchema_Concurrent(t *testing.T) {
	rel := &Relationship{
		RelatedType: reflect.TypeOf(schemaCachePost{}),
	}

	const workers = 32
	schemas := make(chan *EntitySchema, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for range workers {
		go func() {
			defer wg.Done()

			s, err := rel.ResolveRelatedSchema()
			if err != nil {
				errs <- err

				return
			}

			schemas <- s
		}()
	}

	wg.Wait()
	close(errs)
	close(schemas)

	for err := range errs {
		require.NoError(t, err)
	}

	var first *EntitySchema
	for s := range schemas {
		if first == nil {
			first = s
		}
		assert.Same(t, first, s)
	}

	require.NotNil(t, first)
	assert.Same(t, first, rel.RelatedSchema)
}

// TestResolveRelatedSchema_RelatedTypeNoPrimaryKey_ReturnsError verifies that
// ResolveRelatedSchema returns an error when the related entity type has no
// primary key, rather than producing a silently broken schema.
func TestResolveRelatedSchema_RelatedTypeNoPrimaryKey_ReturnsError(t *testing.T) {
	rel := &Relationship{
		RelatedType: reflect.TypeOf(schemaNoPrimaryKey{}),
	}

	_, err := rel.ResolveRelatedSchema()
	require.Error(t, err)
	require.ErrorContains(t, err, "has no primary key")
}

// schemaInvalidBelongsToRelated has a BelongsTo relationship where the declared
// FK column ("nonexistent_id") is not present as a column on the struct.
type schemaInvalidBelongsToRelated struct {
	ID    int64                   `db:"id,primaryKey"`
	Name  string                  `db:"name"`
	Owner schemaTableNamerRelated `db:"-,belongsTo:nonexistent_id"` // FK column missing
}

// schemaInvalidBelongsToParent has a relationship to schemaInvalidBelongsToRelated,
// so that ResolveRelatedSchema will attempt to parse schemaInvalidBelongsToRelated.
type schemaInvalidBelongsToParent struct {
	ID        int64                         `db:"id,primaryKey"`
	Related   schemaInvalidBelongsToRelated `db:"-,belongsTo:related_id"`
	RelatedID int64                         `db:"related_id"`
}

// TestResolveRelatedSchema_ReturnsErrorForInvalidBelongsToFK verifies that
// ResolveRelatedSchema propagates a parse error from a related entity whose
// BelongsTo tag references a non-existent FK column, and that the error message
// identifies the missing column name.
func TestResolveRelatedSchema_ReturnsErrorForInvalidBelongsToFK(t *testing.T) {
	schema, err := ParseSchema[schemaInvalidBelongsToParent]()
	require.NoError(t, err, "parsing the parent schema itself must succeed")

	rel, ok := schema.Relationships["Related"]
	require.True(t, ok, "expected relationship Related on parent")

	_, err = rel.ResolveRelatedSchema()
	require.Error(t, err, "ResolveRelatedSchema must return an error for an invalid BelongsTo FK on the related entity")
	assert.Contains(t, err.Error(), "nonexistent_id")
}

// schemaTableNamerParent has a relationship to schemaTableNamerRelated. When the
// related schema is lazily resolved, it must use the custom TableName().
type schemaTableNamerParent struct {
	ID        int64                   `db:"id,primaryKey"`
	Related   schemaTableNamerRelated `db:"-,belongsTo:related_id"`
	RelatedID int64                   `db:"related_id"`
}

// TestResolveRelatedSchema_HonoursTableNamerOnRelatedEntity verifies that when
// the related entity implements TableNamer, ResolveRelatedSchema uses its custom
// table name rather than the auto-derived snake_case name.
func TestResolveRelatedSchema_HonoursTableNamerOnRelatedEntity(t *testing.T) {
	schema, err := ParseSchema[schemaTableNamerParent]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Related"]
	require.True(t, ok, "expected relationship Related")

	relSchema, err := rel.ResolveRelatedSchema()
	require.NoError(t, err)

	assert.Equal(t, "custom_related_table", relSchema.TableName,
		"ResolveRelatedSchema must honour TableNamer on the related entity")
}

// TestResolveRelatedSchema_FromParsedRelationship_ResolvesNestedSchema verifies
// that a relationship obtained from ParseSchema can resolve its related schema
// through the exported API and continue traversing nested relationships.
func TestResolveRelatedSchema_FromParsedRelationship_ResolvesNestedSchema(t *testing.T) {
	schema, err := ParseSchema[schemaNestedAutoAuthor]()
	require.NoError(t, err)

	postsRel, ok := schema.Relationships["Posts"]
	require.True(t, ok, "expected relationship Posts")

	postSchema, err := postsRel.ResolveRelatedSchema()
	require.NoError(t, err)
	require.NotNil(t, postSchema)

	commentsRel, ok := postSchema.Relationships["Comments"]
	require.True(t, ok, "expected nested relationship Comments")

	commentSchema, err := commentsRel.ResolveRelatedSchema()
	require.NoError(t, err)
	require.NotNil(t, commentSchema)
	assert.Equal(t, tableNameForType(reflect.TypeOf(schemaNestedAutoComment{})), commentSchema.TableName)
}

// TestEntitySchemaResolveRelationPath_ValidRootRelation verifies that a direct
// relation path resolves to its terminal relationship and related schema.
func TestEntitySchemaResolveRelationPath_ValidRootRelation(t *testing.T) {
	schema, err := ParseSchema[schemaHasOneAuthor]()
	require.NoError(t, err)

	rel, relSchema, err := schema.ResolveRelationPath("Profile")
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.NotNil(t, relSchema)

	assert.Equal(t, "Profile", rel.Name)
	assert.Equal(t, HasOne, rel.Type)
	assert.Equal(t, tableNameForType(reflect.TypeOf(schemaHasOneProfile{})), relSchema.TableName)
}

// TestEntitySchemaResolveRelationPath_ValidNestedRelation verifies that a dotted
// relation path resolves segment-by-segment and returns the leaf schema.
func TestEntitySchemaResolveRelationPath_ValidNestedRelation(t *testing.T) {
	schema, err := ParseSchema[schemaNestedAutoAuthor]()
	require.NoError(t, err)

	rel, relSchema, err := schema.ResolveRelationPath("Posts.Comments")
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.NotNil(t, relSchema)

	assert.Equal(t, "Comments", rel.Name)
	assert.Equal(t, HasMany, rel.Type)
	assert.Equal(t, tableNameForType(reflect.TypeOf(schemaNestedAutoComment{})), relSchema.TableName)
}

// TestEntitySchemaResolveRelationPath_InvalidRootSegment verifies that a missing
// top-level relation returns a generic schema-level error.
func TestEntitySchemaResolveRelationPath_InvalidRootSegment(t *testing.T) {
	schema, err := ParseSchema[schemaHasOneAuthor]()
	require.NoError(t, err)

	_, _, err = schema.ResolveRelationPath("Unknown")
	require.Error(t, err)
	assert.ErrorContains(t, err, `relation "Unknown" not found on model schemaHasOneAuthor`)
	assert.ErrorContains(t, err, "valid relations: [Profile]")
}

// TestEntitySchemaResolveRelationPath_InvalidNestedSegment verifies that a
// missing nested relation reports the accumulated dotted path.
func TestEntitySchemaResolveRelationPath_InvalidNestedSegment(t *testing.T) {
	schema, err := ParseSchema[schemaNestedAutoAuthor]()
	require.NoError(t, err)

	_, _, err = schema.ResolveRelationPath("Posts.Unknown")
	require.Error(t, err)
	assert.ErrorContains(t, err, `relation "Posts.Unknown" not found on model schemaNestedAutoPost`)
	assert.ErrorContains(t, err, "valid relations: [Comments]")
}

// TestEntitySchemaResolveRelationPath_MalformedPath verifies that malformed
// dotted paths are rejected centrally by the schema API.
func TestEntitySchemaResolveRelationPath_MalformedPath(t *testing.T) {
	schema, err := ParseSchema[schemaNestedAutoAuthor]()
	require.NoError(t, err)

	testCases := []struct {
		name string
		path string
		err  string
	}{
		{name: "empty", path: "", err: "relation path must not be empty"},
		{name: "leading dot", path: ".Posts", err: `relation path ".Posts" is malformed`},
		{name: "trailing dot", path: "Posts.", err: `relation path "Posts." is malformed`},
		{name: "double dots", path: "Posts..Comments", err: `relation path "Posts..Comments" is malformed`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, resolveErr := schema.ResolveRelationPath(testCase.path)
			require.Error(t, resolveErr)
			assert.ErrorContains(t, resolveErr, testCase.err)
		})
	}
}

// TestEntitySchemaResolveRelationPath_RelatedSchemaResolutionFailure verifies
// that ResolveRelationPath propagates related schema parse failures generically.
func TestEntitySchemaResolveRelationPath_RelatedSchemaResolutionFailure(t *testing.T) {
	schema, err := ParseSchema[schemaInvalidBelongsToParent]()
	require.NoError(t, err)

	_, _, err = schema.ResolveRelationPath("Related")
	require.Error(t, err)
	assert.ErrorContains(t, err, `failed to resolve schema for relation "Related"`)
	assert.ErrorContains(t, err, "nonexistent_id")
}

// TestEntitySchemaResolveRelationPath_ManyToMany verifies that many-to-many
// relations resolve successfully because path validation is schema-based rather
// than operation-specific.
func TestEntitySchemaResolveRelationPath_ManyToMany(t *testing.T) {
	schema, err := ParseSchema[schemaManyToManyArticle]()
	require.NoError(t, err)

	rel, relSchema, err := schema.ResolveRelationPath("Tags")
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.NotNil(t, relSchema)

	assert.Equal(t, "Tags", rel.Name)
	assert.Equal(t, ManyToMany, rel.Type)
	assert.Equal(t, tableNameForType(reflect.TypeOf(schemaManyToManyTag{})), relSchema.TableName)
}

// ============================================================
// Validation errors (non-relationship / structural)
// ============================================================

type schemaMissingFKRelation struct {
	ID    int64               `db:"id,primaryKey"`
	Other schemaHasOneProfile `db:"-,foreignKey:"`
}

type schemaRelationNonStruct struct {
	ID    int64  `db:"id,primaryKey"`
	Other string `db:"-,foreignKey:other_id"`
}

// TestParseSchema_NonStructType_ReturnsError verifies that passing a primitive
// type (e.g. int) to ParseSchema fails with an error indicating it is not a struct.
func TestParseSchema_NonStructType_ReturnsError(t *testing.T) {
	_, err := ParseSchema[int]()
	require.Error(t, err)
	require.ErrorContains(t, err, "is not a struct")
}

// TestParseSchema_MissingForeignKey_ReturnsError verifies that a foreignKey tag
// with an empty key value is rejected with an error stating that a foreignKey
// option is required.
func TestParseSchema_MissingForeignKey_ReturnsError(t *testing.T) {
	_, err := ParseSchema[schemaMissingFKRelation]()
	require.Error(t, err)
	require.ErrorContains(t, err, "requires a foreignKey option")
}

// TestParseSchema_RelationshipOnNonStructField_ReturnsError verifies that
// applying a relationship tag (foreignKey) to a non-struct field (e.g. string)
// is rejected with an error indicating the field must be a struct or slice of structs.
func TestParseSchema_RelationshipOnNonStructField_ReturnsError(t *testing.T) {
	_, err := ParseSchema[schemaRelationNonStruct]()
	require.Error(t, err)
	require.ErrorContains(t, err, "must be a struct or slice of structs")
}

// ============================================================
// Untagged fields / auto-detection
// ============================================================

// schemaUntaggedPublic has a mix of tagged and untagged public fields plus an
// unexported field. The untagged public fields should be mapped to snake_case
// column names; the unexported field should be silently skipped.
type schemaUntaggedPublic struct {
	ID        int64  `db:"id,primaryKey"`
	FirstName string // → first_name
	LastName  string // → last_name
	userScore int    //nolint:unused // unexported — must be skipped
}

// schemaUntaggedFieldNameVariants exercises the snake_case transformer on a
// variety of field naming patterns including digit→uppercase transitions.
type schemaUntaggedFieldNameVariants struct {
	ID            int64  `db:"id,primaryKey"`
	UserID        int64  // → user_id
	HTTPCode      int    // → http_code
	FullName      string // → full_name
	Uint64Value   int64  // → uint64_value (digit→uppercase gets underscore)
	Int32Counter  int    // → int32_counter (digit→uppercase gets underscore)
	Field123Thing string // → field123_thing (digit→uppercase gets underscore)
}

// schemaAutoHasManyChild is the related entity for the HasMany auto-detection test.
type schemaAutoHasManyChild struct {
	ID       int64  `db:"id,primaryKey"`
	ParentID int64  // → parent_id (auto-column)
	Body     string // → body (auto-column)
}

// schemaAutoHasManyParent has an untagged slice field — should be auto-detected
// as HasMany with FK = "schema_auto_has_many_parent_id".
type schemaAutoHasManyParent struct {
	ID       int64                    `db:"id,primaryKey"`
	Name     string                   // → name (auto-column)
	Children []schemaAutoHasManyChild // HasMany, FK = "schema_auto_has_many_parent_id"
}

// schemaAutoBelongsToOwner is the related entity for the BelongsTo auto-detection test.
type schemaAutoBelongsToOwner struct {
	ID   int64  `db:"id,primaryKey"`
	Name string // → name
}

// schemaAutoBelongsToItem has an untagged non-slice struct field — should be
// auto-detected as BelongsTo with FK = "owner_id" (derived from field name "Owner").
type schemaAutoBelongsToItem struct {
	ID      int64                    `db:"id,primaryKey"`
	OwnerID int64                    // → owner_id (auto-column; also serves as the BelongsTo FK)
	Owner   schemaAutoBelongsToOwner // BelongsTo, FK = "owner_id"
}

// TestParseSchema_UntaggedPublicFields_MappedToSnakeCase verifies that public
// fields without a db tag are automatically mapped to snake_case column names.
func TestParseSchema_UntaggedPublicFields_MappedToSnakeCase(t *testing.T) {
	schema, err := ParseSchema[schemaUntaggedPublic]()
	require.NoError(t, err)

	assert.Equal(t, []string{"id", "first_name", "last_name"}, schema.AllColumns())

	_, ok := schema.ColumnByName("first_name")
	assert.True(t, ok, "expected column first_name")

	_, ok = schema.ColumnByName("last_name")
	assert.True(t, ok, "expected column last_name")
}

// TestParseSchema_UntaggedPublicField_UnexportedFieldSkipped verifies that
// unexported fields are silently excluded from the column list even when other
// untagged exported fields on the same struct are auto-mapped.
func TestParseSchema_UntaggedPublicField_UnexportedFieldSkipped(t *testing.T) {
	schema, err := ParseSchema[schemaUntaggedPublic]()
	require.NoError(t, err)

	// Only the three exported fields should be present (id, first_name, last_name).
	assert.Len(t, schema.AllColumns(), 3)
}

// TestParseSchema_UntaggedPublicField_SnakeCaseTransformerVariants verifies the
// snake_case transformer handles a range of common naming patterns correctly:
// consecutive capitals (UserID → user_id), all-caps acronyms (HTTPCode → http_code),
// plain PascalCase (FullName → full_name), and digit→uppercase transitions
// (Uint64Value → uint64_value, Int32Counter → int32_counter).
func TestParseSchema_UntaggedPublicField_SnakeCaseTransformerVariants(t *testing.T) {
	schema, err := ParseSchema[schemaUntaggedFieldNameVariants]()
	require.NoError(t, err)

	expectedCols := []string{"id", "user_id", "http_code", "full_name", "uint64_value", "int32_counter", "field123_thing"}
	assert.Equal(t, expectedCols, schema.AllColumns())
}

// TestParseSchema_UntaggedSliceField_AutoDetectsHasMany verifies that an
// untagged slice field whose element type is a struct is automatically detected
// as a HasMany relationship, with the foreign key derived from the parent type name.
func TestParseSchema_UntaggedSliceField_AutoDetectsHasMany(t *testing.T) {
	schema, err := ParseSchema[schemaAutoHasManyParent]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Children"]
	require.True(t, ok, "expected relationship Children")

	assert.Equal(t, HasMany, rel.Type)
	assert.Equal(t, "schema_auto_has_many_parent_id", rel.ForeignKey)
}

// TestParseSchema_UntaggedStructField_AutoDetectsBelongsTo verifies that an
// untagged non-slice struct field is automatically detected as a BelongsTo
// relationship, with the foreign key derived from the field name.
func TestParseSchema_UntaggedStructField_AutoDetectsBelongsTo(t *testing.T) {
	schema, err := ParseSchema[schemaAutoBelongsToItem]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Owner"]
	require.True(t, ok, "expected relationship Owner")

	assert.Equal(t, BelongsTo, rel.Type)
	assert.Equal(t, "owner_id", rel.ForeignKey)
}

// TestParseSchema_UntaggedStructField_AutoDetectedBelongsToFKMustExist verifies
// that auto-detected BelongsTo succeeds when the inferred FK column (owner_id)
// is present on the entity, and does not produce a spurious error.
func TestParseSchema_UntaggedStructField_AutoDetectedBelongsToFKMustExist(t *testing.T) {
	// schemaAutoBelongsToItem has OwnerID (→ "owner_id") which satisfies the
	// BelongsTo FK requirement; schema parsing must succeed.
	_, err := ParseSchema[schemaAutoBelongsToItem]()
	require.NoError(t, err)
}

// ============================================================
// Value-type struct exclusion from auto-relationship detection
// ============================================================

// schemaValueTypeFields contains untagged fields of stdlib value-type structs
// that must be mapped as columns, not auto-detected as relationships.
type schemaValueTypeFields struct {
	ID        int64          `db:"id,primaryKey"`
	CreatedAt time.Time      // → created_at (column, NOT a relationship)
	NullName  sql.NullString // → null_name (column, NOT a relationship)
}

// TestParseSchema_UntaggedTimeField_MappedAsColumn verifies that an untagged
// time.Time field is treated as a plain column (not auto-detected as a
// BelongsTo relationship), because "time" is in the valueTypePackages exclusion
// list.
func TestParseSchema_UntaggedTimeField_MappedAsColumn(t *testing.T) {
	schema, err := ParseSchema[schemaValueTypeFields]()
	require.NoError(t, err)

	// time.Time and sql.NullString must appear as columns, not relationships.
	assert.Empty(t, schema.Relationships, "time.Time and sql.NullString must not be auto-detected as relationships")

	_, ok := schema.ColumnByName("created_at")
	assert.True(t, ok, "expected column created_at from untagged time.Time field")

	_, ok = schema.ColumnByName("null_name")
	assert.True(t, ok, "expected column null_name from untagged sql.NullString field")
}

// TestIsAutoRelationshipType_ExcludesValueTypePackages verifies that
// isAutoRelationshipType returns false for stdlib value-type structs whose
// package is listed in valueTypePackages, while still returning true for
// user-defined entity structs.
func TestIsAutoRelationshipType_ExcludesValueTypePackages(t *testing.T) {
	// These must be excluded (stdlib value types).
	assert.False(t, isAutoRelationshipType(reflect.TypeOf(time.Time{})), "time.Time must not be a relationship candidate")
	assert.False(t, isAutoRelationshipType(reflect.TypeOf(sql.NullString{})), "sql.NullString must not be a relationship candidate")
	assert.False(t, isAutoRelationshipType(reflect.TypeOf(sql.NullInt64{})), "sql.NullInt64 must not be a relationship candidate")

	// User-defined struct — must still be a candidate.
	assert.True(t, isAutoRelationshipType(reflect.TypeOf(schemaAutoBelongsToOwner{})), "user struct must be a relationship candidate")

	// Slices of value types must also be excluded.
	assert.False(t, isAutoRelationshipType(reflect.TypeOf([]time.Time{})), "[]time.Time must not be a relationship candidate")

	// Primitive types must not be candidates.
	assert.False(t, isAutoRelationshipType(reflect.TypeOf(int64(0))), "int64 must not be a relationship candidate")
	assert.False(t, isAutoRelationshipType(reflect.TypeOf("")), "string must not be a relationship candidate")
}

// ============================================================
// SchemaNameTransformer
// ============================================================

// TestSchemaNameTransformer_CanBeOverridden verifies that replacing the global
// SchemaNameTransformer with a custom function changes how untagged field names
// are mapped to column names, and that the override is fully reverted after
// the test via t.Cleanup.
func TestSchemaNameTransformer_CanBeOverridden(t *testing.T) {
	// Replace with identity function; restore after test.
	original := SchemaNameTransformer
	SchemaNameTransformer = func(s string) string { return s }

	t.Cleanup(func() { SchemaNameTransformer = original })

	type schemaIdentityTransform struct {
		ID        int64  `db:"id,primaryKey"`
		FirstName string // with identity transformer → column name "FirstName"
	}

	schema, err := ParseSchema[schemaIdentityTransform]()
	require.NoError(t, err)

	_, ok := schema.ColumnByName("FirstName")
	assert.True(t, ok, "expected column named 'FirstName' with identity transformer")

	_, ok = schema.ColumnByName("first_name")
	assert.False(t, ok, "column 'first_name' must not exist when transformer is identity")
}

// TestSchemaNameTransformer_UsedForM2MJoinColumnDerivation verifies that the
// global SchemaNameTransformer is applied when deriving join-table column names
// for ManyToMany relationships, so a custom transformer propagates to M2M queries.
func TestSchemaNameTransformer_UsedForM2MJoinColumnDerivation(t *testing.T) {
	original := SchemaNameTransformer
	SchemaNameTransformer = func(s string) string { return "prefix_" + toSnakeCase(s) }

	t.Cleanup(func() { SchemaNameTransformer = original })

	// The M2M join-table column names are built at query time from:
	//   SchemaNameTransformer(parentSchema.entityType.Name()) + "_id"
	//   SchemaNameTransformer(rel.RelatedType.Name())         + "_id"
	// We verify the formula directly so the test does not require a real DB.
	parentName := SchemaNameTransformer("ArticleEntity") + "_id"
	relatedName := SchemaNameTransformer("TagEntity") + "_id"

	assert.Equal(t, "prefix_article_entity_id", parentName)
	assert.Equal(t, "prefix_tag_entity_id", relatedName)
}

// TestSchemaNameTransformer_AffectsTableNameDerivation verifies that overriding
// the global SchemaNameTransformer also changes how table names are auto-derived
// from type names, so there is no inconsistency between column and table naming.
func TestSchemaNameTransformer_AffectsTableNameDerivation(t *testing.T) {
	original := SchemaNameTransformer
	SchemaNameTransformer = func(s string) string { return "x_" + toSnakeCase(s) }

	t.Cleanup(func() { SchemaNameTransformer = original })

	type schemaTableNameTransformEntity struct {
		ID   int64  `db:"id,primaryKey"`
		Name string `db:"name"`
	}

	schema, err := ParseSchema[schemaTableNameTransformEntity]()
	require.NoError(t, err)

	// With the custom transformer the type name "schemaTableNameTransformEntity"
	// becomes "x_schema_table_name_transform_entity", pluralised to
	// "x_schema_table_name_transform_entities".
	assert.Equal(t, "x_schema_table_name_transform_entities", schema.TableName,
		"tableNameForType must use SchemaNameTransformer, not the hard-coded toSnakeCase")
}

// ============================================================
// TableNamer support in auto-preload path collection
// ============================================================

// schemaTableNamerAutoPreloadChild implements TableNamer so that
// parseRelatedSchemaForAutoPreload must honour it when building auto-preload paths.
type schemaTableNamerAutoPreloadChild struct {
	ID       int64  `db:"id,primaryKey"`
	ParentID int64  `db:"parent_id"`
	Body     string `db:"body"`
}

func (schemaTableNamerAutoPreloadChild) TableName() string { return "custom_child_table" }

// schemaTableNamerAutoPreloadParent has an auto-preload to schemaTableNamerAutoPreloadChild.
// collectAutoPreloads calls parseRelatedSchemaForAutoPreload for the child type.
type schemaTableNamerAutoPreloadParent struct {
	ID       int64                              `db:"id,primaryKey"`
	Name     string                             `db:"name"`
	Children []schemaTableNamerAutoPreloadChild `db:"-,foreignKey:parent_id,preload"`
}

// TestParseRelatedSchemaForAutoPreload_HonoursTableNamer verifies that when a
// related entity in an auto-preload chain implements TableNamer, the resolved
// schema uses the custom table name rather than the auto-derived one.
func TestParseRelatedSchemaForAutoPreload_HonoursTableNamer(t *testing.T) {
	// parseRelatedSchemaForAutoPreload is exercised by collectAutoPreloads, which
	// is called from cacheSchemaDerivedValues inside ParseSchema. We parse the
	// parent schema and then lazily resolve the child relation to check its table name.
	schema, err := ParseSchema[schemaTableNamerAutoPreloadParent]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Children"]
	require.True(t, ok, "expected relationship Children")

	relSchema, err := rel.ResolveRelatedSchema()
	require.NoError(t, err)

	assert.Equal(t, "custom_child_table", relSchema.TableName,
		"parseRelatedSchemaForAutoPreload must honour TableNamer on the related entity")
}

// ============================================================
// ManyToMany: join table auto-derivation and column overrides
// ============================================================

// schemaM2MTableNamerSide implements TableNamer to customise its table name.
type schemaM2MTableNamerSide struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

func (schemaM2MTableNamerSide) TableName() string { return "custom_sides" }

type schemaM2MTableNamerOwner struct {
	ID    int64                     `db:"id,primaryKey"`
	Name  string                    `db:"name"`
	Sides []schemaM2MTableNamerSide `db:"-,many2many:"`
}

// TestParseSchema_M2M_AutoDerived_HonoursTableNamer verifies that when the join
// table name is omitted (empty many2many value) and one side implements TableNamer,
// the auto-derived join table name is built from the custom table name rather than
// the default snake_case type name.
func TestParseSchema_M2M_AutoDerived_HonoursTableNamer(t *testing.T) {
	schema, err := ParseSchema[schemaM2MTableNamerOwner]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Sides"]
	require.True(t, ok)

	// One side honours TableNamer → "custom_sides"; other is auto-derived.
	parentTable := tableNameForType(reflect.TypeOf(schemaM2MTableNamerOwner{})) // "schema_m2_m_table_namer_owners"
	relatedTable := "custom_sides"

	expected := []string{parentTable, relatedTable}
	sort.Strings(expected)

	assert.Equal(t, expected[0]+"_"+expected[1], rel.JoinTable)
}

// TestParseSchema_M2M_ColumnOverrides_StoredOnRelationship verifies that
// explicit parentKey and relatedKey options in the many2many tag are stored on
// the Relationship and take precedence over the auto-derived column names.
func TestParseSchema_M2M_ColumnOverrides_StoredOnRelationship(t *testing.T) {
	schema, err := ParseSchema[schemaM2MColOverrideArticle]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Tags"]
	require.True(t, ok)
	assert.Equal(t, "override_table", rel.JoinTable)
	assert.Equal(t, "art_id", rel.JoinParentKey)
	assert.Equal(t, "tag_id", rel.JoinRelatedKey)
}

// TestResolveM2MColumnNames_WithOverrides_UsesOverrides verifies that when both
// JoinParentKey and JoinRelatedKey are set on the Relationship, resolveM2MColumnNames
// returns those values verbatim without consulting the SchemaNameTransformer.
func TestResolveM2MColumnNames_WithOverrides_UsesOverrides(t *testing.T) {
	parentSchema := &EntitySchema{entityType: reflect.TypeOf(schemaM2MColOverrideArticle{})}
	relSchema := &EntitySchema{entityType: reflect.TypeOf(schemaM2MColOverrideTag{})}
	rel := &Relationship{
		JoinParentKey:  "art_id",
		JoinRelatedKey: "tag_id",
	}

	parent, related := resolveM2MColumnNames(rel, parentSchema, relSchema)
	assert.Equal(t, "art_id", parent)
	assert.Equal(t, "tag_id", related)
}

// TestResolveM2MColumnNames_NoOverrides_DerivedFromTransformer verifies that
// when no column overrides are set, resolveM2MColumnNames derives both column
// names from the SchemaNameTransformer applied to the respective entity type names.
func TestResolveM2MColumnNames_NoOverrides_DerivedFromTransformer(t *testing.T) {
	parentSchema := &EntitySchema{entityType: reflect.TypeOf(schemaM2MAutoArticle{})}
	relSchema := &EntitySchema{entityType: reflect.TypeOf(schemaM2MAutoTag{})}
	rel := &Relationship{}

	parent, related := resolveM2MColumnNames(rel, parentSchema, relSchema)
	assert.Equal(t, SchemaNameTransformer("schemaM2MAutoArticle")+"_id", parent)
	assert.Equal(t, SchemaNameTransformer("schemaM2MAutoTag")+"_id", related)
}

// TestResolveM2MColumnNames_PartialOverride_OnlyParentKey verifies that when
// only JoinParentKey is set, resolveM2MColumnNames uses the override for the
// parent column and falls back to the transformer for the related column.
func TestResolveM2MColumnNames_PartialOverride_OnlyParentKey(t *testing.T) {
	parentSchema := &EntitySchema{entityType: reflect.TypeOf(schemaM2MAutoArticle{})}
	relSchema := &EntitySchema{entityType: reflect.TypeOf(schemaM2MAutoTag{})}
	rel := &Relationship{JoinParentKey: "custom_parent_id"}

	parent, related := resolveM2MColumnNames(rel, parentSchema, relSchema)
	assert.Equal(t, "custom_parent_id", parent)
	assert.Equal(t, SchemaNameTransformer("schemaM2MAutoTag")+"_id", related)
}
