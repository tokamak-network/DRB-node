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
