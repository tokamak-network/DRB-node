package database

import (
	"os"
	"testing"
)

var testConfig *Config

func TestMain(m *testing.M) {
	// Setup: load config for test DB (env vars or .env.test)
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_USER", "postgres")
	os.Setenv("POSTGRES_PASSWORD", "123")
	os.Setenv("POSTGRES_NAME", "testdb")
	os.Setenv("POSTGRES_SSLMODE", "disable")

	testConfig = LoadConfig()

	// Initialize DB connection for tests
	err := InitSQLDB(
		testConfig.PostgresPort,
		testConfig.PostgresHost,
		testConfig.PostgresUser,
		testConfig.PostgresPassword,
		testConfig.PostgresName,
	)
	if err != nil {
		panic("Failed to initialize test DB: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Teardown can be done here if needed (e.g., close DB, clean tables)

	os.Exit(code)
}
