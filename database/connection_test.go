package database

import (
	"context"
	"testing"
	"time"

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
	// Since database is already initialized by TestMain,
	// calling InitSQLDB again should work (once.Do won't execute again)
	err := InitSQLDB(5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err, "Should succeed when database is already initialized")
}

func TestInitSQLDB_InvalidHost(t *testing.T) {
	err := InitSQLDB(9999, "invalid-host-that-does-not-exist", "postgres", "123", "nonexistent")
	assert.Error(t, err, "Should fail with invalid host")
}

func TestInitSQLDB_InvalidPort(t *testing.T) {
	// Test with invalid port
	err := InitSQLDB(1, "localhost", "postgres", "123", "testdb")
	// Should fail to connect
	assert.Error(t, err, "Should fail with invalid port")
}

func TestInitSQLDB_ValidParameters(t *testing.T) {
	// Test with valid parameters (same as test database)
	err := InitSQLDB(5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err, "Should succeed with valid parameters")

	// Verify we can get the database
	db := GetDB()
	assert.NotNil(t, db)

	// Verify connection is alive
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
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
	assert.Equal(t, 10, opts.PoolSize, "Pool size should be 10")
	assert.Equal(t, 10, opts.MinIdleConns, "MinIdleConns should be 10")
}

func TestInitSQLDB_WithEmptyPassword(t *testing.T) {
	// Test initialization with empty password (should fail for our test DB)
	err := InitSQLDB(5433, "localhost", "postgres", "", "testdb")
	// This will fail at connection level
	assert.Error(t, err, "Should fail with empty password when password is required")
}

func TestInitSQLDB_WithWrongCredentials(t *testing.T) {
	// Test with wrong username
	err := InitSQLDB(5433, "localhost", "wronguser", "wrongpass", "testdb")
	assert.Error(t, err, "Should fail with wrong credentials")
}

func TestInitSQLDB_WithNonexistentDatabase(t *testing.T) {
	// Test with database that doesn't exist
	err := InitSQLDB(5433, "localhost", "postgres", "123", "database_that_does_not_exist")
	assert.Error(t, err, "Should fail when database doesn't exist")
}

func TestInitSQLDB_ConnectionString(t *testing.T) {

	err := InitSQLDB(5433, "localhost", "postgres", "123", "testdb")
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
	// Test with invalid port on localhost (fast failure)
	err := InitSQLDB(9876, "localhost", "postgres", "123", "testdb")
	assert.Error(t, err, "Should fail when connecting to invalid port")
}

func TestGetDB_AfterSuccessfulInit(t *testing.T) {
	// Ensure database is initialized
	err := InitSQLDB(5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err)

	// Get database and perform operations
	db := GetDB()
	assert.NotNil(t, db)

	// Test that we can ping the database
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = db.Ping(ctx)
	assert.NoError(t, err, "Should be able to ping database after init")
}

func TestInitSQLDB_ContextTimeout(t *testing.T) {
	// Since we're already initialized, this tests the final ping
	err := InitSQLDB(5433, "localhost", "postgres", "123", "testdb")
	assert.NoError(t, err, "Should succeed with valid connection")
}
