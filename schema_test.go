package sqlr

import (
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type schemaPrimaryKeyEmbedded struct {
	ID int64 `db:"id,primaryKey"`
}

type schemaPrimaryKeyEntityWithEmbedded struct {
	schemaPrimaryKeyEmbedded
	Name string `db:"name"`
}

type schemaPrimaryKeyEntityWithDirectField struct {
	Key   string `db:"key,primaryKey"`
	Value string `db:"value"`
}

type schemaNoPrimaryKey struct {
	Name string `db:"name"`
}

type schemaPointerIntPK struct {
	ID   *int64 `db:"id,primaryKey"`
	Name string `db:"name"`
}

type schemaAutoTimestamps struct {
	ID        int64  `db:"id,primaryKey"`
	Name      string `db:"name"`
	CreatedAt string `db:"created_at,autoCreateTime"`
	UpdatedAt string `db:"updated_at,autoUpdateTime"`
}

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

type schemaManyToManyTag struct {
	ID   int64  `db:"id,primaryKey"`
	Name string `db:"name"`
}

type schemaManyToManyArticle struct {
	ID   int64                 `db:"id,primaryKey"`
	Name string                `db:"name"`
	Tags []schemaManyToManyTag `db:"-,many2many:article_tags"`
}

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

type schemaMissingFKRelation struct {
	ID    int64               `db:"id,primaryKey"`
	Other schemaHasOneProfile `db:"-,foreignKey:"`
}

type schemaRelationNonStruct struct {
	ID    int64  `db:"id,primaryKey"`
	Other string `db:"-,foreignKey:other_id"`
}

type schemaDashField struct {
	ID      int64  `db:"id,primaryKey"`
	Name    string `db:"name"`
	Ignored string `db:"-"`
}

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

func TestParseSchema_PrimaryKeyPointsToColumnsEntry(t *testing.T) {
	assertPrimaryKeyPointsToColumnsEntry[schemaPrimaryKeyEntityWithEmbedded](t)
	assertPrimaryKeyPointsToColumnsEntry[schemaPrimaryKeyEntityWithDirectField](t)
}

func assertPrimaryKeyPointsToColumnsEntry[E any](t *testing.T) {
	t.Helper()

	schema, err := parseSchema[E]()
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

func TestParseSchema_ColumnByNamePointsToColumnsEntries(t *testing.T) {
	schema, err := parseSchema[schemaPrimaryKeyEntityWithEmbedded]()
	require.NoError(t, err)
	require.Len(t, schema.columnByName, len(schema.Columns))

	for i := range schema.Columns {
		col := &schema.Columns[i]
		mapped, ok := schema.ColumnByName(col.Name)
		require.True(t, ok)
		assert.Same(t, col, mapped)
	}
}

func TestParseSchema_CachesDerivedValues(t *testing.T) {
	schema, err := parseSchema[schemaCacheAuthor]()
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

func TestParseSchema_AutoPreloadsNested(t *testing.T) {
	schema, err := parseSchema[schemaNestedAutoAuthor]()
	require.NoError(t, err)
	assert.Equal(t, []preloadEntry{
		{relation: "Posts"},
		{relation: "Posts.Comments"},
	}, schema.AutoPreloads())
}

func TestParseSchema_AutoPreloadsCircular(t *testing.T) {
	var schema *EntitySchema
	require.NotPanics(t, func() {
		var err error
		schema, err = parseSchema[schemaCircularAutoParent]()
		require.NoError(t, err)
	})
	assert.Equal(t, []preloadEntry{
		{relation: "Children"},
		{relation: "Children.Parent"},
	}, schema.AutoPreloads())
}

func TestRelationshipResolveRelationSchema_Concurrent(t *testing.T) {
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

			s, err := rel.resolveRelationSchema()
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

func TestParseSchema_BelongsTo(t *testing.T) {
	schema, err := parseSchema[schemaBelongsToPost]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Author"]
	require.True(t, ok)
	assert.Equal(t, BelongsTo, rel.Type)
	assert.Equal(t, "author_id", rel.ForeignKey)
}

func TestParseSchema_BelongsToSliceRejected(t *testing.T) {
	_, err := parseSchema[schemaBelongsToPostInvalidSlice]()
	require.Error(t, err)
	require.ErrorContains(t, err, "belongsTo relationship Author must not be a slice")
}

func TestParseSchema_BelongsToMissingForeignKeyColumnRejected(t *testing.T) {
	_, err := parseSchema[schemaBelongsToPostMissingFK]()
	require.Error(t, err)
	require.ErrorContains(t, err, "belongsTo foreign key column \"author_id\" not found on entity")
}

func TestIsRelationshipField_BelongsTo(t *testing.T) {
	assert.True(t, isRelationshipField([]string{"belongsTo:author_id"}))
}

func TestParseSchema_NonStructType_ReturnsError(t *testing.T) {
	_, err := parseSchema[int]()
	require.Error(t, err)
	require.ErrorContains(t, err, "is not a struct")
}

func TestParseSchema_NoPrimaryKey_ReturnsError(t *testing.T) {
	_, err := parseSchema[schemaNoPrimaryKey]()
	require.Error(t, err)
	require.ErrorContains(t, err, "has no primary key")
}

func TestParseSchema_PointerToStruct_UnwrapsSuccessfully(t *testing.T) {
	schema, err := parseSchema[*schemaPrimaryKeyEntityWithEmbedded]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)
	assert.Equal(t, "id", schema.PrimaryKey.Name)
	assert.Equal(t, []string{"id", "name"}, schema.AllColumns())
}

func TestParseSchema_StringPK_IncludedInInsertColumns(t *testing.T) {
	schema, err := parseSchema[schemaPrimaryKeyEntityWithDirectField]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)
	assert.False(t, schema.PrimaryKey.AutoIncrement)
	assert.Contains(t, schema.InsertColumns(), "key")
	assert.Equal(t, []string{"key", "value"}, schema.InsertColumns())
}

func TestResolveRelationSchema_RelatedTypeNoPrimaryKey_ReturnsError(t *testing.T) {
	rel := &Relationship{
		RelatedType: reflect.TypeOf(schemaNoPrimaryKey{}),
	}

	_, err := rel.resolveRelationSchema()
	require.Error(t, err)
	require.ErrorContains(t, err, "has no primary key")
}

func TestParseSchema_MissingForeignKey_ReturnsError(t *testing.T) {
	_, err := parseSchema[schemaMissingFKRelation]()
	require.Error(t, err)
	require.ErrorContains(t, err, "requires a foreignKey option")
}

func TestParseSchema_RelationshipOnNonStructField_ReturnsError(t *testing.T) {
	_, err := parseSchema[schemaRelationNonStruct]()
	require.Error(t, err)
	require.ErrorContains(t, err, "must be a struct or slice of structs")
}

func TestParseSchema_AutoCreateTimeAndUpdateTime(t *testing.T) {
	schema, err := parseSchema[schemaAutoTimestamps]()
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

func TestParseSchema_PointerIntegerPK_AutoIncrementTrue(t *testing.T) {
	schema, err := parseSchema[schemaPointerIntPK]()
	require.NoError(t, err)
	require.NotNil(t, schema.PrimaryKey)
	assert.True(t, schema.PrimaryKey.AutoIncrement)
	assert.Equal(t, []string{"name"}, schema.InsertColumns())
}

func TestParseSchema_HasOne_Relationship(t *testing.T) {
	schema, err := parseSchema[schemaHasOneAuthor]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Profile"]
	require.True(t, ok)
	assert.Equal(t, HasOne, rel.Type)
	assert.Equal(t, "author_id", rel.ForeignKey)
}

func TestParseSchema_ManyToMany_Relationship(t *testing.T) {
	schema, err := parseSchema[schemaManyToManyArticle]()
	require.NoError(t, err)

	rel, ok := schema.Relationships["Tags"]
	require.True(t, ok)
	assert.Equal(t, ManyToMany, rel.Type)
	assert.Equal(t, "article_tags", rel.JoinTable)
	assert.Equal(t, "", rel.ForeignKey)
}

func TestParseSchema_MixedPreloadAndNonPreload_AutoPreloads(t *testing.T) {
	schema, err := parseSchema[schemaMixedPreloadAuthor]()
	require.NoError(t, err)

	autoPreloads := schema.AutoPreloads()
	require.Len(t, autoPreloads, 1)
	assert.Equal(t, "Posts", autoPreloads[0].relation)
}

func TestIsRelationshipField_ForeignKey(t *testing.T) {
	assert.True(t, isRelationshipField([]string{"foreignKey:post_id"}))
}

func TestIsRelationshipField_ManyToMany(t *testing.T) {
	assert.True(t, isRelationshipField([]string{"many2many:post_tags"}))
}

func TestIsRelationshipField_NoRelationshipOption_ReturnsFalse(t *testing.T) {
	assert.False(t, isRelationshipField([]string{"primaryKey"}))
}

func TestIsRelationshipField_EmptySlice_ReturnsFalse(t *testing.T) {
	assert.False(t, isRelationshipField([]string{}))
}

func TestParseSchema_DbDashWithoutRelationship_FieldSkipped(t *testing.T) {
	schema, err := parseSchema[schemaDashField]()
	require.NoError(t, err)

	_, ok := schema.ColumnByName("-")
	assert.False(t, ok)
	assert.Equal(t, []string{"id", "name"}, schema.AllColumns())
	assert.Empty(t, schema.Relationships)
}

func TestValidRelationNames_SortedOutput(t *testing.T) {
	schema, err := parseSchema[schemaCacheAuthor]()
	require.NoError(t, err)

	names := schema.ValidRelationNames()
	assert.Equal(t, []string{"Comments", "Posts"}, names)
}
