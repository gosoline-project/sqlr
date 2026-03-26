package sqlr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryBuilderCreate_Empty verifies that QueryBuilderCreate starts with no
// preloads configured.
func TestQueryBuilderCreate_Empty(t *testing.T) {
	qbc := NewQueryBuilderCreate()

	assert.False(t, qbc.shouldOmitAllAssociations())
	assert.False(t, qbc.hasPreloads())
	assert.Empty(t, qbc.preloads)
}

// TestQueryBuilderCreate_Preload verifies that QueryBuilderCreate records
// preloads correctly.
func TestQueryBuilderCreate_Preload(t *testing.T) {
	qbc := NewQueryBuilderCreate()
	result := qbc.Preload("Posts")

	assert.Same(t, qbc, result)
	assert.True(t, qbc.hasPreloads())
	require.Len(t, qbc.preloads, 1)
	assert.Equal(t, "Posts", qbc.preloads[0].relation)
	assert.Empty(t, qbc.preloads[0].where)
}

// TestQueryBuilderCreate_PreloadWithCondition verifies that QueryBuilderCreate
// records preload conditions correctly.
func TestQueryBuilderCreate_PreloadWithCondition(t *testing.T) {
	qbc := NewQueryBuilderCreate()
	qbc.Preload("Posts", Condition("status = ?", "published"))

	require.Len(t, qbc.preloads, 1)
	assert.Equal(t, "Posts", qbc.preloads[0].relation)
	require.Len(t, qbc.preloads[0].where, 1)
}

// TestQueryBuilderCreate_ToQueryBuilderRead verifies that QueryBuilderCreate can
// forward configured preloads to a read builder for post-create reloads.
func TestQueryBuilderCreate_ToQueryBuilderRead(t *testing.T) {
	qbc := NewQueryBuilderCreate()
	qbc.Preload("Posts.Comments")

	qbr := qbc.toQueryBuilderRead()

	require.Len(t, qbr.preloads, 1)
	assert.Equal(t, "Posts.Comments", qbr.preloads[0].relation)
}
