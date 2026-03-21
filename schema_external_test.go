package sqlr_test

import (
	"testing"

	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
)

type privateTaggedEmbedded struct {
	ID int64 `db:"id" sqlr:"primaryKey"`
}

type privateTaggedFieldEntity struct {
	ID        int64  `db:"id" sqlr:"primaryKey"`
	secretKey string `db:"secret_key"`
}

type privateSQLRTaggedFieldEntity struct {
	ID      int64  `db:"id" sqlr:"primaryKey"`
	ignored string `sqlr:"primaryKey"`
}

type privateEmbeddedEntity struct {
	ID int64 `db:"id" sqlr:"primaryKey"`
	privateTaggedEmbedded
}

type privateEmbeddedPrimaryKeyOnly struct {
	privateTaggedEmbedded
}

type privateAutoBelongsToEntity struct {
	ID        int64 `db:"id" sqlr:"primaryKey"`
	RelatedID int64 `db:"related_id"`
	Related   privateEmbeddedPrimaryKeyOnly
}

type privateEmbeddedForeignKeyFields struct {
	ParentID int64 `db:"parent_id"`
}

type privateAutoHasManyRelated struct {
	ID int64 `db:"id" sqlr:"primaryKey"`
	privateEmbeddedForeignKeyFields
	Body string `db:"body"`
}

type privateAutoHasManyParent struct {
	ID       int64 `db:"id" sqlr:"primaryKey"`
	Children []privateAutoHasManyRelated
}

// TestParseSchema_PrivateDBTaggedFieldRejected verifies that unexported fields
// cannot opt into schema column mapping via an explicit db tag.
func TestParseSchema_PrivateDBTaggedFieldRejected(t *testing.T) {
	_, err := sqlr.ParseSchema[privateTaggedFieldEntity]()
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse schema for privateTaggedFieldEntity")
	require.ErrorContains(t, err, "field secretKey: unexported fields cannot define db column mappings")
}

// TestParseSchema_PrivateSQLRTaggedFieldRejected verifies that unexported fields
// cannot attach sqlr metadata, even when no db tag is present.
func TestParseSchema_PrivateSQLRTaggedFieldRejected(t *testing.T) {
	_, err := sqlr.ParseSchema[privateSQLRTaggedFieldEntity]()
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse schema for privateSQLRTaggedFieldEntity")
	require.ErrorContains(t, err, "field ignored: unexported fields cannot define sqlr metadata")
}

// TestParseSchema_PrivateAnonymousEmbeddedFieldRejected verifies that private
// anonymous embedded structs fail during schema parsing instead of being
// flattened into later reflection-heavy code paths.
func TestParseSchema_PrivateAnonymousEmbeddedFieldRejected(t *testing.T) {
	_, err := sqlr.ParseSchema[privateEmbeddedEntity]()
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse schema for privateEmbeddedEntity")
	require.ErrorContains(t, err, "field privateTaggedEmbedded: anonymous embedded fields must be exported")
}

// TestParseSchema_PrivateEmbeddedPrimaryKeyIgnoredForAutoRelationshipDetection
// verifies that private embedded fields do not count as primary-key evidence for
// auto-detecting related entities.
func TestParseSchema_PrivateEmbeddedPrimaryKeyIgnoredForAutoRelationshipDetection(t *testing.T) {
	schema, err := sqlr.ParseSchema[privateAutoBelongsToEntity]()
	require.NoError(t, err)
	require.Empty(t, schema.Relationships)

	_, ok := schema.ColumnByName("related")
	require.True(t, ok)
}

// TestParseSchema_PrivateEmbeddedForeignKeyIgnoredForAutoRelationshipDetection
// verifies that private embedded fields do not count as foreign-key evidence for
// auto-detecting has-many relations.
func TestParseSchema_PrivateEmbeddedForeignKeyIgnoredForAutoRelationshipDetection(t *testing.T) {
	schema, err := sqlr.ParseSchema[privateAutoHasManyParent]()
	require.NoError(t, err)
	require.Empty(t, schema.Relationships)

	_, ok := schema.ColumnByName("children")
	require.True(t, ok)
}
