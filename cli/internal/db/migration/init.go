package migration

// Init clears and registers data/behavior migrations. Call before RunAll. Table structure is handled by db.SyncSchema only.
func Init() {
	steps = nil
	// add("20250101_example", exampleMigration)
}
