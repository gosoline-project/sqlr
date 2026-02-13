package sqlr

// Settings controls optional Repository and RepositoryTx behavior.
type Settings struct {
	// PreparedStatements enables lazy caching of prepared statements for
	// CRUD operations (Create, Read, Update, Delete). When true, the SQL
	// template for each operation is prepared once on first use and the
	// resulting *sqlx.Stmt is reused for all subsequent calls. For
	// RepositoryTx, the cached statement is rebound into each transaction
	// via sqlx.Tx.StmtxContext. Default: false.
	PreparedStatements bool
}

// DefaultSettings returns Settings with all options at their zero/default values.
func DefaultSettings() Settings {
	return Settings{}
}
