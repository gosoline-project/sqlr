package sqlr_test

import (
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Where Tests ==========

// TestQueryBuilderSelectWhereWithStringCondition verifies that QueryBuilderSelect supports `Where` with string condition.
func TestQueryBuilderSelectWhereWithStringCondition(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where("status = ?", "active")

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE status = ?", sql)
	assert.Equal(t, []any{"active"}, params)
}

// TestQueryBuilderSelectWhereWithMultipleStringConditions verifies that QueryBuilderSelect supports `Where` with multiple string conditions.
func TestQueryBuilderSelectWhereWithMultipleStringConditions(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where("status = ?", "active").
		Where("age >= ?", 18)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE status = ? AND age >= ?", sql)
	assert.Equal(t, []any{"active", 18}, params)
}

// TestQueryBuilderSelectWhereWithExpression verifies that QueryBuilderSelect supports `Where` with expression.
func TestQueryBuilderSelectWhereWithExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("age").Gt(18))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `age` > ?", sql)
	assert.Equal(t, []any{18}, params)
}

// TestQueryBuilderSelectWhereWithMultipleExpressions verifies that QueryBuilderSelect supports `Where` with multiple expressions.
func TestQueryBuilderSelectWhereWithMultipleExpressions(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("age").Gt(18)).
		Where(sqlc.Col("status").Eq("active"))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `age` > ? AND `status` = ?", sql)
	assert.Equal(t, []any{18, "active"}, params)
}

// TestQueryBuilderSelectWhereWithEqMap verifies that QueryBuilderSelect supports `Where` with equality maps.
func TestQueryBuilderSelectWhereWithEqMap(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Eq{"status": "active", "role": "admin"})

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE (`role` = ? AND `status` = ?)", sql)
	assert.Equal(t, []any{"admin", "active"}, params)
}

// TestQueryBuilderSelectWhereWithAndExpression verifies that QueryBuilderSelect supports `Where` with AND expressions.
func TestQueryBuilderSelectWhereWithAndExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.And(
		sqlc.Col("age").Gte(18),
		sqlc.Col("status").Eq("active"),
	))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE (`age` >= ? AND `status` = ?)", sql)
	assert.Equal(t, []any{18, "active"}, params)
}

// TestQueryBuilderSelectWhereWithOrExpression verifies that QueryBuilderSelect supports `Where` with OR expressions.
func TestQueryBuilderSelectWhereWithOrExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Or(
		sqlc.Col("status").Eq("active"),
		sqlc.Col("status").Eq("pending"),
	))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE (`status` = ? OR `status` = ?)", sql)
	assert.Equal(t, []any{"active", "pending"}, params)
}

// TestQueryBuilderSelectWhereWithInExpression verifies that QueryBuilderSelect supports `Where` with in expression.
func TestQueryBuilderSelectWhereWithInExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").In("active", "pending", "approved"))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` IN (?, ?, ?)", sql)
	assert.Equal(t, []any{"active", "pending", "approved"}, params)
}

// TestQueryBuilderSelectWhereEmpty verifies that QueryBuilderSelect omits `Where` when no conditions are configured.
func TestQueryBuilderSelectWhereEmpty(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// ========== GroupBy Tests ==========

// TestQueryBuilderSelectGroupByWithSingleColumn verifies that QueryBuilderSelect supports `GroupBy` with single column.
func TestQueryBuilderSelectGroupByWithSingleColumn(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.GroupBy("status")

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "GROUP BY `status`", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectGroupByWithMultipleColumns verifies that QueryBuilderSelect supports `GroupBy` with multiple columns.
func TestQueryBuilderSelectGroupByWithMultipleColumns(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.GroupBy("status", "country", "city")

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "GROUP BY `status`, `country`, `city`", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectGroupByWithExpression verifies that QueryBuilderSelect supports `GroupBy` with expression.
func TestQueryBuilderSelectGroupByWithExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.GroupBy(sqlc.Col("DATE(created_at)"))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "GROUP BY `DATE(created_at)`", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectGroupByEmpty verifies that QueryBuilderSelect omits `GroupBy` when no columns are configured.
func TestQueryBuilderSelectGroupByEmpty(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// ========== Having Tests ==========

// TestQueryBuilderSelectHavingWithStringCondition verifies that QueryBuilderSelect supports `Having` with string condition.
func TestQueryBuilderSelectHavingWithStringCondition(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Having("COUNT(*) > ?", 10)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "HAVING COUNT(*) > ?", sql)
	assert.Equal(t, []any{10}, params)
}

// TestQueryBuilderSelectHavingWithMultipleStringConditions verifies that QueryBuilderSelect supports `Having` with multiple string conditions.
func TestQueryBuilderSelectHavingWithMultipleStringConditions(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Having("COUNT(*) > ?", 10).
		Having("SUM(amount) > ?", 1000)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "HAVING COUNT(*) > ? AND SUM(amount) > ?", sql)
	assert.Equal(t, []any{10, 1000}, params)
}

// TestQueryBuilderSelectHavingWithExpression verifies that QueryBuilderSelect supports `Having` with expression.
func TestQueryBuilderSelectHavingWithExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Having(sqlc.Col("*").Count().Gt(10))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "HAVING COUNT(*) > ?", sql)
	assert.Equal(t, []any{10}, params)
}

// TestQueryBuilderSelectHavingWithMultipleExpressions verifies that QueryBuilderSelect supports `Having` with multiple expressions.
func TestQueryBuilderSelectHavingWithMultipleExpressions(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Having(sqlc.Col("*").Count().Gt(10)).
		Having(sqlc.Col("amount").Sum().Gt(1000))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "HAVING COUNT(*) > ? AND SUM(`amount`) > ?", sql)
	assert.Equal(t, []any{10, 1000}, params)
}

// TestQueryBuilderSelectHavingWithAndExpression verifies that QueryBuilderSelect supports `Having` with AND expressions.
func TestQueryBuilderSelectHavingWithAndExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Having(sqlc.And(
		sqlc.Col("*").Count().Gt(10),
		sqlc.Col("amount").Sum().Gt(1000),
	))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "HAVING (COUNT(*) > ? AND SUM(`amount`) > ?)", sql)
	assert.Equal(t, []any{10, 1000}, params)
}

// TestQueryBuilderSelectHavingWithOrExpression verifies that QueryBuilderSelect supports `Having` with OR expressions.
func TestQueryBuilderSelectHavingWithOrExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Having(sqlc.Or(
		sqlc.Col("amount").Sum().Gt(1000),
		sqlc.Col("price").Avg().Lt(50),
	))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "HAVING (SUM(`amount`) > ? OR AVG(`price`) < ?)", sql)
	assert.Equal(t, []any{1000, 50}, params)
}

// TestQueryBuilderSelectHavingEmpty verifies that QueryBuilderSelect omits `Having` when no conditions are configured.
func TestQueryBuilderSelectHavingEmpty(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectHavingProperAggregateUsage demonstrates the correct way to use aggregate functions
func TestQueryBuilderSelectHavingProperAggregateUsage(t *testing.T) {
	t.Run("COUNT(*) using Col(\"*\").Count()", func(t *testing.T) {
		qb := sqlr.NewQueryBuilderSelect()
		qb.Having(sqlc.Col("*").Count().Gt(10))

		sql, params, err := qb.ToSql()
		require.NoError(t, err)

		assert.Equal(t, "HAVING COUNT(*) > ?", sql)
		assert.Equal(t, []any{10}, params)
	})

	t.Run("SUM(column) using Col(\"column\").Sum()", func(t *testing.T) {
		qb := sqlr.NewQueryBuilderSelect()
		qb.Having(sqlc.Col("amount").Sum().Gte(1000))

		sql, params, err := qb.ToSql()
		require.NoError(t, err)

		assert.Equal(t, "HAVING SUM(`amount`) >= ?", sql)
		assert.Equal(t, []any{1000}, params)
	})

	t.Run("AVG(column) using Col(\"column\").Avg()", func(t *testing.T) {
		qb := sqlr.NewQueryBuilderSelect()
		qb.Having(sqlc.Col("rating").Avg().Gt(4.5))

		sql, params, err := qb.ToSql()
		require.NoError(t, err)

		assert.Equal(t, "HAVING AVG(`rating`) > ?", sql)
		assert.Equal(t, []any{4.5}, params)
	})

	t.Run("MAX(column) using Col(\"column\").Max()", func(t *testing.T) {
		qb := sqlr.NewQueryBuilderSelect()
		qb.Having(sqlc.Col("price").Max().Lt(100))

		sql, params, err := qb.ToSql()
		require.NoError(t, err)

		assert.Equal(t, "HAVING MAX(`price`) < ?", sql)
		assert.Equal(t, []any{100}, params)
	})

	t.Run("MIN(column) using Col(\"column\").Min()", func(t *testing.T) {
		qb := sqlr.NewQueryBuilderSelect()
		qb.Having(sqlc.Col("quantity").Min().Gte(1))

		sql, params, err := qb.ToSql()
		require.NoError(t, err)

		assert.Equal(t, "HAVING MIN(`quantity`) >= ?", sql)
		assert.Equal(t, []any{1}, params)
	})
}

// ========== OrderBy Tests ==========

// TestQueryBuilderSelectOrderByWithSingleColumn verifies that QueryBuilderSelect supports `OrderBy` with single column.
func TestQueryBuilderSelectOrderByWithSingleColumn(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.OrderBy("created_at DESC")

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY `created_at` DESC", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectOrderByWithMultipleColumns verifies that QueryBuilderSelect supports `OrderBy` with multiple columns.
func TestQueryBuilderSelectOrderByWithMultipleColumns(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.OrderBy("name ASC", "created_at DESC")

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY `name` ASC, `created_at` DESC", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectOrderByWithExpression verifies that QueryBuilderSelect supports `OrderBy` with expression.
func TestQueryBuilderSelectOrderByWithExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.OrderBy(sqlc.Col("price").Desc())

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY `price` DESC", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectOrderByWithMultipleExpressions verifies that QueryBuilderSelect supports `OrderBy` with multiple expressions.
func TestQueryBuilderSelectOrderByWithMultipleExpressions(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.OrderBy(sqlc.Col("name").Asc(), sqlc.Col("id").Desc())

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY `name` ASC, `id` DESC", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectOrderByEmpty verifies that QueryBuilderSelect omits `OrderBy` when no ordering is configured.
func TestQueryBuilderSelectOrderByEmpty(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// ========== Limit Tests ==========

// TestQueryBuilderSelectLimit verifies that QueryBuilderSelect renders configured `Limit` values.
func TestQueryBuilderSelectLimit(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Limit(10)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "LIMIT 10", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectLimitZero verifies that QueryBuilderSelect handles zero for `Limit`.
func TestQueryBuilderSelectLimitZero(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Limit(0)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "LIMIT 0", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectLimitEmpty verifies that QueryBuilderSelect omits `Limit` when it is unset.
func TestQueryBuilderSelectLimitEmpty(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// ========== Offset Tests ==========

// TestQueryBuilderSelectOffset verifies that QueryBuilderSelect renders configured `Offset` values.
func TestQueryBuilderSelectOffset(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Offset(20)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "OFFSET 20", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectOffsetZero verifies that QueryBuilderSelect handles zero for `Offset`.
func TestQueryBuilderSelectOffsetZero(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Offset(0)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "OFFSET 0", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectOffsetEmpty verifies that QueryBuilderSelect omits `Offset` when it is unset.
func TestQueryBuilderSelectOffsetEmpty(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// ========== Combined Tests ==========

// TestQueryBuilderSelectCombinedWhereAndGroupBy verifies that QueryBuilderSelect composes where and group by correctly.
func TestQueryBuilderSelectCombinedWhereAndGroupBy(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").Eq("active")).
		GroupBy("country")

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` = ? GROUP BY `country`", sql)
	assert.Equal(t, []any{"active"}, params)
}

// TestQueryBuilderSelectCombinedWhereGroupByAndHaving verifies that QueryBuilderSelect composes where group by and having correctly.
func TestQueryBuilderSelectCombinedWhereGroupByAndHaving(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").Eq("active")).
		GroupBy("country").
		Having(sqlc.Col("*").Count().Gt(10))

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` = ? GROUP BY `country` HAVING COUNT(*) > ?", sql)
	assert.Equal(t, []any{"active", 10}, params)
}

// TestQueryBuilderSelectCombinedWhereGroupByHavingAndOrderBy verifies that QueryBuilderSelect composes where group by having and order by correctly.
func TestQueryBuilderSelectCombinedWhereGroupByHavingAndOrderBy(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").Eq("active")).
		GroupBy("country").
		Having(sqlc.Col("*").Count().Gt(10)).
		OrderBy(sqlc.Col("country").Asc())

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` = ? GROUP BY `country` HAVING COUNT(*) > ? ORDER BY `country` ASC", sql)
	assert.Equal(t, []any{"active", 10}, params)
}

// TestQueryBuilderSelectCombinedWhereOrderByAndLimit verifies that QueryBuilderSelect composes where order by and limit correctly.
func TestQueryBuilderSelectCombinedWhereOrderByAndLimit(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").Eq("active")).
		OrderBy("created_at DESC").
		Limit(10)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` = ? ORDER BY `created_at` DESC LIMIT 10", sql)
	assert.Equal(t, []any{"active"}, params)
}

// TestQueryBuilderSelectCombinedWhereOrderByLimitAndOffset verifies that QueryBuilderSelect composes where order by limit and offset correctly.
func TestQueryBuilderSelectCombinedWhereOrderByLimitAndOffset(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").Eq("active")).
		OrderBy("created_at DESC").
		Limit(10).
		Offset(20)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` = ? ORDER BY `created_at` DESC LIMIT 10 OFFSET 20", sql)
	assert.Equal(t, []any{"active"}, params)
}

// TestQueryBuilderSelectCombinedAllClauses verifies that QueryBuilderSelect composes all clauses correctly.
func TestQueryBuilderSelectCombinedAllClauses(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").Eq("active")).
		Where(sqlc.Col("age").Gte(18)).
		GroupBy("country", "city").
		Having(sqlc.Col("*").Count().Gt(5)).
		Having(sqlc.Col("amount").Sum().Gte(1000)).
		OrderBy("country ASC", "city DESC").
		Limit(10).
		Offset(20)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` = ? AND `age` >= ? GROUP BY `country`, `city` HAVING COUNT(*) > ? AND SUM(`amount`) >= ? ORDER BY `country` ASC, `city` DESC LIMIT 10 OFFSET 20", sql)
	assert.Equal(t, []any{"active", 18, 5, 1000}, params)
}

// TestQueryBuilderSelectCombinedComplexExpressions verifies that QueryBuilderSelect composes complex expressions correctly.
func TestQueryBuilderSelectCombinedComplexExpressions(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Or(
		sqlc.Col("status").Eq("active"),
		sqlc.Col("status").Eq("pending"),
	)).
		Where(sqlc.Col("age").Between(18, 65)).
		GroupBy("country").
		Having(sqlc.And(
			sqlc.Col("*").Count().Gt(10),
			sqlc.Or(
				sqlc.Col("amount").Sum().Gt(1000),
				sqlc.Col("price").Avg().Lt(50),
			),
		)).
		OrderBy(sqlc.Col("country").Asc()).
		Limit(25).
		Offset(50)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE (`status` = ? OR `status` = ?) AND `age` BETWEEN ? AND ? GROUP BY `country` HAVING (COUNT(*) > ? AND (SUM(`amount`) > ? OR AVG(`price`) < ?)) ORDER BY `country` ASC LIMIT 25 OFFSET 50", sql)
	assert.Equal(t, []any{"active", "pending", 18, 65, 10, 1000, 50}, params)
}

// ========== Method Chaining Tests ==========

// TestQueryBuilderSelectMethodChainingReturnsInstance verifies that QueryBuilderSelect returns the same builder instance for chaining.
func TestQueryBuilderSelectMethodChainingReturnsInstance(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()

	result := qb.Where("status = ?", "active")
	assert.Equal(t, qb, result)

	result = qb.GroupBy("country")
	assert.Equal(t, qb, result)

	result = qb.Having("COUNT(*) > ?", 10)
	assert.Equal(t, qb, result)

	result = qb.OrderBy("created_at DESC")
	assert.Equal(t, qb, result)

	result = qb.Limit(10)
	assert.Equal(t, qb, result)

	result = qb.Offset(20)
	assert.Equal(t, qb, result)

	result = qb.Preload("Posts")
	assert.Equal(t, qb, result)
}

// ========== Edge Cases ==========

// TestQueryBuilderSelectWithNilExpression verifies that QueryBuilderSelect ignores nil expressions safely.
func TestQueryBuilderSelectWithNilExpression(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	var nilExpr *sqlc.Expression = nil
	qb.Where(nilExpr)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectWithEmptyEqMap verifies that QueryBuilderSelect ignores empty equality maps.
func TestQueryBuilderSelectWithEmptyEqMap(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Eq{})

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectMultipleGroupByReplacesExisting verifies that QueryBuilderSelect replaces earlier values on repeated calls.
func TestQueryBuilderSelectMultipleGroupByReplacesExisting(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.GroupBy("status")
	qb.GroupBy("country") // Should replace, not append

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "GROUP BY `country`", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectMultipleOrderByReplacesExisting verifies that QueryBuilderSelect replaces earlier values on repeated calls.
func TestQueryBuilderSelectMultipleOrderByReplacesExisting(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.OrderBy("name ASC")
	qb.OrderBy("created_at DESC") // Should replace, not append

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "ORDER BY `created_at` DESC", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectMultipleLimitsReplacesExisting verifies that QueryBuilderSelect replaces earlier values on repeated calls.
func TestQueryBuilderSelectMultipleLimitsReplacesExisting(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Limit(10)
	qb.Limit(20) // Should replace, not append

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "LIMIT 20", sql)
	assert.Empty(t, params)
}

// TestQueryBuilderSelectMultipleOffsetsReplacesExisting verifies that QueryBuilderSelect replaces earlier values on repeated calls.
func TestQueryBuilderSelectMultipleOffsetsReplacesExisting(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Offset(10)
	qb.Offset(30) // Should replace, not append

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "OFFSET 30", sql)
	assert.Empty(t, params)
}

// ========== Real World Usage Examples ==========

// TestQueryBuilderSelectRealWorldPaginationQuery verifies that QueryBuilderSelect builds a pagination query shape correctly.
func TestQueryBuilderSelectRealWorldPaginationQuery(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("status").In("active", "pending")).
		Where(sqlc.Col("created_at").Gte("2024-01-01")).
		OrderBy("created_at DESC").
		Limit(20).
		Offset(40)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `status` IN (?, ?) AND `created_at` >= ? ORDER BY `created_at` DESC LIMIT 20 OFFSET 40", sql)
	assert.Equal(t, []any{"active", "pending", "2024-01-01"}, params)
}

// TestQueryBuilderSelectRealWorldAggregationQuery verifies that QueryBuilderSelect builds a real-world aggregation query shape correctly.
func TestQueryBuilderSelectRealWorldAggregationQuery(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("created_at").Between("2024-01-01", "2024-12-31")).
		GroupBy("country", "category").
		Having(sqlc.Col("*").Count().Gte(5)).
		Having(sqlc.Col("revenue").Sum().Gt(10000)).
		OrderBy(sqlc.Col("revenue").Sum().Desc())

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `created_at` BETWEEN ? AND ? GROUP BY `country`, `category` HAVING COUNT(*) >= ? AND SUM(`revenue`) > ? ORDER BY SUM(`revenue`) DESC", sql)
	assert.Equal(t, []any{"2024-01-01", "2024-12-31", 5, 10000}, params)
}

// TestQueryBuilderSelectRealWorldSearchQuery verifies that QueryBuilderSelect builds a search query shape correctly.
func TestQueryBuilderSelectRealWorldSearchQuery(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Or(
		sqlc.Col("name").Like("%john%"),
		sqlc.Col("email").Like("%john%"),
	)).
		Where(sqlc.Col("deleted_at").IsNull()).
		OrderBy("name ASC").
		Limit(10)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE (`name` LIKE ? OR `email` LIKE ?) AND `deleted_at` IS NULL ORDER BY `name` ASC LIMIT 10", sql)
	assert.Equal(t, []any{"%john%", "%john%"}, params)
}

// TestQueryBuilderSelectRealWorldReportQuery verifies that QueryBuilderSelect builds a report query shape correctly.
func TestQueryBuilderSelectRealWorldReportQuery(t *testing.T) {
	qb := sqlr.NewQueryBuilderSelect()
	qb.Where(sqlc.Col("order_date").Between("2024-01-01", "2024-03-31")).
		Where(sqlc.Col("status").Eq("completed")).
		GroupBy("customer_id").
		Having(sqlc.And(
			sqlc.Col("order_id").Count().Gte(3),
			sqlc.Col("total_amount").Sum().Gte(1000),
		)).
		OrderBy(sqlc.Col("total_amount").Sum().Desc()).
		Limit(100)

	sql, params, err := qb.ToSql()
	require.NoError(t, err)

	assert.Equal(t, "WHERE `order_date` BETWEEN ? AND ? AND `status` = ? GROUP BY `customer_id` HAVING (COUNT(`order_id`) >= ? AND SUM(`total_amount`) >= ?) ORDER BY SUM(`total_amount`) DESC LIMIT 100", sql)
	assert.Equal(t, []any{"2024-01-01", "2024-03-31", "completed", 3, 1000}, params)
}
