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
