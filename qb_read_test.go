package sqlr

import (
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryBuilderRead_Empty(t *testing.T) {
	qbr := NewQueryBuilderRead()

	assert.False(t, qbr.hasRelations())
	assert.Empty(t, qbr.joins)
	assert.Empty(t, qbr.preloads)
}

func TestQueryBuilderRead_Preload(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts")

	assert.True(t, qbr.hasRelations())
	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts", qbr.preloads[0].relation)
	assert.Empty(t, qbr.preloads[0].where)
}

func TestQueryBuilderRead_PreloadWithCondition(t *testing.T) {
	cond := Condition("status = ?", "active")
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts", cond)

	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts", qbr.preloads[0].relation)
	require.Len(t, qbr.preloads[0].where, 1)
}

func TestQueryBuilderRead_PreloadNested(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts.Comments")

	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts.Comments", qbr.preloads[0].relation)
}

func TestQueryBuilderRead_MultiplePreloads(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts").Preload("Profile")

	require.Len(t, qbr.preloads, 2)
	assert.Equal(t, "Posts", qbr.preloads[0].relation)
	assert.Equal(t, "Profile", qbr.preloads[1].relation)
}

func TestQueryBuilderRead_LeftJoin(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts")

	assert.True(t, qbr.hasRelations())
	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinLeft, qbr.joins[0].joinType)
	assert.Equal(t, "Posts", qbr.joins[0].relation)
	assert.Empty(t, qbr.joins[0].where)
}

func TestQueryBuilderRead_LeftJoinWithCondition(t *testing.T) {
	cond := Condition("status = ?", "published")
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts", cond)

	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinLeft, qbr.joins[0].joinType)
	require.Len(t, qbr.joins[0].where, 1)
}

func TestQueryBuilderRead_InnerJoin(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.InnerJoin("Posts")

	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinInner, qbr.joins[0].joinType)
	assert.Equal(t, "Posts", qbr.joins[0].relation)
}

func TestQueryBuilderRead_RightJoin(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.RightJoin("Posts")

	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinRight, qbr.joins[0].joinType)
	assert.Equal(t, "Posts", qbr.joins[0].relation)
}

func TestQueryBuilderRead_CrossJoin(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.CrossJoin("Posts")

	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinCross, qbr.joins[0].joinType)
	assert.Equal(t, "Posts", qbr.joins[0].relation)
	assert.Nil(t, qbr.joins[0].where)
}

func TestQueryBuilderRead_Chaining(t *testing.T) {
	qbr := NewQueryBuilderRead()
	result := qbr.LeftJoin("Posts").Preload("Comments").InnerJoin("Profile")

	assert.Same(t, qbr, result)
	assert.Len(t, qbr.joins, 2)
	assert.Len(t, qbr.preloads, 1)
}

func TestQueryBuilderRead_ToQueryBuilderSelect(t *testing.T) {
	cond := Condition("status = ?", "published")
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts", cond).Preload("Comments")

	qb := qbr.toQueryBuilderSelect("test_authors", "id", int64(42))

	// Verify joins and preloads were copied.
	require.Len(t, qb.joins, 1)
	assert.Equal(t, "Posts", qb.joins[0].relation)
	assert.Equal(t, sqlc.JoinLeft, qb.joins[0].joinType)
	require.Len(t, qb.preloads, 1)
	assert.Equal(t, "Comments", qb.preloads[0].relation)

	// Verify WHERE and LIMIT are set.
	require.NotNil(t, qb.limit)
	assert.Equal(t, 1, *qb.limit)

	// Verify the WHERE clause contains the PK condition.
	sql, params, err := qb.ToSql()
	require.NoError(t, err)
	assert.Contains(t, sql, "WHERE")
	assert.Contains(t, params, int64(42))
}

func TestQueryBuilderRead_ToQueryBuilderSelect_PreservesOriginal(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts")

	qb := qbr.toQueryBuilderSelect("test_authors", "id", int64(1))

	// Modifying the returned QueryBuilderSelect should not affect the original.
	qb.Preload("Extra")
	assert.Empty(t, qbr.preloads)
}

func TestQueryBuilderRead_HasRelations_JoinOnly(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts")

	assert.True(t, qbr.hasRelations())
}

func TestQueryBuilderRead_HasRelations_PreloadOnly(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts")

	assert.True(t, qbr.hasRelations())
}
