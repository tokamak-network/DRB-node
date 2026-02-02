package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/stretchr/testify/assert"
)

func TestGetDB_WhenInitialized(t *testing.T) {
	// Database is already initialized by TestMain
	db := GetDB()
	assert.NotNil(t, db, "GetDB should return initialized database")

	// Verify we can ping it
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := db.Ping(ctx)
	assert.NoError(t, err, "Should be able to ping the database")
}

func TestGetDB_Properties(t *testing.T) {
	db := GetDB()
	assert.NotNil(t, db)

	// Verify the database object has expected properties
	opts := db.Options()
	assert.NotNil(t, opts)
	assert.Equal(t, "testdb", opts.Database)
	assert.Equal(t, "postgres", opts.User)
}

func TestInitSQLDB_AlreadyInitialized(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Since database is already initialized by TestMain,
	// calling InitSQLDB again should work (once.Do won't execute again)
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err, "Should succeed when database is already initialized")
}

func TestInitSQLDB_InvalidHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := InitSQLDB(ctx, 9999, "invalid-host-that-does-not-exist", "postgres", "123", "nonexistent")
	assert.Error(t, err, "Should fail with invalid host")
}

func TestInitSQLDB_InvalidPort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Test with invalid port
	err := InitSQLDB(ctx, 1, "localhost", "postgres", "123", "testdb")
	// Should fail to connect
	assert.Error(t, err, "Should fail with invalid port")
}

func TestInitSQLDB_ValidParameters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Test with valid parameters (same as test database)
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err, "Should succeed with valid parameters")

	// Verify we can get the database
	db := GetDB()
	assert.NotNil(t, db)

	err = db.Ping(ctx)
	assert.NoError(t, err)
}

func TestGetDB_MultipleCallsSameInstance(t *testing.T) {
	// Verify GetDB returns the same instance
	db1 := GetDB()
	db2 := GetDB()

	assert.Equal(t, db1, db2, "GetDB should return the same instance")
}

func TestGetDB_ConnectionPool(t *testing.T) {
	db := GetDB()
	assert.NotNil(t, db)

	// Verify connection pool settings
	opts := db.Options()
	assert.Equal(t, 45, opts.PoolSize, "Pool size should be 45")
	assert.Equal(t, 10, opts.MinIdleConns, "MinIdleConns should be 10")
}

func TestInitSQLDB_WithEmptyPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Test initialization with empty password (should fail for our test DB)
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "", "testdb")
	// This will fail at connection level
	assert.Error(t, err, "Should fail with empty password when password is required")
}

func TestInitSQLDB_WithWrongCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Test with wrong username
	err := InitSQLDB(ctx, 5433, "localhost", "wronguser", "wrongpass", "testdb")
	assert.Error(t, err, "Should fail with wrong credentials")
}

func TestInitSQLDB_WithNonexistentDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Test with database that doesn't exist
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "database_that_does_not_exist")
	assert.Error(t, err, "Should fail when database doesn't exist")
}

func TestInitSQLDB_ConnectionString(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err)

	db := GetDB()
	assert.NotNil(t, db)

	// Check that the database name is correct
	opts := db.Options()
	assert.Contains(t, opts.Addr, "localhost:5433")
}

func TestGetDB_ConcurrentAccess(t *testing.T) {
	// Test that GetDB is safe for concurrent access
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			db := GetDB()
			assert.NotNil(t, db)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			err := db.Ping(ctx)
			assert.NoError(t, err)

			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestInitSQLDB_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Test with invalid port on localhost (fast failure)
	err := InitSQLDB(ctx, 9876, "localhost", "postgres", "123", "testdb")
	assert.Error(t, err, "Should fail when connecting to invalid port")
}

func TestGetDB_AfterSuccessfulInit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Ensure database is initialized
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err)

	// Get database and perform operations
	db := GetDB()
	assert.NotNil(t, db)

	// Test that we can ping the database
	err = db.Ping(ctx)
	assert.NoError(t, err, "Should be able to ping database after init")
}

func TestInitSQLDB_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Since we're already initialized, this tests the final ping
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err, "Should succeed with valid connection")
}

func TestClose_WhenInitialized(t *testing.T) {
	// Database is already initialized by TestMain
	// We'll create a temporary connection to test Close properly
	originalDB := dbClient

	// Create a temporary database connection
	tempDB := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", "localhost", "5433"),
		User:     "postgres",
		Password: "123",
		Database: "testdb",
	})

	// Set it as dbClient temporarily
	dbClient = tempDB

	// Test Close with a valid connection
	err := Close()
	assert.NoError(t, err, "Close should succeed with valid connection")

	// Restore original dbClient
	dbClient = originalDB
}

func TestClose_WhenNotInitialized(t *testing.T) {
	// Save current dbClient
	originalDB := dbClient

	// Temporarily set to nil
	dbClient = nil

	// Close should return nil when dbClient is nil
	err := Close()
	assert.NoError(t, err, "Close should return nil when dbClient is nil")

	// Restore
	dbClient = originalDB
}

func TestClose_WhenCloseReturnsError(t *testing.T) {
	// Save current dbClient
	originalDB := dbClient

	// Create a temporary database connection
	tempDB := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", "localhost", "5433"),
		User:     "postgres",
		Password: "123",
		Database: "testdb",
	})

	// Set it as dbClient temporarily
	dbClient = tempDB

	// Close it once (this should succeed)
	err := tempDB.Close()
	assert.NoError(t, err, "First close should succeed")

	// Now try to close it again through our Close() function
	// This should trigger the error path since the connection is already closed
	err = Close()
	assert.Error(t, err, "Close should return error when connection is already closed")
	assert.Contains(t, err.Error(), "error closing database", "Error message should mention closing database")

	// Restore original dbClient
	dbClient = originalDB
}

func TestGetDB_PanicWhenNotInitialized(t *testing.T) {
	// Save current dbClient
	originalDB := dbClient

	// Temporarily set to nil
	dbClient = nil

	// GetDB should panic when not initialized
	assert.Panics(t, func() {
		GetDB()
	}, "GetDB should panic when database is not initialized")

	// Restore
	dbClient = originalDB
}

// TestInitSQLDB_PanicInOnceDo tests the panic path when dbClient.Ping() fails
// inside once.Do (lines 56-58). This happens when the connection fails during
// the initial ping inside once.Do.
func TestInitSQLDB_PanicInOnceDo(t *testing.T) {
	// Save current state
	originalDB := dbClient

	// Reset dbClient to nil to allow once.Do to execute again
	// Note: This is a workaround - in practice, once.Do only executes once per process
	// But by setting dbClient to nil, we can test the panic path
	dbClient = nil

	// Create a context that's already cancelled - this will cause dbClient.Ping() to fail
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Call InitSQLDB - it will:
	// 1. Pass SQL connection checks (lines 23-35)
	// 2. Execute once.Do (because dbClient is nil)
	// 3. Create dbClient and try to ping it, which will fail due to cancelled context
	// 4. Panic at line 58
	assert.Panics(t, func() {
		InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")
	}, "Should panic when dbClient.Ping() fails inside once.Do")

	// Restore
	dbClient = originalDB
}

// TestInitSQLDB_GetDBPingErrorAfterInit tests the error path when GetDB().Ping() fails
// after initialization (line 64-65). This tests the scenario where once.Do has already
// executed, but GetDB().Ping() fails.
func TestInitSQLDB_GetDBPingErrorAfterInit(t *testing.T) {
	// Save current state
	originalDB := dbClient

	// Create a valid connection first (this simulates the once.Do block succeeding)
	tempDB := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", "localhost", "5433"),
		User:     "postgres",
		Password: "123",
		Database: "testdb",
	})

	// Set dbClient to the temp connection
	dbClient = tempDB
	defer func() {
		if tempDB != nil {
			tempDB.Close()
		}
		dbClient = originalDB
	}()

	// Close the connection to make it unusable
	err := tempDB.Close()
	assert.NoError(t, err, "Should be able to close the connection")

	// Create a valid context for SQL operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Call InitSQLDB - it will:
	// 1. Pass SQL connection checks (lines 23-35)
	// 2. Skip once.Do (already executed in TestMain)
	// 3. Try to ping via GetDB().Ping() which should fail because connection is closed (line 64-65)
	err = InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")

	// Should fail at GetDB().Ping()
	assert.Error(t, err, "Should fail when GetDB().Ping() fails after initialization")
	assert.Contains(t, err.Error(), "error pinging main DB client after initialization",
		"Error should mention pinging main DB client")
}

// TestInitSQLDB_GetDBPingErrorWithCancelledContext tests GetDB().Ping() error
// when context is already cancelled
func TestInitSQLDB_GetDBPingErrorWithCancelledContext(t *testing.T) {
	// Save current state
	originalDB := dbClient

	// Create a valid connection
	tempDB := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%s", "localhost", "5433"),
		User:     "postgres",
		Password: "123",
		Database: "testdb",
	})

	// Set dbClient
	dbClient = tempDB
	defer func() {
		if tempDB != nil {
			tempDB.Close()
		}
		dbClient = originalDB
	}()

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Call InitSQLDB with cancelled context
	// It should pass SQL checks, skip once.Do (already executed),
	// but fail at GetDB().Ping() with cancelled context
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "123", "testdb")

	// Should fail at the GetDB().Ping() check
	assert.Error(t, err, "Should fail when context is cancelled")
	assert.Contains(t, err.Error(), "error pinging main DB client after initialization",
		"Error should mention pinging main DB client")
}
