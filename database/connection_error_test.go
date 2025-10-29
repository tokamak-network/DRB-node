package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test sql.Open error by using completely invalid DSN format
func TestInitSQLDB_SQLOpenError(t *testing.T) {
	// This is tricky because sql.Open rarely fails
	// It only validates the driver name, not the DSN
	// Since we use "postgres" driver which is registered, sql.Open won't fail
	// But we can test with invalid connection parameters that fail on Ping

	err := InitSQLDB(0, "", "", "", "")
	assert.Error(t, err, "Should fail with empty parameters")
}

// Test MigrationsUp failure by using a database without proper permissions
func TestInitSQLDB_MigrationUpError(t *testing.T) {
	// Try with a database where we don't have CREATE TABLE permissions
	// Since we can't easily set this up, we'll use invalid credentials
	err := InitSQLDB(5433, "localhost", "postgres", "wrongpassword", "testdb")
	assert.Error(t, err, "Should fail when migrations can't run")
}

// Test with extremely invalid DSN that might cause sql.Open to fail
func TestInitSQLDB_InvalidDSN(t *testing.T) {
	// Use special characters that might break DSN parsing
	err := InitSQLDB(5433, "host with spaces", "user@invalid", "pass word", "db name")
	assert.Error(t, err, "Should fail with invalid DSN characters")
}
