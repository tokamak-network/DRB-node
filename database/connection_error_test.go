package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test sql.Open error by using completely invalid DSN format
func TestInitSQLDB_SQLOpenError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := InitSQLDB(ctx, 0, "", "", "", "")
	assert.Error(t, err, "Should fail with empty parameters")
}

// Test MigrationsUp failure by using a database without proper permissions
func TestInitSQLDB_MigrationUpError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "wrongpassword", "testdb")
	assert.Error(t, err, "Should fail when migrations can't run")
}

// Test with extremely invalid DSN that might cause sql.Open to fail
func TestInitSQLDB_InvalidDSN(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := InitSQLDB(ctx, 5433, "host with spaces", "user@invalid", "pass word", "db name")
	assert.Error(t, err, "Should fail with invalid DSN characters")
}

// Test db.Ping() error path (line 29-30) - sql.Open succeeds but Ping fails
// This tests the error path when the DSN is valid but the connection fails
func TestInitSQLDB_DBPingError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Use a valid DSN format but with wrong port to trigger db.Ping() error
	// sql.Open will succeed (it doesn't actually connect), but db.Ping() will fail
	err := InitSQLDB(ctx, 9999, "localhost", "postgres", "123", "testdb")
	assert.Error(t, err, "Should fail when db.Ping() fails")
	// The error should be from pinging database for migration
	assert.Contains(t, err.Error(), "error pinging database for migration",
		"Error should mention pinging database for migration")
}

// Test db.Ping() error with wrong host
func TestInitSQLDB_DBPingErrorWrongHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Use a host that doesn't exist - sql.Open succeeds but db.Ping() fails
	err := InitSQLDB(ctx, 5433, "nonexistent-host-12345", "postgres", "123", "testdb")
	assert.Error(t, err, "Should fail when host doesn't exist")
	// Should fail at db.Ping() (connection refused or host not found)
	assert.Contains(t, err.Error(), "error pinging database for migration",
		"Error should mention pinging database for migration")
}

// Test db.Ping() error with wrong credentials
// Note: This might fail at db.Ping() or later at MigrationsUp, but we want to ensure
// the db.Ping() error path is covered
func TestInitSQLDB_DBPingErrorWrongPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Wrong password - sql.Open succeeds but db.Ping() fails with authentication error
	err := InitSQLDB(ctx, 5433, "localhost", "postgres", "wrongpassword123", "testdb")
	assert.Error(t, err, "Should fail with wrong password")
	// Should fail at db.Ping() with authentication error
	assert.Contains(t, err.Error(), "error pinging database for migration",
		"Error should mention pinging database for migration")
}

// Note: The GetDB().Ping() error path (line 64-65) and the panic path in once.Do (line 56-58)
// are now covered in connection_test.go:
// - TestInitSQLDB_GetDBPingErrorAfterInit
// - TestInitSQLDB_GetDBPingErrorWithCancelledContext
// - TestInitSQLDB_PanicInOnceDo
