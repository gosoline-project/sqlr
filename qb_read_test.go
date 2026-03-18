package sqlr

import (
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryBuilderRead_Empty verifies that QueryBuilderRead starts with no joins or preloads configured.
func TestQueryBuilderRead_Empty(t *testing.T) {
	qbr := NewQueryBuilderRead()

	assert.False(t, qbr.hasRelations())
	assert.Empty(t, qbr.joins)
	assert.Empty(t, qbr.preloads)
}

// TestQueryBuilderRead_Preload verifies that QueryBuilderRead records preloads correctly.
func TestQueryBuilderRead_Preload(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts")

	assert.True(t, qbr.hasRelations())
	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts", qbr.preloads[0].relation)
	assert.Empty(t, qbr.preloads[0].where)
}

// TestQueryBuilderRead_PreloadWithCondition verifies that QueryBuilderRead records preload conditions correctly.
func TestQueryBuilderRead_PreloadWithCondition(t *testing.T) {
	cond := Condition("status = ?", "active")
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts", cond)

	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts", qbr.preloads[0].relation)
	require.Len(t, qbr.preloads[0].where, 1)
}

// TestQueryBuilderRead_PreloadNested verifies that QueryBuilderRead records nested preload paths correctly.
func TestQueryBuilderRead_PreloadNested(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts.Comments")

	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts.Comments", qbr.preloads[0].relation)
}

// TestQueryBuilderRead_MultiplePreloads verifies that QueryBuilderRead supports multiple preloads.
func TestQueryBuilderRead_MultiplePreloads(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts").Preload("Profile")

	require.Len(t, qbr.preloads, 2)
	assert.Equal(t, "Posts", qbr.preloads[0].relation)
	assert.Equal(t, "Profile", qbr.preloads[1].relation)
}

// TestQueryBuilderRead_LeftJoin verifies that QueryBuilderRead records left joins correctly.
func TestQueryBuilderRead_LeftJoin(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts")

	assert.True(t, qbr.hasRelations())
	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinLeft, qbr.joins[0].joinType)
	assert.Equal(t, "Posts", qbr.joins[0].relation)
	assert.Empty(t, qbr.joins[0].where)
}

// TestQueryBuilderRead_LeftJoinWithCondition verifies that QueryBuilderRead records left joins with conditions correctly.
func TestQueryBuilderRead_LeftJoinWithCondition(t *testing.T) {
	cond := Condition("status = ?", "published")
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts", cond)

	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinLeft, qbr.joins[0].joinType)
	require.Len(t, qbr.joins[0].where, 1)
}

// TestQueryBuilderRead_InnerJoin verifies that QueryBuilderRead records inner joins correctly.
func TestQueryBuilderRead_InnerJoin(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.InnerJoin("Posts")

	require.Len(t, qbr.joins, 1)
	assert.Equal(t, sqlc.JoinInner, qbr.joins[0].joinType)
	assert.Equal(t, "Posts", qbr.joins[0].relation)
}

// TestQueryBuilderRead_Chaining verifies that QueryBuilderRead supports method chaining.
func TestQueryBuilderRead_Chaining(t *testing.T) {
	qbr := NewQueryBuilderRead()
	result := qbr.LeftJoin("Posts").Preload("Comments").InnerJoin("Profile")

	assert.Same(t, qbr, result)
	assert.Len(t, qbr.joins, 2)
	assert.Len(t, qbr.preloads, 1)
}

// TestQueryBuilderRead_ToQueryBuilderSelect_WithJoinOmitsLimit verifies that QueryBuilderRead omits row limits when converting joined reads to QueryBuilderSelect.
func TestQueryBuilderRead_ToQueryBuilderSelect_WithJoinOmitsLimit(t *testing.T) {
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

	// Verify WHERE is set and row-level LIMIT is omitted for joined reads.
	assert.Nil(t, qb.limit)

	// Verify the WHERE clause contains the PK condition.
	sql, params, err := qb.ToSql()
	require.NoError(t, err)
	assert.Contains(t, sql, "WHERE")
	assert.Contains(t, params, int64(42))
}

// TestQueryBuilderRead_ToQueryBuilderSelect_PreloadOnlyAddsLimit verifies that QueryBuilderRead keeps the single-row limit when only preloads are configured.
func TestQueryBuilderRead_ToQueryBuilderSelect_PreloadOnlyAddsLimit(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Comments")

	qb := qbr.toQueryBuilderSelect("test_authors", "id", int64(42))

	require.NotNil(t, qb.limit)
	assert.Equal(t, 1, *qb.limit)
}

// TestQueryBuilderRead_ToQueryBuilderSelect_PreservesOriginal verifies that QueryBuilderRead preserves the original read builder when converting to QueryBuilderSelect.
func TestQueryBuilderRead_ToQueryBuilderSelect_PreservesOriginal(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts")

	qb := qbr.toQueryBuilderSelect("test_authors", "id", int64(1))

	// Modifying the returned QueryBuilderSelect should not affect the original.
	qb.Preload("Extra")
	assert.Empty(t, qbr.preloads)
}

// TestQueryBuilderRead_HasRelations_JoinOnly verifies that QueryBuilderRead treats join-only builders as relation-aware.
func TestQueryBuilderRead_HasRelations_JoinOnly(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.LeftJoin("Posts")

	assert.True(t, qbr.hasRelations())
}

// TestQueryBuilderRead_HasRelations_PreloadOnly verifies that QueryBuilderRead treats preload-only builders as relation-aware.
func TestQueryBuilderRead_HasRelations_PreloadOnly(t *testing.T) {
	qbr := NewQueryBuilderRead()
	qbr.Preload("Posts")

	assert.True(t, qbr.hasRelations())
}
