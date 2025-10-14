package database

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func getTestDB(t *testing.T) *sql.DB {
	// Use environment variable or default test DB connection string
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:123@localhost:5432/testdb?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	assert.NoError(t, err)

	err = db.Ping()
	assert.NoError(t, err)

	return db
}

func TestMigrationsUpDown(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	// Run migrations up
	err := MigrationsUp(db)
	assert.NoError(t, err, "MigrationsUp should run without error")

	// Optionally, you can verify schema changes here by querying tables, etc.

	// Run migrations down
	err = MigrationsDown(db)
	assert.NoError(t, err, "MigrationsDown should run without error")
}
