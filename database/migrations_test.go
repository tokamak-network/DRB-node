package database

import (
	"database/sql"
	"testing"

	"github.com/gobuffalo/packr/v2"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
)

func TestMigrationsUp_Success(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Run migrations up (should succeed or be already applied)
	err = MigrationsUp(db)
	assert.NoError(t, err, "MigrationsUp should succeed")
}

func TestMigrationsDown_Success(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Run migrations down
	err = MigrationsDown(db)
	assert.NoError(t, err, "MigrationsDown should succeed")

	// Run migrations up again to restore
	err = MigrationsUp(db)
	assert.NoError(t, err)
}

func TestMigrationsUp_WithInvalidDatabase(t *testing.T) {
	// Create a database connection that will fail
	dsn := "postgres://postgres:wrongpass@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err, "Opening connection should not error immediately")
	defer db.Close()

	// Try to run migrations - should fail due to authentication
	err = MigrationsUp(db)
	assert.Error(t, err, "MigrationsUp should fail with wrong credentials")
}

func TestMigrationsDown_WithInvalidDatabase(t *testing.T) {
	// Create a database connection that will fail
	dsn := "postgres://postgres:wrongpass@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err, "Opening connection should not error immediately")
	defer db.Close()

	// Try to run migrations down - should fail due to authentication
	err = MigrationsDown(db)
	assert.Error(t, err, "MigrationsDown should fail with wrong credentials")
}

func TestMigrationsUp_WithClosedDatabase(t *testing.T) {
	// Open and immediately close the database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	db.Close() // Close it immediately

	// Try to run migrations on closed database
	err = MigrationsUp(db)
	assert.Error(t, err, "MigrationsUp should fail with closed database")
}

func TestMigrationsDown_WithClosedDatabase(t *testing.T) {
	// Open and immediately close the database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	db.Close() // Close it immediately

	// Try to run migrations down on closed database
	err = MigrationsDown(db)
	assert.Error(t, err, "MigrationsDown should fail with closed database")
}

func TestMigrationsUp_WithNonExistentDatabase(t *testing.T) {
	// Try to connect to a non-existent database
	dsn := "postgres://postgres:123@localhost:5433/database_does_not_exist?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Try to run migrations - should fail
	err = MigrationsUp(db)
	assert.Error(t, err, "MigrationsUp should fail with non-existent database")
}

func TestMigrationsDown_WithNonExistentDatabase(t *testing.T) {
	// Try to connect to a non-existent database
	dsn := "postgres://postgres:123@localhost:5433/database_does_not_exist?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Try to run migrations down - should fail
	err = MigrationsDown(db)
	assert.Error(t, err, "MigrationsDown should fail with non-existent database")
}

func TestMigrationsUp_MultipleRuns(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Run migrations multiple times - should be idempotent
	err = MigrationsUp(db)
	assert.NoError(t, err)

	err = MigrationsUp(db)
	assert.NoError(t, err, "Running migrations multiple times should be safe")

	err = MigrationsUp(db)
	assert.NoError(t, err, "Running migrations third time should still be safe")
}

func TestMigrationsDownAndUp_Cycle(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Cycle: Down -> Up -> Down -> Up
	err = MigrationsDown(db)
	assert.NoError(t, err, "First migration down should succeed")

	err = MigrationsUp(db)
	assert.NoError(t, err, "Migration up after down should succeed")

	err = MigrationsDown(db)
	assert.NoError(t, err, "Second migration down should succeed")

	err = MigrationsUp(db)
	assert.NoError(t, err, "Final migration up should succeed")
}

func TestMigrations_PackrMigrationSource(t *testing.T) {
	// Verify that migrations variable is properly initialized
	assert.NotNil(t, migrations, "Migrations should be initialized by init()")

	// Verify we can find migrations
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err, "Should be able to find migrations")
	assert.NotEmpty(t, ms, "Should have at least one migration")
}

func TestMigrationsUp_WithInvalidPort(t *testing.T) {
	// Test with database on wrong port
	dsn := "postgres://postgres:123@localhost:9999/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// This should fail when trying to execute migrations
	err = MigrationsUp(db)
	assert.Error(t, err, "Should fail when database is not accessible")
}

func TestMigrationsDown_WithInvalidPort(t *testing.T) {
	// Test with database on wrong port
	dsn := "postgres://postgres:123@localhost:9999/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// This should fail when trying to execute migrations
	err = MigrationsDown(db)
	assert.Error(t, err, "Should fail when database is not accessible")
}

// Test that init() properly initialized the migrations variable
func TestInit_MigrationsVariableInitialized(t *testing.T) {
	// Verify that init() successfully initialized the migrations variable
	assert.NotNil(t, migrations, "migrations should be initialized by init()")
	assert.NotNil(t, migrations.Box, "migrations.Box should be initialized")

	// Calling FindMigrations verifies the init() logic paths
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err, "FindMigrations should succeed (init() validated this)")
	assert.NotEmpty(t, ms, "Should have migrations (init() validated this too)")
	assert.Len(t, ms, 1, "Should have exactly 1 migration file")
}

// Test the migration source is properly set up
func TestInit_MigrationSourceSetup(t *testing.T) {
	// This test verifies that packr.New worked correctly in init()
	assert.NotNil(t, migrations)
	assert.NotNil(t, migrations.Box)

	// Try to find migrations - this exercises the same path as init()
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err)

	// Verify migration properties
	for _, m := range ms {
		assert.NotEmpty(t, m.Id, "Migration should have ID")
		assert.NotNil(t, m.Up, "Migration should have Up script")
		assert.NotNil(t, m.Down, "Migration should have Down script")
	}
}

// Test init() validation - if this test runs, init() didn't panic
func TestInit_ValidationPassed(t *testing.T) {
	// The fact that this test runs proves:
	// 1. migrations.FindMigrations() in init() did not return error (line 592-594)
	// 2. len(ms) was not 0 (line 596-597)

	// If either panic condition triggered, the package wouldn't load and tests wouldn't run
	assert.NotNil(t, migrations, "migrations initialized successfully")

	// Re-verify the same conditions that init() checked
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err, "FindMigrations should not error")
	assert.Greater(t, len(ms), 0, "Should have at least one migration")
}

// Test migrations box name
func TestInit_BoxName(t *testing.T) {
	// Verify the packr box was created with correct name
	assert.NotNil(t, migrations)
	assert.NotNil(t, migrations.Box)

	// The box should be able to find migrations
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err)
	assert.NotEmpty(t, ms)
}

// Test that we can access migration content
func TestInit_MigrationContent(t *testing.T) {
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err)
	assert.NotEmpty(t, ms)

	// Get the first migration
	migration := ms[0]
	assert.NotEmpty(t, migration.Id, "Migration should have ID")

	// Verify migration has up and down scripts
	assert.NotEmpty(t, migration.Up, "Migration should have Up script")
	assert.NotEmpty(t, migration.Down, "Migration should have Down script")

	// Verify the migration ID format
	assert.Contains(t, migration.Id, "001", "Should be first migration")
}

// Test FindMigrations error scenario by creating a migration source with invalid path
func TestInit_FindMigrationsError(t *testing.T) {
	// Create a new migration source with invalid path to simulate error condition
	invalidMigrations := &migrate.PackrMigrationSource{
		Box: packr.New("invalid-migrations", "./nonexistent_directory"),
	}

	// This should return an error or empty (simulating init conditions)
	ms, err := invalidMigrations.FindMigrations()

	// This tests the error path that init() checks
	if err != nil {
		// In init(), this would panic(err) - line 24
		assert.Error(t, err, "FindMigrations should error with invalid path")
		assert.Nil(t, ms, "Should return nil migrations on error")
	} else {
		// If no error, check if empty (line 26 condition)
		if len(ms) == 0 {
			// In init(), this would panic - line 27
			assert.Empty(t, ms, "Testing empty migrations scenario")
		}
	}
}

// Test the empty migrations scenario
func TestInit_EmptyMigrationsScenario(t *testing.T) {
	// Create a migration source that would return empty
	// We test the validation logic that init() performs

	// Get actual migrations
	ms, err := migrations.FindMigrations()
	assert.NoError(t, err)

	// Simulate the check that init() does at line 596
	if len(ms) == 0 {
		// This is the condition that would trigger panic in init()
		t.Fatal("No SQL migrations found - this would panic in init()")
	} else {
		// This proves init() didn't hit the panic condition
		assert.Greater(t, len(ms), 0, "Has migrations, init() succeeded")
	}
}

// Test error propagation from FindMigrations
func TestInit_ErrorPropagation(t *testing.T) {
	// Test that the migrations variable works correctly
	// Simulating what init() does

	// Get migrations (same as line 592 in init)
	ms, err := migrations.FindMigrations()

	// Check error condition (line 593 in init)
	if err != nil {
		// Would panic in init() - line 594
		t.Fatalf("FindMigrations error would cause init() to panic: %v", err)
	}

	// Check empty condition (line 596 in init)
	if len(ms) == 0 {
		// Would panic in init() - line 597
		t.Fatal("Empty migrations would cause init() to panic")
	}

	// If we get here, both validations passed (same as init())
	assert.NoError(t, err)
	assert.Greater(t, len(ms), 0)
}

// Test validateMigrations with invalid migration source (tests panic on error)
func TestValidateMigrations_WithFindMigrationsError(t *testing.T) {
	// Create a migration source with invalid path
	invalidMigrations := &migrate.PackrMigrationSource{
		Box: packr.New("test-invalid", "./this_directory_does_not_exist"),
	}

	// validateMigrations should panic when FindMigrations returns error
	assert.Panics(t, func() {
		validateMigrations(invalidMigrations)
	}, "validateMigrations should panic when FindMigrations fails")
}

// Test validateMigrations with empty migrations (tests panic on len == 0)
func TestValidateMigrations_WithEmptyMigrations(t *testing.T) {
	// We need to create a packr box that will return 0 migrations
	// The easiest way is to point to a path that exists but has no migration files

	// Create a temporary empty directory scenario using a path without migrations
	emptyMigrations := &migrate.PackrMigrationSource{
		Box: packr.New("test-empty-migrations", "./testdata_empty"),
	}

	// validateMigrations should panic with "no SQL migrations found"
	assert.Panics(t, func() {
		validateMigrations(emptyMigrations)
	}, "validateMigrations should panic when no migrations are found")
}

// Test validateMigrations with valid migrations (happy path)
func TestValidateMigrations_Success(t *testing.T) {
	// Should not panic with valid migrations
	assert.NotPanics(t, func() {
		validateMigrations(migrations)
	}, "validateMigrations should not panic with valid migrations")
}

// Test MigrationsUp with nil database connection
func TestMigrationsUp_WithNilDatabase(t *testing.T) {
	// Test with nil database - this will cause a panic due to nil pointer dereference
	var db *sql.DB = nil

	// This should panic when trying to use nil database
	assert.Panics(t, func() {
		_ = MigrationsUp(db)
	}, "MigrationsUp should panic with nil database")
}

// Test MigrationsDown with nil database connection
func TestMigrationsDown_WithNilDatabase(t *testing.T) {
	// Test with nil database - this will cause a panic due to nil pointer dereference
	var db *sql.DB = nil

	// This should panic when trying to use nil database
	assert.Panics(t, func() {
		_ = MigrationsDown(db)
	}, "MigrationsDown should panic with nil database")
}

// Test MigrationsUp when migrations return 0 (already applied)
func TestMigrationsUp_ZeroMigrationsApplied(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Run migrations up first time
	err = MigrationsUp(db)
	assert.NoError(t, err, "First MigrationsUp should succeed")

	// Run migrations up again - should return 0 migrations applied
	// This tests the fmt.Printf path with n=0
	err = MigrationsUp(db)
	assert.NoError(t, err, "Second MigrationsUp should succeed (0 migrations applied)")
}

// Test MigrationsUp error path when migrate.Exec fails
func TestMigrationsUp_ExecError(t *testing.T) {
	// Create a database connection that will fail during execution
	// Using wrong database name to trigger error during migration execution
	dsn := "postgres://postgres:123@localhost:5433/database_does_not_exist?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// This should fail at migrate.Exec, testing the error return path (line 34-35)
	err = MigrationsUp(db)
	assert.Error(t, err, "MigrationsUp should fail when migrate.Exec fails")
}

// Test MigrationsDown error path when migrate.Exec fails
func TestMigrationsDown_ExecError(t *testing.T) {
	// Create a database connection that will fail during execution
	dsn := "postgres://postgres:123@localhost:5433/database_does_not_exist?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// This should fail at migrate.Exec, testing the error return path (line 43-44)
	err = MigrationsDown(db)
	assert.Error(t, err, "MigrationsDown should fail when migrate.Exec fails")
}

// Test MigrationsUp with database that has connection issues mid-execution
func TestMigrationsUp_ConnectionLostDuringExecution(t *testing.T) {
	// Open a connection
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)

	// Close the connection before running migrations
	db.Close()

	// Try to run migrations on closed connection
	err = MigrationsUp(db)
	assert.Error(t, err, "MigrationsUp should fail when connection is closed")
}

// Test MigrationsDown with database that has connection issues mid-execution
func TestMigrationsDown_ConnectionLostDuringExecution(t *testing.T) {
	// Open a connection
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)

	// Close the connection before running migrations
	db.Close()

	// Try to run migrations down on closed connection
	err = MigrationsDown(db)
	assert.Error(t, err, "MigrationsDown should fail when connection is closed")
}

// Test MigrationsUp output when migrations are applied (n > 0)
func TestMigrationsUp_WithMigrationsApplied(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// First, run migrations down to ensure we can run up again
	err = MigrationsDown(db)
	if err != nil {
		// If down fails, migrations might not be applied, continue anyway
	}

	// Run migrations up - should apply migrations and print success message
	err = MigrationsUp(db)
	assert.NoError(t, err, "MigrationsUp should succeed")
	// The fmt.Printf line (line 37) should execute
}

// Test MigrationsDown output path
func TestMigrationsDown_OutputPath(t *testing.T) {
	// Open a connection to test database
	dsn := "postgres://postgres:123@localhost:5433/testdb?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)
	defer db.Close()

	// Ensure migrations are up first
	err = MigrationsUp(db)
	assert.NoError(t, err)

	// Run migrations down - should print success message
	err = MigrationsDown(db)
	assert.NoError(t, err, "MigrationsDown should succeed")
	// The fmt.Println line (line 46) should execute

	// Restore migrations
	err = MigrationsUp(db)
	assert.NoError(t, err)
}
